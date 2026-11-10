package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

func handlePubSubSync(ctx context.Context, socketMsgPayload interface{}) error {
	// Check circuit breaker first
	if syncCircuitBreaker != nil && syncCircuitBreaker.IsOpen() {
		slog.Error("Circuit breaker is open, saving sync data locally for later retry")
		if err := syncCircuitBreaker.SaveForRetry(ctx, socketMsgPayload); err != nil {
			slog.Error("Failed to save sync data for retry", slog.Any("err", err))
			return err
		}
		return nil // Return nil to ack the message
	}

	pb, err := json.Marshal(socketMsgPayload)
	if err != nil {
		slog.Error("Failed to marshal the sync payload again for unmarshal", slog.Any("payload", socketMsgPayload))
		return err
	}

	var syncMsg model.PostTrackArgs
	err = json.Unmarshal(pb, &syncMsg)
	if err != nil {
		slog.Error("Failed to parse sync payload", slog.Any("payload", socketMsgPayload))
		return err
	}

	err = sendTrackArgsToServer(ctx, syncMsg)
	if err == nil || syncCircuitBreaker == nil || !isRetryableSyncError(err) {
		return err
	}

	// The CLI moved its cursor past this batch when it handed it over, so this message is
	// the only copy left. A nacked message is redelivered three times within a second and
	// then dropped, which loses the batch whenever the server is away for longer than that.
	// Keep it for the hourly retry instead, like the batches that arrive while the circuit
	// is open.
	if saveErr := syncCircuitBreaker.SaveForRetry(ctx, socketMsgPayload); saveErr != nil {
		slog.Error("Failed to save unsent sync data for retry", slog.Any("err", saveErr))
		return err
	}
	return nil
}

// isRetryableSyncError reports whether sending the same batch again later can work: the
// request never got an answer (network error, timeout), or the server said it is failing
// or busy. A batch the server refused (4xx) would be refused again.
func isRetryableSyncError(err error) bool {
	var statusErr *model.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode >= http.StatusInternalServerError ||
			statusErr.StatusCode == http.StatusTooManyRequests ||
			statusErr.StatusCode == http.StatusRequestTimeout
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// sendTrackArgsToServer enriches a tracking payload (terminal resolution, daemon
// source, optional encryption) and sends it to the server, recording circuit
// breaker state. It is shared by the sync handler and the bolt track handler.
func sendTrackArgsToServer(ctx context.Context, syncMsg model.PostTrackArgs) error {
	// Resolve terminal from PPID (use first data item's PPID). It walks the process tree, a
	// subprocess per level, and needs neither the config nor the encryption key, so it runs
	// while those are read.
	applyTerminal := resolveTerminalAsync(&syncMsg)

	// set as daemon
	syncMsg.Meta.Source = 1

	cfg, err := stConfig.ReadConfigFile(ctx)
	if err != nil {
		// The batch isn't sent, so the terminal isn't needed: the walk finishes on its own.
		slog.Error("Failed to unmarshal sync message", slog.Any("err", err))
		return err
	}

	// only daemon service can enable the encryption mode
	encrypted := cfg.Encrypted != nil && *cfg.Encrypted == true
	var ot *model.OpenTokenPublicKey
	var otErr error
	if encrypted {
		ot, otErr = model.GetOpenTokenPublicKey(ctx, model.Endpoint{
			Token:       cfg.Token,
			APIEndpoint: cfg.APIEndpoint,
		}, 0)
	}
	applyTerminal()

	payload := model.PostTrackArgs{
		CursorID: time.Unix(0, syncMsg.CursorID).UnixNano(), // Convert nano timestamp to time.Time
		Data:     syncMsg.Data,
		Meta:     syncMsg.Meta,
	}

	var realPayload model.PostTrackArgs
	if encrypted {
		if otErr != nil {
			slog.Error("Failed to get the open token public key", slog.Any("err", otErr))
			// Fail closed: encryption was requested but we could not fetch the
			// public key. Returning here both avoids a nil-pointer dereference on
			// ot below and prevents sending data unencrypted against the user's
			// configured intent.
			return otErr
		}
		if strings.TrimSpace(ot.PublicKey) == "" {
			err := fmt.Errorf("encryption is enabled but the open token public key is empty")
			slog.Error(err.Error())
			return err
		}

		rs := model.NewRSAService()
		as := model.NewAESGCMService()

		k, _, err := as.GenerateKeys()
		if err != nil {
			slog.Error("Failed to generate aes-gcm key", slog.Any("err", err))
			return err
		}

		encodedKey, _, err := rs.Encrypt(ot.PublicKey, k)
		if err != nil {
			slog.Error("Failed to encrypt key", slog.Any("err", err))
			return err
		}

		buf, err := json.Marshal(payload)
		if err != nil {
			slog.Error("Failed to marshal payload", slog.Any("err", err))
			return err
		}

		encryptedData, nonce, err := as.Encrypt(string(k), buf)
		if err != nil {
			slog.Error("Failed to encrypt data", slog.Any("err", err))
			return err
		}
		if len(encodedKey) == 0 || len(encryptedData) == 0 || len(nonce) == 0 {
			err := fmt.Errorf("encryption produced an empty key, ciphertext, or nonce")
			slog.Error(err.Error())
			return err
		}

		realPayload = model.PostTrackArgs{
			Encrypted: base64.StdEncoding.EncodeToString(encryptedData),
			AesKey:    base64.StdEncoding.EncodeToString(encodedKey),
			Nonce:     base64.StdEncoding.EncodeToString(nonce),
		}
	}

	if len(realPayload.Encrypted) == 0 {
		realPayload = payload
	}

	err = model.SendLocalDataToServer(
		ctx,
		cfg,
		realPayload,
	)

	if err != nil {
		if syncCircuitBreaker != nil {
			syncCircuitBreaker.RecordFailure()
		}
		slog.Error("Failed to sync data to server", slog.Any("err", err))
		return err
	}

	if syncCircuitBreaker != nil {
		syncCircuitBreaker.RecordSuccess()
	}
	return nil
}

// resolveTerminalAsync starts resolving the terminal of the batch's first command from its
// PPID and returns a function that waits for the result and stores it in syncMsg.Meta. Only
// the returned function touches syncMsg, and it does the work once however often it's called.
// A caller that won't send the batch can return without calling it; the walk only writes its
// own variables and finishes on its own.
func resolveTerminalAsync(syncMsg *model.PostTrackArgs) func() {
	if len(syncMsg.Data) == 0 || syncMsg.Data[0].PPID <= 0 {
		return func() {}
	}
	ppid := syncMsg.Data[0].PPID
	var terminal, multiplexer string
	done := make(chan struct{})
	go func() {
		defer close(done)
		terminal, multiplexer = ResolveTerminal(ppid)
	}()
	return sync.OnceFunc(func() {
		<-done
		syncMsg.Meta.Terminal = terminal
		syncMsg.Meta.Multiplexer = multiplexer
		slog.Debug("Resolved terminal", slog.String("terminal", terminal), slog.String("multiplexer", multiplexer), slog.Int("ppid", ppid))
	})
}

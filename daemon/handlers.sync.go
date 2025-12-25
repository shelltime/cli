package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
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

	// set as daemon
	syncMsg.Meta.Source = 1

	cfg, err := stConfig.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to unmarshal sync message", slog.Any("err", err))
		return err
	}

	payload := model.PostTrackArgs{
		CursorID: time.Unix(0, syncMsg.CursorID).UnixNano(), // Convert nano timestamp to time.Time
		Data:     syncMsg.Data,
		Meta:     syncMsg.Meta,
	}

	// only daemon service can enable the encryption mode
	var realPayload model.PostTrackArgs
	if cfg.Encrypted != nil && *cfg.Encrypted == true {
		ot, err := model.GetOpenTokenPublicKey(ctx, model.Endpoint{
			Token:       cfg.Token,
			APIEndpoint: cfg.APIEndpoint,
		}, 0)

		if err != nil {
			slog.Error("Failed to get the open token public key", slog.Any("err", err))
		}
		if len(ot.PublicKey) > 0 {
			rs := model.NewRSAService()
			as := model.NewAESGCMService()

			k, _, err := as.GenerateKeys()

			if err != nil {
				slog.Error("Failed to generate aes-gcm key", slog.Any("err", err))
			}

			encodedKey, _, err := rs.Encrypt(ot.PublicKey, k)

			if err != nil {
				slog.Error("Failed to encrypt key", slog.Any("err", err))
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

			realPayload = model.PostTrackArgs{
				Encrypted: base64.StdEncoding.EncodeToString(encryptedData),
				AesKey:    base64.StdEncoding.EncodeToString(encodedKey),
				Nonce:     base64.StdEncoding.EncodeToString(nonce),
			}
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

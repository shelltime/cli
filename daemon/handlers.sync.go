package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/vmihailenco/msgpack/v5"
)

func handlePubSubSync(ctx context.Context, socketMsgPayload interface{}) error {
	pb, err := msgpack.Marshal(socketMsgPayload)
	if err != nil {
		slog.Error("Failed to marshal the sync payload again for unmarshal", slog.Any("payload", socketMsgPayload))
		return err
	}

	var syncMsg model.PostTrackArgs
	err = msgpack.Unmarshal(pb, &syncMsg)
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

	// Call SendLocalDataToServer
	slog.Debug("Sending local data to server",
		slog.Any("ctx", ctx),
		slog.Any("cfg", cfg),
		slog.Time("cursor", time.Unix(0, syncMsg.CursorID)),
		slog.Any("data", syncMsg.Data),
		slog.Any("meta", syncMsg.Meta),
	)

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
				Encrypted: string(encryptedData),
				AesKey:    string(encodedKey),
				Nonce:     string(nonce),
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
		slog.Error("Failed to sync data to server", slog.Any("err", err))
		return err
	}
	return nil
}

package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/malamtime/cli/model"
)

func handlePubSubHeartbeat(ctx context.Context, socketMsgPayload interface{}) error {
	pb, err := json.Marshal(socketMsgPayload)
	if err != nil {
		slog.Error("Failed to marshal the heartbeat payload again for unmarshal", slog.Any("payload", socketMsgPayload))
		return err
	}

	var heartbeatPayload model.HeartbeatPayload
	err = json.Unmarshal(pb, &heartbeatPayload)
	if err != nil {
		slog.Error("Failed to parse heartbeat payload", slog.Any("payload", socketMsgPayload))
		return err
	}

	if len(heartbeatPayload.Heartbeats) == 0 {
		slog.Debug("Empty heartbeat payload, skipping")
		return nil
	}

	cfg, err := stConfig.ReadConfigFile(ctx)
	if err != nil {
		slog.Error("Failed to read config file", slog.Any("err", err))
		return err
	}

	// Try to send to server
	err = model.SendHeartbeatsToServer(ctx, cfg, heartbeatPayload)
	if err != nil {
		slog.Warn("Failed to send heartbeats to server, saving to local file", slog.Any("err", err))
		// On failure, save to local file
		if saveErr := saveHeartbeatToFile(heartbeatPayload); saveErr != nil {
			slog.Error("Failed to save heartbeat to local file", slog.Any("err", saveErr))
			return saveErr
		}
		// Return nil because we saved the data locally - don't nack the message
		return nil
	}

	slog.Info("Successfully sent heartbeats to server", slog.Int("count", len(heartbeatPayload.Heartbeats)))
	return nil
}

// saveHeartbeatToFile appends a heartbeat payload as a single JSON line to the log file
func saveHeartbeatToFile(payload model.HeartbeatPayload) error {
	logFilePath := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", model.HEARTBEAT_LOG_FILE))

	// Open file for appending, create if not exists
	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open heartbeat log file: %w", err)
	}
	defer file.Close()

	// Marshal payload to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat payload: %w", err)
	}

	// Write as single line with newline
	_, err = file.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write heartbeat to file: %w", err)
	}

	slog.Debug("Saved heartbeat to local file", slog.String("path", logFilePath), slog.Int("count", len(payload.Heartbeats)))
	return nil
}

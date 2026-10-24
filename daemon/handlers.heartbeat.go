package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"

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

	// The editor dropped these when the daemon acknowledged them: while the daemon stops they
	// go to the local file, and a send it cuts short fails and is saved below.
	if isShuttingDown() {
		return saveHeartbeatsForResync(heartbeatPayload)
	}
	ctx, cancel := withShutdown(ctx)
	defer cancel()

	// Read from disk, so a token from a new `shelltime auth` is used without a daemon restart.
	// While the file can't be read (a typo, or `shelltime auth` rewriting it), the last good
	// config still has the token.
	cfg, err := stConfig.ReadConfigFile(ctx, model.WithSkipCache())
	if err != nil {
		cfg, err = stConfig.ReadConfigFile(ctx)
	}
	if err != nil {
		slog.Error("Failed to read config file, saving heartbeats to local file", slog.Any("err", err))
		return saveHeartbeatsForResync(heartbeatPayload)
	}

	// Try to send to server
	response, err := model.SendHeartbeatsToServer(ctx, cfg, heartbeatPayload)
	if err != nil {
		slog.Warn("Failed to send heartbeats to server, saving to local file", slog.Any("err", err))
		return saveHeartbeatsForResync(heartbeatPayload)
	}

	if invalid := countInvalidHeartbeats(heartbeatPayload); response.Errors > invalid {
		// Servers before chunked inserts answer 200 when storing the batch failed
		slog.Warn("Server did not store valid heartbeats, saving to local file",
			slog.Int("count", len(heartbeatPayload.Heartbeats)), slog.Int("errors", response.Errors))
		return saveHeartbeatsForResync(heartbeatPayload)
	} else if response.Errors > 0 {
		// Rows without an id or entity: a plugin bug, not worth retrying
		slog.Warn("Server rejected some heartbeats", slog.Int("count", len(heartbeatPayload.Heartbeats)), slog.Int("rejected", response.Errors))
		return nil
	}
	slog.Info("Successfully sent heartbeats to server", slog.Int("count", len(heartbeatPayload.Heartbeats)))
	return nil
}

// saveHeartbeatsForResync keeps a batch the server didn't take for the resync service.
// It returns nil once saved, so the message isn't nacked and retried.
func saveHeartbeatsForResync(payload model.HeartbeatPayload) error {
	if err := saveHeartbeatToFile(payload); err != nil {
		slog.Error("Failed to save heartbeat to local file", slog.Any("err", err))
		return err
	}
	return nil
}

// countInvalidHeartbeats counts the rows the server rejects without trying to store them
func countInvalidHeartbeats(payload model.HeartbeatPayload) int {
	invalid := 0
	for _, h := range payload.Heartbeats {
		if h.HeartbeatID == "" || h.Entity == "" {
			invalid++
		}
	}
	return invalid
}

// heartbeatLogMu guards the heartbeat log file: failed sends append to it while the
// resync service reads and rewrites it.
var heartbeatLogMu sync.Mutex

// saveHeartbeatToFile appends a heartbeat payload as a single JSON line to the log file
func saveHeartbeatToFile(payload model.HeartbeatPayload) error {
	heartbeatLogMu.Lock()
	defer heartbeatLogMu.Unlock()

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

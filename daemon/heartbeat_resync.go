package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

const (
	// HeartbeatResyncInterval is the interval for retrying failed heartbeats
	HeartbeatResyncInterval = 30 * time.Minute

	// heartbeatResyncChunkSize caps the heartbeats per resend. Servers before chunked
	// inserts store a request in one insert, which fails past Postgres' 65535 parameters,
	// so a large saved batch would fail on every pass.
	heartbeatResyncChunkSize = 500
)

// HeartbeatResyncService handles periodic resync of failed heartbeats
type HeartbeatResyncService struct {
	config model.ShellTimeConfig
	// configService, when set, is read again at each pass, so a new token or code tracking
	// turned on after the daemon started applies without a restart.
	configService model.ConfigService
	ticker        *time.Ticker
	stopChan      chan struct{}
	wg            sync.WaitGroup
}

// NewHeartbeatResyncService creates a new heartbeat resync service
func NewHeartbeatResyncService(config model.ShellTimeConfig) *HeartbeatResyncService {
	return &HeartbeatResyncService{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// SetConfigService makes each pass use the config on disk instead of the one the daemon
// started with.
func (s *HeartbeatResyncService) SetConfigService(cs model.ConfigService) {
	s.configService = cs
}

func (s *HeartbeatResyncService) currentConfig(ctx context.Context) model.ShellTimeConfig {
	if s.configService != nil {
		if cfg, err := s.configService.ReadConfigFile(ctx, model.WithSkipCache()); err == nil {
			return cfg
		}
	}
	return s.config
}

// Start begins the periodic resync job
func (s *HeartbeatResyncService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(HeartbeatResyncInterval)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		// Run once at startup
		s.resync(ctx)

		for {
			select {
			case <-s.ticker.C:
				s.resync(ctx)
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	slog.Info("Heartbeat resync service started", slog.Duration("interval", HeartbeatResyncInterval))
	return nil
}

// Stop stops the resync service
func (s *HeartbeatResyncService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Heartbeat resync service stopped")
}

// resync reads failed heartbeats from the log file and attempts to send them
func (s *HeartbeatResyncService) resync(ctx context.Context) {
	logFilePath := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", model.HEARTBEAT_LOG_FILE))

	cfg := s.currentConfig(ctx)
	if cfg.CodeTracking == nil || cfg.CodeTracking.Enabled == nil || !*cfg.CodeTracking.Enabled {
		slog.Debug("Code tracking is disabled, keeping saved heartbeats for later")
		return
	}

	heartbeatLogMu.Lock()
	lines, err := readHeartbeatLogLines(logFilePath)
	heartbeatLogMu.Unlock()
	if os.IsNotExist(err) {
		slog.Debug("No heartbeat log file found, nothing to resync")
		return
	}
	if err != nil {
		slog.Error("Error reading heartbeat log file", slog.Any("err", err))
		return
	}

	if len(lines) == 0 {
		slog.Debug("No failed heartbeats to resync")
		return
	}

	slog.Info("Starting heartbeat resync", slog.Int("pendingCount", len(lines)))

	// Process each line
	var failedLines []string
	successCount := 0

	for _, line := range lines {
		var payload model.HeartbeatPayload
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			slog.Error("Failed to parse heartbeat line, discarding", slog.Any("err", err), slog.String("line", line))
			continue
		}

		// Each chunk the server didn't store is kept as its own line
		for _, chunk := range chunkHeartbeatPayload(payload, heartbeatResyncChunkSize) {
			if !resendHeartbeats(ctx, cfg, chunk) {
				kept, err := json.Marshal(chunk)
				if err != nil {
					slog.Error("Failed to encode heartbeats to keep, keeping the whole line", slog.Any("err", err))
					kept = []byte(line)
				}
				failedLines = append(failedLines, string(kept))
				continue
			}
			successCount++
		}
	}

	heartbeatLogMu.Lock()
	defer heartbeatLogMu.Unlock()

	// Heartbeats saved while this resync was sending were appended after the lines it read.
	// If the file no longer starts with them (log cleanup removed it meanwhile), every line
	// in it is new.
	current, err := readHeartbeatLogLines(logFilePath)
	if err != nil && !os.IsNotExist(err) {
		slog.Error("Error re-reading heartbeat log file, keeping it as is", slog.Any("err", err))
		return
	}
	if hasPrefixLines(current, lines) {
		current = current[len(lines):]
	}
	failedLines = append(failedLines, current...)

	// Rewrite the file with only failed lines
	if err := s.rewriteLogFile(logFilePath, failedLines); err != nil {
		slog.Error("Failed to update heartbeat log file", slog.Any("err", err))
		return
	}

	slog.Info("Heartbeat resync completed",
		slog.Int("success", successCount),
		slog.Int("remaining", len(failedLines)))
}

// resendHeartbeats sends a saved batch and reports whether the server stored it, by the same
// rule as handlePubSubHeartbeat: servers before chunked inserts answer 200 with every row as
// an error when storing failed.
func resendHeartbeats(ctx context.Context, cfg model.ShellTimeConfig, payload model.HeartbeatPayload) bool {
	response, err := model.SendHeartbeatsToServer(ctx, cfg, payload)
	if err != nil {
		slog.Warn("Failed to resync heartbeats, keeping for next retry", slog.Any("err", err))
		return false
	}
	if response.Errors > countInvalidHeartbeats(payload) {
		slog.Warn("Server did not store resynced heartbeats, keeping for next retry",
			slog.Int("count", len(payload.Heartbeats)), slog.Int("errors", response.Errors))
		return false
	}
	return true
}

// chunkHeartbeatPayload splits a saved batch into batches of at most size heartbeats
func chunkHeartbeatPayload(payload model.HeartbeatPayload, size int) []model.HeartbeatPayload {
	var chunks []model.HeartbeatPayload
	for heartbeats := range slices.Chunk(payload.Heartbeats, size) {
		chunks = append(chunks, model.HeartbeatPayload{Heartbeats: heartbeats})
	}
	return chunks
}

func hasPrefixLines(lines, prefix []string) bool {
	if len(lines) < len(prefix) {
		return false
	}
	for i := range prefix {
		if lines[i] != prefix[i] {
			return false
		}
	}
	return true
}

// readHeartbeatLogLines returns the non-empty lines of the heartbeat log file. Each line
// is one saved batch, which can be megabytes long, so lines are not size limited.
func readHeartbeatLogLines(logFilePath string) ([]string, error) {
	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if line = strings.TrimRight(line, "\r\n"); line != "" {
			lines = append(lines, line)
		}
		if err == io.EOF {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// rewriteLogFile atomically rewrites the log file with the given lines
func (s *HeartbeatResyncService) rewriteLogFile(logFilePath string, lines []string) error {
	// If no lines remaining, remove the file
	if len(lines) == 0 {
		if err := os.Remove(logFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty log file: %w", err)
		}
		return nil
	}

	// Write to temp file first
	tempFile := logFilePath + ".tmp"
	file, err := os.OpenFile(tempFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	for _, line := range lines {
		if _, err := file.WriteString(line + "\n"); err != nil {
			file.Close()
			os.Remove(tempFile)
			return fmt.Errorf("failed to write to temp file: %w", err)
		}
	}

	if err := file.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile, logFilePath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

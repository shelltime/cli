package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

const (
	// HeartbeatResyncInterval is the interval for retrying failed heartbeats
	HeartbeatResyncInterval = 30 * time.Minute
)

// HeartbeatResyncService handles periodic resync of failed heartbeats
type HeartbeatResyncService struct {
	config   model.ShellTimeConfig
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewHeartbeatResyncService creates a new heartbeat resync service
func NewHeartbeatResyncService(config model.ShellTimeConfig) *HeartbeatResyncService {
	return &HeartbeatResyncService{
		config:   config,
		stopChan: make(chan struct{}),
	}
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

	// Check if file exists
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		slog.Debug("No heartbeat log file found, nothing to resync")
		return
	}

	// Read the file
	file, err := os.Open(logFilePath)
	if err != nil {
		slog.Error("Failed to open heartbeat log file for resync", slog.Any("err", err))
		return
	}

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	file.Close()

	if err := scanner.Err(); err != nil {
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

		// Try to send to server
		if err := model.SendHeartbeatsToServer(ctx, s.config, payload); err != nil {
			slog.Warn("Failed to resync heartbeat, keeping for next retry", slog.Any("err", err))
			failedLines = append(failedLines, line)
		} else {
			successCount++
		}
	}

	// Rewrite the file with only failed lines
	if err := s.rewriteLogFile(logFilePath, failedLines); err != nil {
		slog.Error("Failed to update heartbeat log file", slog.Any("err", err))
		return
	}

	slog.Info("Heartbeat resync completed",
		slog.Int("success", successCount),
		slog.Int("remaining", len(failedLines)))
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

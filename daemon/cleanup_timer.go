package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

const (
	// CleanupInterval is the interval for log cleanup (24 hours)
	CleanupInterval = 24 * time.Hour
)

// CleanupTimerService handles periodic cleanup of large log files
type CleanupTimerService struct {
	config   model.ShellTimeConfig
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewCleanupTimerService creates a new cleanup timer service
func NewCleanupTimerService(config model.ShellTimeConfig) *CleanupTimerService {
	return &CleanupTimerService{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start begins the periodic cleanup job
func (s *CleanupTimerService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(CleanupInterval)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		// NOTE: Do not run at startup, only on timer
		// This avoids slowing daemon startup and prevents cleanup on restart loops

		for {
			select {
			case <-s.ticker.C:
				s.cleanup(ctx)
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	slog.Info("Cleanup timer service started",
		slog.Duration("interval", CleanupInterval),
		slog.Int64("thresholdMB", s.config.LogCleanup.ThresholdMB))
	return nil
}

// Stop stops the cleanup service
func (s *CleanupTimerService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Cleanup timer service stopped")
}

// cleanup performs the log cleanup
func (s *CleanupTimerService) cleanup(ctx context.Context) {
	thresholdBytes := s.config.LogCleanup.ThresholdMB * 1024 * 1024

	slog.Debug("Starting scheduled log cleanup",
		slog.Int64("thresholdMB", s.config.LogCleanup.ThresholdMB))

	var totalFreed int64

	// Clean CLI log files
	freedCLI, err := model.CleanLargeLogFiles(thresholdBytes, false)
	if err != nil {
		slog.Warn("error during CLI log cleanup", slog.Any("err", err))
	}
	totalFreed += freedCLI

	// Clean daemon log files (macOS only)
	freedDaemon, err := model.CleanDaemonLogFiles(thresholdBytes, false)
	if err != nil {
		slog.Warn("error during daemon log cleanup", slog.Any("err", err))
	}
	totalFreed += freedDaemon

	if totalFreed > 0 {
		slog.Info("scheduled log cleanup completed",
			slog.Int64("totalFreedBytes", totalFreed),
			slog.Int64("cliFreedBytes", freedCLI),
			slog.Int64("daemonFreedBytes", freedDaemon))
	} else {
		slog.Debug("scheduled log cleanup completed, no files exceeded threshold")
	}
}

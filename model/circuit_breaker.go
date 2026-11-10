package model

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxConsecutiveFailures = 10
	DefaultCircuitResetInterval   = 1 * time.Hour
)

// CircuitBreaker defines the interface for circuit breaker operations
type CircuitBreaker interface {
	IsOpen() bool
	RecordSuccess()
	RecordFailure()
	SaveForRetry(ctx context.Context, payload []byte) error
}

// CircuitBreakerConfig holds configuration for the circuit breaker service
type CircuitBreakerConfig struct {
	MaxConsecutiveFailures int
	ResetInterval          time.Duration
}

// RepublishFunc is called when retrying pending data
type RepublishFunc func(data []byte) error

// CircuitBreakerService handles circuit breaker with retry functionality
type CircuitBreakerService struct {
	mu                  sync.RWMutex
	consecutiveFailures int
	isOpen              bool
	config              CircuitBreakerConfig
	republishFn         RepublishFunc
	ticker              *time.Ticker
	stopChan            chan struct{}
	wg                  sync.WaitGroup

	// pendingMu guards the pending sync file: SaveForRetry appends to it while
	// the retry timer reads and rewrites it.
	pendingMu sync.Mutex
}

// NewCircuitBreakerService creates a new circuit breaker service
func NewCircuitBreakerService(config CircuitBreakerConfig, republishFn RepublishFunc) *CircuitBreakerService {
	if config.MaxConsecutiveFailures <= 0 {
		config.MaxConsecutiveFailures = DefaultMaxConsecutiveFailures
	}
	if config.ResetInterval <= 0 {
		config.ResetInterval = DefaultCircuitResetInterval
	}
	return &CircuitBreakerService{
		config:      config,
		republishFn: republishFn,
		stopChan:    make(chan struct{}),
	}
}

// Start begins the periodic reset/retry timer
func (s *CircuitBreakerService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(s.config.ResetInterval)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		for {
			select {
			case <-s.ticker.C:
				s.checkAndRetry(ctx)
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	slog.Info("Circuit breaker service started", slog.Duration("interval", s.config.ResetInterval))
	return nil
}

// Stop stops the circuit breaker service
func (s *CircuitBreakerService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Circuit breaker service stopped")
}

// IsOpen returns true if circuit is open
func (s *CircuitBreakerService) IsOpen() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isOpen
}

// RecordSuccess resets failure counter and closes circuit
func (s *CircuitBreakerService) RecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures = 0
	s.isOpen = false
}

// RecordFailure increments failure counter, opens circuit at threshold
func (s *CircuitBreakerService) RecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures++
	if s.consecutiveFailures >= s.config.MaxConsecutiveFailures {
		if !s.isOpen {
			slog.Error("Circuit breaker opened due to consecutive failures - server may be experiencing issues",
				slog.Int("failures", s.consecutiveFailures))
		}
		s.isOpen = true
	}
}

// SaveForRetry saves payload to file for later retry
func (s *CircuitBreakerService) SaveForRetry(ctx context.Context, payload []byte) error {
	filePath := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", SYNC_PENDING_FILE))

	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}

	// One write, so a line is never left without its newline
	line := make([]byte, 0, len(payload)+1)
	line = append(append(line, payload...), '\n')
	if _, err := file.Write(line); err != nil {
		return err
	}

	slog.Info("Saved data for later retry")
	return nil
}

// GetConsecutiveFailures returns the current failure count (for testing)
func (s *CircuitBreakerService) GetConsecutiveFailures() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.consecutiveFailures
}

func (s *CircuitBreakerService) checkAndRetry(ctx context.Context) {
	s.mu.Lock()
	if s.isOpen {
		slog.Info("Circuit breaker reset by timer, attempting to retry saved data")
		s.isOpen = false
		s.consecutiveFailures = 0
	}
	s.mu.Unlock()

	s.retryPendingData(ctx)
}

func (s *CircuitBreakerService) retryPendingData(ctx context.Context) {
	filePath := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", SYNC_PENDING_FILE))

	s.pendingMu.Lock()
	lines, err := readPendingLines(filePath)
	s.pendingMu.Unlock()
	if os.IsNotExist(err) {
		slog.Debug("No pending sync file found, nothing to retry")
		return
	}
	if err != nil {
		slog.Error("Error reading pending sync file", slog.Any("err", err))
		return
	}

	if len(lines) == 0 {
		slog.Debug("No pending sync data to retry")
		return
	}

	slog.Info("Starting sync data retry", slog.Int("pendingCount", len(lines)))

	var failedLines []string
	successCount := 0

	for _, line := range lines {
		if s.republishFn == nil {
			slog.Error("No republish function configured")
			failedLines = append(failedLines, line)
			continue
		}

		if err := s.republishFn([]byte(line)); err != nil {
			slog.Warn("Failed to republish sync data, keeping for next retry", slog.Any("err", err))
			failedLines = append(failedLines, line)
		} else {
			successCount++
		}
	}

	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	// Payloads saved while this retry was republishing were appended after the lines it
	// read. If the file no longer starts with them (log cleanup removed it meanwhile),
	// every line in it is new.
	current, err := readPendingLines(filePath)
	if err != nil && !os.IsNotExist(err) {
		slog.Error("Error re-reading pending sync file, keeping it as is", slog.Any("err", err))
		return
	}
	if hasLinePrefix(current, lines) {
		current = current[len(lines):]
	}
	failedLines = append(failedLines, current...)

	if err := s.rewriteLogFile(filePath, failedLines); err != nil {
		slog.Error("Failed to update pending sync file", slog.Any("err", err))
		return
	}

	slog.Info("Sync data retry completed",
		slog.Int("republished", successCount),
		slog.Int("remaining", len(failedLines)))
}

// readPendingLines returns the non-empty lines of the pending sync file. Each line is one
// saved sync payload, a whole backlog of commands when the server was away for long, so
// lines are not size limited: a bufio.Scanner stops at 64 KB, and a file it can't read is
// never retried.
func readPendingLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
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

func hasLinePrefix(lines, prefix []string) bool {
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

func (s *CircuitBreakerService) rewriteLogFile(logFilePath string, lines []string) error {
	if len(lines) == 0 {
		if err := os.Remove(logFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty log file: %w", err)
		}
		return nil
	}

	tempFile := logFilePath + ".tmp"
	file, err := os.OpenFile(tempFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
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

	if err := file.Chmod(0600); err != nil {
		file.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to restrict pending sync file permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tempFile, logFilePath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

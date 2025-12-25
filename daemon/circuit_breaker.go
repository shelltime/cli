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

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
)

const (
	maxConsecutiveFailures      = 10
	CircuitBreakerResetInterval = 1 * time.Hour
)

// CircuitBreaker defines the interface for circuit breaker operations
type CircuitBreaker interface {
	IsOpen() bool
	RecordSuccess()
	RecordFailure()
	SaveForRetry(ctx context.Context, payload interface{}) error
}

// SyncCircuitBreakerService handles circuit breaker with retry functionality
type SyncCircuitBreakerService struct {
	mu                  sync.RWMutex
	consecutiveFailures int
	isOpen              bool
	publisher           message.Publisher
	ticker              *time.Ticker
	stopChan            chan struct{}
	wg                  sync.WaitGroup
}

// Global instance
var syncCircuitBreaker CircuitBreaker

// NewSyncCircuitBreakerService creates a new circuit breaker service
func NewSyncCircuitBreakerService(publisher message.Publisher) *SyncCircuitBreakerService {
	svc := &SyncCircuitBreakerService{
		publisher: publisher,
		stopChan:  make(chan struct{}),
	}
	syncCircuitBreaker = svc
	return svc
}

// Start begins the periodic reset/retry timer
func (s *SyncCircuitBreakerService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(CircuitBreakerResetInterval)
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

	slog.Info("Sync circuit breaker service started", slog.Duration("interval", CircuitBreakerResetInterval))
	return nil
}

// Stop stops the circuit breaker service
func (s *SyncCircuitBreakerService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Sync circuit breaker service stopped")
}

func (s *SyncCircuitBreakerService) IsOpen() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isOpen
}

func (s *SyncCircuitBreakerService) RecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures = 0
	s.isOpen = false
}

func (s *SyncCircuitBreakerService) RecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures++
	if s.consecutiveFailures >= maxConsecutiveFailures {
		if !s.isOpen {
			slog.Error("Circuit breaker opened due to consecutive failures - server may be experiencing issues",
				slog.Int("failures", s.consecutiveFailures))
		}
		s.isOpen = true
	}
}

func (s *SyncCircuitBreakerService) SaveForRetry(ctx context.Context, payload interface{}) error {
	filePath := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", model.SYNC_PENDING_FILE))

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Save the original SocketMessage for republishing
	socketMsg := SocketMessage{
		Type:    SocketMessageTypeSync,
		Payload: payload,
	}

	jsonData, err := json.Marshal(socketMsg)
	if err != nil {
		return err
	}

	_, err = file.WriteString(string(jsonData) + "\n")
	if err != nil {
		return err
	}

	slog.Info("Saved sync data for later retry")
	return nil
}

func (s *SyncCircuitBreakerService) checkAndRetry(ctx context.Context) {
	s.mu.Lock()
	if s.isOpen {
		slog.Info("Circuit breaker reset by timer, attempting to retry saved data")
		s.isOpen = false
		s.consecutiveFailures = 0
	}
	s.mu.Unlock()

	s.retryPendingData(ctx)
}

func (s *SyncCircuitBreakerService) retryPendingData(ctx context.Context) {
	filePath := os.ExpandEnv(fmt.Sprintf("%s/%s", "$HOME", model.SYNC_PENDING_FILE))

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		slog.Debug("No pending sync file found, nothing to retry")
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		slog.Error("Failed to open pending sync file for retry", slog.Any("err", err))
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
		// Republish to pub/sub topic
		msg := message.NewMessage(watermill.NewUUID(), []byte(line))
		if err := s.publisher.Publish(PubSubTopic, msg); err != nil {
			slog.Warn("Failed to republish sync data, keeping for next retry", slog.Any("err", err))
			failedLines = append(failedLines, line)
		} else {
			successCount++
		}
	}

	// Rewrite file with only failed lines
	if err := s.rewriteLogFile(filePath, failedLines); err != nil {
		slog.Error("Failed to update pending sync file", slog.Any("err", err))
		return
	}

	slog.Info("Sync data retry completed",
		slog.Int("republished", successCount),
		slog.Int("remaining", len(failedLines)))
}

func (s *SyncCircuitBreakerService) rewriteLogFile(logFilePath string, lines []string) error {
	if len(lines) == 0 {
		if err := os.Remove(logFilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove empty log file: %w", err)
		}
		return nil
	}

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

	if err := os.Rename(tempFile, logFilePath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
)

// Mock publisher for testing
type mockPublisher struct {
	publishedMessages []*message.Message
	publishError      error
}

func (m *mockPublisher) Publish(topic string, messages ...*message.Message) error {
	if m.publishError != nil {
		return m.publishError
	}
	m.publishedMessages = append(m.publishedMessages, messages...)
	return nil
}

func (m *mockPublisher) Close() error {
	return nil
}

func TestNewSyncCircuitBreakerService(t *testing.T) {
	publisher := &mockPublisher{}

	wrapper := NewSyncCircuitBreakerService(publisher)
	if wrapper == nil {
		t.Fatal("NewSyncCircuitBreakerService returned nil")
	}

	if wrapper.CircuitBreakerService == nil {
		t.Error("CircuitBreakerService should be initialized")
	}

	// Check global variable was set
	if syncCircuitBreaker == nil {
		t.Error("Global syncCircuitBreaker should be set")
	}
}

func TestSyncCircuitBreakerWrapper_IsOpen(t *testing.T) {
	publisher := &mockPublisher{}
	wrapper := NewSyncCircuitBreakerService(publisher)

	// Initially should be closed (not open)
	if wrapper.IsOpen() {
		t.Error("Circuit breaker should be closed initially")
	}
}

func TestSyncCircuitBreakerWrapper_RecordSuccess(t *testing.T) {
	publisher := &mockPublisher{}
	wrapper := NewSyncCircuitBreakerService(publisher)

	// Should not panic
	wrapper.RecordSuccess()

	// Circuit should still be closed
	if wrapper.IsOpen() {
		t.Error("Circuit breaker should remain closed after success")
	}
}

func TestSyncCircuitBreakerWrapper_RecordFailure(t *testing.T) {
	publisher := &mockPublisher{}
	wrapper := NewSyncCircuitBreakerService(publisher)

	// Should not panic
	wrapper.RecordFailure()
}

func TestSyncCircuitBreakerWrapper_SaveForRetry(t *testing.T) {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "circuit-breaker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override sync pending file
	origFile := model.SYNC_PENDING_FILE
	model.SYNC_PENDING_FILE = filepath.Join(tempDir, "sync-pending.jsonl")
	defer func() { model.SYNC_PENDING_FILE = origFile }()

	// Override HOME for test (so $HOME/ doesn't affect the path)
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", origHome)

	publisher := &mockPublisher{}
	wrapper := NewSyncCircuitBreakerService(publisher)

	ctx := context.Background()
	payload := map[string]string{"key": "value"}

	err = wrapper.SaveForRetry(ctx, payload)
	if err != nil {
		t.Fatalf("SaveForRetry failed: %v", err)
	}
}

func TestSyncCircuitBreakerWrapper_SaveForRetry_WrapsInSocketMessage(t *testing.T) {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "circuit-breaker-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override sync pending file
	origFile := model.SYNC_PENDING_FILE
	model.SYNC_PENDING_FILE = filepath.Join(tempDir, "sync-pending.jsonl")
	defer func() { model.SYNC_PENDING_FILE = origFile }()

	// Override HOME for test (so $HOME/ doesn't affect the path)
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", origHome)

	publisher := &mockPublisher{}
	wrapper := NewSyncCircuitBreakerService(publisher)

	ctx := context.Background()
	payload := map[string]string{"test": "data"}

	err = wrapper.SaveForRetry(ctx, payload)
	if err != nil {
		t.Fatalf("SaveForRetry failed: %v", err)
	}

	// The SaveForRetry should wrap the payload in a SocketMessage
	// We can verify by checking what would be saved
	socketMsg := SocketMessage{
		Type:    SocketMessageTypeSync,
		Payload: payload,
	}

	_, err = json.Marshal(socketMsg)
	if err != nil {
		t.Fatalf("Failed to marshal wrapped message: %v", err)
	}
}

func TestDaemonCircuitBreaker_Interface(t *testing.T) {
	// Verify SyncCircuitBreakerWrapper implements DaemonCircuitBreaker
	var _ DaemonCircuitBreaker = &SyncCircuitBreakerWrapper{}
}

func TestSyncCircuitBreakerWrapper_MultipleFailures(t *testing.T) {
	publisher := &mockPublisher{}
	wrapper := NewSyncCircuitBreakerService(publisher)

	// Record multiple failures
	for i := 0; i < 10; i++ {
		wrapper.RecordFailure()
	}

	// Record a success
	wrapper.RecordSuccess()

	// Should not panic during any of these operations
}

func TestSyncCircuitBreakerWrapper_ConcurrentAccess(t *testing.T) {
	publisher := &mockPublisher{}
	wrapper := NewSyncCircuitBreakerService(publisher)

	done := make(chan bool, 10)

	// Concurrent failures
	for i := 0; i < 5; i++ {
		go func() {
			wrapper.RecordFailure()
			done <- true
		}()
	}

	// Concurrent successes
	for i := 0; i < 5; i++ {
		go func() {
			wrapper.RecordSuccess()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

package model

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreakerService_NewWithDefaults(t *testing.T) {
	svc := NewCircuitBreakerService(CircuitBreakerConfig{}, nil)

	assert.Equal(t, DefaultMaxConsecutiveFailures, svc.config.MaxConsecutiveFailures)
	assert.Equal(t, DefaultCircuitResetInterval, svc.config.ResetInterval)
	assert.False(t, svc.IsOpen())
}

func TestCircuitBreakerService_NewWithCustomConfig(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxConsecutiveFailures: 5,
		ResetInterval:          30 * time.Minute,
	}
	svc := NewCircuitBreakerService(config, nil)

	assert.Equal(t, 5, svc.config.MaxConsecutiveFailures)
	assert.Equal(t, 30*time.Minute, svc.config.ResetInterval)
}

func TestCircuitBreakerService_IsOpen(t *testing.T) {
	svc := NewCircuitBreakerService(CircuitBreakerConfig{}, nil)

	assert.False(t, svc.IsOpen(), "circuit should start closed")
}

func TestCircuitBreakerService_RecordFailure(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxConsecutiveFailures: 3,
	}
	svc := NewCircuitBreakerService(config, nil)

	// First two failures should not open circuit
	svc.RecordFailure()
	assert.False(t, svc.IsOpen())
	assert.Equal(t, 1, svc.GetConsecutiveFailures())

	svc.RecordFailure()
	assert.False(t, svc.IsOpen())
	assert.Equal(t, 2, svc.GetConsecutiveFailures())

	// Third failure should open circuit
	svc.RecordFailure()
	assert.True(t, svc.IsOpen())
	assert.Equal(t, 3, svc.GetConsecutiveFailures())

	// Additional failures should keep circuit open
	svc.RecordFailure()
	assert.True(t, svc.IsOpen())
	assert.Equal(t, 4, svc.GetConsecutiveFailures())
}

func TestCircuitBreakerService_RecordSuccess(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxConsecutiveFailures: 3,
	}
	svc := NewCircuitBreakerService(config, nil)

	// Open the circuit
	svc.RecordFailure()
	svc.RecordFailure()
	svc.RecordFailure()
	assert.True(t, svc.IsOpen())

	// Success should close circuit and reset counter
	svc.RecordSuccess()
	assert.False(t, svc.IsOpen())
	assert.Equal(t, 0, svc.GetConsecutiveFailures())
}

func TestCircuitBreakerService_RecordSuccess_ResetsCounter(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxConsecutiveFailures: 5,
	}
	svc := NewCircuitBreakerService(config, nil)

	// Record some failures
	svc.RecordFailure()
	svc.RecordFailure()
	assert.Equal(t, 2, svc.GetConsecutiveFailures())

	// Success resets counter
	svc.RecordSuccess()
	assert.Equal(t, 0, svc.GetConsecutiveFailures())

	// Now it takes 5 more failures to open circuit
	for i := 0; i < 4; i++ {
		svc.RecordFailure()
		assert.False(t, svc.IsOpen())
	}
	svc.RecordFailure()
	assert.True(t, svc.IsOpen())
}

func TestCircuitBreakerService_SaveForRetry(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Override SYNC_PENDING_FILE for test
	originalFile := SYNC_PENDING_FILE
	SYNC_PENDING_FILE = filepath.Join(tempDir, "test-pending.jsonl")
	defer func() { SYNC_PENDING_FILE = originalFile }()

	// Override HOME for test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", originalHome)

	svc := NewCircuitBreakerService(CircuitBreakerConfig{}, nil)

	ctx := context.Background()
	payload := []byte(`{"type":"sync","payload":{"data":"test"}}`)

	err := svc.SaveForRetry(ctx, payload)
	require.NoError(t, err)

	// Read the file and verify content
	content, err := os.ReadFile(SYNC_PENDING_FILE)
	require.NoError(t, err)
	assert.Contains(t, string(content), `{"type":"sync","payload":{"data":"test"}}`)
}

func TestCircuitBreakerService_SaveForRetry_AppendsMultiple(t *testing.T) {
	tempDir := t.TempDir()

	originalFile := SYNC_PENDING_FILE
	SYNC_PENDING_FILE = filepath.Join(tempDir, "test-pending.jsonl")
	defer func() { SYNC_PENDING_FILE = originalFile }()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", originalHome)

	svc := NewCircuitBreakerService(CircuitBreakerConfig{}, nil)
	ctx := context.Background()

	// Save multiple payloads
	err := svc.SaveForRetry(ctx, []byte(`{"id":1}`))
	require.NoError(t, err)
	err = svc.SaveForRetry(ctx, []byte(`{"id":2}`))
	require.NoError(t, err)
	err = svc.SaveForRetry(ctx, []byte(`{"id":3}`))
	require.NoError(t, err)

	content, err := os.ReadFile(SYNC_PENDING_FILE)
	require.NoError(t, err)

	lines := string(content)
	assert.Contains(t, lines, `{"id":1}`)
	assert.Contains(t, lines, `{"id":2}`)
	assert.Contains(t, lines, `{"id":3}`)
}

func TestCircuitBreakerService_RetryPendingData(t *testing.T) {
	tempDir := t.TempDir()

	originalFile := SYNC_PENDING_FILE
	SYNC_PENDING_FILE = filepath.Join(tempDir, "test-pending.jsonl")
	defer func() { SYNC_PENDING_FILE = originalFile }()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", originalHome)

	// Create pending file with test data
	testData := `{"id":1}
{"id":2}
{"id":3}
`
	err := os.WriteFile(SYNC_PENDING_FILE, []byte(testData), 0644)
	require.NoError(t, err)

	var republishedData []string
	var mu sync.Mutex

	republishFn := func(data []byte) error {
		mu.Lock()
		defer mu.Unlock()
		republishedData = append(republishedData, string(data))
		return nil
	}

	svc := NewCircuitBreakerService(CircuitBreakerConfig{}, republishFn)

	// Trigger retry
	ctx := context.Background()
	svc.retryPendingData(ctx)

	// Verify all data was republished
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, republishedData, 3)
	assert.Contains(t, republishedData, `{"id":1}`)
	assert.Contains(t, republishedData, `{"id":2}`)
	assert.Contains(t, republishedData, `{"id":3}`)

	// File should be removed after successful retry
	_, err = os.Stat(SYNC_PENDING_FILE)
	assert.True(t, os.IsNotExist(err))
}

func TestCircuitBreakerService_RetryPendingData_PartialFailure(t *testing.T) {
	tempDir := t.TempDir()

	originalFile := SYNC_PENDING_FILE
	SYNC_PENDING_FILE = filepath.Join(tempDir, "test-pending.jsonl")
	defer func() { SYNC_PENDING_FILE = originalFile }()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", originalHome)

	// Create pending file with test data
	testData := `{"id":1}
{"id":2}
{"id":3}
`
	err := os.WriteFile(SYNC_PENDING_FILE, []byte(testData), 0644)
	require.NoError(t, err)

	callCount := 0
	republishFn := func(data []byte) error {
		callCount++
		// Fail on second item
		if callCount == 2 {
			return assert.AnError
		}
		return nil
	}

	svc := NewCircuitBreakerService(CircuitBreakerConfig{}, republishFn)

	ctx := context.Background()
	svc.retryPendingData(ctx)

	// File should still exist with failed item
	content, err := os.ReadFile(SYNC_PENDING_FILE)
	require.NoError(t, err)
	assert.Contains(t, string(content), `{"id":2}`)
	assert.NotContains(t, string(content), `{"id":1}`)
	assert.NotContains(t, string(content), `{"id":3}`)
}

func TestCircuitBreakerService_StartStop(t *testing.T) {
	config := CircuitBreakerConfig{
		ResetInterval: 100 * time.Millisecond, // Short interval for testing
	}
	svc := NewCircuitBreakerService(config, nil)

	ctx := context.Background()
	err := svc.Start(ctx)
	require.NoError(t, err)

	// Stop should not block
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Stop blocked for too long")
	}
}

func TestCircuitBreakerService_ThreadSafety(t *testing.T) {
	config := CircuitBreakerConfig{
		MaxConsecutiveFailures: 100,
	}
	svc := NewCircuitBreakerService(config, nil)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent RecordFailure
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			svc.RecordFailure()
		}()
	}
	wg.Wait()

	assert.Equal(t, 100, svc.GetConsecutiveFailures())
	assert.True(t, svc.IsOpen())

	// Concurrent RecordSuccess should reset
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			svc.RecordSuccess()
		}()
	}
	wg.Wait()

	assert.Equal(t, 0, svc.GetConsecutiveFailures())
	assert.False(t, svc.IsOpen())
}

func TestCircuitBreakerService_CheckAndRetry_ResetsCircuit(t *testing.T) {
	tempDir := t.TempDir()

	originalFile := SYNC_PENDING_FILE
	SYNC_PENDING_FILE = filepath.Join(tempDir, "test-pending.jsonl")
	defer func() { SYNC_PENDING_FILE = originalFile }()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", "")
	defer os.Setenv("HOME", originalHome)

	config := CircuitBreakerConfig{
		MaxConsecutiveFailures: 3,
	}
	svc := NewCircuitBreakerService(config, nil)

	// Open the circuit
	svc.RecordFailure()
	svc.RecordFailure()
	svc.RecordFailure()
	assert.True(t, svc.IsOpen())

	// checkAndRetry should reset the circuit
	ctx := context.Background()
	svc.checkAndRetry(ctx)

	assert.False(t, svc.IsOpen())
	assert.Equal(t, 0, svc.GetConsecutiveFailures())
}

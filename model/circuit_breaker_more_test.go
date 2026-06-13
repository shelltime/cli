package model

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// m2pendingPath returns the path the breaker writes pending sync data to under
// the current $HOME (matches SaveForRetry/retryPendingData expansion) and
// ensures the parent .shelltime directory exists, since SaveForRetry only
// O_CREATEs the file, not its directory.
func m2pendingPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(os.Getenv("HOME"), SYNC_PENDING_FILE)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	return p
}

// TestCB_StateTransitions drives the public API through the
// closed -> open lifecycle and asserts IsOpen / GetConsecutiveFailures.
func TestCB_StateTransitions(t *testing.T) {
	cb := NewCircuitBreakerService(CircuitBreakerConfig{MaxConsecutiveFailures: 3}, nil)

	require.False(t, cb.IsOpen(), "fresh breaker is closed")

	cb.RecordFailure()
	cb.RecordFailure()
	assert.False(t, cb.IsOpen(), "below threshold stays closed")
	assert.Equal(t, 2, cb.GetConsecutiveFailures())

	cb.RecordFailure() // hits threshold
	assert.True(t, cb.IsOpen(), "at threshold the breaker opens")

	// Recording another failure while already open keeps it open (covers the
	// !s.isOpen guard being false).
	cb.RecordFailure()
	assert.True(t, cb.IsOpen())

	// Success closes it and resets the counter.
	cb.RecordSuccess()
	assert.False(t, cb.IsOpen())
	assert.Equal(t, 0, cb.GetConsecutiveFailures())
}

// TestCB_StartTimerResetsAndRetries uses a very short reset interval so the
// background timer fires, closes the open circuit, and runs retryPendingData.
// Uses assert.Eventually (no raw sleeps) to observe the half-open->closed reset.
func TestCB_StartTimerResetsAndRetries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var republished int32
	cb := NewCircuitBreakerService(
		CircuitBreakerConfig{MaxConsecutiveFailures: 1, ResetInterval: 5 * time.Millisecond},
		func(data []byte) error {
			atomic.AddInt32(&republished, 1)
			return nil
		},
	)

	// Open the circuit and stage one pending payload (helper creates the dir).
	pending := m2pendingPath(t)
	cb.RecordFailure()
	require.True(t, cb.IsOpen())
	require.NoError(t, cb.SaveForRetry(context.Background(), []byte(`{"a":1}`)))
	require.FileExists(t, pending)

	require.NoError(t, cb.Start(context.Background()))
	defer cb.Stop()

	// Timer should reset the breaker to closed and replay the pending line.
	assert.Eventually(t, func() bool {
		return !cb.IsOpen() && atomic.LoadInt32(&republished) >= 1
	}, 2*time.Second, 5*time.Millisecond, "timer should reset circuit and retry")

	// After a successful replay the pending file is removed (rewriteLogFile
	// empty-lines branch).
	assert.Eventually(t, func() bool {
		_, err := os.Stat(pending)
		return os.IsNotExist(err)
	}, 2*time.Second, 5*time.Millisecond, "pending file removed once drained")
}

// TestCB_RetryPendingData_NoFile covers the early return when there is nothing
// to retry.
func TestCB_RetryPendingData_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cb := NewCircuitBreakerService(CircuitBreakerConfig{}, func([]byte) error { return nil })
	// No file exists yet; should be a no-op and must not panic.
	assert.NotPanics(t, func() { cb.retryPendingData(context.Background()) })
}

// TestCB_RetryPendingData_NilRepublishKeepsLines verifies that with no
// republish function configured, all lines are treated as failed and retained
// in the rewritten file (rewriteLogFile non-empty branch).
func TestCB_RetryPendingData_NilRepublishKeepsLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cb := NewCircuitBreakerService(CircuitBreakerConfig{}, nil) // nil republishFn

	pending := m2pendingPath(t)
	require.NoError(t, cb.SaveForRetry(context.Background(), []byte(`{"x":1}`)))
	require.NoError(t, cb.SaveForRetry(context.Background(), []byte(`{"y":2}`)))

	cb.retryPendingData(context.Background())

	// The file should still exist with both lines retained.
	data, err := os.ReadFile(pending)
	require.NoError(t, err)
	assert.Contains(t, string(data), `{"x":1}`)
	assert.Contains(t, string(data), `{"y":2}`)
}

// TestCB_RetryPendingData_EmptyFile covers the "only blank lines" path: the
// scanner finds no usable lines and returns before rewriting.
func TestCB_RetryPendingData_EmptyFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := m2pendingPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("\n\n\n"), 0o644))

	cb := NewCircuitBreakerService(CircuitBreakerConfig{}, func([]byte) error { return nil })
	assert.NotPanics(t, func() { cb.retryPendingData(context.Background()) })
}

// TestCB_RewriteLogFile_RemovesEmpty exercises rewriteLogFile directly for the
// empty-lines branch (removes the file) and the missing-file sub-case.
func TestCB_RewriteLogFile_RemovesEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cb := NewCircuitBreakerService(CircuitBreakerConfig{}, nil)

	path := filepath.Join(t.TempDir(), "log.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("a\n"), 0o644))

	// Empty slice -> file removed.
	require.NoError(t, cb.rewriteLogFile(path, nil))
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))

	// Removing an already-missing file is not an error.
	require.NoError(t, cb.rewriteLogFile(path, nil))
}

// TestCB_RewriteLogFile_WritesLines covers the non-empty branch: a temp file is
// written and atomically renamed into place.
func TestCB_RewriteLogFile_WritesLines(t *testing.T) {
	cb := NewCircuitBreakerService(CircuitBreakerConfig{}, nil)
	path := filepath.Join(t.TempDir(), "log.jsonl")

	require.NoError(t, cb.rewriteLogFile(path, []string{"line1", "line2"}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2\n", string(data))
	// The temp file must not linger.
	_, err = os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err))
}

// TestCB_SaveForRetry_AppendsAndRetries covers SaveForRetry happy path plus a
// retry that drops a failing line and keeps it for the next pass.
func TestCB_SaveForRetry_PartialFailureRetained(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cb := NewCircuitBreakerService(CircuitBreakerConfig{}, func(data []byte) error {
		if string(data) == "bad" {
			return assert.AnError
		}
		return nil
	})

	pending := m2pendingPath(t)
	require.NoError(t, cb.SaveForRetry(context.Background(), []byte("good")))
	require.NoError(t, cb.SaveForRetry(context.Background(), []byte("bad")))

	cb.retryPendingData(context.Background())

	// "good" replayed and dropped; "bad" retained.
	data, err := os.ReadFile(pending)
	require.NoError(t, err)
	assert.Equal(t, "bad\n", string(data))
}

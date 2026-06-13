package daemon

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestX3CircuitBreaker_SaveForRetryMarshalError covers the json.Marshal error
// branch of SyncCircuitBreakerWrapper.SaveForRetry: a channel value cannot be
// marshaled, so the wrapper returns the marshal error before persisting.
func TestX3CircuitBreaker_SaveForRetryMarshalError(t *testing.T) {
	wrapper := NewSyncCircuitBreakerService(&mockPublisher{})
	// A chan cannot be JSON-marshaled -> error from the wrapper's Marshal.
	err := wrapper.SaveForRetry(context.Background(), make(chan int))
	require.Error(t, err)
}

// TestX3SocketHandler_DecodeError covers the decode-error branch of
// handleConnection: sending non-JSON bytes makes json.Decode fail, so the
// handler logs and closes the connection.
func TestX3SocketHandler_DecodeError(t *testing.T) {
	_, socketPath := startHandler(t, &model.ShellTimeConfig{})

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Garbage that is not valid JSON -> decoder.Decode returns an error.
	_, err = conn.Write([]byte("not-json-at-all\n"))
	require.NoError(t, err)

	// The handler closes the connection; a read should hit EOF without hanging.
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 8)
	_, _ = conn.Read(buf) // error expected; we only assert no panic / no hang
}

// TestX3SyncCodexUsage_FetchErrorPropagates covers the fetch-error branch of
// syncCodexUsage: loadCodexAuth succeeds but fetchCodexUsage returns a generic
// error, which propagates (and is not a known skip reason).
func TestX3SyncCodexUsage_FetchErrorPropagates(t *testing.T) {
	prevLoad := loadCodexAuthFunc
	prevFetch := fetchCodexUsageFunc
	t.Cleanup(func() {
		loadCodexAuthFunc = prevLoad
		fetchCodexUsageFunc = prevFetch
	})

	loadCodexAuthFunc = func() (*codexAuthData, error) {
		return &codexAuthData{AccessToken: "t"}, nil
	}
	fetchCodexUsageFunc = func(ctx context.Context, auth *codexAuthData) (*CodexRateLimitData, error) {
		return nil, assert.AnError
	}

	err := syncCodexUsage(context.Background(), model.ShellTimeConfig{Token: "tok"})
	require.Error(t, err)
	_, isSkip := CodexSyncSkipReason(err)
	assert.False(t, isSkip, "a generic fetch error is not a skip reason")
}

// TestX3CodexUsageSyncService_SyncSkipsOnKnownReason covers the sync() skip-reason
// branch: a known skip error (auth invalid) is logged and swallowed (no panic).
func TestX3CodexUsageSyncService_SyncSkipsOnKnownReason(t *testing.T) {
	prevLoad := loadCodexAuthFunc
	t.Cleanup(func() { loadCodexAuthFunc = prevLoad })
	loadCodexAuthFunc = func() (*codexAuthData, error) {
		return nil, errCodexAuthInvalid
	}

	svc := NewCodexUsageSyncService(model.ShellTimeConfig{Token: "tok", SocketPath: filepath.Join(t.TempDir(), "x.sock")})
	assert.NotPanics(t, func() { svc.sync() })
}

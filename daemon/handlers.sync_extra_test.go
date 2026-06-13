package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// fakeDaemonCB is a controllable DaemonCircuitBreaker for handler tests.
type fakeDaemonCB struct {
	open          bool
	saveErr       error
	savedPayloads []interface{}
	successCount  atomic.Int32
	failureCount  atomic.Int32
}

func (f *fakeDaemonCB) IsOpen() bool     { return f.open }
func (f *fakeDaemonCB) RecordSuccess()   { f.successCount.Add(1) }
func (f *fakeDaemonCB) RecordFailure()   { f.failureCount.Add(1) }
func (f *fakeDaemonCB) SaveForRetry(ctx context.Context, payload interface{}) error {
	f.savedPayloads = append(f.savedPayloads, payload)
	return f.saveErr
}

func withCircuitBreaker(t *testing.T, cb DaemonCircuitBreaker) {
	t.Helper()
	prev := syncCircuitBreaker
	syncCircuitBreaker = cb
	t.Cleanup(func() { syncCircuitBreaker = prev })
}

func TestHandlePubSubSync_CircuitBreakerOpen_SavesAndAcks(t *testing.T) {
	cb := &fakeDaemonCB{open: true}
	withCircuitBreaker(t, cb)
	// stConfig must NOT be consulted when the breaker is open.
	withStConfig(t, model.NewMockConfigService(t))

	payload := model.PostTrackArgs{CursorID: time.Now().UnixNano(), Data: []model.TrackingData{{Command: "ls"}}}
	err := handlePubSubSync(context.Background(), payload)
	require.NoError(t, err) // nil -> message acked
	require.Len(t, cb.savedPayloads, 1)
}

func TestHandlePubSubSync_CircuitBreakerOpen_SaveError(t *testing.T) {
	cb := &fakeDaemonCB{open: true, saveErr: errors.New("disk full")}
	withCircuitBreaker(t, cb)
	withStConfig(t, model.NewMockConfigService(t))

	payload := model.PostTrackArgs{CursorID: time.Now().UnixNano()}
	err := handlePubSubSync(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestSendTrackArgsToServer_SuccessRecordsSuccessAndResolvesTerminal(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/track", r.URL.Path)
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cb := &fakeDaemonCB{open: false}
	withCircuitBreaker(t, cb)

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)
	withStConfig(t, mockCS)

	// PPID 1 triggers ResolveTerminal (returns "unknown",""), exercising the
	// terminal-resolution branch of sendTrackArgsToServer.
	msg := model.PostTrackArgs{
		CursorID: time.Now().UnixNano(),
		Data:     []model.TrackingData{{Command: "ls", PPID: 1}},
		Meta:     model.TrackingMetaData{OS: "linux", Shell: "bash"},
	}

	err := sendTrackArgsToServer(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, int32(1), hits.Load())
	assert.Equal(t, int32(1), cb.successCount.Load())
	assert.Equal(t, int32(0), cb.failureCount.Load())
}

func TestSendTrackArgsToServer_FailureRecordsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cb := &fakeDaemonCB{open: false}
	withCircuitBreaker(t, cb)

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)
	withStConfig(t, mockCS)

	msg := model.PostTrackArgs{
		CursorID: time.Now().UnixNano(),
		Data:     []model.TrackingData{{Command: "ls"}},
		Meta:     model.TrackingMetaData{OS: "linux"},
	}

	err := sendTrackArgsToServer(context.Background(), msg)
	require.Error(t, err)
	assert.Equal(t, int32(1), cb.failureCount.Load())
	assert.Equal(t, int32(0), cb.successCount.Load())
}

func TestSendTrackArgsToServer_ConfigError(t *testing.T) {
	cb := &fakeDaemonCB{open: false}
	withCircuitBreaker(t, cb)

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, errors.New("cfg boom"))
	withStConfig(t, mockCS)

	msg := model.PostTrackArgs{CursorID: time.Now().UnixNano(), Data: []model.TrackingData{{Command: "x"}}}
	err := sendTrackArgsToServer(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cfg boom")
	// Neither success nor failure recorded; we never reached the send.
	assert.Equal(t, int32(0), cb.successCount.Load())
	assert.Equal(t, int32(0), cb.failureCount.Load())
}

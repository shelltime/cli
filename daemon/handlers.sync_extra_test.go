package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
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

func (f *fakeDaemonCB) IsOpen() bool   { return f.open }
func (f *fakeDaemonCB) RecordSuccess() { f.successCount.Add(1) }
func (f *fakeDaemonCB) RecordFailure() { f.failureCount.Add(1) }
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

// syncConfigFor points the daemon's config at a test API.
func syncConfigFor(t *testing.T, apiEndpoint string) {
	t.Helper()
	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: apiEndpoint,
	}, nil)
	withStConfig(t, mockCS)
}

// The CLI advances its cursor as soon as the daemon takes a sync message, so a batch the
// daemon can't deliver exists nowhere else. It used to be nacked, redelivered three times
// within a second and dropped.
func TestHandlePubSubSync_UnsentBatchIsKeptForRetry(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()

	tests := []struct {
		name        string
		apiEndpoint string
	}{
		{name: "server error", apiEndpoint: failing.URL},
		{name: "server unreachable", apiEndpoint: unreachable.URL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := &fakeDaemonCB{open: false}
			withCircuitBreaker(t, cb)
			syncConfigFor(t, tt.apiEndpoint)

			payload := model.PostTrackArgs{CursorID: time.Now().UnixNano(), Data: []model.TrackingData{{Command: "ls"}}}
			err := handlePubSubSync(context.Background(), payload)

			require.NoError(t, err, "saved batches are acked, not redelivered")
			require.Len(t, cb.savedPayloads, 1)
			assert.Equal(t, payload, cb.savedPayloads[0])
			assert.Equal(t, int32(1), cb.failureCount.Load())
		})
	}
}

// A batch the server refuses would be refused again on every retry.
func TestHandlePubSubSync_RefusedBatchIsNotKept(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	cb := &fakeDaemonCB{open: false}
	withCircuitBreaker(t, cb)
	syncConfigFor(t, server.URL)

	payload := model.PostTrackArgs{CursorID: time.Now().UnixNano(), Data: []model.TrackingData{{Command: "ls"}}}
	err := handlePubSubSync(context.Background(), payload)

	require.Error(t, err)
	assert.Empty(t, cb.savedPayloads)
}

// When the batch can't be saved either, the error still nacks the message so the quick
// redeliveries get their chance.
func TestHandlePubSubSync_UnsentBatchSaveError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cb := &fakeDaemonCB{open: false, saveErr: errors.New("disk full")}
	withCircuitBreaker(t, cb)
	syncConfigFor(t, server.URL)

	err := handlePubSubSync(context.Background(), model.PostTrackArgs{CursorID: 1, Data: []model.TrackingData{{Command: "ls"}}})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "disk full", "the send error is the one reported")
}

func TestIsRetryableSyncError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "server error", err: &model.HTTPStatusError{StatusCode: http.StatusBadGateway}, want: true},
		{name: "rate limited", err: &model.HTTPStatusError{StatusCode: http.StatusTooManyRequests}, want: true},
		{name: "request timeout", err: &model.HTTPStatusError{StatusCode: http.StatusRequestTimeout}, want: true},
		{name: "bad request", err: &model.HTTPStatusError{StatusCode: http.StatusBadRequest}, want: false},
		{name: "unprocessable", err: &model.HTTPStatusError{StatusCode: http.StatusUnprocessableEntity}, want: false},
		{name: "not found", err: &model.HTTPStatusError{StatusCode: http.StatusNotFound}, want: false},
		// The sender was refused, not the batch: it goes through after the next sign-in
		{name: "unauthorized", err: &model.HTTPStatusError{StatusCode: http.StatusUnauthorized}, want: true},
		{name: "forbidden", err: &model.HTTPStatusError{StatusCode: http.StatusForbidden}, want: true},
		{name: "payment required", err: &model.HTTPStatusError{StatusCode: http.StatusPaymentRequired}, want: true},
		// Several endpoints: one failure that can work later is enough, in any order
		{name: "refused by one endpoint, failing on another", want: true, err: errors.Join(
			&model.HTTPStatusError{StatusCode: http.StatusBadRequest}, &model.HTTPStatusError{StatusCode: http.StatusBadGateway})},
		{name: "failing on one endpoint, refused by another", want: true, err: errors.Join(
			&model.HTTPStatusError{StatusCode: http.StatusBadGateway}, &model.HTTPStatusError{StatusCode: http.StatusBadRequest})},
		{name: "refused by every endpoint", want: false, err: errors.Join(
			&model.HTTPStatusError{StatusCode: http.StatusBadRequest}, &model.HTTPStatusError{StatusCode: http.StatusUnprocessableEntity})},
		{name: "network error", err: &url.Error{Op: "Post", URL: "http://x", Err: errors.New("connection refused")}, want: true},
		{name: "local error", err: errors.New("encryption produced an empty key"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRetryableSyncError(tt.err))
		})
	}
}

// End to end with the real circuit breaker: the batch a failing server didn't take lands in
// sync-pending.jsonl as a sync message, and that line, handed back to the processor the way
// the hourly retry republishes it, is delivered once the server answers again.
func TestHandlePubSubSync_SavedBatchIsDeliveredWhenRepublished(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER), 0o700))

	var down atomic.Bool
	down.Store(true)
	var delivered atomic.Pointer[model.PostTrackArgs]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var got model.PostTrackArgs
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		delivered.Store(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	prev := syncCircuitBreaker
	t.Cleanup(func() { syncCircuitBreaker = prev })
	NewSyncCircuitBreakerService(&mockPublisher{})
	syncConfigFor(t, server.URL)

	payload := model.PostTrackArgs{
		CursorID: 1717243200000000000,
		Data:     []model.TrackingData{{Command: "make deploy", SessionID: 20240601120000}},
		Meta:     model.TrackingMetaData{Hostname: "h", Shell: "zsh"},
	}
	require.NoError(t, handlePubSubSync(context.Background(), payload))

	saved, err := os.ReadFile(filepath.Join(home, model.SYNC_PENDING_FILE))
	require.NoError(t, err)
	line := strings.TrimSuffix(string(saved), "\n")
	require.NotContains(t, line, "\n", "one batch, one line")

	down.Store(false)
	runTopicProcessor(t, []*message.Message{message.NewMessage("retry", []byte(line))})

	got := delivered.Load()
	require.NotNil(t, got, "the republished line must reach the server")
	assert.Equal(t, payload.CursorID, got.CursorID)
	require.Len(t, got.Data, 1)
	assert.Equal(t, "make deploy", got.Data[0].Command)
	assert.Equal(t, int64(20240601120000), got.Data[0].SessionID)
}

// A token that expired or was revoked gets a 401. The batch is not at fault: it is
// delivered once the user has signed in again, so it is kept like one the server couldn't
// take.
func TestHandlePubSubSync_BatchRefusedForItsTokenIsKept(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			cb := &fakeDaemonCB{open: false}
			withCircuitBreaker(t, cb)
			syncConfigFor(t, server.URL)

			payload := model.PostTrackArgs{CursorID: time.Now().UnixNano(), Data: []model.TrackingData{{Command: "ls"}}}
			require.NoError(t, handlePubSubSync(context.Background(), payload))
			require.Len(t, cb.savedPayloads, 1)
		})
	}
}

// With a second endpoint configured, which failure was reported used to be whichever
// request finished first. A batch one endpoint refused and the other couldn't take was
// kept or dropped by that race.
func TestHandlePubSubSync_SeveralEndpointsKeepTheBatchIfOneCanTakeItLater(t *testing.T) {
	refusing := func(delay time.Duration) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(delay)
			w.WriteHeader(http.StatusBadRequest)
		}))
	}
	failing := func(delay time.Duration) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(delay)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
	}
	const slow = 150 * time.Millisecond

	tests := []struct {
		name          string
		first, second *httptest.Server
		wantSaved     int
	}{
		{name: "the refusal arrives first", first: refusing(0), second: failing(slow), wantSaved: 1},
		{name: "the failure arrives first", first: refusing(slow), second: failing(0), wantSaved: 1},
		{name: "second endpoint refuses first", first: failing(slow), second: refusing(0), wantSaved: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer tt.first.Close()
			defer tt.second.Close()

			cb := &fakeDaemonCB{open: false}
			withCircuitBreaker(t, cb)
			mockCS := model.NewMockConfigService(t)
			mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
				Token:       "tok",
				APIEndpoint: tt.first.URL,
				Endpoints:   []model.Endpoint{{Token: "tok2", APIEndpoint: tt.second.URL}},
			}, nil)
			withStConfig(t, mockCS)

			payload := model.PostTrackArgs{CursorID: time.Now().UnixNano(), Data: []model.TrackingData{{Command: "ls"}}}
			require.NoError(t, handlePubSubSync(context.Background(), payload))
			require.Len(t, cb.savedPayloads, tt.wantSaved)
		})
	}
}

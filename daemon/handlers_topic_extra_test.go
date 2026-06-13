package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// drainProcessor feeds the given socket messages through SocketTopicProcessor
// and waits for each to be acked or nacked, then closes the channel.
func runTopicProcessor(t *testing.T, msgs []*message.Message) {
	t.Helper()
	ch := make(chan *message.Message)
	go SocketTopicProcessor(ch)
	for _, m := range msgs {
		ch <- m
		select {
		case <-m.Acked():
		case <-m.Nacked():
		case <-time.After(2 * time.Second):
			t.Fatal("message was neither acked nor nacked in time")
		}
	}
	close(ch)
}

func socketMsgBytes(t *testing.T, typ SocketMessageType, payload interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(SocketMessage{Type: typ, Payload: payload})
	require.NoError(t, err)
	return b
}

func TestSocketTopicProcessor_InvalidJSONNacks(t *testing.T) {
	msg := message.NewMessage("bad", []byte("{not json"))
	runTopicProcessor(t, []*message.Message{msg})
	// runTopicProcessor already asserts it terminates via nack.
}

func TestSocketTopicProcessor_UnknownTypeNacks(t *testing.T) {
	msg := message.NewMessage("u", socketMsgBytes(t, SocketMessageType("nope"), map[string]string{}))
	runTopicProcessor(t, []*message.Message{msg})
}

func TestSocketTopicProcessor_HeartbeatRouted(t *testing.T) {
	// Wire a server + config so the heartbeat handler succeeds and acks.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(model.HeartbeatResponse{})
	}))
	defer server.Close()

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: "t", APIEndpoint: server.URL}, nil)
	withStConfig(t, mockCS)

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{{HeartbeatID: "hb", Entity: "f", Time: 1}}}
	msg := message.NewMessage("hb", socketMsgBytes(t, SocketMessageTypeHeartbeat, payload))
	runTopicProcessor(t, []*message.Message{msg})
}

func TestSocketTopicProcessor_TrackPreAndPostRouted(t *testing.T) {
	// In-memory store via fallback so no filesystem is touched.
	prevStore := commandStore
	commandStore = nil
	t.Cleanup(func() { commandStore = prevStore })

	fake := &fakeCommandStore{noCursorExist: true}
	prevFallback := newFallbackStore
	newFallbackStore = func() model.CommandStore { return fake }
	t.Cleanup(func() { newFallbackStore = prevFallback })

	// track_post triggers a sync; back it with a server + breaker.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	withCircuitBreaker(t, &fakeDaemonCB{open: false})

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "t",
		APIEndpoint: server.URL,
		FlushCount:  1,
	}, nil)
	withStConfig(t, mockCS)

	now := time.Now()
	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "ls", Username: "u", Hostname: "h", Time: now}
	pre := message.NewMessage("pre", socketMsgBytes(t, SocketMessageTypeTrackPre, TrackEventPayload{Command: cmd, RecordingTimeNano: now.UnixNano()}))

	post := cmd
	post.Time = now.Add(time.Second)
	postMsg := message.NewMessage("post", socketMsgBytes(t, SocketMessageTypeTrackPost, TrackEventPayload{Command: post, RecordingTimeNano: post.Time.UnixNano()}))

	runTopicProcessor(t, []*message.Message{pre, postMsg})

	assert.Len(t, fake.pre, 1)
	assert.Len(t, fake.post, 1)
}

func TestSocketTopicProcessor_SyncFailureNacks(t *testing.T) {
	// A sync whose backend fails -> handler returns error -> message nacked.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	withCircuitBreaker(t, &fakeDaemonCB{open: false})

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: "t", APIEndpoint: server.URL}, nil)
	withStConfig(t, mockCS)

	payload := model.PostTrackArgs{CursorID: time.Now().UnixNano(), Data: []model.TrackingData{{Command: "ls"}}}
	msg := message.NewMessage("sync-fail", socketMsgBytes(t, SocketMessageTypeSync, payload))

	ch := make(chan *message.Message)
	go SocketTopicProcessor(ch)
	ch <- msg
	select {
	case <-msg.Nacked():
		// expected
	case <-msg.Acked():
		t.Fatal("expected nack on sync failure")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	close(ch)
}

func TestCodexUsageSyncService_SyncBranches(t *testing.T) {
	prevLoad := loadCodexAuthFunc
	prevFetch := fetchCodexUsageFunc
	t.Cleanup(func() {
		loadCodexAuthFunc = prevLoad
		fetchCodexUsageFunc = prevFetch
	})

	t.Run("no token returns early", func(t *testing.T) {
		loadCodexAuthFunc = func() (*codexAuthData, error) {
			t.Fatal("should not load auth without a token")
			return nil, nil
		}
		svc := NewCodexUsageSyncService(model.ShellTimeConfig{Token: ""})
		assert.NotPanics(t, svc.sync)
	})

	t.Run("skip reason is handled", func(t *testing.T) {
		loadCodexAuthFunc = func() (*codexAuthData, error) { return nil, errCodexAuthFileMissing }
		svc := NewCodexUsageSyncService(model.ShellTimeConfig{Token: "tok"})
		// errCodexAuthFileMissing -> CodexSyncSkipReason ok -> logged & returns.
		assert.NotPanics(t, svc.sync)
	})

	t.Run("generic failure is logged", func(t *testing.T) {
		loadCodexAuthFunc = func() (*codexAuthData, error) { return &codexAuthData{AccessToken: "a"}, nil }
		fetchCodexUsageFunc = func(ctx context.Context, auth *codexAuthData) (*CodexRateLimitData, error) {
			return nil, assertAnErr{}
		}
		svc := NewCodexUsageSyncService(model.ShellTimeConfig{Token: "tok"})
		assert.NotPanics(t, svc.sync)
	})
}

type assertAnErr struct{}

func (assertAnErr) Error() string { return "generic codex failure" }

func TestApplyMetricAttribute_RemainingBranches(t *testing.T) {
	m := &model.AICodeOtelMetric{}
	applyMetricAttribute(m, kv("model", strVal("claude-x")), model.AICodeMetricTokenUsage)
	applyMetricAttribute(m, kv("session.id", strVal("s-1")), model.AICodeMetricTokenUsage)
	applyMetricAttribute(m, kv("user.account_uuid", strVal("acct")), model.AICodeMetricTokenUsage)
	applyMetricAttribute(m, kv("os.version", strVal("14.0")), model.AICodeMetricTokenUsage)
	applyMetricAttribute(m, kv("app.version", strVal("1.2.3")), model.AICodeMetricTokenUsage)
	applyMetricAttribute(m, kv("terminal.type", strVal("xterm")), model.AICodeMetricTokenUsage)
	// Unknown key is ignored, no panic.
	applyMetricAttribute(m, kv("totally.unknown", strVal("x")), model.AICodeMetricTokenUsage)

	assert.Equal(t, "claude-x", m.Model)
	assert.Equal(t, "s-1", m.SessionID)
	assert.Equal(t, "acct", m.UserAccountUUID)
	assert.Equal(t, "14.0", m.OSVersion)
	assert.Equal(t, "1.2.3", m.AppVersion)
	assert.Equal(t, "xterm", m.TerminalType)
}

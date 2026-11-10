package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Nanosecond values a float64 can't hold: the nearest ones it can are 256 apart.
const (
	exactRecordingNano int64 = 1717243200123456789
	exactCursorNano    int64 = 1717243205987654321
	exactStartNano     int64 = 1717243201111111111
	exactEndNano       int64 = 1717243202222222223
)

// startHandlerWithProcessor is startHandler plus the topic processor on the handler's channel:
// the whole way a message takes from the socket to its handler.
func startHandlerWithProcessor(t *testing.T, config *model.ShellTimeConfig) string {
	t.Helper()
	handler, socketPath := startHandler(t, config)
	msgs, err := handler.channel.Subscribe(context.Background(), PubSubTopic)
	require.NoError(t, err)
	done := startTopicProcessor(msgs)
	// Runs before the cleanups registered earlier, which put back the globals the
	// handlers read: the handler of the last message must have returned by then.
	t.Cleanup(func() {
		handler.Stop()
		<-done
	})
	return socketPath
}

// savedPreStore hands every pre command it is given to the test, with its recording time.
type savedPreStore struct {
	fakeCommandStore
	saved chan model.Command
}

func (s *savedPreStore) SavePre(ctx context.Context, cmd model.Command, recordingTime time.Time) error {
	cmd.RecordingTime = recordingTime
	s.saved <- cmd
	return nil
}

func TestSocket_TrackEventKeepsRecordingTimeExact(t *testing.T) {
	x3SwapTrackGlobals(t)
	store := &savedPreStore{saved: make(chan model.Command, 1)}
	commandStore = store
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{FlushCount: 10}, nil)
	stConfig = mc

	socketPath := startHandlerWithProcessor(t, &model.ShellTimeConfig{})

	cmd := model.Command{Shell: "zsh", SessionID: 20240601120000, Command: "make", Username: "u", Hostname: "h", Time: time.Unix(0, exactRecordingNano)}
	require.NoError(t, SendTrackEvent(context.Background(), socketPath, SocketMessageTypeTrackPre, cmd, time.Unix(0, exactRecordingNano)))

	select {
	case got := <-store.saved:
		assert.Equal(t, exactRecordingNano, got.RecordingTime.UnixNano())
		assert.Equal(t, exactRecordingNano, got.Time.UnixNano())
		assert.Equal(t, int64(20240601120000), got.SessionID)
	case <-time.After(2 * time.Second):
		t.Fatal("the pre command never reached the store")
	}
}

func TestSocket_SyncKeepsCursorAndTimesExact(t *testing.T) {
	var received atomic.Pointer[model.PostTrackArgs]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got model.PostTrackArgs
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		received.Store(&got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	withCircuitBreaker(t, &fakeDaemonCB{})
	syncConfigFor(t, server.URL)
	socketPath := startHandlerWithProcessor(t, &model.ShellTimeConfig{})

	data := []model.TrackingData{{
		SessionID: 20240601120000, Command: "make",
		StartTime: exactStartNano / 1e9, EndTime: exactEndNano / 1e9,
		StartTimeNano: exactStartNano, EndTimeNano: exactEndNano,
	}}
	require.NoError(t, SendLocalDataToSocket(context.Background(), socketPath, model.ShellTimeConfig{},
		time.Unix(0, exactCursorNano), data, model.TrackingMetaData{Hostname: "h", Shell: "zsh"}))

	require.Eventually(t, func() bool { return received.Load() != nil }, 2*time.Second, 5*time.Millisecond)
	got := received.Load()
	assert.Equal(t, exactCursorNano, got.CursorID)
	require.Len(t, got.Data, 1)
	assert.Equal(t, exactStartNano, got.Data[0].StartTimeNano)
	assert.Equal(t, exactEndNano, got.Data[0].EndTimeNano)
}

// A batch saved for retry is written from the decoded payload, so it has to keep its digits.
func TestSocket_SavedSyncBatchKeepsCursorExact(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER), 0o700))

	prev := syncCircuitBreaker
	t.Cleanup(func() { syncCircuitBreaker = prev })
	wrapper := NewSyncCircuitBreakerService(&mockPublisher{})
	for range model.DefaultMaxConsecutiveFailures {
		wrapper.RecordFailure() // open: the handler saves without sending
	}
	withStConfig(t, model.NewMockConfigService(t))

	line := `{"type":"sync","payload":{"cursorId":1717243205987654321,"data":[{"sessionId":1,"command":"ls","startTime":1717243201,"endTime":1717243202,"startTimeNano":1717243201111111111,"endTimeNano":1717243202222222223,"result":0}],"meta":{"hostname":"h","username":"u","os":"","osVersion":"","shell":"zsh","source":0},"encrypted":"","aesKey":"","nonce":""}}`
	var msg SocketMessage
	require.NoError(t, unmarshalSocketMessage([]byte(line), &msg))
	require.NoError(t, handlePubSubSync(context.Background(), msg.Payload))

	saved, err := os.ReadFile(filepath.Join(home, model.SYNC_PENDING_FILE))
	require.NoError(t, err)
	for _, digits := range []string{"1717243205987654321", "1717243201111111111", "1717243202222222223"} {
		assert.Contains(t, string(saved), digits)
	}
}

// decodeAs runs a decoded payload through what every handler does with it.
func decodeAs[T any](t *testing.T, payload interface{}) T {
	t.Helper()
	buf, err := json.Marshal(payload)
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(buf, &out), "payload %s", buf)
	return out
}

func TestUnmarshalSocketMessage_PayloadsHandlersAccept(t *testing.T) {
	t.Run("sync line written by an older daemon, integers already rounded", func(t *testing.T) {
		// What a version that decoded into float64 wrote to sync-pending.jsonl
		line := `{"type":"sync","payload":{"aesKey":"","cursorId":1717243205987654400,"data":[{"command":"ls","endTime":1717243202,"endTimeNano":1717243202222222300,"result":0,"sessionId":20240601120000,"startTime":1717243201,"startTimeNano":1717243201111111200}],"encrypted":"","meta":{"hostname":"h","os":"linux","osVersion":"6","shell":"zsh","source":0,"username":"u"},"nonce":""}}`
		var msg SocketMessage
		require.NoError(t, unmarshalSocketMessage([]byte(line), &msg))
		assert.Equal(t, SocketMessageTypeSync, msg.Type)
		got := decodeAs[model.PostTrackArgs](t, msg.Payload)
		assert.Equal(t, int64(1717243205987654400), got.CursorID)
		require.Len(t, got.Data, 1)
		assert.Equal(t, int64(1717243201111111200), got.Data[0].StartTimeNano)
		assert.Equal(t, int64(20240601120000), got.Data[0].SessionID)
		assert.Equal(t, "linux", got.Meta.OS)
	})

	t.Run("numbers a client wrote with a fraction or an exponent still fit integer fields", func(t *testing.T) {
		line := `{"type":"heartbeat","payload":{"heartbeats":[` +
			`{"heartbeatId":"a","entity":"f.go","time":1717243200.0,"lines":120.0},` +
			`{"heartbeatId":"b","entity":"g.go","time":1.7172432e9,"lineNumber":7,"cursorPosition":1e2}]}}`
		var msg SocketMessage
		require.NoError(t, unmarshalSocketMessage([]byte(line), &msg))
		got := decodeAs[model.HeartbeatPayload](t, msg.Payload)
		require.Len(t, got.Heartbeats, 2)
		assert.Equal(t, int64(1717243200), got.Heartbeats[0].Time)
		assert.Equal(t, 120, *got.Heartbeats[0].Lines)
		assert.Equal(t, int64(1717243200), got.Heartbeats[1].Time)
		assert.Equal(t, 7, *got.Heartbeats[1].LineNumber)
		assert.Equal(t, 100, *got.Heartbeats[1].CursorPosition)
	})

	t.Run("track event", func(t *testing.T) {
		line := `{"type":"track_post","payload":{"command":{"shell":"zsh","sid":20240601120000,"cmd":"make","main":"","hn":"h","un":"u","t":"2024-06-01T12:00:00.123456789Z","et":"0001-01-01T00:00:00Z","result":-1,"phase":0,"ppid":4242,"ssh":true},"recordingTimeNano":1717243200123456789}}`
		var msg SocketMessage
		require.NoError(t, unmarshalSocketMessage([]byte(line), &msg))
		got := decodeAs[TrackEventPayload](t, msg.Payload)
		assert.Equal(t, exactRecordingNano, got.RecordingTimeNano)
		assert.Equal(t, -1, got.Command.Result)
		assert.Equal(t, 4242, got.Command.PPID)
		require.NotNil(t, got.Command.ViaSSH)
		assert.True(t, *got.Command.ViaSSH)
	})

	t.Run("session pull requests and cc info keep their strings", func(t *testing.T) {
		var msg SocketMessage
		require.NoError(t, unmarshalSocketMessage([]byte(`{"type":"session_pull_requests","payload":{"sessionId":"s1","urls":["https://github.com/o/r/pull/1"]}}`), &msg))
		got := decodeAs[SessionPullRequestsRequest](t, msg.Payload)
		assert.Equal(t, "s1", got.SessionID)
		assert.Equal(t, []string{"https://github.com/o/r/pull/1"}, got.URLs)

		require.NoError(t, decodeSocketMessage(strings.NewReader(`{"type":"cc_info","payload":{"timeRange":"week","workingDirectory":"/w"}}`+"\n"), &msg))
		payload, ok := msg.Payload.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "week", payload["timeRange"])
	})

	t.Run("a message without a payload", func(t *testing.T) {
		var msg SocketMessage
		require.NoError(t, unmarshalSocketMessage([]byte(`{"type":"status"}`), &msg))
		assert.Equal(t, SocketMessageTypeStatus, msg.Type)
		assert.Nil(t, msg.Payload)
	})

	t.Run("refused like json.Unmarshal", func(t *testing.T) {
		for _, data := range []string{``, `invalid`, `{"type":"sync"`, `{"type":"sync"} {"type":"sync"}`, `{"type":"sync"}x`} {
			var msg SocketMessage
			assert.Error(t, unmarshalSocketMessage([]byte(data), &msg), "input %q", data)
			assert.Error(t, json.Unmarshal([]byte(data), &msg), "json.Unmarshal accepts %q, so the check above is too strict", data)
		}
		var msg SocketMessage
		require.NoError(t, unmarshalSocketMessage([]byte("  {\"type\":\"sync\"}\n"), &msg), "surrounding whitespace is fine")
	})
}

func TestKeepIntegersExact(t *testing.T) {
	in := map[string]interface{}{
		"int":    json.Number("1717243200123456789"),
		"neg":    json.Number("-42"),
		"frac":   json.Number("1.5"),
		"exp":    json.Number("1e3"),
		"zero":   json.Number("0"),
		"text":   "9007199254740993",
		"nested": []interface{}{json.Number("9007199254740993"), map[string]interface{}{"f": json.Number("2.0")}},
	}
	out := keepIntegersExact(in).(map[string]interface{})
	assert.Equal(t, json.Number("1717243200123456789"), out["int"])
	assert.Equal(t, json.Number("-42"), out["neg"])
	assert.Equal(t, 1.5, out["frac"])
	assert.Equal(t, float64(1000), out["exp"])
	assert.Equal(t, json.Number("0"), out["zero"])
	assert.Equal(t, "9007199254740993", out["text"])
	nested := out["nested"].([]interface{})
	assert.Equal(t, json.Number("9007199254740993"), nested[0])
	assert.Equal(t, float64(2), nested[1].(map[string]interface{})["f"])
}

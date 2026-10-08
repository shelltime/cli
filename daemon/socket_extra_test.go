package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

// startHandler starts a SocketHandler on a fresh temp socket and registers cleanup.
func startHandler(t *testing.T, config *model.ShellTimeConfig) (*SocketHandler, string) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	config.SocketPath = socketPath

	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 10}, nil)
	handler := NewSocketHandler(config, ch)
	require.NoError(t, handler.Start())
	t.Cleanup(handler.Stop)

	// Wait until the socket file is present.
	require.Eventually(t, func() bool {
		_, err := os.Stat(socketPath)
		return err == nil
	}, time.Second, 5*time.Millisecond)

	return handler, socketPath
}

func TestSocketHandler_HeartbeatDisabled(t *testing.T) {
	// codeTracking disabled -> handler replies {"status":"disabled"} and does not publish.
	_, socketPath := startHandler(t, &model.ShellTimeConfig{})

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	msg := SocketMessage{
		Type:    SocketMessageTypeHeartbeat,
		Payload: model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{{HeartbeatID: "x"}}},
	}
	require.NoError(t, json.NewEncoder(conn).Encode(msg))

	var resp map[string]string
	require.NoError(t, json.NewDecoder(conn).Decode(&resp))
	assert.Equal(t, "disabled", resp["status"])
}

func TestSocketHandler_HeartbeatEnabled(t *testing.T) {
	// codeTracking enabled -> handler replies {"status":"ok"} and publishes.
	config := &model.ShellTimeConfig{
		CodeTracking: &model.CodeTracking{Enabled: boolPtr(true)},
	}
	handler, socketPath := startHandler(t, config)

	// Subscribe to the pub/sub topic to confirm the message is published.
	msgs, err := handler.channel.Subscribe(context.Background(), PubSubTopic)
	require.NoError(t, err)

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	msg := SocketMessage{
		Type:    SocketMessageTypeHeartbeat,
		Payload: model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{{HeartbeatID: "hb-pub"}}},
	}
	require.NoError(t, json.NewEncoder(conn).Encode(msg))

	var resp map[string]string
	require.NoError(t, json.NewDecoder(conn).Decode(&resp))
	assert.Equal(t, "ok", resp["status"])

	select {
	case published := <-msgs:
		published.Ack()
		var decoded SocketMessage
		require.NoError(t, json.Unmarshal(published.Payload, &decoded))
		assert.Equal(t, SocketMessageTypeHeartbeat, decoded.Type)
	case <-time.After(time.Second):
		t.Fatal("expected a heartbeat message to be published")
	}
}

func TestSocketHandler_SyncPublishesToTopic(t *testing.T) {
	handler, socketPath := startHandler(t, &model.ShellTimeConfig{})

	msgs, err := handler.channel.Subscribe(context.Background(), PubSubTopic)
	require.NoError(t, err)

	err = SendLocalDataToSocket(
		context.Background(),
		socketPath,
		model.ShellTimeConfig{},
		time.Now(),
		[]model.TrackingData{{Command: "ls", Result: 0}},
		model.TrackingMetaData{OS: "linux", Shell: "bash"},
	)
	require.NoError(t, err)

	select {
	case published := <-msgs:
		published.Ack()
		var decoded SocketMessage
		require.NoError(t, json.Unmarshal(published.Payload, &decoded))
		assert.Equal(t, SocketMessageTypeSync, decoded.Type)
	case <-time.After(time.Second):
		t.Fatal("expected a sync message to be published")
	}
}

func TestSocketHandler_TrackEventPublishes(t *testing.T) {
	handler, socketPath := startHandler(t, &model.ShellTimeConfig{})

	msgs, err := handler.channel.Subscribe(context.Background(), PubSubTopic)
	require.NoError(t, err)

	cmd := model.Command{Shell: "bash", SessionID: 1, Command: "echo hi", Username: "u", Hostname: "h", Time: time.Now()}
	require.NoError(t, SendTrackEvent(context.Background(), socketPath, SocketMessageTypeTrackPost, cmd, time.Now()))

	select {
	case published := <-msgs:
		published.Ack()
		var decoded SocketMessage
		require.NoError(t, json.Unmarshal(published.Payload, &decoded))
		assert.Equal(t, SocketMessageTypeTrackPost, decoded.Type)
	case <-time.After(time.Second):
		t.Fatal("expected a track message to be published")
	}
}

func TestSocketHandler_ListCommands_NilStore(t *testing.T) {
	// With no commandStore set, list_commands returns an empty (non-nil) slice.
	prev := commandStore
	commandStore = nil
	t.Cleanup(func() { commandStore = prev })

	_, socketPath := startHandler(t, &model.ShellTimeConfig{})

	resp, err := RequestListCommands(socketPath, 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Commands)
}

func TestSocketHandler_SessionProject(t *testing.T) {
	// session_project is fire-and-forget; the handler should not error or hang
	// even though the (empty) APIEndpoint update goroutine will quietly fail.
	_, socketPath := startHandler(t, &model.ShellTimeConfig{})

	assert.NotPanics(t, func() {
		SendSessionProject(socketPath, "sess-1", "/path/proj")
		// Give the handler a brief moment to process.
		time.Sleep(50 * time.Millisecond)
	})
}

func TestSocketHandler_SessionPullRequestsSendsToServer(t *testing.T) {
	type request struct {
		path, auth string
		body       map[string]interface{}
	}
	got := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		got <- request{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	_, socketPath := startHandler(t, &model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})
	urls := []string{"https://github.com/o/r/pull/1", "https://github.com/o/r2/pull/2"}
	require.NoError(t, SendSessionPullRequests(socketPath, "sess-1", urls))

	select {
	case req := <-got:
		assert.Equal(t, "/api/v1/cc/session-pull-requests", req.path)
		assert.Equal(t, "CLI tok", req.auth)
		assert.Equal(t, "sess-1", req.body["sessionId"])
		assert.Equal(t, []interface{}{urls[0], urls[1]}, req.body["urls"])
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not send session pull requests to the server")
	}
}

func TestSocketHandler_SessionPullRequestsIgnoresEmpty(t *testing.T) {
	requests := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	_, socketPath := startHandler(t, &model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})
	require.NoError(t, SendSessionPullRequests(socketPath, "", []string{"https://github.com/o/r/pull/1"}))
	require.NoError(t, SendSessionPullRequests(socketPath, "sess-1", nil))

	select {
	case <-requests:
		t.Fatal("daemon sent a request without a session id or urls")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestSocketHandler_UnknownMessageType(t *testing.T) {
	_, socketPath := startHandler(t, &model.ShellTimeConfig{})

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	defer conn.Close()

	// Unknown type: handler logs and returns; the connection is simply closed.
	msg := SocketMessage{Type: SocketMessageType("totally-unknown")}
	require.NoError(t, json.NewEncoder(conn).Encode(msg))

	// Reading should hit EOF / closed connection without panicking.
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 16)
	_, _ = conn.Read(buf) // error expected; we only assert no panic / no hang
}

func TestRequestListCommands_NoSocket(t *testing.T) {
	_, err := RequestListCommands(filepath.Join(t.TempDir(), "missing.sock"), 100*time.Millisecond)
	assert.Error(t, err)
}

func TestRequestCCInfo_OverRealHandler(t *testing.T) {
	// Drives RequestCCInfo against the actual SocketHandler.handleCCInfo path
	// (rather than a stub responder), exercising the server-side encode logic.
	_, socketPath := startHandler(t, &model.ShellTimeConfig{})

	resp, err := RequestCCInfo(socketPath, CCInfoTimeRangeWeek, "", "", 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "week", resp.TimeRange)
}

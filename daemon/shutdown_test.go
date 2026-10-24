package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func savedHeartbeatIDs(t *testing.T, home string) []string {
	t.Helper()
	f, err := os.Open(filepath.Join(home, ".shelltime", "coding-heartbeat.data.log"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	defer f.Close()

	var ids []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var payload model.HeartbeatPayload
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &payload))
		for _, h := range payload.Heartbeats {
			ids = append(ids, h.HeartbeatID)
		}
	}
	return ids
}

func TestGoChannel_DrainWaitsForHandledMessages(t *testing.T) {
	ch := NewGoChannel(PubSubConfig{}, nil)
	defer ch.Close()
	assert.True(t, ch.Drain(0), "nothing published")

	msgs, err := ch.Subscribe(context.Background(), PubSubTopic)
	require.NoError(t, err)
	release := make(chan struct{})
	var handled atomic.Int32
	go func() {
		for msg := range msgs {
			<-release
			handled.Add(1)
			msg.Ack()
		}
	}()

	for i := 0; i < 3; i++ {
		require.NoError(t, ch.Publish(PubSubTopic, message.NewMessage(watermill.NewUUID(), []byte("{}"))))
	}

	assert.False(t, ch.Drain(50*time.Millisecond), "the subscriber hasn't handled them")
	close(release)
	assert.True(t, ch.Drain(2*time.Second))
	assert.Equal(t, int32(3), handled.Load())
}

func TestHandlePubSubHeartbeat_SavesWhileShuttingDown(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(model.HeartbeatResponse{Success: true})
	}))
	defer server.Close()
	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything, mock.Anything).
		Return(model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}, nil).Maybe()
	withStConfig(t, mockCS)
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".shelltime"), 0o755))

	beginShutdown()
	t.Cleanup(resetShutdown)

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{{HeartbeatID: "hb-1", Entity: "f.go", Time: 1}}}
	require.NoError(t, handlePubSubHeartbeat(context.Background(), payload))

	assert.Equal(t, int32(0), hits.Load(), "a stopping daemon doesn't start sends")
	assert.Equal(t, []string{"hb-1"}, savedHeartbeatIDs(t, home))
}

func TestHandlePubSubHeartbeat_SendCutShortByShutdownIsSaved(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server notices the client hanging up only once the body is read
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything, mock.Anything).
		Return(model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}, nil)
	withStConfig(t, mockCS)
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".shelltime"), 0o755))
	t.Cleanup(resetShutdown)

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{{HeartbeatID: "hb-1", Entity: "f.go", Time: 1}}}
	done := make(chan error, 1)
	go func() { done <- handlePubSubHeartbeat(context.Background(), payload) }()

	<-started
	beginShutdown()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the send wasn't cut short")
	}
	assert.Equal(t, []string{"hb-1"}, savedHeartbeatIDs(t, home))
}

func TestSocketHandler_ShutdownKeepsAcknowledgedHeartbeats(t *testing.T) {
	// A slow server: the first batch is in flight and two more wait when the daemon stops
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		requests.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	enabled := true
	cfg := model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL, CodeTracking: &model.CodeTracking{Enabled: &enabled}}
	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything, mock.Anything).Return(cfg, nil)
	withStConfig(t, mockCS)
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".shelltime"), 0o755))
	t.Cleanup(resetShutdown)

	handler, socketPath := startHandler(t, &cfg)
	msgs, err := handler.channel.Subscribe(context.Background(), PubSubTopic)
	require.NoError(t, err)
	go SocketTopicProcessor(msgs)

	for _, id := range []string{"a", "b", "c"} {
		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(conn).Encode(SocketMessage{
			Type:    SocketMessageTypeHeartbeat,
			Payload: model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{{HeartbeatID: id, Entity: "f.go", Time: 1}}},
		}))
		var resp map[string]string
		require.NoError(t, json.NewDecoder(conn).Decode(&resp))
		require.Equal(t, "ok", resp["status"])
		conn.Close()
	}
	require.Eventually(t, func() bool { return requests.Load() == 1 }, 2*time.Second, 5*time.Millisecond)

	handler.Shutdown(2 * time.Second)

	assert.ElementsMatch(t, []string{"a", "b", "c"}, savedHeartbeatIDs(t, home))
	assert.Equal(t, int32(1), requests.Load(), "queued batches are saved, not sent")
	_, err = net.Dial("unix", socketPath)
	assert.Error(t, err, "a stopped daemon takes no more messages")
}

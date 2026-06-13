package daemon

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestX3SendTrackEvent_DialFailure covers the dial-error branch of SendTrackEvent
// when no socket exists at the path.
func TestX3SendTrackEvent_DialFailure(t *testing.T) {
	err := SendTrackEvent(
		context.Background(),
		filepath.Join(t.TempDir(), "absent.sock"),
		SocketMessageTypeTrackPost,
		model.Command{Command: "ls"},
		time.Now(),
	)
	require.Error(t, err)
}

// TestX3SendSessionProject_DialFailureNoPanic covers the dial-error early return
// of the fire-and-forget SendSessionProject (no socket present).
func TestX3SendSessionProject_DialFailureNoPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		SendSessionProject(filepath.Join(t.TempDir(), "absent.sock"), "sess", "/proj")
	})
}

// TestX3SendSessionProject_DeliversToServer covers the success encode path of
// SendSessionProject against a live listener.
func TestX3SendSessionProject_DeliversToServer(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "sp.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	got := make(chan SocketMessage, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		var msg SocketMessage
		if derr := json.NewDecoder(conn).Decode(&msg); derr == nil {
			got <- msg
		}
	}()

	SendSessionProject(socketPath, "sess-1", "/proj/dir")

	select {
	case msg := <-got:
		assert.Equal(t, SocketMessageTypeSessionProject, msg.Type)
	case <-time.After(time.Second):
		t.Fatal("session_project message not delivered")
	}
}

// TestX3SocketHandler_StartListenError covers the net.Listen failure branch of
// SocketHandler.Start: a socket path inside a non-existent directory cannot be
// bound.
func TestX3SocketHandler_StartListenError(t *testing.T) {
	cfg := &model.ShellTimeConfig{
		SocketPath: filepath.Join(t.TempDir(), "no-such-dir", "x.sock"),
	}
	ch := NewGoChannel(PubSubConfig{OutputChannelBuffer: 1}, nil)
	h := NewSocketHandler(cfg, ch)
	err := h.Start()
	require.Error(t, err, "binding inside a missing directory must fail")
	ch.Close()
}

// TestX3SendLocalDataToSocket_DeliversSyncMessage covers the full write path of
// SendLocalDataToSocket against a live listener (encode + write succeed).
func TestX3SendLocalDataToSocket_DeliversSyncMessage(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "sync.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	got := make(chan SocketMessage, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		var msg SocketMessage
		if derr := json.NewDecoder(conn).Decode(&msg); derr == nil {
			got <- msg
		}
	}()

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
	case msg := <-got:
		assert.Equal(t, SocketMessageTypeSync, msg.Type)
	case <-time.After(time.Second):
		t.Fatal("sync message not delivered")
	}
}

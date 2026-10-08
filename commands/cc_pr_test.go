package commands

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func setupCCPullRequest(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

func runCCPullRequest(args ...string) error {
	app := &cli.App{Name: "t", Commands: []*cli.Command{CCCommand}}
	return app.Run(append([]string{"t", "cc", "pr"}, args...))
}

func TestCCPullRequest_SendsToDaemon(t *testing.T) {
	mc := setupCCPullRequest(t)

	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	got := make(chan daemon.SocketMessage, 1)
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		defer conn.Close()
		var msg daemon.SocketMessage
		if derr := json.NewDecoder(conn).Decode(&msg); derr == nil {
			got <- msg
		}
	}()

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:      "tok",
		SocketPath: socketPath,
	}, nil)

	require.NoError(t, runCCPullRequest(
		"--session-id", "sess-1",
		"https://github.com/o/r/pull/1",
		" https://github.com/o/r/pull/1 ",
		"https://github.com/o/r2/pull/2",
	))

	select {
	case msg := <-got:
		assert.Equal(t, daemon.SocketMessageTypeSessionPullRequests, msg.Type)
		payload, ok := msg.Payload.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "sess-1", payload["sessionId"])
		assert.Equal(t, []interface{}{"https://github.com/o/r/pull/1", "https://github.com/o/r2/pull/2"}, payload["urls"])
	case <-time.After(time.Second):
		t.Fatal("daemon did not receive the pull requests")
	}
}

func TestCCPullRequest_FallsBackToServerWithoutDaemon(t *testing.T) {
	mc := setupCCPullRequest(t)

	var gotPath string
	var body struct {
		SessionID string   `json:"sessionId"`
		URLs      []string `json:"urls"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
		SocketPath:  filepath.Join(t.TempDir(), "absent.sock"),
	}, nil)

	require.NoError(t, runCCPullRequest("--session-id", "sess-1", "https://github.com/o/r/pull/1"))
	assert.Equal(t, "/api/v1/cc/session-pull-requests", gotPath)
	assert.Equal(t, "sess-1", body.SessionID)
	assert.Equal(t, []string{"https://github.com/o/r/pull/1"}, body.URLs)
}

func TestCCPullRequest_ServerErrorIsReturned(t *testing.T) {
	mc := setupCCPullRequest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request body"}`))
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
		SocketPath:  filepath.Join(t.TempDir(), "absent.sock"),
	}, nil)

	err := runCCPullRequest("--session-id", "sess-1", "not-a-pr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid request body")
}

func TestCCPullRequest_SkipsWithoutToken(t *testing.T) {
	mc := setupCCPullRequest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		SocketPath: filepath.Join(t.TempDir(), "absent.sock"),
	}, nil)

	require.NoError(t, runCCPullRequest("--session-id", "sess-1", "https://github.com/o/r/pull/1"))
}

func TestCCPullRequest_RequiresSessionAndURL(t *testing.T) {
	setupCCPullRequest(t)

	assert.Error(t, runCCPullRequest("https://github.com/o/r/pull/1"), "no --session-id")
	assert.Error(t, runCCPullRequest("--session-id", "sess-1"), "no url")
	assert.Error(t, runCCPullRequest("--session-id", " ", "https://github.com/o/r/pull/1"), "blank session id")
	assert.Error(t, runCCPullRequest("--session-id", "sess-1", " "), "blank url")
}

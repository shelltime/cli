package commands

import (
	"context"
	"encoding/json"
	"net"
	"os"
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

// c2SetupStatusline swaps in a mock ConfigService for the statusline tests,
// restoring the original on cleanup.
func c2SetupStatusline(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

// c2StatuslineStdin replaces os.Stdin with a pipe carrying payload for the test,
// restoring it on cleanup.
func c2StatuslineStdin(t *testing.T, payload []byte) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		r.Close()
	})
	go func() {
		_, _ = w.Write(payload)
		w.Close()
	}()
}

// TestC2CommandCCStatusline_SendsSessionProjectViaDaemon drives commandCCStatusline
// down the daemon SendSessionProject branch: a non-empty SessionID + a Workspace
// with CurrentDir + a ready daemon socket. The fake daemon accepts both the
// session-project fire-and-forget message and the subsequent CCInfo request,
// replying with cost/git data so the full formatting path runs.
func TestC2CommandCCStatusline_SendsSessionProjectViaDaemon(t *testing.T) {
	mc := c2SetupStatusline(t)

	socketPath := filepath.Join(t.TempDir(), "statusline-daemon.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	gotMsg := make(chan daemon.SocketMessage, 4)
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var msg daemon.SocketMessage
				if derr := json.NewDecoder(c).Decode(&msg); derr != nil {
					return
				}
				gotMsg <- msg
				// For a CCInfo request, reply with a populated response so the
				// daemon-success path in getDaemonInfoWithFallback is taken.
				if msg.Type == daemon.SocketMessageTypeCCInfo {
					_ = json.NewEncoder(c).Encode(daemon.CCInfoResponse{
						TotalCostUSD:        3.21,
						TotalSessionSeconds: 1800,
						TimeRange:           "today",
						CachedAt:            time.Now(),
						GitBranch:           "main",
						GitDirty:            true,
						UserLogin:           "tester",
					})
				}
			}(conn)
		}
	}()

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		SocketPath:  socketPath,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	input := model.CCStatuslineInput{
		SessionID: "sess-xyz",
		Model:     model.CCStatuslineModel{DisplayName: "claude-opus-4"},
		Cost:      model.CCStatuslineCost{TotalCostUSD: 1.5},
		ContextWindow: model.CCStatuslineContextWindow{
			ContextWindowSize: 100000,
			TotalInputTokens:  20000,
			TotalOutputTokens: 5000,
		},
		Workspace: &model.CCStatuslineWorkspace{CurrentDir: "/some/project"},
		Cwd:       "/some/project",
	}
	payload, err := json.Marshal(input)
	require.NoError(t, err)
	c2StatuslineStdin(t, payload)

	app := &cli.App{Name: "t", Commands: []*cli.Command{CCStatuslineCommand}}
	require.NoError(t, app.Run([]string{"t", "statusline"}))

	// At least one message (the session-project mapping) must have reached the daemon.
	select {
	case msg := <-gotMsg:
		assert.NotEmpty(t, string(msg.Type))
	default:
		t.Fatal("daemon did not receive any socket message")
	}
}

// TestC2CommandCCStatusline_WorkspaceProjectDirFallback covers the projectPath
// fallback to Workspace.ProjectDir when CurrentDir is empty. No daemon socket is
// present, so SendSessionProject's dial fails silently and the action still
// completes via the cached-API fallback.
func TestC2CommandCCStatusline_WorkspaceProjectDirFallback(t *testing.T) {
	mc := c2SetupStatusline(t)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		SocketPath: filepath.Join(t.TempDir(), "absent.sock"),
		Token:      "",
	}, nil)

	input := model.CCStatuslineInput{
		SessionID:     "sess-1",
		Model:         model.CCStatuslineModel{DisplayName: "claude"},
		Cost:          model.CCStatuslineCost{TotalCostUSD: 0.1},
		ContextWindow: model.CCStatuslineContextWindow{ContextWindowSize: 0},
		// CurrentDir empty -> falls back to ProjectDir for the session mapping.
		Workspace: &model.CCStatuslineWorkspace{ProjectDir: "/proj/dir"},
		Cwd:       "",
	}
	payload, err := json.Marshal(input)
	require.NoError(t, err)
	c2StatuslineStdin(t, payload)

	app := &cli.App{Name: "t", Commands: []*cli.Command{CCStatuslineCommand}}
	require.NoError(t, app.Run([]string{"t", "statusline"}))
}

// TestC2ReadStdinWithTimeout_ContextCancelled covers the ctx.Done() branch of
// readStdinWithTimeout: an already-cancelled context returns ctx.Err() before
// stdin data is read.
func TestC2ReadStdinWithTimeout_ContextCancelled(t *testing.T) {
	// A pipe whose writer is never closed: the reader goroutine blocks, so the
	// cancelled context wins the select.
	r, _, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		r.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, rerr := readStdinWithTimeout(ctx)
	require.Error(t, rerr)
	assert.ErrorIs(t, rerr, context.Canceled)
}

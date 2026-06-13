package commands

import (
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

// startFakeStatusDaemon spins up a Unix socket server that answers a single
// status request with the supplied StatusResponse, then returns.
func startFakeStatusDaemon(t *testing.T, socketPath string, resp daemon.StatusResponse) net.Listener {
	t.Helper()
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var msg daemon.SocketMessage
		_ = json.NewDecoder(conn).Decode(&msg)
		_ = json.NewEncoder(conn).Encode(resp)
	}()
	return ln
}

// --- checkSocketFileExists ----------------------------------------------------

func TestCheckSocketFileExists(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no.sock")
	assert.False(t, checkSocketFileExists(missing))

	present := filepath.Join(dir, "yes.sock")
	require.NoError(t, os.WriteFile(present, []byte{}, 0644))
	assert.True(t, checkSocketFileExists(present))
}

// --- requestDaemonStatus ------------------------------------------------------

func TestRequestDaemonStatus_DialError(t *testing.T) {
	// Nothing listening at this path -> dial error.
	resp, latency, err := requestDaemonStatus(filepath.Join(t.TempDir(), "absent.sock"), 200*time.Millisecond)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, time.Duration(0), latency)
}

func TestRequestDaemonStatus_Success(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "status.sock")
	want := daemon.StatusResponse{
		Version:   "v1.2.3",
		StartedAt: time.Now().Add(-time.Hour),
		Uptime:    "1h0m0s",
		GoVersion: "go1.26",
		Platform:  "linux/amd64",
	}
	ln := startFakeStatusDaemon(t, socketPath, want)
	t.Cleanup(func() { ln.Close(); os.Remove(socketPath) })

	resp, latency, err := requestDaemonStatus(socketPath, 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "v1.2.3", resp.Version)
	assert.Equal(t, "go1.26", resp.GoVersion)
	assert.Equal(t, "linux/amd64", resp.Platform)
	assert.Greater(t, latency, time.Duration(0))
}

func TestRequestDaemonStatus_ConnectionClosedBeforeResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "closing.sock")
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close(); os.Remove(socketPath) })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Close immediately without sending a response -> decode error.
		conn.Close()
	}()

	resp, _, err := requestDaemonStatus(socketPath, 2*time.Second)
	require.Error(t, err)
	assert.Nil(t, resp)
}

// --- commandDaemonStatus action -----------------------------------------------

func setupDaemonStatusTest(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

func TestCommandDaemonStatus_Connected(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	ln := startFakeStatusDaemon(t, socketPath, daemon.StatusResponse{
		Version:   "v9.9.9",
		StartedAt: time.Now(),
		Uptime:    "5m",
		GoVersion: "go1.26",
		Platform:  "linux/amd64",
	})
	t.Cleanup(func() { ln.Close(); os.Remove(socketPath) })

	enabled := true
	mc := setupDaemonStatusTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		SocketPath:   socketPath,
		AICodeOtel:   &model.AICodeOtel{Enabled: &enabled, GRPCPort: 54027},
		CodeTracking: &model.CodeTracking{Enabled: &enabled},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DaemonStatusCommand}}
	// The action always returns nil; it prints status. We assert it runs cleanly
	// against a responding daemon socket.
	err := app.Run([]string{"t", "status"})
	require.NoError(t, err)
}

func TestCommandDaemonStatus_DaemonNotRunning(t *testing.T) {
	mc := setupDaemonStatusTest(t)
	// Socket path points nowhere -> dial error / "not running" branch.
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		SocketPath: filepath.Join(t.TempDir(), "missing.sock"),
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DaemonStatusCommand}}
	err := app.Run([]string{"t", "status"})
	require.NoError(t, err)
}

func TestCommandDaemonStatus_ConfigReadErrorUsesDefaultSocket(t *testing.T) {
	mc := setupDaemonStatusTest(t)
	// On config error, the action falls back to model.DefaultSocketPath and still
	// completes (no daemon there in test env).
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DaemonStatusCommand}}
	err := app.Run([]string{"t", "status"})
	require.NoError(t, err)
}

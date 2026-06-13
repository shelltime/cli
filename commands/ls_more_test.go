package commands

import (
	"encoding/json"
	"net"
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

// c2SetupLs swaps in a mock ConfigService and isolates HOME, restoring the
// original ConfigService on cleanup.
func c2SetupLs(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

// c2ServeListCommandsOnce starts a unix-socket listener that accepts one
// connection, reads the request, and replies with the given ListCommandsResponse.
// The listener is closed via t.Cleanup. A buffered done channel lets us avoid a
// goroutine leak without time.Sleep based synchronization.
func c2ServeListCommandsOnce(t *testing.T, socketPath string, resp daemon.ListCommandsResponse) {
	t.Helper()
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Drain the incoming request (a single JSON SocketMessage).
		var msg daemon.SocketMessage
		_ = json.NewDecoder(conn).Decode(&msg)
		_ = json.NewEncoder(conn).Encode(resp)
	}()
}

// --- commandList: unsupported format ------------------------------------------

func TestC2CommandList_UnsupportedFormat(t *testing.T) {
	c2SetupLs(t)
	// configService is not consulted before the format check; no expectation set.
	app := &cli.App{Name: "t", Commands: []*cli.Command{LsCommand}}
	err := app.Run([]string{"t", "ls", "-f", "xml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

// --- commandList: bolt mode over the socket -----------------------------------

func TestC2CommandList_BoltModeReadsFromDaemon(t *testing.T) {
	mc := c2SetupLs(t)

	socketPath := filepath.Join(t.TempDir(), "ls-bolt.sock")
	now := time.Now()
	c2ServeListCommandsOnce(t, socketPath, daemon.ListCommandsResponse{
		Commands: []model.ListedCommand{
			{
				Command:   "git status",
				Shell:     "bash",
				StartTime: now,
				EndTime:   now.Add(2 * time.Second),
				Result:    0,
				Username:  "u",
				Hostname:  "h",
			},
		},
	})

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Storage:    &model.StorageConfig{Engine: model.StorageEngineBolt},
		SocketPath: socketPath,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{LsCommand}}
	// Bolt engine + ready socket -> commands fetched via RequestListCommands, then
	// rendered as JSON.
	require.NoError(t, app.Run([]string{"t", "ls", "-f", "json"}))
}

func TestC2CommandList_BoltModeTableOutput(t *testing.T) {
	mc := c2SetupLs(t)

	socketPath := filepath.Join(t.TempDir(), "ls-bolt-table.sock")
	now := time.Now()
	c2ServeListCommandsOnce(t, socketPath, daemon.ListCommandsResponse{
		Commands: []model.ListedCommand{
			{Command: "ls -la", Shell: "zsh", StartTime: now, EndTime: now.Add(time.Second), Result: 1, Username: "u", Hostname: "h"},
		},
	})

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Storage:    &model.StorageConfig{Engine: model.StorageEngineBolt},
		SocketPath: socketPath,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{LsCommand}}
	// Default table format drives outputTable over the daemon-provided rows.
	require.NoError(t, app.Run([]string{"t", "ls"}))
}

// TestC2CommandList_BoltModeDaemonErrorPropagates covers the error return when
// the daemon connection drops mid-request: the listener accepts then closes the
// connection immediately, so RequestListCommands fails to decode a response.
func TestC2CommandList_BoltModeDaemonErrorPropagates(t *testing.T) {
	mc := c2SetupLs(t)

	socketPath := filepath.Join(t.TempDir(), "ls-bolt-err.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}
		// Close immediately without writing a response -> decode error.
		conn.Close()
	}()

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Storage:    &model.StorageConfig{Engine: model.StorageEngineBolt},
		SocketPath: socketPath,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{LsCommand}}
	err = app.Run([]string{"t", "ls", "-f", "json"})
	require.Error(t, err)
}

// --- outputJSON marshal error -------------------------------------------------

// TestC2OutputJSON_MarshalError feeds an unmarshalable value (a channel) to
// outputJSON to exercise the json.MarshalIndent error branch.
func TestC2OutputJSON_MarshalError(t *testing.T) {
	err := outputJSON(make(chan int))
	require.Error(t, err)
}

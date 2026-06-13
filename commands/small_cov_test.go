package commands

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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

// x3SetupCfgCmd swaps a mock ConfigService + isolated HOME for command-action
// coverage tests, restoring the original ConfigService on cleanup.
func x3SetupCfgCmd(t *testing.T) *model.MockConfigService {
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

// --- config view (table with token masking + json format) --------------------

// TestX3ConfigView_TableMasksToken covers outputConfigTable + the token-masking
// branch of flattenConfig (a >8-char Token is rendered as "abcd****wxyz").
func TestX3ConfigView_TableMasksToken(t *testing.T) {
	mc := x3SetupCfgCmd(t)
	enabled := true
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "abcdef1234567890", // >8 chars -> masked
		APIEndpoint: "https://api.shelltime.xyz",
		WebEndpoint: "https://shelltime.xyz",
		FlushCount:  10,
		DataMasking: &enabled,
		Exclude:     []string{"secret*"}, // non-empty slice -> JSON-marshal branch
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	require.NoError(t, app.Run([]string{"t", "view"}))
}

// TestX3ConfigView_JSONFormat covers the json output branch (outputConfigJSON).
func TestX3ConfigView_JSONFormat(t *testing.T) {
	mc := x3SetupCfgCmd(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: "https://api.shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	require.NoError(t, app.Run([]string{"t", "view", "--format", "json"}))
}

// TestX3ConfigView_UnsupportedFormat covers the format-validation error branch.
func TestX3ConfigView_UnsupportedFormat(t *testing.T) {
	x3SetupCfgCmd(t) // config not consulted before the format check
	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	err := app.Run([]string{"t", "view", "--format", "xml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

// TestX3ConfigView_ConfigReadError covers the config-read error branch.
func TestX3ConfigView_ConfigReadError(t *testing.T) {
	mc := x3SetupCfgCmd(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	err := app.Run([]string{"t", "view"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config")
}

// --- flattenConfig direct (token <= 8 chars masked branch) --------------------

// TestX3FlattenConfig_ShortTokenFullyMasked exercises the len(value) <= 8 token
// path which masks the whole value as "****".
func TestX3FlattenConfig_ShortTokenFullyMasked(t *testing.T) {
	pairs := flattenConfig(model.ShellTimeConfig{Token: "short"}, "")
	var tokenVal string
	for _, p := range pairs {
		if p.key == "token" {
			tokenVal = p.value
		}
	}
	assert.Equal(t, "****", tokenVal, "short tokens are fully masked")
}

// --- grep table-format success ------------------------------------------------

// TestX3CommandGrep_SuccessTable covers the table-output success branch (line
// 175 outputGrepTable) of commandGrep with a non-empty result set.
func TestX3CommandGrep_SuccessTable(t *testing.T) {
	mc := x3SetupCfgCmd(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"fetchCommands":{"count":2,"edges":[{"id":1,"shell":"bash","command":"git status","result":0},{"id":2,"shell":"zsh","command":"ls","result":0}]}}}`)
	}))
	t.Cleanup(srv.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	// Default format is table -> exercises outputGrepTable rendering.
	require.NoError(t, app.Run([]string{"t", "rg", "git"}))
}

// --- ls bolt-over-socket success ----------------------------------------------

// TestX3CommandList_BoltOverSocket drives commandList through the bolt branch:
// Storage.Engine=bolt + a ready unix socket make it request commands from the
// daemon. A minimal fake daemon answers the list_commands request, so
// daemon.RequestListCommands succeeds and the table is rendered.
func TestX3CommandList_BoltOverSocket(t *testing.T) {
	mc := x3SetupCfgCmd(t)

	socketPath := filepath.Join(t.TempDir(), "ls-daemon.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

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
				if msg.Type == daemon.SocketMessageTypeListCommands {
					_ = json.NewEncoder(c).Encode(daemon.ListCommandsResponse{
						Commands: []model.ListedCommand{
							{Command: "git status", Shell: "bash", Result: 0, Username: "u", Hostname: "h", StartTime: time.Now(), EndTime: time.Now()},
						},
					})
				}
			}(conn)
		}
	}()

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:      "tok",
		SocketPath: socketPath,
		Storage:    &model.StorageConfig{Engine: model.StorageEngineBolt},
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{LsCommand}}
	require.NoError(t, app.Run([]string{"t", "ls", "--format", "json"}))
}

// TestX3CommandList_UnsupportedFormat covers the format-validation error.
func TestX3CommandList_UnsupportedFormat(t *testing.T) {
	x3SetupCfgCmd(t)
	app := &cli.App{Name: "t", Commands: []*cli.Command{LsCommand}}
	err := app.Run([]string{"t", "ls", "--format", "xml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

// TestX3CommandList_ConfigReadError covers the config-read error branch.
func TestX3CommandList_ConfigReadError(t *testing.T) {
	mc := x3SetupCfgCmd(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	app := &cli.App{Name: "t", Commands: []*cli.Command{LsCommand}}
	err := app.Run([]string{"t", "ls", "--format", "json"})
	require.Error(t, err)
}

// --- codex install failure ----------------------------------------------------

// TestX3CodexInstall_DirCreateFails covers the install error branch of
// commandCodexInstall: making ~/.codex a regular file forces os.MkdirAll to
// fail, so Install() returns an error which the command surfaces.
func TestX3CodexInstall_DirCreateFails(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Create a *file* named ".codex" so MkdirAll on ~/.codex fails.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".codex"), []byte("x"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{CodexCommand}}
	err := app.Run([]string{"t", "codex", "install"})
	require.Error(t, err)
}

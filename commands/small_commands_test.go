package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func setupSmallCmdTest(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

// --- schema command -----------------------------------------------------------

func TestSchemaCommand_Stdout(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	app := &cli.App{Name: "t", Commands: []*cli.Command{SchemaCommand}}
	// Stdout path: just prints valid JSON schema and returns nil.
	err := app.Run([]string{"t", "schema"})
	require.NoError(t, err)
}

func TestSchemaCommand_WritesToFile(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	out := filepath.Join(t.TempDir(), "schema.json")
	app := &cli.App{Name: "t", Commands: []*cli.Command{SchemaCommand}}
	err := app.Run([]string{"t", "schema", "--output", out})
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	// Produced file must be valid JSON with the expected title.
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "ShellTime Configuration", parsed["title"])
}

func TestSchemaCommand_WriteErrorOnBadPath(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	// Output path points into a non-existent directory -> write fails.
	bad := filepath.Join(t.TempDir(), "no-such-dir", "schema.json")
	app := &cli.App{Name: "t", Commands: []*cli.Command{SchemaCommand}}
	err := app.Run([]string{"t", "schema", "-o", bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write schema file")
}

// --- web command (error branches; success would open a browser) ---------------

func TestWebCommand_ConfigReadError(t *testing.T) {
	mc := setupSmallCmdTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	app := &cli.App{Name: "t", Commands: []*cli.Command{WebCommand}}
	err := app.Run([]string{"t", "web"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config")
}

func TestWebCommand_EmptyWebEndpoint(t *testing.T) {
	mc := setupSmallCmdTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{WebEndpoint: ""}, nil)
	app := &cli.App{Name: "t", Commands: []*cli.Command{WebCommand}}
	err := app.Run([]string{"t", "web"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "web endpoint is not configured")
}

// --- sync command -------------------------------------------------------------

func TestSyncCommand_ConfigReadError(t *testing.T) {
	mc := setupSmallCmdTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	app := &cli.App{Name: "t", Commands: []*cli.Command{SyncCommand}}
	err := app.Run([]string{"t", "sync"})
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestSyncCommand_NoLocalDataSucceeds(t *testing.T) {
	mc := setupSmallCmdTest(t)

	// Isolate storage to an empty temp home so the file store has no data and
	// trySyncLocalToServer short-circuits with nil (nothing to sync). Paths are
	// derived from the model helpers (which honor HOME) rather than mutating
	// model's shared storage-folder globals.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// The file store requires post.txt to exist; create an empty commands dir +
	// post.txt so GetPostCommands returns zero rows (rather than an open error).
	cmdDir := model.GetCommandsStoragePath()
	require.NoError(t, os.MkdirAll(cmdDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "post.txt"), []byte(""), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: "https://example.invalid",
		FlushCount:  10,
		SocketPath:  filepath.Join(home, "missing.sock"),
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{SyncCommand}}
	// No post commands -> trySyncLocalToServer short-circuits with nil.
	err := app.Run([]string{"t", "sync", "--dry-run"})
	require.NoError(t, err)
}

// --- codex command (writes to temp ~/.codex/config.toml) ----------------------

func TestCodexInstall_WritesConfig(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)

	app := &cli.App{Name: "t", Commands: []*cli.Command{CodexCommand}}
	err := app.Run([]string{"t", "codex", "install"})
	require.NoError(t, err)

	cfgPath := filepath.Join(home, ".codex", "config.toml")
	data, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr, "codex config should be written")
	assert.Contains(t, string(data), "otlp-grpc")
}

func TestCodexUninstall_RemovesConfig(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)

	app := &cli.App{Name: "t", Commands: []*cli.Command{CodexCommand}}
	require.NoError(t, app.Run([]string{"t", "codex", "install"}))

	cfgPath := filepath.Join(home, ".codex", "config.toml")
	data, _ := os.ReadFile(cfgPath)
	require.Contains(t, string(data), "otlp-grpc")

	require.NoError(t, app.Run([]string{"t", "codex", "uninstall"}))
	data, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	assert.NotContains(t, string(data), "otlp-grpc", "uninstall should strip OTEL config")
}

func TestCodexUninstall_NoConfigSucceeds(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	app := &cli.App{Name: "t", Commands: []*cli.Command{CodexCommand}}
	// Nothing to uninstall -> Uninstall returns nil.
	err := app.Run([]string{"t", "codex", "uninstall"})
	require.NoError(t, err)
}

// --- doctor print helpers (stdout glue) ---------------------------------------

func TestDoctorPrintHelpers_DoNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		printSectionHeader("Section")
		printSuccess("ok")
		printError("bad")
		printWarning("warn")
		printInfo("info")
	})
}

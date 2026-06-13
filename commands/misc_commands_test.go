package commands

import (
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

// --- base.go injectors (trivial) ----------------------------------------------

func TestInjectVarAndAIService(t *testing.T) {
	origCommit := commitID
	origConfig := configService
	origAI := aiService
	t.Cleanup(func() {
		commitID = origCommit
		configService = origConfig
		aiService = origAI
	})

	cs := model.NewMockConfigService(t)
	InjectVar("abc123", cs)
	assert.Equal(t, "abc123", commitID)
	assert.Equal(t, model.ConfigService(cs), configService)

	ai := model.NewMockAIService(t)
	InjectAIService(ai)
	assert.Equal(t, model.AIService(ai), aiService)
}

// --- doctor command -----------------------------------------------------------

func setupMiscTest(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

func TestCommandDoctor_Success(t *testing.T) {
	mc := setupMiscTest(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	// Create the .shelltime dir so the directory check reports success.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".shelltime"), 0755))

	enabled := true
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		DataMasking:   &enabled,
		Encrypted:     &enabled,
		EnableMetrics: &enabled,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DoctorCommand}}
	// commandDoctor ignores daemon Check() failures and returns nil on a valid
	// config.
	err := app.Run([]string{"t", "doctor"})
	require.NoError(t, err)
}

func TestCommandDoctor_ConfigError(t *testing.T) {
	mc := setupMiscTest(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".shelltime"), 0755))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DoctorCommand}}
	err := app.Run([]string{"t", "doctor"})
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestCommandDoctor_NoShelltimeDir(t *testing.T) {
	mc := setupMiscTest(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "")
	// .shelltime dir intentionally missing -> "does not exist" branch; the action
	// still proceeds to read config and returns nil.
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DoctorCommand}}
	err := app.Run([]string{"t", "doctor"})
	require.NoError(t, err)
}

// --- hooks install / uninstall ------------------------------------------------

func TestCommandHooksInstall_BinaryNotFound(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	// PATH stripped so exec.LookPath("shelltime") fails, and the bin folder under
	// the temp HOME does not exist -> "binary not found" branch, returns nil.
	t.Setenv("PATH", "")

	app := &cli.App{Name: "t", Commands: []*cli.Command{HooksInstallCommand}}
	err := app.Run([]string{"t", "install"})
	require.NoError(t, err)
}

func TestCommandHooksUninstall_NoConfigsSucceeds(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/bash")
	// No shell config files exist -> each Uninstall() returns nil -> action nil.
	app := &cli.App{Name: "t", Commands: []*cli.Command{HooksUninstallCommand}}
	err := app.Run([]string{"t", "uninstall"})
	require.NoError(t, err)
}

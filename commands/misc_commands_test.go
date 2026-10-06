package commands

import (
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
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

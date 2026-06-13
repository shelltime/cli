package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func setupCCTest(t *testing.T) string {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// const must match the markers used by model/aicode_otel_env.go.
const ccOtelMarker = "# >>> shelltime cc otel >>>"

func TestCCInstall_WritesOtelBlockToShellConfigs(t *testing.T) {
	home := setupCCTest(t)

	// Pre-create zsh and fish configs so their Install paths succeed (bash is
	// auto-created). This exercises the "happy path" for all three shells.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# zsh\n"), 0644))
	fishDir := filepath.Join(home, ".config", "fish")
	require.NoError(t, os.MkdirAll(fishDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(fishDir, "config.fish"), []byte("# fish\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{CCCommand}}
	err := app.Run([]string{"t", "cc", "install"})
	require.NoError(t, err)

	// All three config files should now contain the OTEL marker.
	for _, p := range []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(fishDir, "config.fish"),
	} {
		data, readErr := os.ReadFile(p)
		require.NoError(t, readErr, "config %s should exist after install", p)
		assert.Contains(t, string(data), ccOtelMarker, "OTEL block should be present in %s", p)
	}
}

func TestCCInstall_MissingZshAndFishStillSucceeds(t *testing.T) {
	home := setupCCTest(t)
	// No zsh/fish configs present. zsh & fish Install() return errors that the
	// command swallows (prints), bash is auto-created. Action returns nil.
	app := &cli.App{Name: "t", Commands: []*cli.Command{CCCommand}}
	err := app.Run([]string{"t", "cc", "install"})
	require.NoError(t, err)

	// bash config is created and contains the marker.
	data, readErr := os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, readErr)
	assert.Contains(t, string(data), ccOtelMarker)
}

func TestCCUninstall_RemovesOtelBlock(t *testing.T) {
	home := setupCCTest(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# zsh\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{CCCommand}}
	// Install first, then uninstall, and confirm the block is gone.
	require.NoError(t, app.Run([]string{"t", "cc", "install"}))

	zshrc := filepath.Join(home, ".zshrc")
	data, _ := os.ReadFile(zshrc)
	require.Contains(t, string(data), ccOtelMarker)

	require.NoError(t, app.Run([]string{"t", "cc", "uninstall"}))
	data, readErr := os.ReadFile(zshrc)
	require.NoError(t, readErr)
	assert.NotContains(t, string(data), ccOtelMarker, "uninstall should strip the OTEL block")
}

func TestCCUninstall_NoConfigsSucceeds(t *testing.T) {
	setupCCTest(t)
	// Nothing exists; Uninstall() returns nil for missing files. Action nil.
	app := &cli.App{Name: "t", Commands: []*cli.Command{CCCommand}}
	err := app.Run([]string{"t", "cc", "uninstall"})
	require.NoError(t, err)
}

func TestCCInstall_IdempotentNoDuplicateBlock(t *testing.T) {
	home := setupCCTest(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# zsh\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{CCCommand}}
	require.NoError(t, app.Run([]string{"t", "cc", "install"}))
	require.NoError(t, app.Run([]string{"t", "cc", "install"}))

	// Install removes any existing block before re-adding, so the marker should
	// appear exactly once even after two installs.
	data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), ccOtelMarker),
		"install should be idempotent (single OTEL block)")
}

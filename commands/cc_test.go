package commands

import (
	"encoding/json"
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

func ccSettingsEnv(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	require.NoError(t, err)
	var settings struct {
		Env map[string]any `json:"env"`
	}
	require.NoError(t, json.Unmarshal(data, &settings))
	return settings.Env
}

func runCC(t *testing.T, args ...string) {
	t.Helper()
	app := &cli.App{Name: "t", Commands: []*cli.Command{CCCommand}}
	require.NoError(t, app.Run(append([]string{"t", "cc"}, args...)))
}

func TestCCInstall_WritesClaudeSettingsEnv(t *testing.T) {
	home := setupCCTest(t)

	runCC(t, "install")

	env := ccSettingsEnv(t, home)
	assert.Equal(t, "1", env["CLAUDE_CODE_ENABLE_TELEMETRY"])
	assert.Equal(t, "http://localhost:54027", env["OTEL_EXPORTER_OTLP_ENDPOINT"])

	// Shell rc files are no longer created.
	_, err := os.Stat(filepath.Join(home, ".bashrc"))
	assert.True(t, os.IsNotExist(err), ".bashrc should not be created")
}

func TestCCInstall_MigratesLegacyShellBlocks(t *testing.T) {
	home := setupCCTest(t)

	fishDir := filepath.Join(home, ".config", "fish")
	require.NoError(t, os.MkdirAll(fishDir, 0755))
	rcFiles := map[string]string{
		filepath.Join(home, ".zshrc"):         "# zsh\n",
		filepath.Join(home, ".bashrc"):        "# bash\n",
		filepath.Join(fishDir, "config.fish"): "# fish\n",
	}
	for p, content := range rcFiles {
		require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	}
	// Seed the legacy blocks the way older versions of `cc install` did.
	for _, svc := range legacyAICodeOtelEnvServices() {
		require.NoError(t, svc.Install())
	}

	runCC(t, "install")

	for p, content := range rcFiles {
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.NotContains(t, string(data), ccOtelMarker, "legacy block should be removed from %s", p)
		assert.Contains(t, string(data), strings.TrimSpace(content), "other content should stay in %s", p)
	}
	assert.Equal(t, "1", ccSettingsEnv(t, home)["CLAUDE_CODE_ENABLE_TELEMETRY"])
}

func TestCCInstall_LeavesRcFilesWithoutBlockUntouched(t *testing.T) {
	home := setupCCTest(t)
	zshrc := filepath.Join(home, ".zshrc")
	content := "# zsh without trailing newline"
	require.NoError(t, os.WriteFile(zshrc, []byte(content), 0644))

	runCC(t, "install")

	data, err := os.ReadFile(zshrc)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestCCUninstall_RemovesSettingsEnvAndLegacyBlock(t *testing.T) {
	home := setupCCTest(t)
	zshrc := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(zshrc, []byte("# zsh\n"), 0644))
	require.NoError(t, legacyAICodeOtelEnvServices()[0].Install())

	runCC(t, "install")
	runCC(t, "uninstall")

	assert.Empty(t, ccSettingsEnv(t, home))
	data, err := os.ReadFile(zshrc)
	require.NoError(t, err)
	assert.NotContains(t, string(data), ccOtelMarker)
}

func TestCCUninstall_NoConfigsSucceeds(t *testing.T) {
	setupCCTest(t)
	// Nothing exists; every Uninstall() returns nil for missing files.
	runCC(t, "uninstall")
}

func TestCCInstall_IdempotentNoDuplicateKeys(t *testing.T) {
	home := setupCCTest(t)
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	runCC(t, "install")
	first, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	runCC(t, "install")
	second, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second), "install should be idempotent")
	assert.Equal(t, 1, strings.Count(string(second), "OTEL_EXPORTER_OTLP_ENDPOINT"))
}

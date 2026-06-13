package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countMarkers counts how many times the OTEL start-marker appears in a file.
func countMarkers(t *testing.T, path string) int {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Count(string(content), aiCodeOtelMarkerStart)
}

func TestAICodeOtelEnv_Match_TableDriven(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cases := []struct {
		name      string
		svc       AICodeOtelEnvService
		shellName string
		want      bool
	}{
		{"bash exact", NewBashAICodeOtelEnvService(), "bash", true},
		{"bash uppercase", NewBashAICodeOtelEnvService(), "BASH", true},
		{"bash full path", NewBashAICodeOtelEnvService(), "/bin/bash", true},
		{"bash mismatch", NewBashAICodeOtelEnvService(), "zsh", false},
		{"zsh exact", NewZshAICodeOtelEnvService(), "zsh", true},
		{"zsh in path", NewZshAICodeOtelEnvService(), "/usr/bin/ZSH", true},
		{"zsh mismatch", NewZshAICodeOtelEnvService(), "fish", false},
		{"fish exact", NewFishAICodeOtelEnvService(), "fish", true},
		{"fish in path", NewFishAICodeOtelEnvService(), "/opt/Fish", true},
		{"fish mismatch", NewFishAICodeOtelEnvService(), "bash", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.svc.Match(tc.shellName))
		})
	}
}

func TestAICodeOtelEnv_ShellName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	assert.Equal(t, "bash", NewBashAICodeOtelEnvService().ShellName())
	assert.Equal(t, "zsh", NewZshAICodeOtelEnvService().ShellName())
	assert.Equal(t, "fish", NewFishAICodeOtelEnvService().ShellName())
}

func TestBashAICodeOtelEnv_InstallCreatesFileAndMarkers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewBashAICodeOtelEnvService()
	bashrc := filepath.Join(home, ".bashrc")

	// File does not exist yet; bash Install should create it.
	_, statErr := os.Stat(bashrc)
	require.True(t, os.IsNotExist(statErr))

	require.NoError(t, svc.Install())

	content, err := os.ReadFile(bashrc)
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, aiCodeOtelMarkerStart)
	assert.Contains(t, s, aiCodeOtelMarkerEnd)
	assert.Contains(t, s, "export CLAUDE_CODE_ENABLE_TELEMETRY=1")
	assert.Contains(t, s, "export OTEL_EXPORTER_OTLP_ENDPOINT="+aiCodeOtelEndpoint)
	assert.Equal(t, 1, countMarkers(t, bashrc))

	// Installing twice must not duplicate the marker block (remove-then-add).
	require.NoError(t, svc.Install())
	assert.Equal(t, 1, countMarkers(t, bashrc), "running Install twice should keep exactly one marker block")
}

func TestBashAICodeOtelEnv_CheckAndUninstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewBashAICodeOtelEnvService()
	bashrc := filepath.Join(home, ".bashrc")

	// Check on missing file -> error.
	err := svc.Check()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Uninstall on missing file -> nil (nothing to do).
	require.NoError(t, svc.Uninstall())

	// Create file without markers -> Check should report not installed.
	require.NoError(t, os.WriteFile(bashrc, []byte("# my bashrc\nexport FOO=bar\n"), 0644))
	err = svc.Check()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// After Install, Check passes.
	require.NoError(t, svc.Install())
	require.NoError(t, svc.Check())

	// Pre-existing content survives install.
	content, err := os.ReadFile(bashrc)
	require.NoError(t, err)
	assert.Contains(t, string(content), "export FOO=bar")

	// Uninstall removes markers; Check then fails again, but user content remains.
	require.NoError(t, svc.Uninstall())
	assert.Equal(t, 0, countMarkers(t, bashrc))
	content, err = os.ReadFile(bashrc)
	require.NoError(t, err)
	assert.Contains(t, string(content), "export FOO=bar")
	assert.NotContains(t, string(content), "CLAUDE_CODE_ENABLE_TELEMETRY")
	require.Error(t, svc.Check())
}

func TestZshAICodeOtelEnv_InstallRequiresExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewZshAICodeOtelEnvService()
	zshrc := filepath.Join(home, ".zshrc")

	// Zsh Install errors when the config file is missing (unlike bash).
	err := svc.Install()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "zsh config file not found")

	// Once the file exists, Install succeeds and is idempotent.
	require.NoError(t, os.WriteFile(zshrc, []byte("# zshrc\n"), 0644))
	require.NoError(t, svc.Install())
	require.NoError(t, svc.Check())
	assert.Equal(t, 1, countMarkers(t, zshrc))
	require.NoError(t, svc.Install())
	assert.Equal(t, 1, countMarkers(t, zshrc))

	require.NoError(t, svc.Uninstall())
	require.Error(t, svc.Check())
}

func TestFishAICodeOtelEnv_InstallRequiresExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewFishAICodeOtelEnvService()
	fishConfig := filepath.Join(home, ".config", "fish", "config.fish")

	// Missing file -> Install and Check both error.
	require.Error(t, svc.Install())
	require.Error(t, svc.Check())
	// Uninstall on missing file -> nil.
	require.NoError(t, svc.Uninstall())

	// Create the file and install fish-syntax env vars.
	require.NoError(t, os.MkdirAll(filepath.Dir(fishConfig), 0755))
	require.NoError(t, os.WriteFile(fishConfig, []byte("# fish config\n"), 0644))

	require.NoError(t, svc.Install())
	require.NoError(t, svc.Check())
	content, err := os.ReadFile(fishConfig)
	require.NoError(t, err)
	assert.Contains(t, string(content), "set -gx CLAUDE_CODE_ENABLE_TELEMETRY 1")
	assert.Contains(t, string(content), "set -gx OTEL_EXPORTER_OTLP_ENDPOINT "+aiCodeOtelEndpoint)

	require.NoError(t, svc.Uninstall())
	assert.Equal(t, 0, countMarkers(t, fishConfig))
	require.Error(t, svc.Check())
}

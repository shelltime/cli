package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexOtelConfig_InstallCreatesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewCodexOtelConfigService()
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)

	// Initially not installed.
	ok, err := svc.Check()
	require.NoError(t, err)
	assert.False(t, ok, "Check on missing file should report not configured")

	// Install creates ~/.codex/config.toml with an [otel] table.
	require.NoError(t, svc.Install())

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, toml.Unmarshal(data, &parsed))
	otel, ok := parsed["otel"].(map[string]interface{})
	require.True(t, ok, "otel table should be present")
	assert.Equal(t, true, otel["log_user_prompt"])

	exporter, ok := otel["exporter"].(map[string]interface{})
	require.True(t, ok)
	grpc, ok := exporter["otlp-grpc"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, aiCodeOtelEndpoint, grpc["endpoint"])

	// Now Check reports installed.
	ok, err = svc.Check()
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCodexOtelConfig_InstallPreservesExistingKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	// Pre-existing unrelated config that must survive the install.
	require.NoError(t, os.WriteFile(configPath, []byte("model = \"gpt-5\"\n"), 0644))

	svc := NewCodexOtelConfigService()
	require.NoError(t, svc.Install())

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, toml.Unmarshal(data, &parsed))
	assert.Equal(t, "gpt-5", parsed["model"], "existing keys must be preserved")
	assert.Contains(t, parsed, "otel")
}

func TestCodexOtelConfig_Uninstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewCodexOtelConfigService()
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)

	// Uninstall on a missing file is a no-op.
	require.NoError(t, svc.Uninstall())

	// Install then uninstall should drop the otel table but keep other keys.
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte("model = \"gpt-5\"\n"), 0644))
	require.NoError(t, svc.Install())
	require.NoError(t, svc.Uninstall())

	ok, err := svc.Check()
	require.NoError(t, err)
	assert.False(t, ok, "otel should be gone after uninstall")

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, toml.Unmarshal(data, &parsed))
	assert.Equal(t, "gpt-5", parsed["model"], "unrelated keys survive uninstall")
	assert.NotContains(t, parsed, "otel")
}

func TestCodexOtelConfig_CheckMalformedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte("this is = = not valid toml ]["), 0644))

	svc := NewCodexOtelConfigService()
	ok, err := svc.Check()
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), "failed to parse config")
}

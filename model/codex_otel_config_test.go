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
	assert.Equal(t, true, otel["log_agent_responses"])

	exporter, ok := otel["exporter"].(map[string]interface{})
	require.True(t, ok)
	grpc, ok := exporter["otlp-grpc"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, AICodeOtelEndpoint, grpc["endpoint"])

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

func TestCodexOtelConfig_CheckRequiresShellTimeExporter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := NewCodexOtelConfigService()
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))

	// The user's own [otel] table without our exporter is not "installed".
	require.NoError(t, os.WriteFile(configPath, []byte("[otel]\nenvironment = \"prod\"\n"), 0644))
	ok, err := svc.Check()
	require.NoError(t, err)
	assert.False(t, ok)

	// An exporter pointing somewhere else isn't ours either.
	require.NoError(t, os.WriteFile(configPath, []byte(
		"[otel]\nexporter = { otlp-grpc = { endpoint = \"http://collector:4317\" } }\n"), 0644))
	ok, err = svc.Check()
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, svc.Install())
	ok, err = svc.Check()
	require.NoError(t, err)
	assert.True(t, ok)
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

func TestCodexOtelConfig_Endpoint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := NewCodexOtelConfigService()
	configPath := filepath.Join(home, ".codex", "config.toml")

	endpoint, err := svc.Endpoint()
	require.NoError(t, err, "missing config is not an error")
	assert.Empty(t, endpoint)

	require.NoError(t, svc.Install())
	endpoint, err = svc.Endpoint()
	require.NoError(t, err)
	assert.Equal(t, AICodeOtelEndpoint, endpoint)

	// `exporter` may be a plain string in a valid Codex config.
	require.NoError(t, os.WriteFile(configPath, []byte("[otel]\nexporter = \"none\"\n"), 0644))
	endpoint, err = svc.Endpoint()
	require.NoError(t, err)
	assert.Empty(t, endpoint)

	require.NoError(t, os.WriteFile(configPath, []byte("[otel\n"), 0644))
	_, err = svc.Endpoint()
	assert.Error(t, err)
}

func readCodexConfig(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, toml.Unmarshal(data, &parsed))
	return parsed
}

// userCodexOtelConfig has [otel] keys of the user's own next to an exporter shelltime replaces.
const userCodexOtelConfig = `model = "gpt-5-codex"

[otel]
environment = "dev"
log_user_prompt = false
trace_exporter = "none"

[otel.exporter.otlp-http]
endpoint = "https://collector.example.com/v1/logs"
protocol = "binary"

[profiles.fast]
model = "gpt-5-mini"
`

func TestCodexOtelConfig_InstallMergesIntoExistingOtelTable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte(userCodexOtelConfig), 0644))

	svc := NewCodexOtelConfigService()
	require.NoError(t, svc.Install())

	parsed := readCodexConfig(t, configPath)
	assert.Equal(t, "gpt-5-codex", parsed["model"])
	assert.Contains(t, parsed, "profiles", "other tables survive")

	otel := parsed["otel"].(map[string]interface{})
	assert.Equal(t, "dev", otel["environment"], "the user's other [otel] keys are kept")
	assert.Equal(t, "none", otel["trace_exporter"])
	assert.Equal(t, true, otel["log_user_prompt"])
	assert.Equal(t, true, otel["log_agent_responses"])
	// exporter selects one exporter, so it is replaced, not merged with otlp-http.
	assert.Equal(t, map[string]interface{}{"otlp-grpc": map[string]interface{}{"endpoint": AICodeOtelEndpoint}}, otel["exporter"])

	endpoint, err := svc.Endpoint()
	require.NoError(t, err)
	assert.Equal(t, AICodeOtelEndpoint, endpoint)
}

func TestCodexOtelConfig_InstallIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte(userCodexOtelConfig), 0644))

	svc := NewCodexOtelConfigService()
	require.NoError(t, svc.Install())
	first, err := os.ReadFile(configPath)
	require.NoError(t, err)

	require.NoError(t, svc.Install())
	second, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

func TestCodexOtelConfig_UninstallRemovesOnlyManagedKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, os.WriteFile(configPath, []byte(userCodexOtelConfig), 0644))

	svc := NewCodexOtelConfigService()
	require.NoError(t, svc.Install())
	require.NoError(t, svc.Uninstall())

	parsed := readCodexConfig(t, configPath)
	otel, ok := parsed["otel"].(map[string]interface{})
	require.True(t, ok, "[otel] stays while the user's keys are in it")
	assert.Equal(t, map[string]interface{}{"environment": "dev", "trace_exporter": "none"}, otel)
	assert.Equal(t, "gpt-5-codex", parsed["model"])

	endpoint, err := svc.Endpoint()
	require.NoError(t, err)
	assert.Empty(t, endpoint)
}

func TestCodexOtelConfig_UninstallKeepsForeignExporter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	foreign := "[otel]\nlog_user_prompt = true\n\n[otel.exporter.otlp-grpc]\nendpoint = \"http://collector:4317\"\n"
	require.NoError(t, os.WriteFile(configPath, []byte(foreign), 0644))

	svc := NewCodexOtelConfigService()
	require.NoError(t, svc.Uninstall())

	endpoint, err := svc.Endpoint()
	require.NoError(t, err)
	assert.Equal(t, "http://collector:4317", endpoint, "an exporter pointing elsewhere isn't shelltime's")
	otel := readCodexConfig(t, configPath)["otel"].(map[string]interface{})
	assert.NotContains(t, otel, "log_user_prompt")
}

func TestCodexOtelConfig_MissingManagedKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, codexConfigDir, codexConfigFile)
	svc := NewCodexOtelConfigService()

	missing, err := svc.MissingManagedKeys()
	require.NoError(t, err)
	assert.Equal(t, []string{"log_user_prompt", "log_agent_responses"}, missing)

	// A config written by an older `codex install`.
	require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0755))
	older := "[otel]\nlog_user_prompt = true\n\n[otel.exporter.otlp-grpc]\nendpoint = \"" + AICodeOtelEndpoint + "\"\n"
	require.NoError(t, os.WriteFile(configPath, []byte(older), 0644))
	missing, err = svc.MissingManagedKeys()
	require.NoError(t, err)
	assert.Equal(t, []string{"log_agent_responses"}, missing)

	require.NoError(t, svc.Install())
	missing, err = svc.MissingManagedKeys()
	require.NoError(t, err)
	assert.Empty(t, missing)

	// An explicit false is the user's choice.
	require.NoError(t, os.WriteFile(configPath, []byte("[otel]\nlog_user_prompt = false\nlog_agent_responses = false\n"), 0644))
	missing, err = svc.MissingManagedKeys()
	require.NoError(t, err)
	assert.Empty(t, missing)

	require.NoError(t, os.WriteFile(configPath, []byte("[otel\n"), 0644))
	_, err = svc.MissingManagedKeys()
	assert.Error(t, err)
}

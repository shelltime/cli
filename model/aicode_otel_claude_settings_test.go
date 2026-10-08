package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realisticClaudeSettings mirrors the shape of a typical user settings.json: an existing env var,
// non-alphabetical top-level keys, nested objects and arrays.
const realisticClaudeSettings = `{
  "env": {
    "CLAUDE_CODE_NO_FLICKER": "1"
  },
  "permissions": {
    "defaultMode": "auto",
    "allow": ["Bash(ls:*)", "Read"]
  },
  "model": "opus",
  "enabledPlugins": {
    "shelltime-statusline@shelltime": true
  },
  "alwaysThinkingEnabled": true
}
`

func setupClaudeSettingsTest(t *testing.T) (*ClaudeSettingsAICodeOtelEnvService, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return NewClaudeSettingsAICodeOtelEnvService(), filepath.Join(home, ".claude", "settings.json")
}

func writeClaudeSettings(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func readOrderedKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	fields, err := decodeOrderedObject(raw)
	require.NoError(t, err)
	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}
	return keys
}

func readClaudeSettingsEnv(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var settings struct {
		Env map[string]any `json:"env"`
	}
	require.NoError(t, json.Unmarshal(data, &settings))
	return settings.Env
}

func TestClaudeSettingsOtel_InstallCreatesMissingFile(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)

	require.NoError(t, svc.Install())

	env := readClaudeSettingsEnv(t, path)
	for _, v := range claudeSettingsOtelEnvVars() {
		assert.Equal(t, v.Value, env[v.Key], "env var %s", v.Key)
	}
	assert.Equal(t, AICodeOtelEndpoint, env["OTEL_EXPORTER_OTLP_ENDPOINT"])

	attrs, _ := env["OTEL_RESOURCE_ATTRIBUTES"].(string)
	assert.Contains(t, attrs, "team.id=shelltime")
	assert.NotContains(t, attrs, "pwd=")
	assert.NotContains(t, attrs, "$(", "settings.json values are not shell-expanded")

	assert.NoError(t, svc.Check())
}

func TestClaudeSettingsOtel_InstallPreservesExistingSettings(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	writeClaudeSettings(t, path, realisticClaudeSettings)

	require.NoError(t, svc.Install())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	// Top-level key order is unchanged.
	assert.Equal(t, []string{"env", "permissions", "model", "enabledPlugins", "alwaysThinkingEnabled"}, readOrderedKeys(t, data))

	// Unrelated values survive.
	var settings map[string]any
	require.NoError(t, json.Unmarshal(data, &settings))
	assert.Equal(t, "opus", settings["model"])
	assert.Equal(t, true, settings["alwaysThinkingEnabled"])
	perms := settings["permissions"].(map[string]any)
	assert.Equal(t, []any{"Bash(ls:*)", "Read"}, perms["allow"])

	// The user's env var stays first; managed vars are appended after it.
	fields, err := decodeOrderedObject(data)
	require.NoError(t, err)
	envRaw, ok := getField(fields, "env")
	require.True(t, ok)
	envKeys := readOrderedKeys(t, envRaw)
	require.NotEmpty(t, envKeys)
	assert.Equal(t, "CLAUDE_CODE_NO_FLICKER", envKeys[0])
	assert.Len(t, envKeys, 1+len(claudeSettingsOtelEnvVars()))

	// Output uses the 2-space indentation Claude Code writes.
	assert.Contains(t, string(data), "\n  \"model\": \"opus\"")
}

func TestClaudeSettingsOtel_InstallIsIdempotent(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	writeClaudeSettings(t, path, realisticClaudeSettings)

	require.NoError(t, svc.Install())
	first, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, svc.Install())
	second, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
}

func TestClaudeSettingsOtel_InstallReplacesStaleValueInPlace(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	writeClaudeSettings(t, path, `{"env":{"OTEL_EXPORTER_OTLP_ENDPOINT":"http://localhost:4317","FOO":"bar"}}`)

	require.NoError(t, svc.Install())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	fields, err := decodeOrderedObject(data)
	require.NoError(t, err)
	envRaw, _ := getField(fields, "env")
	envKeys := readOrderedKeys(t, envRaw)
	assert.Equal(t, []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "FOO"}, envKeys[:2], "existing keys keep their position")

	env := readClaudeSettingsEnv(t, path)
	assert.Equal(t, AICodeOtelEndpoint, env["OTEL_EXPORTER_OTLP_ENDPOINT"])
	assert.Equal(t, "bar", env["FOO"])
}

func TestClaudeSettingsOtel_InstallNullEnv(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	writeClaudeSettings(t, path, `{"env":null,"model":"opus"}`)

	require.NoError(t, svc.Install())
	assert.NoError(t, svc.Check())
}

func TestClaudeSettingsOtel_InvalidJSONIsLeftUntouched(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"malformed", `{"env": {`},
		{"not an object", `["env"]`},
		{"trailing data", `{"env":{}} {}`},
		{"env not an object", `{"env":"oops"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, path := setupClaudeSettingsTest(t)
			writeClaudeSettings(t, path, tc.content)

			assert.Error(t, svc.Install())

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tc.content, string(data))
		})
	}
}

func TestClaudeSettingsOtel_UninstallRemovesOnlyManagedKeys(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	writeClaudeSettings(t, path, realisticClaudeSettings)
	require.NoError(t, svc.Install())

	require.NoError(t, svc.Uninstall())

	env := readClaudeSettingsEnv(t, path)
	assert.Equal(t, map[string]any{"CLAUDE_CODE_NO_FLICKER": "1"}, env)
	assert.Error(t, svc.Check())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"env", "permissions", "model", "enabledPlugins", "alwaysThinkingEnabled"}, readOrderedKeys(t, data))
}

func TestClaudeSettingsOtel_UninstallDropsEmptyEnv(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	writeClaudeSettings(t, path, `{"model":"opus"}`)
	require.NoError(t, svc.Install())

	require.NoError(t, svc.Uninstall())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"model"}, readOrderedKeys(t, data))
}

func TestClaudeSettingsOtel_UninstallWithoutManagedKeysDoesNotRewrite(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	content := "{\"model\":\"opus\",\"env\":{\"FOO\":\"bar\"}}"
	writeClaudeSettings(t, path, content)

	require.NoError(t, svc.Uninstall())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestClaudeSettingsOtel_UninstallMissingFile(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)

	require.NoError(t, svc.Uninstall())

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "uninstall must not create the settings file")
}

func TestClaudeSettingsOtel_CheckMissingFile(t *testing.T) {
	svc, _ := setupClaudeSettingsTest(t)
	assert.Error(t, svc.Check())
}

func TestClaudeSettingsOtel_InstallKeepsFileMode(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)
	writeClaudeSettings(t, path, `{}`)
	require.NoError(t, os.Chmod(path, 0600))

	require.NoError(t, svc.Install())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestClaudeSettings_StatusLineCommand(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)

	cmd, err := svc.StatusLineCommand()
	require.NoError(t, err, "missing settings.json is not an error")
	assert.Empty(t, cmd)

	writeClaudeSettings(t, path, realisticClaudeSettings)
	cmd, err = svc.StatusLineCommand()
	require.NoError(t, err)
	assert.Empty(t, cmd, "no statusLine key")

	writeClaudeSettings(t, path, `{"statusLine": {"type": "command", "command": "shelltime cc statusline"}}`)
	cmd, err = svc.StatusLineCommand()
	require.NoError(t, err)
	assert.Equal(t, "shelltime cc statusline", cmd)

	writeClaudeSettings(t, path, `{"statusLine": `)
	_, err = svc.StatusLineCommand()
	assert.Error(t, err)
}

func TestClaudeSettingsOtel_InstallEnablesDetailsAndDeltaTemporality(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)

	require.NoError(t, svc.Install())

	env := readClaudeSettingsEnv(t, path)
	assert.Equal(t, "1", env["OTEL_LOG_TOOL_DETAILS"])
	assert.Equal(t, "1", env["OTEL_LOG_ASSISTANT_RESPONSES"])
	assert.Equal(t, "true", env["OTEL_METRICS_INCLUDE_ENTRYPOINT"])
	assert.Equal(t, "true", env["OTEL_METRICS_INCLUDE_REPOSITORY"])
	assert.Equal(t, "delta", env["OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE"])
}

func TestClaudeSettingsOtel_MissingManagedKeys(t *testing.T) {
	svc, path := setupClaudeSettingsTest(t)

	all := make([]string, 0, len(claudeSettingsOtelEnvVars()))
	for _, v := range claudeSettingsOtelEnvVars() {
		all = append(all, v.Key)
	}

	missing, err := svc.MissingManagedKeys()
	require.NoError(t, err, "a missing file is not an error")
	assert.Equal(t, all, missing)

	// Settings written by an older `cc install`.
	writeClaudeSettings(t, path, `{"env": {
		"CLAUDE_CODE_ENABLE_TELEMETRY": "1", "OTEL_METRICS_EXPORTER": "otlp", "OTEL_LOGS_EXPORTER": "otlp",
		"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc", "OTEL_EXPORTER_OTLP_ENDPOINT": "`+AICodeOtelEndpoint+`",
		"OTEL_METRIC_EXPORT_INTERVAL": "10000", "OTEL_LOGS_EXPORT_INTERVAL": "5000", "OTEL_LOG_USER_PROMPTS": "1",
		"OTEL_METRICS_INCLUDE_SESSION_ID": "true", "OTEL_METRICS_INCLUDE_VERSION": "true",
		"OTEL_METRICS_INCLUDE_ACCOUNT_UUID": "true", "OTEL_RESOURCE_ATTRIBUTES": "team.id=shelltime"}}`)
	missing, err = svc.MissingManagedKeys()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"OTEL_LOG_TOOL_DETAILS",
		"OTEL_LOG_ASSISTANT_RESPONSES",
		"OTEL_METRICS_INCLUDE_ENTRYPOINT",
		"OTEL_METRICS_INCLUDE_REPOSITORY",
		"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE",
	}, missing)

	require.NoError(t, svc.Install())
	missing, err = svc.MissingManagedKeys()
	require.NoError(t, err)
	assert.Empty(t, missing)

	// An explicit opt-out is the user's choice, not a missing key.
	writeClaudeSettings(t, path, `{"env":{"OTEL_LOG_TOOL_DETAILS":"0"}}`)
	missing, err = svc.MissingManagedKeys()
	require.NoError(t, err)
	assert.NotContains(t, missing, "OTEL_LOG_TOOL_DETAILS")

	writeClaudeSettings(t, path, `{"env": `)
	_, err = svc.MissingManagedKeys()
	assert.Error(t, err)
}

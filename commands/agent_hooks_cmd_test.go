package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func setupAgentHooksCommand(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	orig := hookBinaryPath
	hookBinaryPath = func() (string, error) { return "/usr/local/bin/shelltime", nil }
	t.Cleanup(func() { hookBinaryPath = orig })
	return home
}

func TestCursorInstallAndUninstallCommands(t *testing.T) {
	home := setupAgentHooksCommand(t)
	path := filepath.Join(home, ".cursor", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"hooks":{"stop":[{"command":"say done"}]}}`), 0644))
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "cursor", "install"}))
	text := stripANSI(out.String())
	assert.Contains(t, text, "Cursor hooks installed in "+path)
	assert.Contains(t, text, "Restart Cursor")
	assert.Contains(t, text, "Privacy: the Cursor hooks send your prompts")
	assert.Contains(t, text, "shelltime config disable agentHooks.sendContent")

	status, err := model.NewCursorHooksService().Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Empty(t, status.MissingEvents)
	assert.Equal(t, []string{"/usr/local/bin/shelltime"}, status.Binaries)

	require.NoError(t, app.Run([]string{"t", "cursor", "u"}))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":1,"hooks":{"stop":[{"command":"say done"}]}}`, string(data))
}

func TestCursorInstallCommand_RefusesInvalidFile(t *testing.T) {
	home := setupAgentHooksCommand(t)
	path := filepath.Join(home, ".cursor", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,`), 0644))
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	err := app.Run([]string{"t", "cursor", "i"})
	assert.ErrorIs(t, err, model.ErrHookFileInvalid)
	assert.Contains(t, out.String(), "Failed to install the Cursor hooks")
	assert.NotContains(t, out.String(), "Privacy")
}

func TestAgyInstallAndUninstallCommands(t *testing.T) {
	home := setupAgentHooksCommand(t)
	path := filepath.Join(home, ".gemini", "config", "hooks.json")
	app := &cli.App{Name: "t", Commands: []*cli.Command{AgyCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "antigravity", "install"}))
	text := stripANSI(out.String())
	assert.Contains(t, text, "Antigravity hooks installed in "+path)
	assert.Contains(t, text, "Restart Antigravity")
	assert.Contains(t, text, "Privacy: the Antigravity hooks send")

	var root map[string]map[string]json.RawMessage
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &root))
	assert.Len(t, root["shelltime"], 5)

	require.NoError(t, app.Run([]string{"t", "agy", "uninstall"}))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))
}

func TestCursorProjectInstallAndUninstallCommands(t *testing.T) {
	home := setupAgentHooksCommand(t)
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0755))
	sub := filepath.Join(repo, "web")
	require.NoError(t, os.Mkdir(sub, 0755))
	t.Chdir(sub)
	path := filepath.Join(repo, ".cursor", "hooks.json")
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "cursor", "install", "--project"}))
	text := stripANSI(out.String())
	assert.Contains(t, text, "Cursor project hooks installed in "+path)
	assert.Contains(t, text, "Commit it: Cursor Cloud Agents run only the repository's hooks")
	assert.Contains(t, text, "SHELLTIME_CLOUD=true")
	assert.Contains(t, text, "Privacy: the Cursor hooks send")
	assert.NotContains(t, text, "Restart Cursor")

	status, err := model.NewCursorProjectHooksService(repo).Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, []string{model.ProjectHookBinary}, status.Binaries, "the project file runs shelltime from PATH")
	_, err = os.Stat(filepath.Join(home, ".cursor", "hooks.json"))
	assert.True(t, os.IsNotExist(err), "the user's hook file is left alone")

	require.NoError(t, app.Run([]string{"t", "cursor", "uninstall", "--project"}))
	status, err = model.NewCursorProjectHooksService(repo).Status()
	require.NoError(t, err)
	assert.False(t, status.Installed)
}

func TestAgyProjectInstallAndUninstallCommands(t *testing.T) {
	home := setupAgentHooksCommand(t)
	repo := t.TempDir()
	t.Chdir(repo) // not a repository: the folder itself
	path := filepath.Join(repo, ".agents", "hooks.json")
	app := &cli.App{Name: "t", Commands: []*cli.Command{AgyCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "agy", "i", "--project"}))
	text := stripANSI(out.String())
	assert.Contains(t, text, "Antigravity project hooks installed in "+path)
	assert.Contains(t, text, "Gemini API sandbox")

	status, err := model.NewAntigravityProjectHooksService(repo).Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, []string{model.ProjectHookBinary}, status.Binaries)
	_, err = os.Stat(filepath.Join(home, ".gemini", "config", "hooks.json"))
	assert.True(t, os.IsNotExist(err), "the user's hook file is left alone")

	require.NoError(t, app.Run([]string{"t", "agy", "u", "--project"}))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))
}

func TestConfigDisableAgentHooksSendContent(t *testing.T) {
	path := setupConfigSetTest(t, "token: abc\n")
	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "config", "disable", "agentHooks.sendContent"}))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "token: abc\nagentHooks:\n  sendContent: false\n", string(data))
	assert.Contains(t, out.String(), "agentHooks.sendContent is off")

	require.NoError(t, app.Run([]string{"t", "config", "enable", "agentHooks", "sendContent"}))
	cfg, err := configService.ReadConfigFile(t.Context(), model.WithSkipCache())
	require.NoError(t, err)
	assert.True(t, cfg.AgentHooksSendContent())
}

func TestConfigDisableAgentHooksSendContent_TOML(t *testing.T) {
	yamlPath := setupConfigSetTest(t, "token: abc\n")
	dir := filepath.Dir(yamlPath)
	require.NoError(t, os.Remove(yamlPath))
	tomlPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(tomlPath, []byte("Token = \"abc\"\n"), 0600))
	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigCommand}}

	captureColor(t)
	require.NoError(t, app.Run([]string{"t", "config", "disable", "agentHooks.sendContent"}))
	cfg, err := configService.ReadConfigFile(t.Context(), model.WithSkipCache())
	require.NoError(t, err)
	assert.False(t, cfg.AgentHooksSendContent())
}

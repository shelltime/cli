package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/agenthook"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// setupAgentHooksCommand gives the test a temp HOME and runs it in a new repository, whose root
// it returns with the HOME.
func setupAgentHooksCommand(t *testing.T) (home, repo string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	repo = t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0755))
	t.Chdir(repo)
	return home, repo
}

func TestCursorInstallAndUninstallCommands(t *testing.T) {
	home, repo := setupAgentHooksCommand(t)
	sub := filepath.Join(repo, "web")
	require.NoError(t, os.Mkdir(sub, 0755))
	t.Chdir(sub)
	path := filepath.Join(repo, ".cursor", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"hooks":{"stop":[{"command":"say done"}]}}`), 0644))
	// What an older version installed in the user's own file
	userPath := filepath.Join(home, ".cursor", "hooks.json")
	installUserHooksFor(t, userPath, "cursor", []string{"stop"}, "/usr/local/bin/shelltime")
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "cursor", "install"}))
	text := stripANSI(out.String())
	assert.Contains(t, text, "Cursor hooks installed in "+path)
	assert.Contains(t, text, "The ShellTime hooks an older version put in "+userPath+" stay and keep recording your other folders")
	assert.Contains(t, text, "`shelltime cursor uninstall` removes both.")
	assert.Contains(t, text, "Commit it: Cursor Cloud Agents run only the hooks in the repository.")
	assert.Contains(t, text, "Restart Cursor")
	assert.Contains(t, text, "SHELLTIME_CLOUD=true")
	assert.Contains(t, text, "Privacy: the Cursor hooks send your prompts")
	assert.Contains(t, text, "shelltime config disable agentHooks.sendContent")

	status, err := model.NewCursorProjectHooksService(repo).Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Empty(t, status.MissingEvents)
	assert.Equal(t, []string{model.ProjectHookCLI}, status.Binaries)
	userStatus, err := model.NewCursorHooksService().Status()
	require.NoError(t, err)
	assert.True(t, userStatus.Installed, "the older version's hooks still record the other folders")

	require.NoError(t, app.Run([]string{"t", "cursor", "u"}))
	userStatus, err = model.NewCursorHooksService().Status()
	require.NoError(t, err)
	assert.False(t, userStatus.Installed, "uninstall removes them too")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"version":1,"hooks":{"stop":[{"command":"say done"}]}}`, string(data))
}

func TestCursorInstallCommand_RefusesInvalidFile(t *testing.T) {
	_, repo := setupAgentHooksCommand(t)
	path := filepath.Join(repo, ".cursor", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,`), 0644))
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	err := app.Run([]string{"t", "cursor", "i"})
	assert.ErrorIs(t, err, model.ErrHookFileInvalid)
	assert.Contains(t, out.String(), "Failed to install the Cursor hooks")
	assert.NotContains(t, out.String(), "Privacy")
}

func TestCursorInstallCommand_RefusesWindows(t *testing.T) {
	_, repo := setupAgentHooksCommand(t)
	orig := agentHooksGOOS
	agentHooksGOOS = "windows"
	t.Cleanup(func() { agentHooksGOOS = orig })
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	err := app.Run([]string{"t", "cursor", "install"})
	assert.ErrorIs(t, err, model.ErrProjectHooksUnsupported)
	assert.Contains(t, stripANSI(out.String()), "macOS and Linux only")
	assert.NoFileExists(t, filepath.Join(repo, ".cursor", "hooks.json"))
}

func TestCursorInstallCommand_RefusesTheHomeFolder(t *testing.T) {
	home, _ := setupAgentHooksCommand(t)
	t.Chdir(home)
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	err := app.Run([]string{"t", "cursor", "install"})
	assert.ErrorIs(t, err, model.ErrHomeIsNotAProject)
	assert.Contains(t, stripANSI(out.String()), "Failed to find the project")
	assert.NoFileExists(t, filepath.Join(home, ".cursor", "hooks.json"), "Cursor's user-level file is never written")
}

func TestCursorUninstallCommand_OutsideAProjectCleansTheUserFile(t *testing.T) {
	home, _ := setupAgentHooksCommand(t)
	t.Chdir(home)
	userPath := filepath.Join(home, ".cursor", "hooks.json")
	installUserHooksFor(t, userPath, "cursor", agenthook.CursorEvents, "/usr/local/bin/shelltime")
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "cursor", "uninstall"}))
	assert.Contains(t, stripANSI(out.String()), "Cursor hooks removed from "+userPath)
	status, err := model.NewCursorHooksService().Status()
	require.NoError(t, err)
	assert.False(t, status.Installed)
}

func TestCursorUninstallCommand_NothingToRemoveOutsideAProject(t *testing.T) {
	home, _ := setupAgentHooksCommand(t)
	t.Chdir(home)
	app := &cli.App{Name: "t", Commands: []*cli.Command{CursorCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "cursor", "uninstall"}))
	assert.Contains(t, stripANSI(out.String()), "No Cursor hooks to remove here")
}

func TestAgyUninstallCommand_InvalidGlobalFileIsAWarning(t *testing.T) {
	home, repo := setupAgentHooksCommand(t)
	app := &cli.App{Name: "t", Commands: []*cli.Command{AgyCommand}}
	captureColor(t)
	require.NoError(t, app.Run([]string{"t", "agy", "install"}))
	userPath := filepath.Join(home, ".gemini", "config", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0755))
	require.NoError(t, os.WriteFile(userPath, []byte("{\n// mine\n}"), 0644))

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "agy", "uninstall"}), "the project's hooks were removed, which is what was asked")
	text := stripANSI(out.String())
	assert.Contains(t, text, "Antigravity hooks removed from "+filepath.Join(repo, ".agents", "hooks.json"))
	assert.Contains(t, text, "Couldn't read "+userPath)
	assert.NotContains(t, text, "Failed")
}

func TestAgyInstallAndUninstallCommands(t *testing.T) {
	home, _ := setupAgentHooksCommand(t)
	repo := t.TempDir()
	t.Chdir(repo) // not a repository: the folder itself
	path := filepath.Join(repo, ".agents", "hooks.json")
	app := &cli.App{Name: "t", Commands: []*cli.Command{AgyCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "antigravity", "install"}))
	text := stripANSI(out.String())
	assert.Contains(t, text, "Antigravity hooks installed in "+path)
	assert.Contains(t, text, "Gemini API sandbox")
	assert.Contains(t, text, "Restart Antigravity")
	assert.Contains(t, text, "Privacy: the Antigravity hooks send")
	assert.NotContains(t, text, "older version", "there was nothing to remove")

	var root map[string]map[string]json.RawMessage
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &root))
	assert.Len(t, root["shelltime"], 5)
	assert.NoFileExists(t, filepath.Join(home, ".gemini", "config", "hooks.json"), "the global file is left alone")

	require.NoError(t, app.Run([]string{"t", "agy", "uninstall"}))
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))
}

func TestAgyInstallCommand_LeavesAnInvalidGlobalFileAlone(t *testing.T) {
	home, repo := setupAgentHooksCommand(t)
	userPath := filepath.Join(home, ".gemini", "config", "hooks.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0755))
	require.NoError(t, os.WriteFile(userPath, []byte("{\n// mine\n}"), 0644))
	app := &cli.App{Name: "t", Commands: []*cli.Command{AgyCommand}}

	out := captureColor(t)
	require.NoError(t, app.Run([]string{"t", "agy", "install"}))
	assert.NotContains(t, stripANSI(out.String()), "older version")
	status, err := model.NewAntigravityProjectHooksService(repo).Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, "{\n// mine\n}", string(mustReadFile(t, userPath)), "left as it was")
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
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

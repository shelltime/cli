package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malamtime/cli/agenthook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyHookCommand is the command older versions registered in the user-level hook files.
func legacyHookCommand(bin, agent, event string) string {
	return bin + " hook-ingest --agent " + agent + " --event " + event
}

// newTestCursorHooks returns the hook service of a project under a temp HOME.
func newTestCursorHooks(t *testing.T) (*CursorHooksService, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	svc := NewCursorProjectHooksService(root)
	require.Equal(t, filepath.Join(root, ".cursor", "hooks.json"), svc.Path())
	return svc, svc.Path()
}

// cursorHooksFile is the parts of a .cursor/hooks.json the tests read.
type cursorHooksFile struct {
	Version int                         `json:"version"`
	Hooks   map[string][]map[string]any `json:"hooks"`
}

func readCursorHooks(t *testing.T, path string) cursorHooksFile {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var f cursorHooksFile
	require.NoError(t, json.Unmarshal(data, &f), string(data))
	return f
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	require.NoError(t, os.Chmod(path, mode))
}

func assertCursorInstalled(t *testing.T, path string) cursorHooksFile {
	t.Helper()
	f := readCursorHooks(t, path)
	assert.Equal(t, 1, f.Version)
	for _, event := range agenthook.CursorEvents {
		var ours []string
		for _, entry := range f.Hooks[event] {
			if cmd, _ := entry["command"].(string); strings.Contains(cmd, "hook-ingest --agent cursor") {
				ours = append(ours, cmd)
			}
		}
		assert.Equal(t, []string{CursorProjectHookCommand(event)}, ours, event)
	}
	return f
}

func TestCursorHooks_InstallCreatesFile(t *testing.T) {
	svc, path := newTestCursorHooks(t)

	require.NoError(t, svc.Install())

	assertCursorInstalled(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "{\n  \"version\": 1,\n  \"hooks\": {\n    \"beforeSubmitPrompt\""), string(data))
	assert.Contains(t, string(data), `out=$(shelltime hook-ingest --agent cursor --event beforeSubmitPrompt --project)`)
	assert.Contains(t, string(data), `|| echo '{\"continue\":true}'"`)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestCursorHooks_InstallIntoEmptyAndBareFiles(t *testing.T) {
	for _, content := range []string{"", "  \n", "{}", "null"} {
		t.Run(content, func(t *testing.T) {
			svc, path := newTestCursorHooks(t)
			writeFile(t, path, content, 0644)
			require.NoError(t, svc.Install())
			assertCursorInstalled(t, path)
		})
	}
}

func TestCursorHooks_InstallKeepsUserHooksAndOrder(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	writeFile(t, path, `{
  "hooks": {
    "beforeShellExecution": [{"command": "./audit.sh"}],
    "stop": [{"command": "say done", "timeout": 5}],
    "afterFileEdit": [{"command": "prettier --write"}]
  },
  "version": 1,
  "x-mine": {"keep": true}
}
`, 0600)

	require.NoError(t, svc.Install())

	f := assertCursorInstalled(t, path)
	assert.Equal(t, []map[string]any{{"command": "./audit.sh"}}, f.Hooks["beforeShellExecution"])
	assert.Equal(t, []map[string]any{{"command": "prettier --write"}}, f.Hooks["afterFileEdit"])
	require.Len(t, f.Hooks["stop"], 2)
	assert.Equal(t, map[string]any{"command": "say done", "timeout": float64(5)}, f.Hooks["stop"][0], "the user's hook stays first")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Less(t, strings.Index(text, `"hooks"`), strings.Index(text, `"version"`), "root keys keep their order")
	assert.Less(t, strings.Index(text, `"version"`), strings.Index(text, `"x-mine"`))
	assert.Less(t, strings.Index(text, `"beforeShellExecution"`), strings.Index(text, `"stop"`), "event keys keep their order")
	assert.Less(t, strings.Index(text, `"stop"`), strings.Index(text, `"afterFileEdit"`))
	assert.Less(t, strings.Index(text, `"afterFileEdit"`), strings.Index(text, `"beforeSubmitPrompt"`), "new events come after the user's")
	assert.Equal(t, 1, strings.Count(text, `"version"`))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "the file keeps its mode")
}

func TestCursorHooks_InstallIsIdempotentAndReplacesOlderCommands(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	writeFile(t, path, `{"version":1,"hooks":{"stop":[{"command":"say done"}]}}`, 0644)

	require.NoError(t, svc.Install())
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, svc.Install())
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))

	// An older form of the command is replaced in place, keeping keys the user added to it
	writeFile(t, path, `{"version":1,"hooks":{"stop":[{"command":"say done"},{"command":"/usr/local/bin/shelltime hook-ingest --agent cursor --event stop","timeout":3}]}}`, 0644)
	require.NoError(t, svc.Install())

	f := assertCursorInstalled(t, path)
	require.Len(t, f.Hooks["stop"], 2)
	assert.Equal(t, "say done", f.Hooks["stop"][0]["command"])
	assert.Equal(t, float64(3), f.Hooks["stop"][1]["timeout"])
	assert.NotContains(t, string(mustRead(t, path)), "/usr/local/bin/shelltime")
}

func TestCursorHooks_InstallDropsDuplicatesAndRetiredEvents(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	writeFile(t, path, `{"version":1,"hooks":{
		"stop":[{"command":"/old/shelltime hook-ingest --agent cursor --event stop"},{"command":"mine"},{"command":"/old/shelltime hook-ingest --agent cursor --event stop"}],
		"beforeReadFile":[{"command":"/old/shelltime hook-ingest --agent cursor --event beforeReadFile"}],
		"afterFileEdit":[{"command":"mine too"},{"command":"/old/shelltime hook-ingest --agent cursor --event afterFileEdit"}]
	}}`, 0644)

	require.NoError(t, svc.Install())

	f := assertCursorInstalled(t, path)
	assert.Equal(t, []map[string]any{{"command": CursorProjectHookCommand("stop")}, {"command": "mine"}}, f.Hooks["stop"])
	assert.NotContains(t, f.Hooks, "beforeReadFile", "an event only we used goes")
	assert.Equal(t, []map[string]any{{"command": "mine too"}}, f.Hooks["afterFileEdit"])
}

func TestCursorHooks_Uninstall(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	writeFile(t, path, `{"version":1,"hooks":{"stop":[{"command":"say done"}],"afterFileEdit":[{"command":"fmt"}]},"other":true}`, 0644)
	require.NoError(t, svc.Install())

	require.NoError(t, svc.Uninstall())

	f := readCursorHooks(t, path)
	assert.Equal(t, 1, f.Version, "version stays")
	assert.Equal(t, map[string][]map[string]any{
		"stop":          {{"command": "say done"}},
		"afterFileEdit": {{"command": "fmt"}},
	}, f.Hooks, "only arrays we emptied go")
	assert.Contains(t, string(mustRead(t, path)), `"other": true`)

	// Nothing of ours left: the file isn't touched
	before := mustRead(t, path)
	require.NoError(t, svc.Uninstall())
	assert.Equal(t, before, mustRead(t, path))
}

func TestCursorHooks_UninstallKeepsUserEmptyArrays(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	writeFile(t, path, `{"version":1,"hooks":{"stop":[],"sessionEnd":[{"command":"/x/shelltime hook-ingest --agent cursor --event sessionEnd"}]}}`, 0644)

	require.NoError(t, svc.Uninstall())

	f := readCursorHooks(t, path)
	assert.Contains(t, f.Hooks, "stop", "an array the user left empty stays")
	assert.NotContains(t, f.Hooks, "sessionEnd")
}

func TestCursorHooks_UninstallMissingFile(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	require.NoError(t, svc.Uninstall())
	assert.NoFileExists(t, path)
	assert.NoDirExists(t, filepath.Dir(path))
}

func TestCursorHooks_RefusesInvalidFiles(t *testing.T) {
	for name, content := range map[string]string{
		"invalid JSON":   `{"version":1,`,
		"BOM":            "\xEF\xBB\xBF{\"version\":1}",
		"comment":        "{\n// mine\n\"version\":1}",
		"trailing comma": `{"version":1,}`,
		"not an object":  `[1]`,
		"hooks array":    `{"hooks":[]}`,
		"event object":   `{"hooks":{"stop":{"command":"x"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc, path := newTestCursorHooks(t)
			writeFile(t, path, content, 0644)

			err := svc.Install()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrHookFileInvalid)
			assert.Equal(t, content, string(mustRead(t, path)), "left untouched")

			if name != "event object" {
				assert.ErrorIs(t, svc.Uninstall(), ErrHookFileInvalid)
				_, err = svc.Status()
				assert.ErrorIs(t, err, ErrHookFileInvalid)
			}
			assert.Equal(t, content, string(mustRead(t, path)), "left untouched")
		})
	}
}

func TestCursorHooks_Status(t *testing.T) {
	svc, path := newTestCursorHooks(t)

	status, err := svc.Status()
	require.NoError(t, err)
	assert.False(t, status.Installed)
	assert.Equal(t, agenthook.CursorEvents, status.MissingEvents)

	require.NoError(t, svc.Install())
	status, err = svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Empty(t, status.MissingEvents)
	assert.Equal(t, []string{ProjectHookCLI}, status.Binaries)

	// An install from an older version, without the session events
	writeFile(t, path, `{"version":1,"hooks":{
		"beforeSubmitPrompt":[{"command":"/a/shelltime hook-ingest --agent cursor --event beforeSubmitPrompt"}],
		"stop":[{"command":"/b/shelltime hook-ingest --agent cursor --event stop"}]}}`, 0644)
	status, err = svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, []string{"postToolUse", "postToolUseFailure", "afterAgentResponse", "sessionStart", "sessionEnd"}, status.MissingEvents)
	assert.Equal(t, []string{"/a/shelltime", "/b/shelltime"}, status.Binaries)
}

func TestHookCommandBinary(t *testing.T) {
	assert.Equal(t, "/usr/bin/shelltime", hookCommandBinary("/usr/bin/shelltime hook-ingest --agent cursor --event stop"))
	assert.Equal(t, "/a b/shelltime", hookCommandBinary(`'/a b/shelltime' hook-ingest --agent agy --event Stop`))
	assert.Equal(t, "/a b/shelltime", hookCommandBinary(`"/a b/shelltime" hook-ingest --agent agy --event Stop`))
	assert.Equal(t, "", hookCommandBinary("say done"))
	assert.Equal(t, ProjectHookCLI, hookCommandBinary(CursorProjectHookCommand("stop")))
	assert.Equal(t, ProjectHookCLI, hookCommandBinary(AntigravityProjectHookCommand("Stop")))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestCursorHooks_WritesThroughASymlink(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	real := filepath.Join(t.TempDir(), "dotfiles", "cursor-hooks.json")
	writeFile(t, real, `{"version":1,"hooks":{}}`, 0644)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.Symlink(real, path))

	require.NoError(t, svc.Install())

	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link stays a link")
	assertCursorInstalled(t, real)
}

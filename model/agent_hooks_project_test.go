package model

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malamtime/cli/agenthook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectHooksRoot(t *testing.T) {
	t.Run("the folder that holds .git", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0755))
		sub := filepath.Join(repo, "a", "b")
		require.NoError(t, os.MkdirAll(sub, 0755))

		root, err := ProjectHooksRoot(sub)
		require.NoError(t, err)
		assert.Equal(t, repo, root)

		root, err = ProjectHooksRoot(repo)
		require.NoError(t, err)
		assert.Equal(t, repo, root)
	})

	t.Run("a worktree's .git file", func(t *testing.T) {
		worktree := t.TempDir()
		writeFile(t, filepath.Join(worktree, ".git"), "gitdir: /elsewhere/.git/worktrees/x\n", 0644)
		sub := filepath.Join(worktree, "pkg")
		require.NoError(t, os.Mkdir(sub, 0755))

		root, err := ProjectHooksRoot(sub)
		require.NoError(t, err)
		assert.Equal(t, worktree, root)
	})

	t.Run("outside a repository, the folder itself", func(t *testing.T) {
		dir := t.TempDir()
		root, err := ProjectHooksRoot(dir)
		require.NoError(t, err)
		assert.Equal(t, dir, root)
	})

	t.Run("a relative folder comes back absolute", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		root, err := ProjectHooksRoot(".")
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(root), root)
	})

	t.Run("the home folder is no project", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		sub := filepath.Join(home, "notes")
		require.NoError(t, os.Mkdir(sub, 0755))

		_, err := ProjectHooksRoot(home)
		assert.ErrorIs(t, err, ErrHomeIsNotAProject, "its .cursor/hooks.json is Cursor's user-level file")

		root, err := ProjectHooksRoot(sub)
		require.NoError(t, err)
		assert.Equal(t, sub, root, "a folder in it, outside a repository, is one")

		require.NoError(t, os.Mkdir(filepath.Join(home, ".git"), 0755)) // dotfiles kept in git
		root, err = ProjectHooksRoot(sub)
		require.NoError(t, err)
		assert.Equal(t, sub, root, "a dotfiles repository in home doesn't swallow the folders in it")
		_, err = ProjectHooksRoot(home)
		assert.ErrorIs(t, err, ErrHomeIsNotAProject)

		repo := filepath.Join(home, "code", "app")
		require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0755))
		require.NoError(t, os.Mkdir(filepath.Join(repo, "web"), 0755))
		root, err = ProjectHooksRoot(filepath.Join(repo, "web"))
		require.NoError(t, err)
		assert.Equal(t, repo, root, "a repository under home is found as before")
	})
}

func TestProjectHookCommand(t *testing.T) {
	assert.Equal(t,
		`PATH=$PATH:$HOME/.shelltime/bin:/opt/homebrew/bin:/usr/local/bin; command -v shelltime >/dev/null 2>&1 && command -v shelltime-daemon >/dev/null 2>&1 && out=$(shelltime hook-ingest --agent cursor --event beforeSubmitPrompt --project) && printf '%s\n' "$out" || echo '{"continue":true}'`,
		CursorProjectHookCommand("beforeSubmitPrompt"))
	assert.True(t, strings.HasSuffix(CursorProjectHookCommand("stop"), `--event stop --project) && printf '%s\n' "$out" || echo '{}'`))
	assert.True(t, strings.HasSuffix(AntigravityProjectHookCommand("Stop"), `--agent agy --event Stop --project) && printf '%s\n' "$out" || echo '{"decision":"allow"}'`))
	assert.True(t, strings.HasSuffix(AntigravityProjectHookCommand("PostToolUse"), `--event PostToolUse --project) && printf '%s\n' "$out" || echo '{}'`))
}

// runProjectHookCommand runs a project hook command in sh the way the agents do, with PATH and
// HOME as given and payload on stdin, and returns its stdout.
func runProjectHookCommand(t *testing.T, command, path, home, payload string) string {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = []string{"PATH=" + path, "HOME=" + home}
	cmd.Stdin = strings.NewReader(payload)
	cmd.Dir = t.TempDir() // the agents disagree on the folder a hook runs in
	out, err := cmd.Output()
	require.NoError(t, err, "the hook never fails")
	return string(out)
}

func TestProjectHookCommand_RunsHookIngestOnlyWithTheCLIAndDaemon(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	sysPath := "/usr/bin:/bin"
	home := t.TempDir()
	fake := func(dir, name, script string) {
		writeFile(t, filepath.Join(dir, name), "#!/bin/sh\n"+script, 0755)
	}
	// A fake shelltime that shows how it was run
	cli := `echo "ran $*"; cat >/dev/null` + "\n"

	command := CursorProjectHookCommand("beforeSubmitPrompt")
	assert.Equal(t, "{\"continue\":true}\n", runProjectHookCommand(t, command, sysPath, home, "{}"), "nothing installed: the agent carries on")

	bin := t.TempDir()
	fake(bin, "shelltime", cli)
	assert.Equal(t, "{\"continue\":true}\n", runProjectHookCommand(t, command, bin+":"+sysPath, home, "{}"), "no daemon: nothing is recorded")

	fake(bin, "shelltime-daemon", "")
	assert.Equal(t, "ran hook-ingest --agent cursor --event beforeSubmitPrompt --project\n", runProjectHookCommand(t, command, bin+":"+sysPath, home, "{}"))

	// A shelltime from before hook-ingest: an error on stderr and a non-zero exit, so only the
	// reply reaches the agent
	old := t.TempDir()
	fake(old, "shelltime", `echo "No help topic for 'hook-ingest'" >&2; exit 3`+"\n")
	fake(old, "shelltime-daemon", "")
	assert.Equal(t, "{\"continue\":true}\n", runProjectHookCommand(t, command, old+":"+sysPath, home, "{}"))

	// Not on PATH, but where the install script puts them
	shelltimeBin := filepath.Join(home, ".shelltime", "bin")
	fake(shelltimeBin, "shelltime", cli)
	fake(shelltimeBin, "shelltime-daemon", "")
	assert.Equal(t, "ran hook-ingest --agent agy --event Stop --project\n", runProjectHookCommand(t, AntigravityProjectHookCommand("Stop"), sysPath, home, "{}"))
}

func TestCursorProjectHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	svc := NewCursorProjectHooksService(repo)
	path := filepath.Join(repo, ".cursor", "hooks.json")
	require.Equal(t, path, svc.Path())
	writeFile(t, path, `{"version":1,"hooks":{"afterFileEdit":[{"command":".cursor/hooks/format.sh"}]}}`, 0644)

	require.NoError(t, svc.Install())

	f := readCursorHooks(t, path)
	for _, event := range agenthook.CursorEvents {
		require.Len(t, f.Hooks[event], 1, event)
		assert.Equal(t, CursorProjectHookCommand(event), f.Hooks[event][0]["command"], event)
	}
	assert.Equal(t, ".cursor/hooks/format.sh", f.Hooks["afterFileEdit"][0]["command"], "the project's own hooks stay")
	_, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".cursor", "hooks.json"))
	assert.True(t, os.IsNotExist(err), "the user's hook file is left alone")

	status, err := svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Empty(t, status.MissingEvents)
	assert.Equal(t, []string{ProjectHookCLI}, status.Binaries)

	// Installing again changes nothing
	before := mustRead(t, path)
	require.NoError(t, svc.Install())
	assert.Equal(t, before, mustRead(t, path))

	require.NoError(t, svc.Uninstall())
	assert.JSONEq(t, `{"version":1,"hooks":{"afterFileEdit":[{"command":".cursor/hooks/format.sh"}]}}`, string(mustRead(t, path)))
}

func TestProjectHookSearchDirs(t *testing.T) {
	assert.Equal(t, []string{"/home/me/.shelltime/bin", "/opt/homebrew/bin", "/usr/local/bin"}, ProjectHookSearchDirs("/home/me"))
}

func TestStatus_EventBinaries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cursor := NewCursorHooksService()
	writeFile(t, cursor.Path(), `{"version":1,"hooks":{
		"beforeSubmitPrompt":[{"command":"`+legacyHookCommand("/a/shelltime", "cursor", "beforeSubmitPrompt")+`"}],
		"stop":[{"command":"`+legacyHookCommand("/b/shelltime", "cursor", "stop")+`"},{"command":"say done"}]}}`, 0644)
	status, err := cursor.Status()
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"beforeSubmitPrompt": {"/a/shelltime"}, "stop": {"/b/shelltime"}}, status.EventBinaries)
	assert.Equal(t, []string{"/a/shelltime", "/b/shelltime"}, status.Binaries)

	agy := NewAntigravityHooksService()
	writeFile(t, agy.Path(), `{"shelltime":{"Stop":[{"type":"command","command":"`+legacyHookCommand("/c/shelltime", "agy", "Stop")+`","timeout":15}]}}`, 0644)
	status, err = agy.Status()
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"Stop": {"/c/shelltime"}}, status.EventBinaries)
}

func TestUserHooksAreOnlyCleaned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cursor := NewCursorHooksService()
	assert.Equal(t, filepath.Join(home, ".cursor", "hooks.json"), cursor.Path())
	assert.ErrorIs(t, cursor.Install(), ErrUserHooksRetired)
	assert.NoFileExists(t, cursor.Path())

	agy := NewAntigravityHooksService()
	assert.Equal(t, filepath.Join(home, ".gemini", "config", "hooks.json"), agy.Path())
	assert.ErrorIs(t, agy.Install(), ErrUserHooksRetired)
	assert.NoFileExists(t, agy.Path())

	// What an older version left there is still read and removed
	writeFile(t, cursor.Path(), `{"version":1,"hooks":{"stop":[{"command":"say done"},{"command":"`+legacyHookCommand("/usr/local/bin/shelltime", "cursor", "stop")+`"}]}}`, 0644)
	status, err := cursor.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, []string{"/usr/local/bin/shelltime"}, status.Binaries)
	require.NoError(t, cursor.Uninstall())
	assert.JSONEq(t, `{"version":1,"hooks":{"stop":[{"command":"say done"}]}}`, string(mustRead(t, cursor.Path())))
}

func TestAntigravityProjectHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	svc := NewAntigravityProjectHooksService(repo)
	path := filepath.Join(repo, ".agents", "hooks.json")
	require.Equal(t, path, svc.Path())

	require.NoError(t, svc.Install())
	assertAntigravityGroup(t, path)

	status, err := svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, []string{ProjectHookCLI}, status.Binaries)

	require.NoError(t, svc.Uninstall())
	assert.JSONEq(t, `{}`, string(mustRead(t, path)))
}

package model

import (
	"os"
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
}

func TestCursorProjectHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	svc := NewCursorProjectHooksService(repo)
	path := filepath.Join(repo, ".cursor", "hooks.json")
	require.Equal(t, path, svc.Path())
	writeFile(t, path, `{"version":1,"hooks":{"afterFileEdit":[{"command":".cursor/hooks/format.sh"}]}}`, 0644)

	require.NoError(t, svc.Install(ProjectHookBinary))

	f := readCursorHooks(t, path)
	for _, event := range agenthook.CursorEvents {
		require.Len(t, f.Hooks[event], 1, event)
		assert.Equal(t, "shelltime hook-ingest --agent cursor --event "+event+" --project", f.Hooks[event][0]["command"], event)
	}
	assert.Equal(t, ".cursor/hooks/format.sh", f.Hooks["afterFileEdit"][0]["command"], "the project's own hooks stay")
	_, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".cursor", "hooks.json"))
	assert.True(t, os.IsNotExist(err), "the user's hook file is left alone")

	status, err := svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Empty(t, status.MissingEvents)
	assert.Equal(t, []string{"shelltime"}, status.Binaries)

	// Installing again changes nothing
	before := mustRead(t, path)
	require.NoError(t, svc.Install(ProjectHookBinary))
	assert.Equal(t, before, mustRead(t, path))

	require.NoError(t, svc.Uninstall())
	assert.JSONEq(t, `{"version":1,"hooks":{"afterFileEdit":[{"command":".cursor/hooks/format.sh"}]}}`, string(mustRead(t, path)))
}

func TestCursorUserHooksHaveNoProjectFlag(t *testing.T) {
	svc, path := newTestCursorHooks(t)
	require.NoError(t, svc.Install(testHookBin))
	assert.NotContains(t, string(mustRead(t, path)), ProjectHookFlag)
}

func TestAntigravityProjectHooks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	svc := NewAntigravityProjectHooksService(repo)
	path := filepath.Join(repo, ".agents", "hooks.json")
	require.Equal(t, path, svc.Path())

	require.NoError(t, svc.Install(ProjectHookBinary))

	group := readAntigravityGroup(t, path)
	assert.Len(t, group, len(agenthook.AntigravityEvents))
	for _, event := range []string{"SessionStart", "PreInvocation", "PostInvocation", "Stop"} {
		require.Len(t, group[event], 1, event)
		assert.Equal(t, "shelltime hook-ingest --agent agy --event "+event+" --project", group[event][0]["command"], event)
	}
	tool := group["PostToolUse"][0]["hooks"].([]any)[0].(map[string]any)
	assert.Equal(t, "shelltime hook-ingest --agent agy --event PostToolUse --project", tool["command"])

	status, err := svc.Status()
	require.NoError(t, err)
	assert.True(t, status.Installed)
	assert.Equal(t, []string{"shelltime"}, status.Binaries)

	require.NoError(t, svc.Uninstall())
	assert.JSONEq(t, `{}`, string(mustRead(t, path)))

	user, userPath := newTestAntigravityHooks(t)
	require.NoError(t, user.Install(testHookBin))
	assert.False(t, strings.Contains(string(mustRead(t, userPath)), ProjectHookFlag), "the user's hooks run without the flag")
}

package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookTestHome points HOME at a temp dir and pre-seeds bash-preexec.sh, so installing the
// bash hook never downloads it.
func hookTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	hooksDir := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash-preexec.sh"), []byte("# preexec"), 0644))
	return home
}

func statuses(results []HookInstallResult) map[string]HookInstallStatus {
	m := map[string]HookInstallStatus{}
	for _, r := range results {
		m[r.Shell] = r.Status
	}
	return m
}

func TestInstallShellHooks_OnlyShellsTheUserHas(t *testing.T) {
	home := hookTestHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# mine\n"), 0644))

	results := InstallShellHooks(ShellHookServices(), "/bin/bash")

	assert.Equal(t, map[string]HookInstallStatus{
		"zsh":  HookSkipped,
		"fish": HookSkipped,
		"bash": HookInstalled,
	}, statuses(results))
	for _, r := range results {
		assert.NoError(t, r.Err, r.Shell)
	}
	// Skipped shells get no config file.
	assert.NoFileExists(t, filepath.Join(home, ".zshrc"))
	assert.NoDirExists(t, filepath.Join(home, ".config", "fish"))
}

func TestInstallShellHooks_ConfigFileWithoutLoginShell(t *testing.T) {
	home := hookTestHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# mine\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# mine\n"), 0644))

	results := InstallShellHooks(ShellHookServices(), "/bin/zsh")

	assert.Equal(t, map[string]HookInstallStatus{
		"zsh":  HookInstalled,
		"fish": HookSkipped,
		"bash": HookInstalled,
	}, statuses(results))
}

func TestInstallShellHooks_CreatesTheLoginShellsConfig(t *testing.T) {
	home := hookTestHome(t)

	results := InstallShellHooks(ShellHookServices(), "/usr/local/bin/fish")

	assert.Equal(t, map[string]HookInstallStatus{
		"zsh":  HookSkipped,
		"fish": HookInstalled,
		"bash": HookSkipped,
	}, statuses(results))
	assert.FileExists(t, filepath.Join(home, ".config", "fish", "config.fish"))
	assert.NoFileExists(t, filepath.Join(home, ".bashrc"))
}

func TestInstallShellHooks_NoShellFound(t *testing.T) {
	hookTestHome(t)

	for _, loginShell := range []string{"", "/bin/sh"} {
		results := InstallShellHooks(ShellHookServices(), loginShell)
		require.Len(t, results, 3)
		for _, r := range results {
			assert.Equal(t, HookSkipped, r.Status, "%s with $SHELL=%q", r.Shell, loginShell)
			assert.NotEmpty(t, r.ConfigPath)
		}
	}
}

func TestInstallShellHooks_AlreadyInstalledLeavesNoBackup(t *testing.T) {
	home := hookTestHome(t)
	rc := filepath.Join(home, ".bashrc")
	require.NoError(t, os.WriteFile(rc, []byte("# mine\n"), 0644))

	first := InstallShellHooks(ShellHookServices(), "/bin/bash")
	require.Equal(t, HookInstalled, statuses(first)["bash"])
	backups, _ := filepath.Glob(rc + ".bak.*")
	require.Len(t, backups, 1, "the first install backs up the file it changes")

	second := InstallShellHooks(ShellHookServices(), "/bin/bash")
	assert.Equal(t, HookAlreadyInstalled, statuses(second)["bash"])
	backups, _ = filepath.Glob(rc + ".bak.*")
	assert.Len(t, backups, 1, "an install that changes nothing writes no backup")
	assert.Equal(t, 1, shCount(t, rc, "# Added by shelltime CLI"))
}

func TestInstallShellHooks_ReportsFailures(t *testing.T) {
	home := hookTestHome(t)
	// A directory where ~/.zshrc should be: it exists, so zsh is tried, and fails.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".zshrc"), 0755))

	results := InstallShellHooks(ShellHookServices(), "/bin/zsh")

	var zsh HookInstallResult
	for _, r := range results {
		if r.Shell == "zsh" {
			zsh = r
		}
	}
	assert.Equal(t, HookFailed, zsh.Status)
	assert.Error(t, zsh.Err)
	assert.Equal(t, filepath.Join(home, ".zshrc"), zsh.ConfigPath)
}

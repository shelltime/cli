package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shCount returns how many times sub appears in the file at path.
func shCount(t *testing.T, path, sub string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Count(string(b), sub)
}

// shHooksDir returns $HOME/.shelltime/hooks for the current (sandboxed) HOME.
func shHooksDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(os.Getenv("HOME"), COMMAND_BASE_STORAGE_FOLDER, "hooks")
}

func TestBashHookService_Install_Lifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Pre-create the hooks dir + bash-preexec.sh so Install does NOT hit the
	// network (ensureBashPreexec returns early when the file already exists).
	hooksDir := shHooksDir(t)
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash-preexec.sh"), []byte("# preexec"), 0644))

	svc := NewBashHookService()

	// No .bashrc yet -> Install auto-creates it and adds the hook lines.
	require.NoError(t, svc.Install())

	rc := filepath.Join(home, ".bashrc")
	assert.FileExists(t, rc)
	assert.NoError(t, svc.Check(), "Check should pass after Install")
	assert.Equal(t, 1, shCount(t, rc, "# Added by shelltime CLI"))
	// The embedded hook file should have been materialised.
	assert.FileExists(t, filepath.Join(hooksDir, "bash.bash"))

	// Idempotent: a second Install detects the existing hook and does not
	// duplicate the lines.
	require.NoError(t, svc.Install())
	assert.Equal(t, 1, shCount(t, rc, "# Added by shelltime CLI"))

	// Uninstall removes the hook lines; Check then fails.
	require.NoError(t, svc.Uninstall())
	assert.Equal(t, 0, shCount(t, rc, "# Added by shelltime CLI"))
	assert.Error(t, svc.Check())
}

func TestBashHookService_Install_PreservesExistingContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hooksDir := shHooksDir(t)
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash-preexec.sh"), []byte("# preexec"), 0644))

	rc := filepath.Join(home, ".bashrc")
	require.NoError(t, os.WriteFile(rc, []byte("export EXISTING=1\n"), 0644))

	svc := NewBashHookService()
	require.NoError(t, svc.Install())

	b, err := os.ReadFile(rc)
	require.NoError(t, err)
	content := string(b)
	assert.Contains(t, content, "export EXISTING=1", "pre-existing content must be preserved")
	assert.Contains(t, content, "# Added by shelltime CLI")
	// A backup of the original should have been written alongside it.
	matches, _ := filepath.Glob(rc + ".bak.*")
	assert.NotEmpty(t, matches, "backup file should be created")
}

func TestZshHookService_Install_Lifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewZshHookService()

	// zsh Install requires the rc file to already exist.
	err := svc.Install()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	rc := filepath.Join(home, ".zshrc")
	require.NoError(t, os.WriteFile(rc, []byte("# my zsh\n"), 0644))

	require.NoError(t, svc.Install())
	assert.NoError(t, svc.Check())
	assert.Equal(t, 1, shCount(t, rc, "# Added by shelltime CLI"))
	assert.FileExists(t, filepath.Join(shHooksDir(t), "zsh.zsh"))

	// Idempotent.
	require.NoError(t, svc.Install())
	assert.Equal(t, 1, shCount(t, rc, "# Added by shelltime CLI"))

	require.NoError(t, svc.Uninstall())
	assert.Error(t, svc.Check())
}

func TestFishHookService_Install_Lifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewFishHookService()

	// fish Install requires the config file to already exist.
	err := svc.Install()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	rc := filepath.Join(home, ".config", "fish", "config.fish")
	require.NoError(t, os.MkdirAll(filepath.Dir(rc), 0755))
	require.NoError(t, os.WriteFile(rc, []byte("# my fish\n"), 0644))

	require.NoError(t, svc.Install())
	assert.NoError(t, svc.Check())
	assert.Equal(t, 1, shCount(t, rc, "# Added by shelltime CLI"))
	assert.FileExists(t, filepath.Join(shHooksDir(t), "fish.fish"))

	// Idempotent.
	require.NoError(t, svc.Install())
	assert.Equal(t, 1, shCount(t, rc, "# Added by shelltime CLI"))

	require.NoError(t, svc.Uninstall())
	assert.Error(t, svc.Check())
}

func TestEnsureHookFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("writes when missing", func(t *testing.T) {
		p := filepath.Join(dir, "nested", "hook.sh")
		require.NoError(t, ensureHookFile(p, []byte("HOOK")))
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.Equal(t, "HOOK", string(b))
	})

	t.Run("no-op when present", func(t *testing.T) {
		p := filepath.Join(dir, "exists.sh")
		require.NoError(t, os.WriteFile(p, []byte("ORIGINAL"), 0644))
		require.NoError(t, ensureHookFile(p, []byte("REPLACEMENT")))
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.Equal(t, "ORIGINAL", string(b), "existing file must not be overwritten")
	})
}

func TestEnsureBashPreexec_AlreadyPresent(t *testing.T) {
	// When bash-preexec.sh already exists, ensureBashPreexec returns nil without
	// performing any network access.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bash-preexec.sh"), []byte("x"), 0644))
	assert.NoError(t, ensureBashPreexec(dir))
}

package model

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// m3baseApp builds a BaseApp under a fresh temp HOME so tilde-expansion and any
// absolute temp paths are isolated per test.
func m3baseApp(t *testing.T) *BaseApp {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return &BaseApp{name: "m3app"}
}

// TestBaseApp_Save_DryRunNewFile covers the dry-run branch for a file that does
// not yet exist: it prints but must NOT create the file.
func TestBaseApp_Save_DryRunNewFile(t *testing.T) {
	app := m3baseApp(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "new.conf")

	err := app.Save(context.Background(), map[string]string{target: "hello\n"}, true)
	require.NoError(t, err)

	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr), "dry-run must not write a new file")
}

// TestBaseApp_Save_DryRunExistingDiff covers the dry-run branch where the file
// exists with different content: the diff is computed/printed but the file is
// left unmodified.
func TestBaseApp_Save_DryRunExistingDiff(t *testing.T) {
	app := m3baseApp(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "existing.conf")
	require.NoError(t, os.WriteFile(target, []byte("original\n"), 0o644))

	err := app.Save(context.Background(), map[string]string{target: "original\nadded line\n"}, true)
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "original\n", string(got), "dry-run must not modify an existing file")
}

// TestBaseApp_Save_DiffMergeExisting covers the real (non-dry-run) diff-merge
// path: the new content's additions are merged into the existing file.
func TestBaseApp_Save_DiffMergeExisting(t *testing.T) {
	app := m3baseApp(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "merge.conf")
	require.NoError(t, os.WriteFile(target, []byte("keep me\n"), 0o644))

	err := app.Save(context.Background(), map[string]string{target: "keep me\nbrand new line\n"}, false)
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	// The merge preserves the original and appends additions.
	assert.Contains(t, string(got), "keep me")
	assert.Contains(t, string(got), "brand new line")
}

// TestBaseApp_Save_ExpandPathErrorSkipped feeds a path that can't be expanded
// (HOME unset + tilde) so expandPath errors and the entry is skipped without
// failing the whole Save.
func TestBaseApp_Save_ExpandPathErrorSkipped(t *testing.T) {
	app := &BaseApp{name: "m3app"}
	// Unset HOME so os.UserHomeDir() fails for a "~"-prefixed path.
	t.Setenv("HOME", "")
	// On linux os.UserHomeDir reads $HOME; empty -> error.
	dir := t.TempDir()
	good := filepath.Join(dir, "good.conf")

	err := app.Save(context.Background(), map[string]string{
		"~/cannot-expand": "x\n",
		good:              "written\n",
	}, false)
	require.NoError(t, err)

	got, err := os.ReadFile(good)
	require.NoError(t, err)
	assert.Equal(t, "written\n", string(got), "the expandable path is still written")
}

// TestBaseApp_Backup_DryRun covers the dry-run branch: an existing file is NOT
// copied to a .backup file.
func TestBaseApp_Backup_DryRun(t *testing.T) {
	app := m3baseApp(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "file.conf")
	require.NoError(t, os.WriteFile(f, []byte("data\n"), 0o644))

	require.NoError(t, app.Backup(context.Background(), []string{f}, true))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.Contains(e.Name(), ".backup."), "dry-run must not create a backup file")
	}
}

// TestBaseApp_IsEqual_ExpandError covers IsEqual's expandPath-error branch: a
// tilde path with HOME unset can't expand, so it is recorded as not-equal.
func TestBaseApp_IsEqual_ExpandError(t *testing.T) {
	t.Setenv("HOME", "")
	app := &BaseApp{name: "m3app"}
	res, err := app.IsEqual(context.Background(), map[string]string{"~/cannot": "x"})
	require.NoError(t, err)
	assert.False(t, res["~/cannot"])
}

// TestBaseApp_Backup_ExpandError covers Backup's expandPath-error branch (tilde
// path, HOME unset) which logs a warning and skips without failing.
func TestBaseApp_Backup_ExpandError(t *testing.T) {
	t.Setenv("HOME", "")
	app := &BaseApp{name: "m3app"}
	require.NoError(t, app.Backup(context.Background(), []string{"~/cannot"}, false))
}

// TestBaseApp_CollectFromPaths_ExpandErrorSkipped covers CollectFromPaths'
// expandPath-error continue branch (tilde path, HOME unset) alongside a valid
// absolute path that is still collected.
func TestBaseApp_CollectFromPaths_ExpandErrorSkipped(t *testing.T) {
	t.Setenv("HOME", "")
	app := &BaseApp{name: "m3app"}
	dir := t.TempDir()
	good := filepath.Join(dir, "ok.conf")
	require.NoError(t, os.WriteFile(good, []byte("data\n"), 0o644))

	skip := true
	items, err := app.CollectFromPaths(context.Background(), "m3app", []string{"~/cannot", good}, &skip)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, good, items[0].Path)
}

// TestBaseApp_Backup_MultipleFilesWritesBackups covers the real backup-write path
// for several existing files.
func TestBaseApp_Backup_MultipleFilesWritesBackups(t *testing.T) {
	app := m3baseApp(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.conf")
	b := filepath.Join(dir, "b.conf")
	require.NoError(t, os.WriteFile(a, []byte("AA"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("BB"), 0o644))

	require.NoError(t, app.Backup(context.Background(), []string{a, b}, false))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	backups := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), ".backup.") {
			backups++
		}
	}
	assert.Equal(t, 2, backups, "a backup written per existing file")
}

// TestBaseApp_IsEqual_MultiResult covers IsEqual with a mix of equal, unequal
// and missing files in one call.
func TestBaseApp_IsEqual_MultiResult(t *testing.T) {
	app := m3baseApp(t)
	dir := t.TempDir()
	same := filepath.Join(dir, "same.conf")
	diff := filepath.Join(dir, "diff.conf")
	require.NoError(t, os.WriteFile(same, []byte("identical\n"), 0o644))
	require.NoError(t, os.WriteFile(diff, []byte("local\n"), 0o644))

	res, err := app.IsEqual(context.Background(), map[string]string{
		same:                       "identical\n",
		diff:                       "remote\n",
		filepath.Join(dir, "gone"): "whatever",
	})
	require.NoError(t, err)
	assert.True(t, res[same])
	assert.False(t, res[diff])
	assert.False(t, res[filepath.Join(dir, "gone")])
}

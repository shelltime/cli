package model

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}

// The daemon installer treats "<daemon>.bak" as "a newer binary, restore it".
// The updater must therefore never write its backup to that name, or a swap
// followed by `daemon install` silently rolls back to the old binary.
func TestReplaceBinaryWithBackupSuffix_DoesNotCollideWithInstallerBak(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime-daemon")
	src := filepath.Join(dir, "new-daemon")

	writeExecutable(t, dest, "OLD")
	writeExecutable(t, src, "NEW")

	require.NoError(t, ReplaceBinaryWithBackupSuffix(src, dest, BackupSuffixUpdate))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "NEW", string(got), "destination must hold the new binary")

	backup, err := os.ReadFile(dest + BackupSuffixUpdate)
	require.NoError(t, err)
	assert.Equal(t, "OLD", string(backup), "previous binary must be preserved at .prev")

	_, err = os.Stat(dest + BackupSuffixLegacy)
	assert.True(t, os.IsNotExist(err),
		"must not create a .bak — daemon install would restore it over the new binary")
}

func TestReplaceBinary_LegacySuffixStillHonored(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime")
	src := filepath.Join(dir, "new")

	writeExecutable(t, dest, "OLD")
	writeExecutable(t, src, "NEW")

	require.NoError(t, ReplaceBinary(src, dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "NEW", string(got))

	backup, err := os.ReadFile(dest + BackupSuffixLegacy)
	require.NoError(t, err)
	assert.Equal(t, "OLD", string(backup))
}

// Every shell hook execs `shelltime` by name on every command. If the binary
// vanishes for even an instant during a swap, a hook landing in that window
// prints "command not found".
func TestReplaceBinaryWithBackupSuffix_DestinationNeverDisappears(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime")
	src := filepath.Join(dir, "new")

	writeExecutable(t, dest, "OLD")
	writeExecutable(t, src, "NEW")

	var missing atomic.Bool
	stop := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				missing.Store(true)
				return
			}
		}
	}()

	// Run the swap repeatedly so the watcher gets many chances to observe a gap.
	for i := range 20 {
		writeExecutable(t, src, "NEW")
		require.NoError(t, ReplaceBinaryWithBackupSuffix(src, dest, BackupSuffixUpdate), "iteration %d", i)
	}

	close(stop)
	<-watcherDone

	assert.False(t, missing.Load(), "destination path disappeared during replace")
}

func TestReplaceBinaryWithBackupSuffix_NoExistingDest(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime")
	src := filepath.Join(dir, "new")
	writeExecutable(t, src, "NEW")

	require.NoError(t, ReplaceBinaryWithBackupSuffix(src, dest, BackupSuffixUpdate))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "NEW", string(got))

	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestReplaceBinaryWithBackupSuffix_EmptySuffixDefaults(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime")
	src := filepath.Join(dir, "new")
	writeExecutable(t, dest, "OLD")
	writeExecutable(t, src, "NEW")

	require.NoError(t, ReplaceBinaryWithBackupSuffix(src, dest, ""))

	backup, err := os.ReadFile(dest + BackupSuffixUpdate)
	require.NoError(t, err)
	assert.Equal(t, "OLD", string(backup))
}

func TestRestoreBinaryBackup(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime")
	src := filepath.Join(dir, "new")
	writeExecutable(t, dest, "OLD")
	writeExecutable(t, src, "NEW")

	require.NoError(t, ReplaceBinaryWithBackupSuffix(src, dest, BackupSuffixUpdate))
	require.NoError(t, RestoreBinaryBackup(dest, BackupSuffixUpdate))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "OLD", string(got), "rollback must restore the previous binary")
}

func TestRestoreBinaryBackup_MissingBackup(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "shelltime")
	writeExecutable(t, dest, "CURRENT")

	err := RestoreBinaryBackup(dest, BackupSuffixUpdate)
	assert.Error(t, err)
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.2.3", "v1.2.3", 0},
		{"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.2.4", -1},
		{"1.3.0", "1.2.99", 1},
		{"2.0.0", "1.99.99", 1},
		{"0.1.90", "0.1.89", 1},
		{"0.1.9", "0.1.10", -1},
		{"1.2", "1.2.0", 0},
		{"1.2.3-rc1", "1.2.3", 0},
		{"1.2.3+build", "1.2.3", 0},
		{"dev", "1.2.3", -1},
		{"", "0.0.0", 0},
		{"garbage", "also-garbage", 0},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, CompareVersions(c.a, c.b), "CompareVersions(%q, %q)", c.a, c.b)
	}
}

func TestDetectInstallKind_IntelMacCaskroom(t *testing.T) {
	// goreleaser publishes a Cask; on Intel macs EvalSymlinks lands here, which
	// matches neither /Cellar/ nor the /opt/homebrew/ prefix.
	assert.Equal(t, InstallKindHomebrew,
		DetectInstallKind("/usr/local/Caskroom/shelltime/0.1.89/shelltime"))
	assert.Equal(t, InstallKindHomebrew,
		DetectInstallKind("/opt/homebrew/Caskroom/shelltime/0.1.89/shelltime"))
	assert.Equal(t, InstallKindHomebrew,
		DetectInstallKind("/opt/homebrew/Cellar/shelltime/0.1.89/bin/shelltime"))
}

package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Moved from commands/gc_test.go and commands/gc_more_test.go together with the helper,
// which `shelltime gc` now reaches through CompactCommandFiles.

// --- backupAndWriteFile (pure) ------------------------------------------------

func TestBackupAndWriteFile_NoExistingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	require.NoError(t, backupAndWriteFile(p, []byte("hello")))

	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	// No backup created when the original did not exist.
	_, err = os.Stat(p + ".bak")
	assert.True(t, os.IsNotExist(err))
}

func TestBackupAndWriteFile_BacksUpExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(p, []byte("old"), 0644))

	require.NoError(t, backupAndWriteFile(p, []byte("new")))

	// New content written.
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))

	// Old content preserved in .bak.
	bak, err := os.ReadFile(p + ".bak")
	require.NoError(t, err)
	assert.Equal(t, "old", string(bak))
}

// --- backupAndWriteFile failure branch ----------------------------------------

// TestBackupAndWriteFile_WriteIntoMissingDirFails hits the os.WriteFile error
// branch: the target's parent directory does not exist, so the write fails (and
// no prior file existed, so the rename/backup branch is skipped).
func TestBackupAndWriteFile_WriteIntoMissingDirFails(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	target := filepath.Join(missingDir, "data.txt")

	err := backupAndWriteFile(target, []byte("payload"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write file")
}

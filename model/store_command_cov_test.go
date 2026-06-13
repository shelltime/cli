package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// m3homeWithShelltimeFile sets HOME to a temp dir and creates a *regular file*
// at $HOME/.shelltime so that GetCommandsStoragePath()'s MkdirAll fails with
// ENOTDIR — deterministically exercising the ensureStorageFolder error branch
// even when running as root.
func m3homeWithShelltimeFile(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	InitFolder("") // reset globals under new HOME
	// .shelltime is a file, not a directory.
	require.NoError(t, os.WriteFile(filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER), []byte("x"), 0o644))
}

// TestCommand_DoSavePre_StorageFolderError covers DoSavePre's ensureStorageFolder
// error branch.
func TestCommand_DoSavePre_StorageFolderError(t *testing.T) {
	m3homeWithShelltimeFile(t)
	err := Command{Shell: "bash", SessionID: 1, Command: "x", Time: time.Now()}.DoSavePre()
	require.Error(t, err)
}

// TestCommand_DoUpdate_StorageFolderError covers DoUpdate's ensureStorageFolder
// error branch.
func TestCommand_DoUpdate_StorageFolderError(t *testing.T) {
	m3homeWithShelltimeFile(t)
	err := Command{Shell: "bash", SessionID: 1, Command: "x", Time: time.Now()}.DoUpdate(0)
	require.Error(t, err)
}

// TestFileStore_SavePre_StorageFolderError covers appendLine's
// ensureStorageFolder error branch via the file store SavePre.
func TestFileStore_SavePre_StorageFolderError(t *testing.T) {
	m3homeWithShelltimeFile(t)
	err := newFileStore().SavePre(context.Background(), Command{Command: "x", Time: time.Now()}, time.Now())
	require.Error(t, err)
}

// TestFileStore_SetCursor_StorageFolderError covers SetCursor's
// ensureStorageFolder error branch.
func TestFileStore_SetCursor_StorageFolderError(t *testing.T) {
	m3homeWithShelltimeFile(t)
	err := newFileStore().SetCursor(context.Background(), time.Now())
	require.Error(t, err)
}

// TestFileStore_Prune_PostReadErrorPropagates covers fileStore.Prune's
// GetPostCommands error branch: when the post file cannot be opened (its parent
// is a regular file), Prune returns the error rather than succeeding.
func TestFileStore_Prune_PostReadErrorPropagates(t *testing.T) {
	m3homeWithShelltimeFile(t)
	// GetPostCommands opens commands/post.txt; the .shelltime parent is a file so
	// the open fails with ENOTDIR.
	err := newFileStore().Prune(context.Background(), time.Now())
	require.Error(t, err)
}

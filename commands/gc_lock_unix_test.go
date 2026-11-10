//go:build unix && !aix && (!solaris || illumos)

package commands

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gc runs at every shell start. When another process is compacting the store (another gc,
// or the daemon after a sync), it leaves the store to that process: no error, nothing
// changed, and it comes back within the lock's bounded wait.
func TestCleanCommandFiles_SkipsWhileAnotherProcessHoldsTheStoreLock(t *testing.T) {
	_, cmdDir, _ := setupGCTest(t)
	require.NoError(t, os.MkdirAll(cmdDir, 0755))

	ctx := context.Background()
	store := model.NewFileStore()
	base := time.Now().Add(-time.Hour)
	synced := model.Command{Shell: "bash", SessionID: 1, Command: "git status", Username: "u", Hostname: "h", Time: base}
	require.NoError(t, store.SavePre(ctx, synced, base))
	require.NoError(t, store.SavePost(ctx, synced, 0, base.Add(time.Second)))
	require.NoError(t, store.SetCursor(ctx, base.Add(2*time.Second)))

	// What another process compacting the store holds
	lock, err := os.OpenFile(filepath.Join(cmdDir, "store.lock"), os.O_RDWR|os.O_CREATE, 0600)
	require.NoError(t, err)
	defer lock.Close()
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX))

	start := time.Now()
	require.NoError(t, cleanCommandFiles(ctx))
	assert.Less(t, time.Since(start), 3*time.Second, "a shell start must not hang on the lock")

	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	assert.Len(t, posts, 1, "nothing was compacted")

	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN))
	require.NoError(t, cleanCommandFiles(ctx))
	posts, err = store.GetPostCommands(ctx)
	require.NoError(t, err)
	assert.Empty(t, posts, "with the lock free, the synced command is dropped")
}

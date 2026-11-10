package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFileStoreRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	InitFolder("") // reset globals to the default .shelltime under the temp HOME

	store := NewFileStore()
	ctx := context.Background()
	start := time.Now()
	cmd := Command{Shell: "bash", SessionID: 7, Command: "go build", Username: "u", Hostname: "h", Time: start}

	require.NoError(t, store.SavePre(ctx, cmd, start))
	preInfo, err := os.Stat(GetPreCommandFilePath())
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), preInfo.Mode().Perm())

	_, noCursor, err := store.GetLastCursor(ctx)
	require.NoError(t, err)
	require.True(t, noCursor, "fresh store has no cursor")

	post := cmd
	post.Time = start.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))
	postInfo, err := os.Stat(GetPostCommandFilePath())
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), postInfo.Mode().Perm())

	tree, err := store.GetPreTree(ctx)
	require.NoError(t, err)
	require.Len(t, tree[cmd.GetUniqueKey()], 1)

	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	require.Equal(t, CommandPhasePost, int(posts[0].Phase))

	pres, err := store.GetPreCommands(ctx)
	require.NoError(t, err)
	require.Len(t, pres, 1)

	require.NoError(t, store.SetCursor(ctx, post.Time))
	c, noCursor, err := store.GetLastCursor(ctx)
	require.NoError(t, err)
	require.False(t, noCursor)
	require.Equal(t, post.Time.UnixNano(), c.UnixNano())

	// finished + synced => pruned from both files
	require.NoError(t, store.Prune(ctx, post.Time))
	pres, err = store.GetPreCommands(ctx)
	require.NoError(t, err)
	require.Empty(t, pres)
	posts, err = store.GetPostCommands(ctx)
	require.NoError(t, err)
	require.Empty(t, posts)

	require.NoError(t, store.Close())
}

// The first post can reach a store that has no pre.txt yet: the hooks were installed,
// or the daemon started, between a command's pre and its post.
func TestFileStore_MissingPreFileIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	at := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	post := Command{Shell: "zsh", SessionID: 1, Command: "source ~/.zshrc", Username: "u", Hostname: "h", Time: at}
	require.NoError(t, store.SavePost(ctx, post, 0, at))
	require.NoFileExists(t, GetPreCommandFilePath())

	tree, err := store.GetPreTree(ctx)
	require.NoError(t, err)
	require.Empty(t, tree)

	pres, err := store.GetPreCommands(ctx)
	require.NoError(t, err)
	require.Empty(t, pres)

	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{})
	require.NoError(t, err)
	require.Empty(t, res.Data, "a post without its pre is not uploaded")

	listed, err := BuildListedCommands(ctx, store)
	require.NoError(t, err)
	require.Empty(t, listed)

	require.NoError(t, store.Prune(ctx, at))
	posts, err := store.GetPostCommands(ctx)
	require.NoError(t, err)
	require.Empty(t, posts, "the synced post is pruned even though no pre file exists")
}

// Any other failure to read pre.txt is still an error: treating it as empty would make
// every waiting post look like it has no pre.
func TestFileStore_UnreadablePreFileIsAnError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, os.MkdirAll(GetPreCommandFilePath(), 0o700)) // a directory, not a file

	store := newFileStore()
	_, err := store.GetPreTree(context.Background())
	require.Error(t, err)
	_, err = store.GetPreCommands(context.Background())
	require.Error(t, err)
}

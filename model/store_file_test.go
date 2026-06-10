package model

import (
	"context"
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

	_, noCursor, err := store.GetLastCursor(ctx)
	require.NoError(t, err)
	require.True(t, noCursor, "fresh store has no cursor")

	post := cmd
	post.Time = start.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))

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

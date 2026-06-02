package model

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildListedCommands(t *testing.T) {
	store, err := newBoltStore(filepath.Join(t.TempDir(), "commands.db"))
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	start := time.Now()
	cmd := Command{Shell: "bash", SessionID: 1, Command: "make build", Username: "u", Hostname: "h", Time: start}

	// pre then post for the same command -> one paired row
	require.NoError(t, store.SavePre(ctx, cmd, start))
	post := cmd
	post.Time = start.Add(3 * time.Second)
	require.NoError(t, store.SavePost(ctx, post, 0, post.Time))

	// a post with no matching pre -> skipped
	orphan := Command{Shell: "zsh", SessionID: 2, Command: "orphan", Username: "u", Time: start}
	require.NoError(t, store.SavePost(ctx, orphan, 1, start))

	listed, err := BuildListedCommands(ctx, store)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "make build", listed[0].Command)
	require.Equal(t, "bash", listed[0].Shell)
	require.Equal(t, start.Unix(), listed[0].StartTime.Unix())
	require.Equal(t, post.Time.Unix(), listed[0].EndTime.Unix())
}

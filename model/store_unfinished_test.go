package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A pre command without a post is never dropped for its age. Nothing in the store tells a
// command that is still running from one whose post will never come, and a post without
// its pre is not uploaded, so dropping the pre of a command that really runs for months
// would lose that command when it ends.
func TestCompact_KeepsAPreWithoutAPostHoweverOld(t *testing.T) {
	compactions := map[string]func(ctx context.Context, store *fileStore, cursor time.Time) error{
		"gc": func(ctx context.Context, store *fileStore, cursor time.Time) error {
			return CompactCommandFiles(ctx)
		},
		"daemon prune": func(ctx context.Context, store *fileStore, cursor time.Time) error {
			return store.Prune(ctx, cursor)
		},
	}
	for name, compact := range compactions {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			ctx := context.Background()
			store := newFileStore()

			// Started 400 days ago and still running
			started := time.Now().Add(-400 * 24 * time.Hour)
			longRunning := storeTestCommand("kubectl port-forward svc/db 5432", started)
			require.NoError(t, store.SavePre(ctx, longRunning, started))

			// The rest of the store: a finished command, synced
			cursor := savePair(t, store, "git status", 0)
			require.NoError(t, store.SetCursor(ctx, cursor))

			require.NoError(t, compact(ctx, store, cursor))
			assert.Equal(t, []string{"kubectl port-forward svc/db 5432"}, preCommandNames(t, store),
				"the synced command's pre goes, the unfinished one stays")

			// It ends. Its post still finds its pre, and the command is uploaded with the
			// start it really had.
			done := longRunning
			done.Time = time.Now().Add(-time.Minute)
			require.NoError(t, store.SavePost(ctx, done, 0, done.Time))

			res, err := BuildTrackingData(ctx, store, ShellTimeConfig{})
			require.NoError(t, err)
			require.Len(t, res.Data, 1)
			assert.Equal(t, "kubectl port-forward svc/db 5432", res.Data[0].Command)
			assert.Equal(t, started.Unix(), res.Data[0].StartTime)
			assert.Equal(t, done.Time.Unix(), res.Data[0].EndTime)
		})
	}
}

// A store with pre commands and no post at all is left exactly as it is.
func TestCompact_WithoutPostsLeavesPreCommandsAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	old := storeTestCommand("left running a year ago", time.Now().Add(-365*24*time.Hour))
	require.NoError(t, store.SavePre(ctx, old, old.Time))

	require.NoError(t, CompactCommandFiles(ctx))
	require.NoError(t, store.Prune(ctx, time.Now()))

	assert.Equal(t, []string{"left running a year ago"}, preCommandNames(t, store))
	assert.NoFileExists(t, GetPreCommandFilePath()+".bak")
}

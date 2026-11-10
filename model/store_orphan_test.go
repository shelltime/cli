package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// savePre saves a command that started age ago and has no post.
func savePreAged(t *testing.T, store *fileStore, name string, age time.Duration) Command {
	t.Helper()
	pre := storeTestCommand(name, time.Now().Add(-age))
	require.NoError(t, store.SavePre(context.Background(), pre, pre.Time))
	return pre
}

const day = 24 * time.Hour

// What the old gc left behind: it dropped the synced posts and kept their pre commands,
// which now have no post and look like commands that are still running. Both compactions
// drop the ones older than orphanPreMaxAge, also when the store holds no post at all.
func TestCompact_DropsPreCommandsWithoutAPostAfterThirtyDays(t *testing.T) {
	compactions := map[string]func(ctx context.Context, store *fileStore) error{
		"gc":           func(ctx context.Context, store *fileStore) error { return CompactCommandFiles(ctx) },
		"daemon prune": func(ctx context.Context, store *fileStore) error { return store.Prune(ctx, time.Now().Add(-time.Hour)) },
	}
	for name, compact := range compactions {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			ctx := context.Background()
			store := newFileStore()

			savePreAged(t, store, "left behind a year ago", 365*day)
			savePreAged(t, store, "left behind 31 days ago", 31*day)
			savePreAged(t, store, "started 29 days ago", 29*day)
			savePreAged(t, store, "started an hour ago", time.Hour)
			require.NoError(t, store.SetCursor(ctx, time.Now().Add(-2*time.Hour)))

			require.NoError(t, compact(ctx, store))

			assert.Equal(t, []string{"started 29 days ago", "started an hour ago"}, preCommandNames(t, store))
		})
	}
}

func TestOrphanPreMaxAge(t *testing.T) {
	assert.Equal(t, 30*24*time.Hour, orphanPreMaxAge)
}

// Age alone never drops a pre: one with a post at or after its start is a finished
// command, and while that post is not synced the pre is what gives it its start time.
func TestCompact_KeepsAnOldPreWhosePostIsNotSynced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	// A command that ran for 40 days and ended a minute ago
	longRunning := savePreAged(t, store, "kubectl port-forward svc/db 5432", 40*day)
	done := longRunning
	done.Time = time.Now().Add(-time.Minute)
	require.NoError(t, store.SavePost(ctx, done, 0, done.Time))
	// Its neighbours: an orphan of the same age, and an old run of another command whose
	// only post is from before it started
	savePreAged(t, store, "left behind 40 days ago", 40*day)
	rerun := savePreAged(t, store, "make build", 35*day)
	earlier := rerun
	earlier.Time = rerun.Time.Add(-day)
	require.NoError(t, store.SavePost(ctx, earlier, 0, earlier.Time))

	require.NoError(t, store.SetCursor(ctx, time.Now().Add(-time.Hour)))
	require.NoError(t, CompactCommandFiles(ctx))

	assert.Equal(t, []string{"kubectl port-forward svc/db 5432"}, preCommandNames(t, store),
		"the pre of the unsynced post stays; the two with no post at or after their start go")

	res, err := BuildTrackingData(ctx, store, ShellTimeConfig{})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "kubectl port-forward svc/db 5432", res.Data[0].Command)
	assert.Equal(t, longRunning.Time.Unix(), res.Data[0].StartTime, "with the start it really had")
	assert.Equal(t, done.Time.Unix(), res.Data[0].EndTime)
}

// Both of a line's times have to be old. A pre written recently is kept whatever start
// time it carries, and so is one that started recently.
func TestCompact_OrphanAgeNeedsBothTimesToBeOld(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	now := time.Now()
	oldStart := storeTestCommand("old start, written today", now.Add(-40*day))
	require.NoError(t, store.SavePre(ctx, oldStart, now.Add(-time.Hour)))
	oldRecord := storeTestCommand("started today, old recording time", now.Add(-time.Hour))
	require.NoError(t, store.SavePre(ctx, oldRecord, now.Add(-40*day)))
	justInside := storeTestCommand("a minute short of 30 days", now.Add(-30*day+time.Minute))
	require.NoError(t, store.SavePre(ctx, justInside, justInside.Time))
	justOutside := storeTestCommand("a minute over 30 days", now.Add(-30*day-time.Minute))
	require.NoError(t, store.SavePre(ctx, justOutside, justOutside.Time))

	require.NoError(t, CompactCommandFiles(ctx))

	assert.ElementsMatch(t,
		[]string{"old start, written today", "started today, old recording time", "a minute short of 30 days"},
		preCommandNames(t, store))
}

// A store without posts and without an aged pre is left exactly as it is: no rewrite, no
// backup files.
func TestCompact_NothingToDropLeavesTheFilesAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	savePreAged(t, store, "started 29 days ago", 29*day)
	savePreAged(t, store, "started an hour ago", time.Hour)
	before, err := os.ReadFile(GetPreCommandFilePath())
	require.NoError(t, err)
	info, err := os.Stat(GetPreCommandFilePath())
	require.NoError(t, err)

	require.NoError(t, CompactCommandFiles(ctx))
	require.NoError(t, store.Prune(ctx, time.Now()))

	after, err := os.ReadFile(GetPreCommandFilePath())
	require.NoError(t, err)
	assert.Equal(t, before, after)
	infoAfter, err := os.Stat(GetPreCommandFilePath())
	require.NoError(t, err)
	assert.True(t, os.SameFile(info, infoAfter), "the file was not replaced")
	assert.NoFileExists(t, GetPreCommandFilePath()+".bak")
	assert.NoFileExists(t, GetPostCommandFilePath())
	assert.NoFileExists(t, GetCursorFilePath())
}

// A store that never synced has no cursor. Its aged pre commands go all the same, and
// it still has no cursor afterwards.
func TestCompact_AgedPreInAStoreThatNeverSynced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	savePreAged(t, store, "left behind 60 days ago", 60*day)
	savePair(t, store, "git status", 0)

	require.NoError(t, CompactCommandFiles(ctx))

	assert.Equal(t, []string{"git status"}, preCommandNames(t, store))
	_, noCursor, err := store.GetLastCursor(ctx)
	require.NoError(t, err)
	assert.True(t, noCursor)
	assert.Equal(t, []string{"git status"}, waitingCommands(t, store), "nothing that waits for its first sync is touched")
}

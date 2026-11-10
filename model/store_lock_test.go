package model

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeTestBase is when the commands of these tests ran: recently, because a compaction
// drops a pre command that has had no post for orphanPreMaxAge, and several of the tests
// count on unfinished commands staying.
var storeTestBase = time.Now().Add(-2 * time.Hour).Truncate(time.Second)

func storeTestCommand(name string, at time.Time) Command {
	return Command{Shell: "zsh", SessionID: 20240601120000, Command: name, Username: "u", Hostname: "h", Time: at}
}

// savePair saves a finished command that started offset after storeTestBase.
func savePair(t testing.TB, store *fileStore, name string, offset time.Duration) (post time.Time) {
	t.Helper()
	ctx := context.Background()
	pre := storeTestCommand(name, storeTestBase.Add(offset))
	require.NoError(t, store.SavePre(ctx, pre, pre.Time))
	done := pre
	done.Time = pre.Time.Add(time.Second)
	require.NoError(t, store.SavePost(ctx, done, 0, done.Time))
	return done.Time
}

// appendWithoutLock appends to one of the store's files the way a shelltime binary from
// before the lock does: straight to the file.
func appendWithoutLock(t testing.TB, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func commandLine(t testing.TB, cmd Command) []byte {
	t.Helper()
	line, err := cmd.ToLine(cmd.Time)
	require.NoError(t, err)
	return line
}

func preCommandNames(t testing.TB, store *fileStore) []string {
	t.Helper()
	pres, err := store.GetPreCommands(context.Background())
	require.NoError(t, err)
	names := make([]string, 0, len(pres))
	for _, pre := range pres {
		names = append(names, pre.Command)
	}
	return names
}

func setCompactAfterSnapshot(t *testing.T, hook func()) {
	t.Helper()
	compactAfterSnapshot = hook
	t.Cleanup(func() { compactAfterSnapshot = nil })
}

func setStoreLockWait(t *testing.T, wait time.Duration) {
	t.Helper()
	prev := commandStoreLockWait
	commandStoreLockWait = wait
	t.Cleanup(func() { commandStoreLockWait = prev })
}

// A command starts in another shell after a compaction has read the files and before it
// replaces them. Its pre line used to be gone with the old file, and the command, having
// no pre when it finished, was never uploaded. Both compactions are covered: gc's
// (CompactCommandFiles) and the daemon's (Prune).
func TestCompact_KeepsLinesAppendedSinceItReadTheFiles(t *testing.T) {
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

			cursor := savePair(t, store, "git status", 0)
			require.NoError(t, store.SetCursor(ctx, cursor))
			savePair(t, store, "go build ./...", time.Minute)

			late := storeTestCommand("npm run dev", storeTestBase.Add(2*time.Minute))
			latePost := storeTestCommand("ls", storeTestBase.Add(3*time.Minute))
			lateCursor := storeTestBase.Add(90 * time.Second)
			setCompactAfterSnapshot(t, func() {
				appendWithoutLock(t, GetPreCommandFilePath(), commandLine(t, late))
				appendWithoutLock(t, GetPostCommandFilePath(), commandLine(t, latePost))
				// as SetCursor writes it
				appendWithoutLock(t, GetCursorFilePath(), []byte(fmt.Sprintf("\n%d\n", lateCursor.UnixNano())))
			})

			require.NoError(t, compact(ctx, store, cursor))

			assert.Equal(t, []string{"go build ./...", "npm run dev"}, preCommandNames(t, store),
				"the synced command's pre is dropped, the one appended meanwhile is kept")

			posts, err := store.GetPostCommands(ctx)
			require.NoError(t, err)
			require.Len(t, posts, 2)
			assert.Equal(t, "go build ./...", posts[0].Command)
			assert.Equal(t, "ls", posts[1].Command)

			got, noCursor, err := store.GetLastCursor(ctx)
			require.NoError(t, err)
			assert.False(t, noCursor)
			assert.Equal(t, lateCursor.UnixNano(), got.UnixNano(), "a cursor written meanwhile wins over the one the compaction read")
		})
	}
}

// A compaction that finds the files rewritten under it (by a gc or daemon from before the
// lock) must not put its own, older, view of them back.
func TestCompact_LeavesFilesAloneWhenTheyWereRewrittenMeanwhile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	store := newFileStore()

	cursor := savePair(t, store, "git status", 0)
	require.NoError(t, store.SetCursor(ctx, cursor))
	savePair(t, store, "go build ./...", time.Minute)

	// Different content every time, so it never starts with what a compaction just read
	var rewritten []byte
	rewrites := 0
	setCompactAfterSnapshot(t, func() {
		rewrites++
		rewritten = []byte(fmt.Sprintf("rewrite %d by someone else\n", rewrites))
		require.NoError(t, os.WriteFile(GetPreCommandFilePath(), rewritten, 0o600))
	})
	postBefore, err := os.ReadFile(GetPostCommandFilePath())
	require.NoError(t, err)

	// The caller is told: a compaction that didn't happen is not a success
	err = store.Prune(ctx, cursor)
	require.ErrorIs(t, err, ErrCommandStoreChanged)
	assert.Contains(t, err.Error(), GetPreCommandFilePath())
	assert.ErrorIs(t, CompactCommandFiles(ctx), ErrCommandStoreChanged)

	pre, err := os.ReadFile(GetPreCommandFilePath())
	require.NoError(t, err)
	assert.Equal(t, rewritten, pre)
	postAfter, err := os.ReadFile(GetPostCommandFilePath())
	require.NoError(t, err)
	assert.Equal(t, postBefore, postAfter, "no file is replaced once one of them changed")
}

// An append waits for a compaction that holds the lock, and then lands in the new file.
func TestStoreLock_AppendWaitsForACompaction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setStoreLockWait(t, 10*time.Second)
	ctx := context.Background()
	store := newFileStore()
	savePair(t, store, "git status", 0)

	unlock, state := lockCommandStore(true)
	require.Equal(t, storeLockHeld, state)

	appended := make(chan error, 1)
	go func() {
		pre := storeTestCommand("npm run dev", storeTestBase.Add(time.Minute))
		appended <- store.SavePre(ctx, pre, pre.Time)
	}()

	select {
	case err := <-appended:
		unlock()
		t.Fatalf("the append went ahead while the store was locked exclusively (err: %v)", err)
	case <-time.After(100 * time.Millisecond):
	}
	assert.Equal(t, []string{"git status"}, preCommandNames(t, store))

	unlock()
	select {
	case err := <-appended:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the append never got the lock after it was released")
	}
	assert.Equal(t, []string{"git status", "npm run dev"}, preCommandNames(t, store))
}

// Nothing may hang on the lock: `track` runs before and after every shell command. An
// append that can't get it in time goes ahead unlocked, as it did before there was a lock.
func TestStoreLock_AppendGoesAheadAfterTheWait(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setStoreLockWait(t, 30*time.Millisecond)
	ctx := context.Background()
	store := newFileStore()
	savePair(t, store, "git status", 0)

	unlock, state := lockCommandStore(true)
	require.Equal(t, storeLockHeld, state)
	defer unlock()

	start := time.Now()
	pre := storeTestCommand("npm run dev", storeTestBase.Add(time.Minute))
	require.NoError(t, store.SavePre(ctx, pre, pre.Time))
	require.NoError(t, pre.DoSavePre())
	require.NoError(t, pre.DoUpdate(0))
	require.NoError(t, store.SetCursor(ctx, pre.Time))
	waited := time.Since(start)

	assert.GreaterOrEqual(t, waited, 4*30*time.Millisecond, "each of the four appends waits for the lock first")
	assert.Less(t, waited, 3*time.Second, "and gives up after the bounded wait")
	assert.Equal(t, []string{"git status", "npm run dev", "npm run dev"}, preCommandNames(t, store))
}

// A compaction that can't get the lock changes nothing and says so.
func TestStoreLock_CompactionGivesUpWhenBusy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setStoreLockWait(t, 30*time.Millisecond)
	ctx := context.Background()
	store := newFileStore()
	cursor := savePair(t, store, "git status", 0)
	require.NoError(t, store.SetCursor(ctx, cursor))

	// Another process appending holds it shared; one compacting holds it exclusively
	for _, exclusive := range []bool{false, true} {
		unlock, state := lockCommandStore(exclusive)
		require.Equal(t, storeLockHeld, state)

		assert.ErrorIs(t, store.Prune(ctx, cursor), ErrCommandStoreBusy)
		assert.ErrorIs(t, CompactCommandFiles(ctx), ErrCommandStoreBusy)
		assert.Equal(t, []string{"git status"}, preCommandNames(t, store))
		unlock()
	}

	require.NoError(t, store.Prune(ctx, cursor))
	assert.Empty(t, preCommandNames(t, store))
}

// Appends share the lock: many `track` processes never wait for each other.
func TestStoreLock_SharedHoldersDoNotBlockEachOther(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setStoreLockWait(t, 10*time.Second)
	require.NoError(t, ensureStorageFolder())

	first, state := lockCommandStore(false)
	require.Equal(t, storeLockHeld, state)
	defer first()

	start := time.Now()
	second, state := lockCommandStore(false)
	require.Equal(t, storeLockHeld, state)
	second()
	assert.Less(t, time.Since(start), time.Second)
}

// Without the storage folder there is no lock file to open: callers go on unlocked.
func TestStoreLock_UnavailableWithoutTheStorageFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	unlock, state := lockCommandStore(true)
	require.NotNil(t, unlock)
	unlock()
	assert.Equal(t, storeLockUnavailable, state)
	require.NoError(t, CompactCommandFiles(context.Background()))
}

// Appenders and compactions at the same time, in one process. Every line appended must be
// in the file at the end, whichever compactions ran in between. Run with -race.
func TestStoreLock_ConcurrentAppendsAndCompactionsLoseNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setStoreLockWait(t, 30*time.Second)
	ctx := context.Background()
	store := newFileStore()

	// Nothing is at or before this cursor, so a compaction keeps every line: any line
	// missing at the end was lost, not pruned.
	cursor := storeTestBase.Add(-time.Hour)
	savePair(t, store, "seed", 0)
	require.NoError(t, store.SetCursor(ctx, cursor))

	const appenders, perAppender = 8, 60
	var stop atomic.Bool
	var compactions atomic.Int64
	var compacting sync.WaitGroup
	compacting.Go(func() {
		for i := 0; !stop.Load(); i++ {
			var err error
			if i%2 == 0 {
				err = store.Prune(ctx, cursor)
			} else {
				err = CompactCommandFiles(ctx)
			}
			if assert.NoError(t, err) {
				compactions.Add(1)
			}
			// A compaction that takes the lock again at once starves the appends, which
			// poll for it. Real ones are a shell start or a sync apart.
			time.Sleep(time.Millisecond)
		}
	})

	var appending sync.WaitGroup
	for a := range appenders {
		appending.Go(func() {
			for i := range perAppender {
				cmd := storeTestCommand(fmt.Sprintf("appender-%d-command-%d", a, i), storeTestBase.Add(time.Duration(i)*time.Millisecond))
				// Every way a command line is appended
				var err error
				switch i % 3 {
				case 0:
					err = store.SavePre(ctx, cmd, cmd.Time)
				case 1:
					err = cmd.DoSavePre()
				default:
					err = store.SavePre(ctx, cmd, cmd.Time)
					if err == nil {
						err = store.SetCursor(ctx, cursor)
					}
				}
				assert.NoError(t, err)
			}
		})
	}
	appending.Wait()
	stop.Store(true)
	compacting.Wait()

	assertEveryAppendedCommand(t, store, appenders, perAppender)
	assert.Positive(t, compactions.Load(), "at least one compaction must have run among the appends")
}

func assertEveryAppendedCommand(t *testing.T, store *fileStore, appenders, perAppender int) {
	t.Helper()
	seen := make(map[string]int)
	for _, name := range preCommandNames(t, store) {
		seen[name]++
	}
	missing, duplicated := 0, 0
	for a := range appenders {
		for i := range perAppender {
			switch seen[fmt.Sprintf("appender-%d-command-%d", a, i)] {
			case 0:
				missing++
			case 1:
			default:
				duplicated++
			}
		}
	}
	assert.Zero(t, missing, "pre commands lost")
	assert.Zero(t, duplicated, "pre commands written twice")
	assert.Equal(t, appenders*perAppender+1, len(seen), "the appended commands and the seed, nothing else")
}

const storeAppenderEnv = "SHELLTIME_TEST_STORE_APPENDER"

// runStoreAppenderProcess is the body of a child process started by
// TestStoreLock_AppendsFromOtherProcessesSurviveCompactions: it appends its commands to
// the store in $HOME, like a `shelltime track` does, and exits.
func runStoreAppenderProcess(spec string) int {
	fields := strings.Split(spec, ":")
	id, _ := strconv.Atoi(fields[0])
	count, _ := strconv.Atoi(fields[1])
	commandStoreLockWait = 30 * time.Second
	store := newFileStore()
	for i := range count {
		cmd := storeTestCommand(fmt.Sprintf("appender-%d-command-%d", id, i), storeTestBase.Add(time.Duration(i)*time.Millisecond))
		if err := store.SavePre(context.Background(), cmd, cmd.Time); err != nil {
			fmt.Fprintln(os.Stderr, "append failed:", err)
			return 1
		}
	}
	return 0
}

// The lock is for separate processes, so this is the test that matters: other processes
// append while this one compacts, and every line they wrote must still be there.
func TestStoreLock_AppendsFromOtherProcessesSurviveCompactions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no advisory lock on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	setStoreLockWait(t, 30*time.Second)
	ctx := context.Background()
	store := newFileStore()

	cursor := storeTestBase.Add(-time.Hour)
	savePair(t, store, "seed", 0)
	require.NoError(t, store.SetCursor(ctx, cursor))

	const processes, perProcess = 4, 150
	children := make([]*exec.Cmd, processes)
	for id := range processes {
		child := exec.Command(os.Args[0], "-test.run=^$")
		child.Env = append(os.Environ(), "HOME="+home, fmt.Sprintf("%s=%d:%d", storeAppenderEnv, id, perProcess))
		child.Stderr = os.Stderr
		require.NoError(t, child.Start())
		children[id] = child
	}

	var stop atomic.Bool
	var compactions atomic.Int64
	var compacting sync.WaitGroup
	compacting.Go(func() {
		for !stop.Load() {
			if assert.NoError(t, store.Prune(ctx, cursor)) {
				compactions.Add(1)
			}
			time.Sleep(time.Millisecond)
		}
	})
	for _, child := range children {
		assert.NoError(t, child.Wait())
	}
	stop.Store(true)
	compacting.Wait()

	assertEveryAppendedCommand(t, store, processes, perProcess)
	assert.Positive(t, compactions.Load())
	t.Logf("%d compactions ran while %d processes appended %d lines each", compactions.Load(), processes, perProcess)
}

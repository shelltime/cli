package model

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withTempHome points the storage helpers at a temp dir for the duration of a test.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return filepath.Join(dir, COMMAND_BASE_STORAGE_FOLDER)
}

func TestAcquireUpdateLock_ExclusiveThenReleasable(t *testing.T) {
	withTempHome(t)

	release, ok := AcquireUpdateLock(time.Minute)
	require.True(t, ok, "first acquire should succeed")

	_, ok2 := AcquireUpdateLock(time.Minute)
	assert.False(t, ok2, "second acquire must fail while held")

	release()

	release3, ok3 := AcquireUpdateLock(time.Minute)
	assert.True(t, ok3, "acquire should succeed after release")
	release3()
}

// A process killed mid-update must not wedge updates forever.
func TestAcquireUpdateLock_StealsStaleLock(t *testing.T) {
	withTempHome(t)

	release, ok := AcquireUpdateLock(time.Hour)
	require.True(t, ok)
	defer release()

	// Backdate the lock past the staleness threshold.
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(GetUpdateLockPath(), old, old))

	release2, ok2 := AcquireUpdateLock(time.Minute)
	assert.True(t, ok2, "a lock older than staleAfter should be stolen")
	release2()
}

func TestAcquireUpdateLock_DoesNotStealFreshLock(t *testing.T) {
	withTempHome(t)

	release, ok := AcquireUpdateLock(time.Millisecond)
	require.True(t, ok)
	defer release()

	// staleAfter is long, so the just-taken lock is not stale.
	_, ok2 := AcquireUpdateLock(time.Hour)
	assert.False(t, ok2)
}

// N shells can sample the drift check at the same moment; exactly one must win.
func TestAcquireUpdateLock_ConcurrentAcquireHasSingleWinner(t *testing.T) {
	withTempHome(t)

	var winners atomic.Int32
	var wg sync.WaitGroup
	releases := make(chan func(), 20)

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, ok := AcquireUpdateLock(time.Hour)
			if ok {
				winners.Add(1)
				releases <- release
			}
		}()
	}
	wg.Wait()
	close(releases)
	for r := range releases {
		r()
	}

	assert.EqualValues(t, 1, winners.Load(), "exactly one goroutine should hold the lock")
}

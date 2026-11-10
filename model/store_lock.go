package model

import (
	"errors"
	"time"
)

// The txt command store (pre.txt, post.txt, cursor.txt) is shared by processes that know
// nothing of each other: a `shelltime track` per shell command, a `shelltime gc` per new
// shell, `shelltime sync`, and the daemon. Most of them only append a line. Two rewrite the
// files: gc and the daemon's prune after a sync. A rewrite reads a file, works out what to
// keep and puts a new file in its place, and a line appended in between would be gone, with
// the command it belongs to.
//
// store.lock, next to the files, is an advisory lock (flock) against that: whoever appends
// holds it shared for the append, whoever rewrites holds it exclusively from reading the
// files to replacing them. Readers don't take it. They see each file either as it was or
// as it is after the rewrite, because a rewrite replaces a file with a rename.
//
// The lock only ever helps. Nobody waits for it longer than commandStoreLockWait, and where
// it can't be had at all (Windows, a filesystem without flock) everything goes on as it
// did before the lock existed. See lockCommandStore for what each caller does then.

// commandStoreLockWait bounds the wait for store.lock. An append holds it for a few
// microseconds and a rewrite for the few milliseconds it takes to read and write the files,
// so only something stuck holds it longer, and a shell must not hang on that: `track` runs
// before and after every command.
var commandStoreLockWait = 200 * time.Millisecond

// ErrCommandStoreBusy is returned by a compaction that couldn't get the store's lock in
// time. Nothing was changed; the next compaction does the work.
var ErrCommandStoreBusy = errors.New("the command store is locked by another process")

type storeLockState int

const (
	// storeLockHeld: the lock is held until the returned func is called
	storeLockHeld storeLockState = iota
	// storeLockUnavailable: there is no lock to take here (platform, filesystem, or the
	// storage folder is missing)
	storeLockUnavailable
	// storeLockBusy: another process held the lock for the whole wait
	storeLockBusy
)

func commandStoreLockPath() string {
	return GetStoragePath("commands", "store.lock")
}

// lockCommandStore takes the store's lock, shared or exclusive, waiting at most
// commandStoreLockWait. The returned func releases it and is never nil.
//
// An appender ignores the state: without the lock it appends anyway, as it always did, and
// a compaction carries over what it finds appended since it read the files. A compaction
// that finds the lock busy gives up, and one that finds it unavailable goes on unlocked.
func lockCommandStore(exclusive bool) (unlock func(), state storeLockState) {
	return acquireStoreLock(commandStoreLockPath(), exclusive, commandStoreLockWait)
}

// lockStoreForAppend holds the shared lock around one append.
func lockStoreForAppend() (unlock func()) {
	unlock, _ = lockCommandStore(false)
	return unlock
}

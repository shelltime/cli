package model

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// AcquireUpdateLock takes an exclusive lock so only one process at a time
// replaces binaries. Both the daemon's download and the CLI's daemon swap take
// it, because N shells can sample the drift check simultaneously and would
// otherwise all try to repair at once.
//
// Returns ok=false (with a no-op release) when another process holds it. A lock
// older than staleAfter is stolen once, so a process killed mid-update does not
// wedge updates forever.
//
// Uses O_CREATE|O_EXCL rather than flock: it is atomic on POSIX, needs no x/sys
// import and no build-tag split, and matches the plain-os style used elsewhere
// in this package.
func AcquireUpdateLock(staleAfter time.Duration) (release func(), ok bool) {
	noop := func() {}
	path := GetUpdateLockPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return noop, false
	}

	if acquireLockFile(path) {
		return func() { _ = os.Remove(path) }, true
	}

	// Held. Steal it only if it is provably stale.
	info, err := os.Stat(path)
	if err != nil {
		// Vanished between the create and the stat — try once more.
		if acquireLockFile(path) {
			return func() { _ = os.Remove(path) }, true
		}
		return noop, false
	}
	if time.Since(info.ModTime()) <= staleAfter {
		return noop, false
	}

	slog.Warn("stealing stale update lock",
		slog.String("path", path),
		slog.Time("heldSince", info.ModTime()))
	if err := os.Remove(path); err != nil {
		return noop, false
	}
	if acquireLockFile(path) {
		return func() { _ = os.Remove(path) }, true
	}
	return noop, false
}

func acquireLockFile(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	// Record who holds it to make a wedged lock diagnosable.
	_, _ = fmt.Fprintf(f, "%d\n%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
	_ = f.Close()
	return true
}

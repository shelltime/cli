package model

import (
	"context"
	"testing"
	"time"
)

// BenchmarkFileStoreAppend is what `shelltime track` without a daemon, and the daemon for
// every event, pay to save one command: the store's lock (shared) around an open, a chmod
// check, a write and a close.
func BenchmarkFileStoreAppend(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	ctx := context.Background()
	store := newFileStore()
	at := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	cmd := Command{Shell: "zsh", SessionID: 20240601120000, Command: "git status --short", Username: "u", Hostname: "h", Time: at, PPID: 4242}
	if err := store.SavePre(ctx, cmd, at); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := store.SavePre(ctx, cmd, at); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStoreLock is the lock on its own: open store.lock, flock, close.
func BenchmarkStoreLock(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	if err := ensureStorageFolder(); err != nil {
		b.Fatal(err)
	}
	for _, mode := range []struct {
		name      string
		exclusive bool
	}{{"shared", false}, {"exclusive", true}} {
		b.Run(mode.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				unlock, state := lockCommandStore(mode.exclusive)
				if state != storeLockHeld {
					b.Fatalf("lock not held: state %d", state)
				}
				unlock()
			}
		})
	}
}

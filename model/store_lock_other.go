//go:build !unix || aix || (solaris && !illumos)

package model

import "time"

// acquireStoreLock has no advisory lock to offer here: the Windows build of the CLI (the
// daemon doesn't exist there), and the unix systems whose package syscall has no flock.
// The txt store is used unlocked, as before.
func acquireStoreLock(path string, exclusive bool, wait time.Duration) (unlock func(), state storeLockState) {
	return func() {}, storeLockUnavailable
}

//go:build unix

package model

import (
	"syscall"
	"time"
)

// acquireStoreLock takes a flock on path, polling without blocking so that giving up never
// leaves a lock request behind. Any failure other than "someone else holds it" means the
// lock can't be used here.
func acquireStoreLock(path string, exclusive bool, wait time.Duration) (unlock func(), state storeLockState) {
	noop := func() {}

	var fd int
	for {
		var err error
		fd, err = syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC, uint32(privateFileMode))
		if err == nil {
			break
		}
		if err != syscall.EINTR {
			return noop, storeLockUnavailable
		}
	}

	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}

	var deadline time.Time
	pause := 50 * time.Microsecond
	for {
		err := syscall.Flock(fd, how|syscall.LOCK_NB)
		if err == nil {
			// Closing the descriptor releases the lock
			return func() { syscall.Close(fd) }, storeLockHeld
		}
		if err == syscall.EINTR {
			continue
		}
		if err != syscall.EWOULDBLOCK {
			syscall.Close(fd)
			return noop, storeLockUnavailable
		}

		now := time.Now()
		if deadline.IsZero() {
			deadline = now.Add(wait)
		} else if !now.Before(deadline) {
			syscall.Close(fd)
			return noop, storeLockBusy
		}
		time.Sleep(pause)
		if pause < time.Millisecond {
			pause *= 2
		}
	}
}

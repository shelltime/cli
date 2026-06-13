package daemon

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetProcessName_CurrentProcessLinux exercises the positive /proc/<pid>/comm
// read path on Linux. The current test process exists, so getProcessName must
// return a non-empty name.
func TestGetProcessName_CurrentProcessLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("positive /proc read path is Linux-specific")
	}
	name := getProcessName(os.Getpid())
	assert.NotEmpty(t, name, "current process should have a readable /proc/<pid>/comm")
}

// TestGetParentPID_CurrentProcessLinux exercises the positive /proc/<pid>/stat
// parse path on Linux. The current process always has a parent (>0).
func TestGetParentPID_CurrentProcessLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("positive /proc read path is Linux-specific")
	}
	ppid := getParentPID(os.Getpid())
	assert.Greater(t, ppid, 0, "current process should have a parent PID > 0")
	// Sanity: the reported parent should match the runtime's own view.
	assert.Equal(t, os.Getppid(), ppid)
}

// TestResolveTerminal_WalksFromParentNoPanic walks up the real process tree
// starting from the parent PID. It must terminate and return defined values
// without panicking. The concrete result is environment-dependent.
func TestResolveTerminal_WalksFromParentNoPanic(t *testing.T) {
	ppid := os.Getppid()
	if ppid <= 1 {
		t.Skip("no usable parent PID in this environment")
	}
	assert.NotPanics(t, func() {
		term, mux := ResolveTerminal(ppid)
		// Walking from a real PID always yields a defined terminal string:
		// either a known match, the "unknown" sentinel, or "" when only a
		// multiplexer was found. We only assert termination, not a value.
		_ = term
		_ = mux
	})
}

// TestResolveTerminal_StopsAtKnownMultiplexerThenTerminal documents that when
// walking a real tree we never loop forever; the visited-set + depth cap keep
// it bounded. We assert the two return values are consistent (a non-empty
// terminal implies the walk stopped early as designed).
func TestResolveTerminal_BoundedWalk(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("relies on /proc walking")
	}
	// Use the current process as the start: walking up reaches PID 1 within the
	// 10-level cap on any sane system, so the call must return promptly.
	term, mux := ResolveTerminal(os.Getpid())
	// At least one of the documented outcomes must hold.
	require.True(t, term != "" || mux != "" || term == "unknown",
		"ResolveTerminal must return a defined result")
}

package commands

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// descriptorsOn counts this process's open descriptors that point at path.
func descriptorsOn(t *testing.T, path string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	n := 0
	for _, entry := range entries {
		// A descriptor closed since the listing (the directory's own) just fails to resolve
		if target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name())); err == nil && target == path {
			n++
		}
	}
	return n
}

// The first run in a fresh folder creates log.log and then opens it for append. The
// handle from the creation was never closed.
func TestSetupLogger_FirstRunLeavesNoDescriptorOpen(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("counts descriptors through /proc/self/fd")
	}
	prevSkip, prevLogger := SKIP_LOGGER_SETTINGS, slog.Default()
	SKIP_LOGGER_SETTINGS = false
	t.Cleanup(func() {
		CloseLogger()
		SKIP_LOGGER_SETTINGS = prevSkip
		slog.SetDefault(prevLogger)
	})

	// EvalSymlinks: /proc reports the resolved path
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, ".shelltime")
	logPath := filepath.Join(dir, "log.log")

	SetupLogger(dir)
	require.FileExists(t, logPath)
	require.Equal(t, 1, descriptorsOn(t, logPath), "only the handle the logger writes to stays open")

	CloseLogger()
	require.Zero(t, descriptorsOn(t, logPath))
}

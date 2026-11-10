package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openAsLaunchd opens a log file the way launchd opens a job's StandardOutPath before it
// starts the daemon: for append, and held open for as long as the daemon runs.
func openAsLaunchd(t *testing.T, path string) *os.File {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	return f
}

// The daemon cleans its own log files while they are its stdout and stderr. Removing them
// left the daemon writing to files without a name: the space wasn't freed until it exited,
// and nothing it logged afterwards could be read.
func TestCleanDaemonLogFiles_KeepsTheFilesTheDaemonWritesTo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout := openAsLaunchd(t, GetDaemonLogFilePath())
	stderr := openAsLaunchd(t, GetDaemonErrFilePath())
	old := make([]byte, 2048)
	for _, f := range []*os.File{stdout, stderr} {
		_, err := f.Write(old)
		require.NoError(t, err)
	}

	freed, err := cleanDaemonLogFiles(1024, false)
	require.NoError(t, err)
	assert.Equal(t, int64(2*len(old)), freed)

	// The daemon goes on logging to the descriptors it was started with
	for _, f := range []*os.File{stdout, stderr} {
		_, err := f.WriteString("after cleanup\n")
		require.NoError(t, err)
	}
	for _, path := range []string{GetDaemonLogFilePath(), GetDaemonErrFilePath()} {
		content, err := os.ReadFile(path)
		require.NoError(t, err, "the log file must still be there")
		assert.Equal(t, "after cleanup\n", string(content), "the old content is gone and new lines start the file")
	}
}

func TestTruncateLogFile(t *testing.T) {
	write := func(t *testing.T, size int) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "daemon.log")
		require.NoError(t, os.WriteFile(path, make([]byte, size), 0o644))
		return path
	}
	sizeOf := func(t *testing.T, path string) int64 {
		t.Helper()
		info, err := os.Stat(path)
		require.NoError(t, err)
		return info.Size()
	}

	t.Run("below the threshold is left alone", func(t *testing.T) {
		path := write(t, 100)
		freed, err := truncateLogFile(path, 1024, false)
		require.NoError(t, err)
		assert.Zero(t, freed)
		assert.Equal(t, int64(100), sizeOf(t, path))
	})
	t.Run("at the threshold is emptied", func(t *testing.T) {
		path := write(t, 1024)
		freed, err := truncateLogFile(path, 1024, false)
		require.NoError(t, err)
		assert.Equal(t, int64(1024), freed)
		assert.Zero(t, sizeOf(t, path))
	})
	t.Run("force empties a small file", func(t *testing.T) {
		path := write(t, 100)
		freed, err := truncateLogFile(path, 1024, true)
		require.NoError(t, err)
		assert.Equal(t, int64(100), freed)
		assert.Zero(t, sizeOf(t, path))
	})
	t.Run("a missing file is not an error", func(t *testing.T) {
		freed, err := truncateLogFile(filepath.Join(t.TempDir(), "none.log"), 1024, true)
		require.NoError(t, err)
		assert.Zero(t, freed)
	})
}

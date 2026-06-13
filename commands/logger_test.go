package commands

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withLoggerEnabled temporarily flips SKIP_LOGGER_SETTINGS to false so the real
// SetupLogger/CloseLogger bodies run, and restores it afterwards. SetupLogger
// rebinds the default slog handler to a file inside t.TempDir(); we restore the
// original default handler on cleanup so later tests don't log into a removed
// temp directory.
func withLoggerEnabled(t *testing.T) {
	t.Helper()
	prev := SKIP_LOGGER_SETTINGS
	prevDefault := slog.Default()
	SKIP_LOGGER_SETTINGS = false
	t.Cleanup(func() {
		if loggerFile != nil {
			loggerFile.Close()
			loggerFile = nil
		}
		slog.SetDefault(prevDefault)
		SKIP_LOGGER_SETTINGS = prev
	})
}

func TestSetupLogger_CreatesDirAndFile(t *testing.T) {
	withLoggerEnabled(t)

	base := filepath.Join(t.TempDir(), "nested", "logdir")
	SetupLogger(base)

	// The log file should now exist under baseFolder/log.log.
	logPath := filepath.Join(base, "log.log")
	_, err := os.Stat(logPath)
	require.NoError(t, err, "log file should be created")
	assert.NotNil(t, loggerFile, "loggerFile handle should be set")

	// Closing should release and nil the handle.
	CloseLogger()
	assert.Nil(t, loggerFile, "CloseLogger should nil out the handle")
}

func TestSetupLogger_AppendsToExistingFile(t *testing.T) {
	withLoggerEnabled(t)

	base := t.TempDir()
	logPath := filepath.Join(base, "log.log")
	// Pre-create the file with content; SetupLogger should open in append mode
	// and not truncate the existing bytes.
	require.NoError(t, os.WriteFile(logPath, []byte("preexisting\n"), 0644))

	SetupLogger(base)
	require.NotNil(t, loggerFile)
	CloseLogger()

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "preexisting", "existing log content must be preserved (append mode)")
}

func TestSetupLogger_SkippedWhenFlagSet(t *testing.T) {
	prev := SKIP_LOGGER_SETTINGS
	SKIP_LOGGER_SETTINGS = true
	t.Cleanup(func() { SKIP_LOGGER_SETTINGS = prev })

	base := filepath.Join(t.TempDir(), "should-not-exist")
	SetupLogger(base)

	// With the skip flag set, no directory/file is created.
	_, err := os.Stat(filepath.Join(base, "log.log"))
	assert.True(t, os.IsNotExist(err), "no log file should be created when skipped")
}

func TestCloseLogger_NilHandleIsSafe(t *testing.T) {
	prev := SKIP_LOGGER_SETTINGS
	SKIP_LOGGER_SETTINGS = false
	t.Cleanup(func() { SKIP_LOGGER_SETTINGS = prev })

	// Ensure handle is nil and CloseLogger does not panic.
	if loggerFile != nil {
		loggerFile.Close()
		loggerFile = nil
	}
	assert.NotPanics(t, func() { CloseLogger() })
}

func TestCloseLogger_SkippedWhenFlagSet(t *testing.T) {
	prev := SKIP_LOGGER_SETTINGS
	SKIP_LOGGER_SETTINGS = true
	t.Cleanup(func() { SKIP_LOGGER_SETTINGS = prev })
	// Should early-return without touching loggerFile.
	assert.NotPanics(t, func() { CloseLogger() })
}

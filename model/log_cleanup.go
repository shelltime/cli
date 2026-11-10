package model

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
)

// CleanLogFile removes a log file if it exceeds the threshold or if force is true.
// thresholdBytes: size threshold in bytes (e.g., config.LogCleanup.ThresholdMB * 1024 * 1024)
// Returns the size of the deleted file (0 if not deleted or file doesn't exist).
func CleanLogFile(filePath string, thresholdBytes int64, force bool) (int64, error) {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	fileSize := info.Size()
	if !force && fileSize < thresholdBytes {
		return 0, nil
	}

	if err := os.Remove(filePath); err != nil {
		return 0, fmt.Errorf("failed to remove file %s: %w", filePath, err)
	}

	slog.Info("cleaned log file", slog.String("file", filePath), slog.Int64("size_bytes", fileSize))
	return fileSize, nil
}

// CleanLargeLogFiles checks CLI log files and removes those exceeding the size threshold.
// thresholdBytes: size threshold in bytes
// If force is true, removes all log files regardless of size.
func CleanLargeLogFiles(thresholdBytes int64, force bool) (int64, error) {
	logFiles := []string{
		GetLogFilePath(),
		GetHeartbeatLogFilePath(),
		GetSyncPendingFilePath(),
	}

	var totalFreed int64
	for _, filePath := range logFiles {
		freed, err := CleanLogFile(filePath, thresholdBytes, force)
		if err != nil {
			slog.Warn("failed to clean log file", slog.String("file", filePath), slog.Any("err", err))
			continue
		}
		totalFreed += freed
	}

	return totalFreed, nil
}

// truncateLogFile empties a log file in place if it exceeds the threshold or if force is
// true, and returns how many bytes that freed (0 if it was left alone or doesn't exist).
//
// It is for log files a running process holds open. Removing such a file only removes its
// name: the process keeps writing to the nameless file, whose space is not given back until
// the process exits, and no log is visible until then. A file opened for append (as launchd
// opens a job's stdout and stderr) continues at the start after the truncation.
func truncateLogFile(filePath string, thresholdBytes int64, force bool) (int64, error) {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	fileSize := info.Size()
	if !force && fileSize < thresholdBytes {
		return 0, nil
	}

	if err := os.Truncate(filePath, 0); err != nil {
		return 0, fmt.Errorf("failed to truncate file %s: %w", filePath, err)
	}

	slog.Info("truncated log file", slog.String("file", filePath), slog.Int64("size_bytes", fileSize))
	return fileSize, nil
}

// CleanDaemonLogFiles cleans daemon-specific log files.
// On macOS, daemon logs go to ~/.shelltime/logs/shelltime-daemon.{log,err}
// On Linux, daemon logs go to systemd journal and can't be cleaned from here.
// thresholdBytes: size threshold in bytes
// If force is true, empties all log files regardless of size.
func CleanDaemonLogFiles(thresholdBytes int64, force bool) (int64, error) {
	// Only clean daemon logs on macOS (darwin)
	// On Linux, daemon uses systemd journal which is managed by journald
	if runtime.GOOS != "darwin" {
		return 0, nil
	}
	return cleanDaemonLogFiles(thresholdBytes, force)
}

// cleanDaemonLogFiles empties the daemon's log files. They are the stdout and stderr of
// the running daemon, which is also the process that cleans them, so they are truncated
// in place, never removed (see truncateLogFile).
func cleanDaemonLogFiles(thresholdBytes int64, force bool) (int64, error) {
	logFiles := []string{
		GetDaemonLogFilePath(),
		GetDaemonErrFilePath(),
	}

	var totalFreed int64
	for _, filePath := range logFiles {
		freed, err := truncateLogFile(filePath, thresholdBytes, force)
		if err != nil {
			slog.Warn("failed to clean daemon log file", slog.String("file", filePath), slog.Any("err", err))
			continue
		}
		totalFreed += freed
	}

	return totalFreed, nil
}

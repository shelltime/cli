package model

import (
	"os"
	"path/filepath"
)

// GetBaseStoragePath returns the base storage path for shelltime
// e.g., /Users/username/.shelltime
func GetBaseStoragePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback for environments where home dir is not available.
		return filepath.Join(os.TempDir(), COMMAND_BASE_STORAGE_FOLDER)
	}
	return filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER)
}

// GetStoragePath returns the full path for a given subpath within the storage folder
// e.g., GetStoragePath("config.toml") returns /Users/username/.shelltime/config.toml
func GetStoragePath(subpaths ...string) string {
	return filepath.Join(append([]string{GetBaseStoragePath()}, subpaths...)...)
}

// GetConfigFilePath returns the path to the main config file
func GetConfigFilePath() string {
	return GetStoragePath("config.toml")
}

// GetLocalConfigFilePath returns the path to the local config file
func GetLocalConfigFilePath() string {
	return GetStoragePath("config.local.toml")
}

// GetLogFilePath returns the path to the log file
func GetLogFilePath() string {
	return GetStoragePath("log.log")
}

// GetCommandsStoragePath returns the path to the commands storage folder
func GetCommandsStoragePath() string {
	return GetStoragePath("commands")
}

// GetPreCommandFilePath returns the path to the pre-command storage file
func GetPreCommandFilePath() string {
	return GetStoragePath("commands", "pre.txt")
}

// GetPostCommandFilePath returns the path to the post-command storage file
func GetPostCommandFilePath() string {
	return GetStoragePath("commands", "post.txt")
}

// GetCursorFilePath returns the path to the cursor storage file
func GetCursorFilePath() string {
	return GetStoragePath("commands", "cursor.txt")
}

// GetHeartbeatLogFilePath returns the path to the heartbeat log file
func GetHeartbeatLogFilePath() string {
	return GetStoragePath("coding-heartbeat.data.log")
}

// GetSyncPendingFilePath returns the path to the sync pending file
func GetSyncPendingFilePath() string {
	return GetStoragePath("sync-pending.jsonl")
}

// GetBinFolderPath returns the path to the bin folder
func GetBinFolderPath() string {
	return GetStoragePath("bin")
}

// GetHooksFolderPath returns the path to the hooks folder
func GetHooksFolderPath() string {
	return GetStoragePath("hooks")
}

// GetDaemonLogsPath returns the path to the daemon logs folder
func GetDaemonLogsPath() string {
	return GetStoragePath("logs")
}

// GetDaemonLogFilePath returns the path to the daemon output log file (macOS)
func GetDaemonLogFilePath() string {
	return GetStoragePath("logs", "shelltime-daemon.log")
}

// GetDaemonErrFilePath returns the path to the daemon error log file (macOS)
func GetDaemonErrFilePath() string {
	return GetStoragePath("logs", "shelltime-daemon.err")
}

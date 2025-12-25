package model

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetBaseStoragePath returns the base storage path for shelltime
// e.g., /Users/username/.shelltime
func GetBaseStoragePath() string {
	return os.ExpandEnv("$HOME/" + COMMAND_BASE_STORAGE_FOLDER)
}

// GetStoragePath returns the full path for a given subpath within the storage folder
// e.g., GetStoragePath("config.toml") returns /Users/username/.shelltime/config.toml
func GetStoragePath(subpath string) string {
	return filepath.Join(GetBaseStoragePath(), subpath)
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
	return os.ExpandEnv("$HOME/" + COMMAND_STORAGE_FOLDER)
}

// GetPreCommandFilePath returns the path to the pre-command storage file
func GetPreCommandFilePath() string {
	return os.ExpandEnv("$HOME/" + COMMAND_PRE_STORAGE_FILE)
}

// GetPostCommandFilePath returns the path to the post-command storage file
func GetPostCommandFilePath() string {
	return os.ExpandEnv("$HOME/" + COMMAND_POST_STORAGE_FILE)
}

// GetCursorFilePath returns the path to the cursor storage file
func GetCursorFilePath() string {
	return os.ExpandEnv("$HOME/" + COMMAND_CURSOR_STORAGE_FILE)
}

// GetHeartbeatLogFilePath returns the path to the heartbeat log file
func GetHeartbeatLogFilePath() string {
	return os.ExpandEnv("$HOME/" + HEARTBEAT_LOG_FILE)
}

// GetSyncPendingFilePath returns the path to the sync pending file
func GetSyncPendingFilePath() string {
	return os.ExpandEnv("$HOME/" + SYNC_PENDING_FILE)
}

// GetBinFolderPath returns the path to the bin folder
func GetBinFolderPath() string {
	return os.ExpandEnv(fmt.Sprintf("$HOME/%s/bin", COMMAND_BASE_STORAGE_FOLDER))
}

// GetHooksFolderPath returns the path to the hooks folder
func GetHooksFolderPath() string {
	return os.ExpandEnv(fmt.Sprintf("$HOME/%s/hooks", COMMAND_BASE_STORAGE_FOLDER))
}

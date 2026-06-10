package model

import (
	"fmt"
	"os"
	"os/exec"
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

// GetConfigFilePath returns the path to the main config file (TOML)
func GetConfigFilePath() string {
	return GetStoragePath("config.toml")
}

// GetLocalConfigFilePath returns the path to the local config file (TOML)
func GetLocalConfigFilePath() string {
	return GetStoragePath("config.local.toml")
}

// GetYAMLConfigFilePath returns the path to the main config file (YAML)
func GetYAMLConfigFilePath() string {
	return GetStoragePath("config.yaml")
}

// GetYMLConfigFilePath returns the path to the main config file (YML)
func GetYMLConfigFilePath() string {
	return GetStoragePath("config.yml")
}

// GetLocalYAMLConfigFilePath returns the path to the local config file (YAML)
func GetLocalYAMLConfigFilePath() string {
	return GetStoragePath("config.local.yaml")
}

// GetLocalYMLConfigFilePath returns the path to the local config file (YML)
func GetLocalYMLConfigFilePath() string {
	return GetStoragePath("config.local.yml")
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

// GetBoltDBPath returns the path to the bbolt command database (daemon-owned).
func GetBoltDBPath() string {
	return GetStoragePath("commands", "commands.db")
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

// GetCurlInstallerDaemonPath returns the legacy curl-installer daemon location
// (~/.shelltime/bin/shelltime-daemon).
func GetCurlInstallerDaemonPath() string {
	return filepath.Join(GetBaseStoragePath(), "bin", "shelltime-daemon")
}

// daemonHomebrewSearchPaths lists explicit Homebrew/Linuxbrew bin dirs to
// probe when PATH is stripped (e.g. launchd-spawned shells). Exposed as a var
// so tests can swap it out.
var daemonHomebrewSearchPaths = []string{
	"/opt/homebrew/bin",
	"/usr/local/bin",
	"/home/linuxbrew/.linuxbrew/bin",
}

// ResolveDaemonBinaryPath finds the shelltime-daemon binary.
// It prefers a system-managed binary (Homebrew or anything on PATH) over the
// legacy curl-installer location, so that `brew upgrade shelltime` is what
// actually drives the running daemon.
func ResolveDaemonBinaryPath() (string, error) {
	const binaryName = "shelltime-daemon"
	curlPath := GetCurlInstallerDaemonPath()

	// 1. Check PATH (covers Homebrew and other package managers). Ignore the
	// result if it happens to resolve to the curl-installer path — we want
	// step 3 to be the only branch that returns that location.
	if path, err := exec.LookPath(binaryName); err == nil {
		if resolved, _ := filepath.Abs(path); resolved != curlPath {
			return path, nil
		}
	}

	// 2. Explicit Homebrew/Linuxbrew fallback paths, in case PATH was stripped.
	for _, dir := range daemonHomebrewSearchPaths {
		p := filepath.Join(dir, binaryName)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}

	// 3. Curl-installer fallback.
	if info, err := os.Stat(curlPath); err == nil && !info.IsDir() {
		return curlPath, nil
	}

	return "", fmt.Errorf("%s not found on PATH, in standard Homebrew locations, or at %s", binaryName, curlPath)
}

package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetBaseStoragePath(t *testing.T) {
	path := GetBaseStoragePath()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		// If home dir is not available, should fallback to temp dir
		if !strings.Contains(path, os.TempDir()) {
			t.Errorf("Expected path to be in temp dir when home not available, got %s", path)
		}
	} else {
		expected := filepath.Join(homeDir, COMMAND_BASE_STORAGE_FOLDER)
		if path != expected {
			t.Errorf("Expected %s, got %s", expected, path)
		}
	}
}

func TestGetStoragePath(t *testing.T) {
	basePath := GetBaseStoragePath()

	testCases := []struct {
		name     string
		subpaths []string
		expected string
	}{
		{
			"single subpath",
			[]string{"config.toml"},
			filepath.Join(basePath, "config.toml"),
		},
		{
			"multiple subpaths",
			[]string{"commands", "pre.txt"},
			filepath.Join(basePath, "commands", "pre.txt"),
		},
		{
			"no subpaths",
			[]string{},
			basePath,
		},
		{
			"nested subpaths",
			[]string{"logs", "daemon", "output.log"},
			filepath.Join(basePath, "logs", "daemon", "output.log"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetStoragePath(tc.subpaths...)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetConfigFilePath(t *testing.T) {
	path := GetConfigFilePath()
	expected := GetStoragePath("config.toml")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}

	if !strings.HasSuffix(path, "config.toml") {
		t.Error("Config file path should end with config.toml")
	}
}

func TestGetLocalConfigFilePath(t *testing.T) {
	path := GetLocalConfigFilePath()
	expected := GetStoragePath("config.local.toml")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}

	if !strings.HasSuffix(path, "config.local.toml") {
		t.Error("Local config file path should end with config.local.toml")
	}
}

func TestGetYAMLConfigFilePath(t *testing.T) {
	path := GetYAMLConfigFilePath()
	expected := GetStoragePath("config.yaml")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetYMLConfigFilePath(t *testing.T) {
	path := GetYMLConfigFilePath()
	expected := GetStoragePath("config.yml")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetLocalYAMLConfigFilePath(t *testing.T) {
	path := GetLocalYAMLConfigFilePath()
	expected := GetStoragePath("config.local.yaml")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetLocalYMLConfigFilePath(t *testing.T) {
	path := GetLocalYMLConfigFilePath()
	expected := GetStoragePath("config.local.yml")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetLogFilePath(t *testing.T) {
	path := GetLogFilePath()
	expected := GetStoragePath("log.log")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetCommandsStoragePath(t *testing.T) {
	path := GetCommandsStoragePath()
	expected := GetStoragePath("commands")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetPreCommandFilePath(t *testing.T) {
	path := GetPreCommandFilePath()
	expected := GetStoragePath("commands", "pre.txt")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetPostCommandFilePath(t *testing.T) {
	path := GetPostCommandFilePath()
	expected := GetStoragePath("commands", "post.txt")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetCursorFilePath(t *testing.T) {
	path := GetCursorFilePath()
	expected := GetStoragePath("commands", "cursor.txt")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetHeartbeatLogFilePath(t *testing.T) {
	path := GetHeartbeatLogFilePath()
	expected := GetStoragePath("coding-heartbeat.data.log")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetSyncPendingFilePath(t *testing.T) {
	path := GetSyncPendingFilePath()
	expected := GetStoragePath("sync-pending.jsonl")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetBinFolderPath(t *testing.T) {
	path := GetBinFolderPath()
	expected := GetStoragePath("bin")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetHooksFolderPath(t *testing.T) {
	path := GetHooksFolderPath()
	expected := GetStoragePath("hooks")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetDaemonLogsPath(t *testing.T) {
	path := GetDaemonLogsPath()
	expected := GetStoragePath("logs")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetDaemonLogFilePath(t *testing.T) {
	path := GetDaemonLogFilePath()
	expected := GetStoragePath("logs", "shelltime-daemon.log")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestGetDaemonErrFilePath(t *testing.T) {
	path := GetDaemonErrFilePath()
	expected := GetStoragePath("logs", "shelltime-daemon.err")

	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}
}

func TestPathConsistency(t *testing.T) {
	// All paths should be absolute
	paths := []struct {
		name string
		path string
	}{
		{"BaseStoragePath", GetBaseStoragePath()},
		{"ConfigFilePath", GetConfigFilePath()},
		{"LocalConfigFilePath", GetLocalConfigFilePath()},
		{"LogFilePath", GetLogFilePath()},
		{"CommandsStoragePath", GetCommandsStoragePath()},
		{"PreCommandFilePath", GetPreCommandFilePath()},
		{"PostCommandFilePath", GetPostCommandFilePath()},
		{"CursorFilePath", GetCursorFilePath()},
		{"HeartbeatLogFilePath", GetHeartbeatLogFilePath()},
		{"SyncPendingFilePath", GetSyncPendingFilePath()},
		{"BinFolderPath", GetBinFolderPath()},
		{"HooksFolderPath", GetHooksFolderPath()},
		{"DaemonLogsPath", GetDaemonLogsPath()},
		{"DaemonLogFilePath", GetDaemonLogFilePath()},
		{"DaemonErrFilePath", GetDaemonErrFilePath()},
	}

	basePath := GetBaseStoragePath()
	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			// All paths should start with base path
			if !strings.HasPrefix(p.path, basePath) {
				t.Errorf("%s should start with base path %s, got %s", p.name, basePath, p.path)
			}

			// Paths should be absolute (start with /)
			if !filepath.IsAbs(p.path) {
				t.Errorf("%s should be an absolute path, got %s", p.name, p.path)
			}
		})
	}
}

func TestPathsAreClean(t *testing.T) {
	// All paths should be clean (no . or ..)
	paths := []string{
		GetBaseStoragePath(),
		GetConfigFilePath(),
		GetPreCommandFilePath(),
		GetPostCommandFilePath(),
	}

	for _, p := range paths {
		cleaned := filepath.Clean(p)
		if p != cleaned {
			t.Errorf("Path %s is not clean (cleaned: %s)", p, cleaned)
		}
	}
}

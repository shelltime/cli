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

// writeFakeDaemon creates an executable file at the given path so exec.LookPath
// will accept it as resolvable. Returns the absolute path actually written.
func writeFakeDaemon(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to mkdir %s: %v", dir, err)
	}
	p := filepath.Join(dir, "shelltime-daemon")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake daemon: %v", err)
	}
	return p
}

// withIsolatedDaemonResolution sets up a hermetic environment for
// ResolveDaemonBinaryPath: a temp $HOME, an empty $PATH (caller can re-add
// dirs), and an empty Homebrew search list so the host machine's installs
// (e.g. real /opt/homebrew/bin/shelltime-daemon) don't leak into assertions.
// Returns the temp home directory.
func withIsolatedDaemonResolution(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")

	prev := daemonHomebrewSearchPaths
	daemonHomebrewSearchPaths = nil
	t.Cleanup(func() { daemonHomebrewSearchPaths = prev })

	return home
}

func TestResolveDaemonBinaryPath(t *testing.T) {
	t.Run("returns curl-installer path when nothing else is on PATH", func(t *testing.T) {
		home := withIsolatedDaemonResolution(t)

		curl := writeFakeDaemon(t, filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin"))

		got, err := ResolveDaemonBinaryPath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != curl {
			t.Errorf("expected %s, got %s", curl, got)
		}
	})

	t.Run("prefers PATH-resolved binary over curl-installer path", func(t *testing.T) {
		home := withIsolatedDaemonResolution(t)

		brewDir := t.TempDir()
		brewPath := writeFakeDaemon(t, brewDir)
		t.Setenv("PATH", brewDir)

		// stale curl-installer copy that should be ignored
		writeFakeDaemon(t, filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin"))

		got, err := ResolveDaemonBinaryPath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != brewPath {
			t.Errorf("expected PATH-resolved %s, got %s", brewPath, got)
		}
	})

	t.Run("uses explicit Homebrew search list when PATH is empty", func(t *testing.T) {
		home := withIsolatedDaemonResolution(t)

		brewDir := t.TempDir()
		brewPath := writeFakeDaemon(t, brewDir)
		daemonHomebrewSearchPaths = []string{brewDir}

		// stale curl-installer copy that should be ignored
		writeFakeDaemon(t, filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin"))

		got, err := ResolveDaemonBinaryPath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != brewPath {
			t.Errorf("expected Homebrew search result %s, got %s", brewPath, got)
		}
	})

	t.Run("skips PATH result that points back at the curl-installer path", func(t *testing.T) {
		home := withIsolatedDaemonResolution(t)

		curlBin := filepath.Join(home, COMMAND_BASE_STORAGE_FOLDER, "bin")
		curl := writeFakeDaemon(t, curlBin)
		// Put only the curl-installer dir on PATH; LookPath would return curl,
		// but the resolver must fall through and still return the curl path
		// from step 3 (so the cleanup logic in daemon.install can detect it).
		t.Setenv("PATH", curlBin)

		got, err := ResolveDaemonBinaryPath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != curl {
			t.Errorf("expected curl-installer fallback %s, got %s", curl, got)
		}
	})

	t.Run("returns error when no daemon is found anywhere", func(t *testing.T) {
		withIsolatedDaemonResolution(t)

		_, err := ResolveDaemonBinaryPath()
		if err == nil {
			t.Fatal("expected error when no daemon binary exists, got nil")
		}
	})
}

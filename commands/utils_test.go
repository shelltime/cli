package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath_Tilde(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	testCases := []struct {
		input    string
		expected string
	}{
		{"~", homeDir},
		{"~/", homeDir},
		{"~/Documents", filepath.Join(homeDir, "Documents")},
		{"~/.config/shelltime", filepath.Join(homeDir, ".config/shelltime")},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := expandPath(tc.input)
			if err != nil {
				t.Fatalf("expandPath(%q) failed: %v", tc.input, err)
			}
			if result != tc.expected {
				t.Errorf("expandPath(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestExpandPath_Absolute(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"/usr/bin", "/usr/bin"},
		{"/tmp", "/tmp"},
		{"/home/user/file.txt", "/home/user/file.txt"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := expandPath(tc.input)
			if err != nil {
				t.Fatalf("expandPath(%q) failed: %v", tc.input, err)
			}
			if result != tc.expected {
				t.Errorf("expandPath(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestExpandPath_Relative(t *testing.T) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Cannot get cwd: %v", err)
	}

	testCases := []struct {
		input    string
		expected string
	}{
		{"file.txt", filepath.Join(cwd, "file.txt")},
		{"subdir/file.txt", filepath.Join(cwd, "subdir/file.txt")},
		{"./file.txt", filepath.Join(cwd, "file.txt")},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := expandPath(tc.input)
			if err != nil {
				t.Fatalf("expandPath(%q) failed: %v", tc.input, err)
			}
			if result != tc.expected {
				t.Errorf("expandPath(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestAdjustPathForCurrentUser_UsersPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"Users path with 4 parts",
			"/Users/someuser/Documents/file.txt",
			homeDir + "/Documents/file.txt",
		},
		{
			"Users path with nested dirs",
			"/Users/anotheruser/projects/app/src/main.go",
			homeDir + "/projects/app/src/main.go",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AdjustPathForCurrentUser(tc.input)
			if result != tc.expected {
				t.Errorf("AdjustPathForCurrentUser(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestAdjustPathForCurrentUser_HomePath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"home path with 4 parts",
			"/home/someuser/Documents/file.txt",
			homeDir + "/Documents/file.txt",
		},
		{
			"home path with nested dirs",
			"/home/anotheruser/.config/app/config.yaml",
			homeDir + "/.config/app/config.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AdjustPathForCurrentUser(tc.input)
			if result != tc.expected {
				t.Errorf("AdjustPathForCurrentUser(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestAdjustPathForCurrentUser_RootPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"root path",
			"/root/.bashrc",
			homeDir + "/.bashrc",
		},
		{
			"root path with subdir",
			"/root/scripts/deploy.sh",
			homeDir + "/scripts/deploy.sh",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AdjustPathForCurrentUser(tc.input)
			if result != tc.expected {
				t.Errorf("AdjustPathForCurrentUser(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestAdjustPathForCurrentUser_NoMatch(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"absolute path", "/usr/bin/ls"},
		{"var path", "/var/log/messages"},
		{"tmp path", "/tmp/file.txt"},
		{"etc path", "/etc/hosts"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := AdjustPathForCurrentUser(tc.input)
			// Should return unchanged
			if result != tc.input {
				t.Errorf("AdjustPathForCurrentUser(%q) = %q, expected unchanged", tc.input, result)
			}
		})
	}
}

func TestAdjustPathForCurrentUser_ShortPaths(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"short Users path", "/Users/user"},
		{"short home path", "/home/user"},
		{"very short", "/home"},
		{"root only", "/"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic on short paths
			result := AdjustPathForCurrentUser(tc.input)
			// Just verify it doesn't panic and returns something
			if result == "" {
				t.Error("Result should not be empty")
			}
		})
	}
}

func TestAdjustPathForCurrentUser_EmptyPath(t *testing.T) {
	result := AdjustPathForCurrentUser("")
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestAdjustPathForCurrentUser_PreservesSubpath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	input := "/Users/someuser/very/deeply/nested/path/to/file.txt"
	result := AdjustPathForCurrentUser(input)

	// Should preserve the subpath after username
	expectedSuffix := "/very/deeply/nested/path/to/file.txt"
	if !strings.HasSuffix(result, expectedSuffix) {
		t.Errorf("Result should preserve subpath. Got: %q, expected suffix: %q", result, expectedSuffix)
	}

	// Should start with home dir
	if !strings.HasPrefix(result, homeDir) {
		t.Errorf("Result should start with home dir. Got: %q, expected prefix: %q", result, homeDir)
	}
}

func TestExpandPath_EmptyString(t *testing.T) {
	// Empty string should be expanded to current directory
	cwd, _ := os.Getwd()
	result, err := expandPath("")
	if err != nil {
		t.Fatalf("expandPath(\"\") failed: %v", err)
	}
	if result != cwd {
		t.Errorf("expandPath(\"\") = %q, expected %q", result, cwd)
	}
}

func TestExpandPath_SingleDot(t *testing.T) {
	cwd, _ := os.Getwd()
	result, err := expandPath(".")
	if err != nil {
		t.Fatalf("expandPath(\".\") failed: %v", err)
	}
	if result != cwd {
		t.Errorf("expandPath(\".\") = %q, expected %q", result, cwd)
	}
}

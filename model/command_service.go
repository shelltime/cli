package model

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

// CommandService defines the interface for command-related operations
type CommandService interface {
	LookPath(name string) (string, error)
}

// commandService implements the CommandService interface
type commandService struct{}

// NewCommandService creates a new command service
func NewCommandService() CommandService {
	return &commandService{}
}

// LookPath searches for an executable in common locations, falling back to system PATH.
// This is necessary because when running as a daemon service, the PATH environment
// variable may not include user-specific Node.js installation paths.
func (s *commandService) LookPath(name string) (string, error) {
	// First, try the standard exec.LookPath which checks system PATH
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	// Get the current user's home directory
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH and unable to get user home directory: %w", name, err)
	}
	homeDir := currentUser.HomeDir

	// Common installation locations for node package managers
	var searchPaths []string

	if runtime.GOOS == "windows" {
		// Windows paths
		searchPaths = []string{
			filepath.Join(homeDir, "AppData", "Roaming", "npm", name+".cmd"),
			filepath.Join(homeDir, "AppData", "Roaming", "npm", name+".ps1"),
			filepath.Join(homeDir, ".bun", "bin", name+".exe"),
			filepath.Join(homeDir, ".bun", "bin", name),
			filepath.Join(os.Getenv("ProgramFiles"), "nodejs", name+".cmd"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "nodejs", name+".cmd"),
		}
	} else {
		// Unix-like systems (Linux, macOS, etc.)
		searchPaths = []string{
			// User-specific npm global installations
			filepath.Join(homeDir, ".npm-global", "bin", name),
			filepath.Join(homeDir, ".npm", "bin", name),
			// User-specific pnpm installation
			filepath.Join(homeDir, ".local", "share", "pnpm", name),
			// Bun installation
			filepath.Join(homeDir, ".bun", "bin", name),
			// NVM (Node Version Manager) current version
			filepath.Join(homeDir, ".nvm", "current", "bin", name),
			// fnm (Fast Node Manager) installations
			filepath.Join(homeDir, ".local", "share", "fnm", "node-versions", "*", "installation", "bin", name),
			filepath.Join(homeDir, ".fnm", "node-versions", "*", "installation", "bin", name),
			// Homebrew on macOS (Intel)
			filepath.Join("/usr/local/bin", name),
			// Homebrew on macOS (Apple Silicon)
			filepath.Join("/opt/homebrew/bin", name),
			// Common system paths
			filepath.Join("/usr/bin", name),
			filepath.Join("/bin", name),
		}

		// Add Node.js versions from nvm if NVM_DIR is set
		if nvmDir := os.Getenv("NVM_DIR"); nvmDir != "" {
			// Try to find the default/current version
			searchPaths = append(searchPaths,
				filepath.Join(nvmDir, "current", "bin", name),
				filepath.Join(nvmDir, "versions", "node", "*", "bin", name),
			)
		}

		// Add Node.js versions from fnm if FNM_DIR is set
		if fnmDir := os.Getenv("FNM_DIR"); fnmDir != "" {
			searchPaths = append(searchPaths,
				filepath.Join(fnmDir, "node-versions", "*", "installation", "bin", name),
			)
		}
	}

	// Search each path
	for _, path := range searchPaths {
		// Handle glob patterns (like nvm versions)
		if matches, err := filepath.Glob(path); err == nil && len(matches) > 0 {
			// Use the last match, which is likely to be the latest version
			// since Glob returns a sorted list.
			path = matches[len(matches)-1]
		}

		// Check if the file exists and is executable
		if info, err := os.Stat(path); err == nil {
			if !info.IsDir() {
				// On Unix-like systems, check if it's executable
				if runtime.GOOS != "windows" {
					if info.Mode()&0111 == 0 {
						continue
					}
				}
				slog.Debug("Found executable", "name", name, "path", path)
				return path, nil
			}
		}
	}

	// As a last resort, try using 'which' command from the user's current shell
	// This can help find the binary in user-specific PATH configurations
	slog.Debug("Trying to find executable using 'which' command", "name", name)

	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
		} else {
			shell = "/bin/sh"
		}
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// On Windows, use 'where' command instead of 'which'
		cmd = exec.Command("cmd", "/c", "where", name)
	} else {
		// On Unix-like systems, use the shell to run 'which'
		// Using shell -l to load the login shell environment
		cmd = exec.Command(shell, "-l", "-c", fmt.Sprintf("which %s", name))
	}

	output, err := cmd.Output()
	if err == nil {
		// Trim whitespace and newlines from the output
		path := strings.TrimSpace(string(output))
		if path != "" {
			// Verify the path exists and is executable
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				if runtime.GOOS != "windows" {
					if info.Mode()&0111 != 0 {
						slog.Debug("Found executable via shell which command", "name", name, "path", path, "shell", shell)
						return path, nil
					}
				} else {
					slog.Debug("Found executable via where command", "name", name, "path", path)
					return path, nil
				}
			}
		}
	} else {
		slog.Debug("Failed to find executable via shell command", "name", name, "error", err)
	}

	return "", fmt.Errorf("%s not found in PATH, common installation locations, or via shell which command", name)
}

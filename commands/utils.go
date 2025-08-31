package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(homeDir, path[1:]), nil
	}
	return filepath.Abs(path)
}

// AdjustPathForCurrentUser replaces the username in the path with the current user's home directory
func AdjustPathForCurrentUser(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logrus.Warnf("Failed to get home directory: %v", err)
		return path
	}

	// Handle paths with /Users/username or /home/username
	if strings.HasPrefix(path, "/Users/") {
		parts := strings.SplitN(path, "/", 4) // ["", "Users", "username", "rest..."]
		if len(parts) >= 4 {
			return fmt.Sprintf("%s/%s", homeDir, parts[3])
		}
	} else if strings.HasPrefix(path, "/home/") {
		parts := strings.SplitN(path, "/", 4) // ["", "home", "username", "rest..."]
		if len(parts) >= 4 {
			return fmt.Sprintf("%s/%s", homeDir, parts[3])
		}
	} else if strings.HasPrefix(path, "/root/") {
		// For root user paths
		parts := strings.SplitN(path, "/", 3) // ["", "root", "rest..."]
		if len(parts) >= 3 {
			return fmt.Sprintf("%s/%s", homeDir, parts[2])
		}
	}

	// If no standard pattern matched, return the path as-is
	return path
}

package commands

import (
	"os"
	"path/filepath"
)

// cursorDetected reports whether Cursor looks installed: ~/.cursor exists, or cursor or
// cursor-agent is on PATH.
func cursorDetected(lookPath func(string) (string, error)) bool {
	return homeDirExists(".cursor") || onPath(lookPath, "cursor", "cursor-agent")
}

// antigravityDetected reports whether Google Antigravity looks installed: the app's or the
// agy CLI's folder under ~/.gemini exists, or agy is on PATH.
func antigravityDetected(lookPath func(string) (string, error)) bool {
	return homeDirExists(filepath.Join(".gemini", "antigravity")) ||
		homeDirExists(filepath.Join(".gemini", "antigravity-cli")) ||
		onPath(lookPath, "agy")
}

func homeDirExists(rel string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(home, rel))
	return err == nil && info.IsDir()
}

func onPath(lookPath func(string) (string, error), names ...string) bool {
	for _, name := range names {
		if _, err := lookPath(name); err == nil {
			return true
		}
	}
	return false
}

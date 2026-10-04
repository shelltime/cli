package daemon

import (
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// usageSyncInterval is how often the daemon syncs Claude and Codex usage in the background.
const usageSyncInterval = 10 * time.Minute

// Process name keywords (lowercase) that mean an AI coding client is running.
var (
	claudeProcessKeywords = []string{"claude"}
	codexProcessKeywords  = []string{"codex", "chatgpt"}
)

// listProcessNamesFunc lists running process names. It is a var so tests can stub it.
var listProcessNamesFunc = listProcessNames

// listProcessNames returns the command name of every running process.
// macOS reports the full executable path, Linux the short comm name.
func listProcessNames() ([]string, error) {
	out, err := exec.Command("ps", "-A", "-o", "comm=").Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// isProcessRunning reports whether any running process name contains one of keywords (case-insensitive).
// Only the executable basename is matched, so helpers living under ~/.claude or ~/.codex don't count.
// If the process list can't be read it returns true, so usage sync falls back to always fetching.
func isProcessRunning(keywords []string) bool {
	names, err := listProcessNamesFunc()
	if err != nil {
		slog.Debug("Failed to list processes, assuming client is running", slog.Any("err", err))
		return true
	}

	for _, name := range names {
		if matchKnownName(filepath.Base(name), keywords) != "" {
			return true
		}
	}
	return false
}

func isClaudeRunning() bool {
	return isProcessRunning(claudeProcessKeywords)
}

func isCodexRunning() bool {
	return isProcessRunning(codexProcessKeywords)
}

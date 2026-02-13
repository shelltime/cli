package daemon

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

const gitCmdTimeout = 2 * time.Second

// GitInfo contains git repository information
type GitInfo struct {
	Branch string
	Dirty  bool
	IsRepo bool
}

// GetGitInfo returns git branch and dirty status for a directory.
// It uses the native git CLI which is significantly faster and more memory-efficient
// than the pure-Go go-git implementation, especially on large repositories.
func GetGitInfo(workingDir string) GitInfo {
	if workingDir == "" {
		return GitInfo{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
	defer cancel()

	// Check if this is a git repo
	if err := exec.CommandContext(ctx, "git", "-C", workingDir, "rev-parse", "--git-dir").Run(); err != nil {
		return GitInfo{}
	}

	info := GitInfo{IsRepo: true}

	// Get branch name from HEAD
	if out, err := exec.CommandContext(ctx, "git", "-C", workingDir, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		info.Branch = strings.TrimSpace(string(out))
	}

	// Check dirty status via porcelain output (empty = clean)
	if out, err := exec.CommandContext(ctx, "git", "-C", workingDir, "status", "--porcelain").Output(); err == nil {
		info.Dirty = len(strings.TrimSpace(string(out))) > 0
	}

	return info
}

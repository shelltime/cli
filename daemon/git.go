package daemon

import (
	"github.com/go-git/go-git/v5"
)

// GitInfo contains git repository information
type GitInfo struct {
	Branch string
	Dirty  bool
	IsRepo bool
}

// GetGitInfo returns git branch and dirty status for a directory.
// It walks up the directory tree to find the git repository root.
func GetGitInfo(workingDir string) GitInfo {
	if workingDir == "" {
		return GitInfo{}
	}

	repo, err := git.PlainOpenWithOptions(workingDir, &git.PlainOpenOptions{
		DetectDotGit: true, // Walk up to find .git
	})
	if err != nil {
		return GitInfo{} // Not a git repo
	}

	info := GitInfo{IsRepo: true}

	// Get branch name from HEAD
	head, err := repo.Head()
	if err == nil {
		info.Branch = head.Name().Short()
	}

	// Check dirty status by examining worktree status
	worktree, err := repo.Worktree()
	if err == nil {
		status, err := worktree.Status()
		if err == nil {
			info.Dirty = !status.IsClean()
		}
	}

	return info
}

package daemon

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/malamtime/cli/model"
)

const (
	gitContextCommits = 3
	gitContextRemotes = 5
)

// GetGitContext describes the repository containing dir for `shelltime q`,
// or returns nil when dir is not inside a git work tree. Every git call
// shares ctx, so the caller's deadline bounds the whole collection. When
// `git status` misses it, the branch is read from HEAD directly and
// StatusIncomplete is set.
func GetGitContext(ctx context.Context, dir string) *model.QueryGitContext {
	if dir == "" {
		return nil
	}
	out, err := gitCmd(ctx, "-C", dir, "rev-parse", "--show-toplevel", "--absolute-git-dir", "--show-prefix").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 2 {
		return nil
	}
	gitDir := lines[1]
	gc := &model.QueryGitContext{}
	if len(lines) > 2 {
		gc.PathInRepo = model.SanitizeContextString(strings.TrimSuffix(lines[2], "/"), model.QueryContextMaxRunes)
	}

	var (
		wg                      sync.WaitGroup
		status, logOut, remotes []byte
		statusErr               error
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		status, statusErr = gitCmd(ctx, "-C", dir, "status", "--porcelain=v2", "--branch", "--ignore-submodules=dirty").Output()
	}()
	go func() {
		defer wg.Done()
		logOut, _ = gitCmd(ctx, "-C", dir, "log", "-n", strconv.Itoa(gitContextCommits), "--no-show-signature", "--format=%h %s").Output()
	}()
	go func() {
		defer wg.Done()
		remotes, _ = gitCmd(ctx, "-C", dir, "remote", "-v").Output()
	}()
	gc.Operation = detectGitOperation(gitDir)
	wg.Wait()

	if statusErr == nil {
		parsePorcelainV2(string(status), gc)
	} else {
		gc.StatusIncomplete = true
		gc.Branch, gc.Detached = readGitHEAD(gitDir)
	}
	gc.RecentCommits = parseGitLog(string(logOut))
	gc.Remotes = parseGitRemotes(string(remotes))
	return gc
}

// parsePorcelainV2 fills branch, upstream, ahead/behind and file counts from
// `git status --porcelain=v2 --branch` output.
func parsePorcelainV2(out string, gc *model.QueryGitContext) {
	var oid string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			oid = strings.TrimPrefix(line, "# branch.oid ")
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head == "(detached)" {
				gc.Detached = true
			} else {
				gc.Branch = model.SanitizeContextString(head, model.QueryContextMaxRunes)
			}
		case strings.HasPrefix(line, "# branch.upstream "):
			gc.Upstream = model.SanitizeContextString(strings.TrimPrefix(line, "# branch.upstream "), model.QueryContextMaxRunes)
		case strings.HasPrefix(line, "# branch.ab "):
			for _, f := range strings.Fields(strings.TrimPrefix(line, "# branch.ab ")) {
				n, err := strconv.Atoi(f[1:])
				if err != nil {
					continue
				}
				switch f[0] {
				case '+':
					gc.Ahead = n
				case '-':
					gc.Behind = n
				}
			}
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			if len(line) < 4 {
				continue
			}
			if line[2] != '.' {
				gc.Staged++
			}
			if line[3] != '.' {
				gc.Unstaged++
			}
		case strings.HasPrefix(line, "u "):
			gc.Conflicted++
		case strings.HasPrefix(line, "? "):
			gc.Untracked++
		}
	}
	if gc.Detached && len(oid) >= 7 && oid != "(initial)" {
		gc.Branch = oid[:7]
	}
}

// readGitHEAD reads the branch straight from the HEAD file, for when
// `git status` did not finish in time.
func readGitHEAD(gitDir string) (branch string, detached bool) {
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	head := strings.TrimSpace(string(b))
	if ref, ok := strings.CutPrefix(head, "ref: "); ok {
		return model.SanitizeContextString(strings.TrimPrefix(ref, "refs/heads/"), model.QueryContextMaxRunes), false
	}
	if len(head) >= 7 {
		return head[:7], true
	}
	return "", false
}

// detectGitOperation reports an in-progress merge, rebase, am, cherry-pick,
// revert or bisect from the marker files git leaves in the git directory.
func detectGitOperation(gitDir string) string {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(gitDir, name))
		return err == nil
	}
	switch {
	case exists("rebase-merge"):
		return "rebase"
	case exists("rebase-apply"):
		if exists(filepath.Join("rebase-apply", "applying")) {
			return "am"
		}
		return "rebase"
	case exists("MERGE_HEAD"):
		return "merge"
	case exists("CHERRY_PICK_HEAD"):
		return "cherry-pick"
	case exists("REVERT_HEAD"):
		return "revert"
	case exists("BISECT_LOG"):
		return "bisect"
	}
	return ""
}

// parseGitLog returns "<short sha> <subject>" lines, sanitized and capped.
func parseGitLog(out string) []string {
	var commits []string
	for _, line := range strings.Split(out, "\n") {
		if len(commits) >= gitContextCommits {
			break
		}
		if s := model.SanitizeContextString(line, 100); s != "" {
			commits = append(commits, s)
		}
	}
	return commits
}

// parseGitRemotes reduces `git remote -v` output to "name=host" entries.
// Only the host is kept: credentials, owners and repository paths are
// dropped.
func parseGitRemotes(out string) []string {
	var remotes []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		host := remoteHost(fields[1])
		if host == "" {
			continue
		}
		entry := model.SanitizeContextString(fields[0]+"="+host, 100)
		remotes = append(remotes, entry)
		if len(remotes) >= gitContextRemotes {
			break
		}
	}
	return remotes
}

// remoteHost extracts the host from a git remote URL: scheme URLs
// (https://user:token@host/x), scp-like addresses (git@host:x) and local
// paths ("local").
func remoteHost(raw string) string {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		if u.Scheme == "file" {
			return "local"
		}
		return u.Hostname()
	}
	// A one-letter prefix is a Windows drive (C:\repo), not an scp host
	if before, _, ok := strings.Cut(raw, ":"); ok && len(before) > 1 && !strings.ContainsAny(before, "/\\") {
		if _, host, found := strings.Cut(before, "@"); found {
			return host
		}
		return before
	}
	return "local"
}

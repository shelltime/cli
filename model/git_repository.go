package model

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// AICodeRepositoryAttribute is the OTEL attribute that names the git repository a coding agent
// ran in. Claude Code sets it itself (OTEL_METRICS_INCLUDE_REPOSITORY, which `cc install`
// turns on); the daemon and backfill add it from the working directory when a record lacks
// it. The server groups projects by the repository instead of the folder, so a renamed or
// moved clone, a worktree or a subfolder stays one project.
const AICodeRepositoryAttribute = "vcs.repository.url.full"

// CanonicalRepositoryURL turns a git remote URL into https://<host>/<path>, the form Claude
// Code reports: scp-like (git@host:owner/repo.git), ssh://, git:// and http(s) remotes all
// become the same URL. Credentials, the port, a query, a fragment, trailing slashes and the
// .git suffix are dropped, and the host is lowercased; the path keeps its case. It returns
// false for a remote that is a local path or not a URL at all.
func CanonicalRepositoryURL(remote string) (string, bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" || strings.ContainsAny(remote, "\\ \t\r\n") {
		return "", false
	}

	var host, path string
	if i := strings.Index(remote, "://"); i >= 0 {
		u, err := url.Parse(remote)
		if err != nil {
			return "", false
		}
		switch strings.ToLower(u.Scheme) {
		case "https", "http", "ssh", "git", "git+ssh", "ssh+git":
		default:
			return "", false // file://, and anything git can't fetch from a host
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp-like: [user@]host:path. A colon after a slash is part of a local path.
		colon := strings.Index(remote, ":")
		if colon <= 0 || strings.Contains(remote[:colon], "/") {
			return "", false
		}
		host = remote[:colon]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		path = remote[colon+1:]
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	// A one-letter "host" is a Windows drive (C:/repo), not a server.
	if len(host) < 2 || strings.ContainsAny(host, "/@[]") {
		return "", false
	}

	segments := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if len(segments) > 0 {
		last := len(segments) - 1
		segments[last] = strings.TrimSuffix(segments[last], ".git")
		if segments[last] == "" {
			segments = segments[:last]
		}
	}
	if len(segments) == 0 || slices.Contains(segments, "..") || slices.Contains(segments, ".") {
		return "", false
	}
	return "https://" + host + "/" + strings.Join(segments, "/"), true
}

// GitRepository is what git says about a directory.
type GitRepository struct {
	// Root is the top level of the work tree; empty when the directory isn't in one.
	Root string
	// URL is the canonical URL of the repository's remote (see CanonicalRepositoryURL); empty
	// for a repository without a usable remote.
	URL string
}

const (
	gitRepositoryTimeout    = 2 * time.Second
	gitRepositoryMaxEntries = 512
)

// GitRepositoryResolver finds the repository a directory belongs to with the git CLI, and
// remembers the answer for a while, negative ones included: the daemon asks for the same few
// directories on every OTEL export and statusline render.
type GitRepositoryResolver struct {
	ttl time.Duration
	// git runs git with args in dir; tests replace it.
	git func(ctx context.Context, dir string, args ...string) (string, error)
	now func() time.Time

	mu    sync.Mutex
	cache map[string]gitRepositoryEntry
}

type gitRepositoryEntry struct {
	repo    GitRepository
	expires time.Time
}

// NewGitRepositoryResolver returns a resolver that keeps answers for ttl (0 keeps nothing).
func NewGitRepositoryResolver(ttl time.Duration) *GitRepositoryResolver {
	return &GitRepositoryResolver{
		ttl:   ttl,
		git:   runGit,
		now:   time.Now,
		cache: make(map[string]gitRepositoryEntry),
	}
}

// runGit runs git without taking optional locks (it must never get in the way of the user's
// own git commands), without prompting and under a deadline.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitRepositoryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Resolve returns the repository dir belongs to. A directory that doesn't exist, isn't in a
// work tree or can't be read returns the zero GitRepository.
func (r *GitRepositoryResolver) Resolve(ctx context.Context, dir string) GitRepository {
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return GitRepository{}
	}
	dir = filepath.Clean(dir)
	if dir == string(filepath.Separator) {
		return GitRepository{}
	}

	now := r.now()
	r.mu.Lock()
	if e, ok := r.cache[dir]; ok && now.Before(e.expires) {
		r.mu.Unlock()
		return e.repo
	}
	r.mu.Unlock()

	repo := r.lookup(ctx, dir)
	if ctx.Err() != nil {
		return repo // a cancelled lookup says nothing about the directory
	}

	if r.ttl > 0 {
		r.mu.Lock()
		if len(r.cache) >= gitRepositoryMaxEntries {
			for k, e := range r.cache {
				if !now.Before(e.expires) {
					delete(r.cache, k)
				}
			}
			if len(r.cache) >= gitRepositoryMaxEntries {
				clear(r.cache)
			}
		}
		r.cache[dir] = gitRepositoryEntry{repo: repo, expires: now.Add(r.ttl)}
		r.mu.Unlock()
	}
	return repo
}

func (r *GitRepositoryResolver) lookup(ctx context.Context, dir string) GitRepository {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return GitRepository{}
	}
	root, err := r.git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return GitRepository{}
	}
	repo := GitRepository{Root: filepath.Clean(root)}

	remotes, err := r.git(ctx, dir, "remote")
	if err != nil {
		return repo
	}
	name := pickGitRemote(strings.Fields(remotes))
	if name == "" {
		return repo
	}
	// get-url applies the user's url.<base>.insteadOf rewrites, unlike reading the config.
	raw, err := r.git(ctx, dir, "remote", "get-url", name)
	if err != nil {
		return repo
	}
	if u, ok := CanonicalRepositoryURL(raw); ok {
		repo.URL = u
	}
	return repo
}

// pickGitRemote prefers origin, the remote a clone creates and the one the user pushes to (so a
// fork is its own project), then upstream, then the first remote by name.
func pickGitRemote(remotes []string) string {
	for _, preferred := range []string{"origin", "upstream"} {
		if slices.Contains(remotes, preferred) {
			return preferred
		}
	}
	if len(remotes) == 0 {
		return ""
	}
	return slices.Min(remotes)
}

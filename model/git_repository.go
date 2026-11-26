package model

import (
	"context"
	"errors"
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
	// gitRepositoryTimeout bounds a whole lookup (up to three git runs), well under an OTLP
	// exporter's timeout, since the daemon resolves on the export path.
	gitRepositoryTimeout    = 2 * time.Second
	gitRepositoryMaxEntries = 512
)

// gitExitError is git exiting with an error status: a definitive answer (not a work tree, no
// such remote), unlike a timeout or git failing to start, which say nothing about the
// directory and are not cached.
type gitExitError struct{ err error }

func (e gitExitError) Error() string { return e.err.Error() }
func (e gitExitError) Unwrap() error { return e.err }

// GitRepositoryResolver finds the repository a directory belongs to with the git CLI, and
// remembers definitive answers for a while, negative ones included: the daemon asks for the same
// few directories on every OTEL export and statusline render. Concurrent lookups of one
// directory share one run of git.
type GitRepositoryResolver struct {
	ttl time.Duration
	// git runs git with args in dir; tests replace it.
	git func(ctx context.Context, dir string, args ...string) (string, error)
	now func() time.Time

	mu       sync.Mutex
	cache    map[string]gitRepositoryEntry
	inflight map[string]*gitRepositoryCall
}

type gitRepositoryEntry struct {
	repo    GitRepository
	expires time.Time
}

type gitRepositoryCall struct {
	done chan struct{}
	repo GitRepository
}

// NewGitRepositoryResolver returns a resolver that keeps answers for ttl (0 keeps nothing).
func NewGitRepositoryResolver(ttl time.Duration) *GitRepositoryResolver {
	return &GitRepositoryResolver{
		ttl:      ttl,
		git:      runGit,
		now:      time.Now,
		cache:    make(map[string]gitRepositoryEntry),
		inflight: make(map[string]*gitRepositoryCall),
	}
}

var (
	gitBinaryOnce sync.Once
	gitBinary     = "git"
)

// gitExecutable finds git once, also where the daemon's limited PATH doesn't reach (Homebrew,
// /usr/local/bin), through the CommandService's fallback paths.
func gitExecutable() string {
	gitBinaryOnce.Do(func() {
		if path, err := NewCommandService().LookPath("git"); err == nil {
			gitBinary = path
		}
	})
	return gitBinary
}

// runGit runs git without taking optional locks (it must never get in the way of the user's
// own git commands) and without prompting, under ctx's deadline.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, gitExecutable(), append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if err != nil && errors.As(err, &exitErr) && ctx.Err() == nil {
		return "", gitExitError{err}
	}
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

	r.mu.Lock()
	if e, ok := r.cache[dir]; ok && r.now().Before(e.expires) {
		r.mu.Unlock()
		return e.repo
	}
	if call, ok := r.inflight[dir]; ok {
		r.mu.Unlock()
		select {
		case <-call.done:
			return call.repo
		case <-ctx.Done():
			return GitRepository{}
		}
	}
	call := &gitRepositoryCall{done: make(chan struct{})}
	r.inflight[dir] = call
	r.mu.Unlock()

	// The lookup isn't tied to this caller's context, so a caller that gives up doesn't spoil
	// the answer for the others waiting on it; the budget bounds it instead.
	lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gitRepositoryTimeout)
	repo, definitive := r.lookup(lookupCtx, dir)
	cancel()

	r.mu.Lock()
	call.repo = repo
	delete(r.inflight, dir)
	if definitive && r.ttl > 0 {
		now := r.now()
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
	}
	r.mu.Unlock()
	close(call.done)
	return repo
}

// lookup asks git about dir. definitive is false when git timed out or couldn't run, so the
// answer isn't cached and the next record asks again.
func (r *GitRepositoryResolver) lookup(ctx context.Context, dir string) (repo GitRepository, definitive bool) {
	answered := func(err error) bool {
		var exit gitExitError
		return errors.As(err, &exit)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return GitRepository{}, true
	}
	root, err := r.git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return GitRepository{}, err == nil || answered(err)
	}
	repo = GitRepository{Root: filepath.Clean(root)}

	remotes, err := r.git(ctx, dir, "remote")
	if err != nil {
		return repo, answered(err)
	}
	name := pickGitRemote(strings.Fields(remotes))
	if name == "" {
		return repo, true
	}
	// get-url applies the user's url.<base>.insteadOf rewrites, unlike reading the config.
	raw, err := r.git(ctx, dir, "remote", "get-url", name)
	if err != nil {
		return repo, answered(err)
	}
	if u, ok := CanonicalRepositoryURL(raw); ok {
		repo.URL = u
	}
	return repo, true
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

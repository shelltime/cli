package model

import (
	"context"
	"errors"
	"io/fs"
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
		if unescaped, err := url.PathUnescape(path); err == nil {
			path = unescaped
		}
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
	// Each segment is escaped again, so every spelling of a name ("My Project", "My%20Project",
	// a non-ASCII name raw or escaped) gives one URL, and a %23 or %3F stays part of the name.
	// The server escapes the same way (model.CanonicalRepositoryURL there).
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
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
	gitRepositoryTimeout = 2 * time.Second
	// gitRepositoryRetryAfter is how long a lookup that timed out or couldn't start git is
	// remembered: long enough that a hung mount doesn't cost every export the whole budget,
	// short enough that the directory is asked about again soon.
	gitRepositoryRetryAfter = 30 * time.Second
	// gitRepositoryGrace is how long past the budget a lookup is waited for. A stat or a git
	// run stuck in the kernel (a hung network mount) ignores the context, so the waiting stops
	// here instead and the directory counts as stuck until that lookup returns.
	gitRepositoryGrace      = 250 * time.Millisecond
	gitRepositoryMaxEntries = 512
)

// gitExitError is git exiting with an error status: a definitive answer (not a work tree, no
// such remote), unlike a timeout, a signal, git failing to start or refusing a repository it
// doesn't trust (safe.directory), which say nothing about the directory and are not cached.
type gitExitError struct{ err error }

func (e gitExitError) Error() string { return e.err.Error() }
func (e gitExitError) Unwrap() error { return e.err }

// GitRepositoryResolver finds the repository a directory belongs to with the git CLI, and
// remembers answers for a while, negative ones included: the daemon asks for the same few
// directories on every OTEL export and statusline render. A definitive answer is kept for the
// resolver's TTL, one from a lookup that timed out or couldn't start git for
// gitRepositoryRetryAfter at most. Concurrent lookups of one directory share one run of git.
type GitRepositoryResolver struct {
	ttl time.Duration
	// timeout is the budget of one lookup, gitRepositoryTimeout unless a test sets it.
	timeout time.Duration
	// git runs git with args in dir; tests replace it.
	git func(ctx context.Context, dir string, args ...string) (string, error)
	now func() time.Time

	mu       sync.Mutex
	cache    map[string]gitRepositoryEntry
	inflight map[string]*gitRepositoryCall
	// stuck holds directories whose lookup outlived its budget and hasn't returned yet; they get
	// no second one meanwhile, so a hung mount costs one blocked thread, not one per retry.
	stuck map[string]struct{}
}

type gitRepositoryEntry struct {
	repo       GitRepository
	definitive bool
	expires    time.Time
}

type gitRepositoryAnswer struct {
	repo       GitRepository
	definitive bool
}

type gitRepositoryCall struct {
	done       chan struct{}
	repo       GitRepository
	definitive bool
}

// NewGitRepositoryResolver returns a resolver that keeps answers for ttl (0 keeps nothing).
func NewGitRepositoryResolver(ttl time.Duration) *GitRepositoryResolver {
	return &GitRepositoryResolver{
		ttl:      ttl,
		timeout:  gitRepositoryTimeout,
		git:      runGit,
		now:      time.Now,
		cache:    make(map[string]gitRepositoryEntry),
		inflight: make(map[string]*gitRepositoryCall),
		stuck:    make(map[string]struct{}),
	}
}

var (
	gitBinaryMu       sync.Mutex
	gitBinary         string
	gitBinaryLookedAt time.Time
)

// gitExecutable finds git, also where the daemon's limited PATH doesn't reach (Homebrew,
// /usr/local/bin), through the CommandService's fallback paths. A path found is kept; while
// none is, it looks again every gitRepositoryRetryAfter, so git installed after the daemon
// started is picked up, and runs the bare "git" meanwhile.
func gitExecutable() string {
	gitBinaryMu.Lock()
	defer gitBinaryMu.Unlock()
	if gitBinary == "" && time.Since(gitBinaryLookedAt) >= gitRepositoryRetryAfter {
		gitBinaryLookedAt = time.Now()
		if path, err := NewCommandService().LookPath("git"); err == nil {
			gitBinary = path
		}
	}
	if gitBinary == "" {
		return "git"
	}
	return gitBinary
}

// runGit runs git without taking optional locks (it must never get in the way of the user's
// own git commands) and without prompting, under ctx's deadline.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, gitExecutable(), append([]string{"-C", dir}, args...)...)
	// Messages in English: gitExitIsAnswer reads them.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "LANGUAGE=")
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if err != nil && errors.As(err, &exitErr) && ctx.Err() == nil && gitExitIsAnswer(exitErr) {
		return "", gitExitError{err}
	}
	return strings.TrimSpace(string(out)), err
}

// gitExitIsAnswer reports whether git's error exit says something about the directory. Killed
// by a signal (ExitCode -1), or refusing a repository owned by someone else, it doesn't: the
// directory may well be a clone.
func gitExitIsAnswer(err *exec.ExitError) bool {
	if err.ExitCode() < 0 {
		return false
	}
	stderr := strings.ToLower(string(err.Stderr))
	return !strings.Contains(stderr, "dubious ownership") &&
		!strings.Contains(stderr, "safe.directory") &&
		!strings.Contains(stderr, "unsafe repository")
}

// Resolve returns the repository dir belongs to. A directory that doesn't exist, isn't in a
// work tree or can't be read returns the zero GitRepository.
func (r *GitRepositoryResolver) Resolve(ctx context.Context, dir string) GitRepository {
	repo, _ := r.Lookup(ctx, dir)
	return repo
}

// Lookup is Resolve, and also says whether the answer is definitive: false when git timed out or
// couldn't run (or ctx ended first), so the directory's repository is unknown rather than none.
func (r *GitRepositoryResolver) Lookup(ctx context.Context, dir string) (GitRepository, bool) {
	dir = strings.TrimSpace(dir)
	if dir == "" || !filepath.IsAbs(dir) {
		return GitRepository{}, true
	}
	dir = filepath.Clean(dir)
	if dir == string(filepath.Separator) {
		return GitRepository{}, true
	}

	r.mu.Lock()
	if e, ok := r.cache[dir]; ok && r.now().Before(e.expires) {
		r.mu.Unlock()
		return e.repo, e.definitive
	}
	if _, ok := r.stuck[dir]; ok {
		r.mu.Unlock()
		return GitRepository{}, false
	}
	if call, ok := r.inflight[dir]; ok {
		r.mu.Unlock()
		select {
		case <-call.done:
			return call.repo, call.definitive
		case <-ctx.Done():
			return GitRepository{}, false
		}
	}
	call := &gitRepositoryCall{done: make(chan struct{})}
	r.inflight[dir] = call
	r.mu.Unlock()

	var repo GitRepository
	var definitive bool
	// Every waiter is released however this ends.
	defer func() {
		r.finish(dir, call, repo, definitive)
		close(call.done)
	}()

	// The lookup isn't tied to this caller's context, so a caller that gives up doesn't spoil
	// the answer for the others waiting on it; the budget bounds it instead. It runs on its own
	// goroutine because the budget can't interrupt a stat or a git run blocked in the kernel.
	answers := make(chan gitRepositoryAnswer, 1)
	timeout := r.timeout
	go func() {
		lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		a := gitRepositoryAnswer{}
		a.repo, a.definitive = r.lookup(lookupCtx, dir)
		answers <- a
	}()
	timer := time.NewTimer(timeout + gitRepositoryGrace)
	defer timer.Stop()
	select {
	case a := <-answers:
		repo, definitive = a.repo, a.definitive
	case <-ctx.Done():
		// This caller gave up (its exporter went away, the daemon is stopping).
		r.abandon(dir, answers)
	case <-timer.C:
		r.abandon(dir, answers)
	}
	return repo, definitive
}

// abandon stops waiting for a lookup that is still running: the directory gets no other lookup
// until this one returns, and its answer is kept then for whoever asks next.
func (r *GitRepositoryResolver) abandon(dir string, answers <-chan gitRepositoryAnswer) {
	r.mu.Lock()
	r.stuck[dir] = struct{}{}
	r.mu.Unlock()
	go func() {
		a := <-answers
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.stuck, dir)
		if ttl := r.entryTTL(a.definitive); ttl > 0 && len(r.cache) < gitRepositoryMaxEntries {
			r.cache[dir] = gitRepositoryEntry{repo: a.repo, definitive: a.definitive, expires: r.now().Add(ttl)}
		}
	}()
}

// finish publishes a lookup's answer to the cache and to the callers waiting on it.
func (r *GitRepositoryResolver) finish(dir string, call *gitRepositoryCall, repo GitRepository, definitive bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	call.repo, call.definitive = repo, definitive
	delete(r.inflight, dir)
	// An abandoned lookup that answered in the meantime has cached its answer; "unknown" never
	// replaces it.
	if e, ok := r.cache[dir]; ok && !definitive && e.definitive && r.now().Before(e.expires) {
		return
	}
	if ttl := r.entryTTL(definitive); ttl > 0 {
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
		r.cache[dir] = gitRepositoryEntry{repo: repo, definitive: definitive, expires: now.Add(ttl)}
	}
}

// dirState is what a stat of a project's folder said.
type dirState int

const (
	dirExists   dirState = iota
	dirMissing           // nothing there, or not a directory
	dirUnknown           // the stat failed some other way (permissions, I/O)
	dirTimedOut          // the stat didn't return in time, as on a hung network mount
)

// statDir stats dir, giving up after timeout: on a hung network mount the stat blocks in the
// kernel, where no context reaches it. Tests replace it.
var statDir = func(dir string, timeout time.Duration) dirState {
	result := make(chan dirState, 1)
	go func() {
		info, err := os.Stat(dir)
		switch {
		case err == nil && info.IsDir():
			result <- dirExists
		case err == nil, errors.Is(err, fs.ErrNotExist):
			result <- dirMissing
		default:
			result <- dirUnknown
		}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case state := <-result:
		return state
	case <-timer.C:
		return dirTimedOut
	}
}

// entryTTL is how long an answer is kept.
func (r *GitRepositoryResolver) entryTTL(definitive bool) time.Duration {
	if definitive {
		return r.ttl
	}
	return min(r.ttl, gitRepositoryRetryAfter)
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

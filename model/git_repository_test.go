package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRepositoryURL(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"https://github.com/shelltime/cli.git", "https://github.com/shelltime/cli"},
		{"https://github.com/shelltime/cli", "https://github.com/shelltime/cli"},
		{"https://github.com/shelltime/cli/", "https://github.com/shelltime/cli"},
		{"http://GitHub.com/ShellTime/CLI.git", "https://github.com/ShellTime/CLI"},
		{"https://x-access-token:ghp_secret@github.com/acme/web.git", "https://github.com/acme/web"},
		{"https://user@gitlab.example.com:8443/group/sub/repo.git?ref=1#frag", "https://gitlab.example.com/group/sub/repo"},
		{"git@github.com:shelltime/cli.git", "https://github.com/shelltime/cli"},
		{"git@github.com:/shelltime/cli.git", "https://github.com/shelltime/cli"},
		{"github.com:shelltime/cli", "https://github.com/shelltime/cli"},
		{"ssh://git@github.com/shelltime/cli.git", "https://github.com/shelltime/cli"},
		{"ssh://git@gitea.local:2222/team/app", "https://gitea.local/team/app"},
		{"git+ssh://git@github.com/shelltime/cli.git", "https://github.com/shelltime/cli"},
		{"git://git.kernel.org/pub/scm/git/git.git", "https://git.kernel.org/pub/scm/git/git"},
		{"  https://github.com/a/b.git\n", "https://github.com/a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			got, ok := CanonicalRepositoryURL(tt.remote)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Names are escaped the same way whatever the remote's form, like the server does.
func TestCanonicalRepositoryURLEscapesNames(t *testing.T) {
	for remote, want := range map[string]string{
		"https://dev.azure.com/org/My%20Project/_git/repo": "https://dev.azure.com/org/My%20Project/_git/repo",
		"https://example.com/acme/repo%23one.git":          "https://example.com/acme/repo%23one",
		"https://github.com/acme/Ⱥ":                        "https://github.com/acme/%C8%BA",
		"git@github.com:acme/%c8%ba.git":                   "https://github.com/acme/%C8%BA",
	} {
		got, ok := CanonicalRepositoryURL(remote)
		require.True(t, ok, remote)
		assert.Equal(t, want, got, remote)
	}
}

func TestCanonicalRepositoryURLRejectsLocalRemotes(t *testing.T) {
	for _, remote := range []string{
		"",
		"/srv/git/repo.git",
		"../other",
		"./repo:with-colon",
		"file:///srv/git/repo.git",
		"C:/repos/app",
		`C:\repos\app`,
		"https://github.com",
		"https://github.com/.git",
		"git@github.com:",
		"https://github.com/a/../b",
		"ftp://example.com/a/b",
		"has space:repo",
	} {
		t.Run(remote, func(t *testing.T) {
			_, ok := CanonicalRepositoryURL(remote)
			assert.False(t, ok)
		})
	}
}

func TestPickGitRemote(t *testing.T) {
	assert.Equal(t, "origin", pickGitRemote([]string{"upstream", "origin", "fork"}))
	assert.Equal(t, "upstream", pickGitRemote([]string{"zeta", "upstream"}))
	assert.Equal(t, "alpha", pickGitRemote([]string{"zeta", "alpha"}))
	assert.Equal(t, "", pickGitRemote(nil))
}

// fakeGit answers like git for the directories it knows and counts the calls.
type fakeGit struct {
	root    string
	remotes string
	url     string
	calls   int
}

func (f *fakeGit) run(_ context.Context, dir string, args ...string) (string, error) {
	f.calls++
	switch {
	case args[0] == "rev-parse":
		if f.root == "" {
			return "", gitExitError{errors.New("not a git repository")}
		}
		return f.root, nil
	case len(args) == 1 && args[0] == "remote":
		return f.remotes, nil
	case len(args) == 3 && args[1] == "get-url":
		return f.url, nil
	}
	return "", errors.New("unexpected git call")
}

func TestGitRepositoryResolverCachesAnswers(t *testing.T) {
	dir := t.TempDir()
	git := &fakeGit{root: dir, remotes: "origin\n", url: "git@github.com:acme/web.git"}
	now := time.Unix(1_700_000_000, 0)
	r := NewGitRepositoryResolver(time.Minute)
	r.git = git.run
	r.now = func() time.Time { return now }

	want := GitRepository{Root: dir, URL: "https://github.com/acme/web"}
	assert.Equal(t, want, r.Resolve(context.Background(), dir))
	assert.Equal(t, 3, git.calls)

	assert.Equal(t, want, r.Resolve(context.Background(), dir+"/"), "a trailing slash is the same directory")
	assert.Equal(t, 3, git.calls, "the second lookup is served from the cache")

	now = now.Add(2 * time.Minute)
	git.url = "https://github.com/acme/renamed.git"
	assert.Equal(t, "https://github.com/acme/renamed", r.Resolve(context.Background(), dir).URL)
	assert.Equal(t, 6, git.calls, "an expired answer is looked up again")
}

func TestGitRepositoryResolverCachesNegativeAnswers(t *testing.T) {
	dir := t.TempDir()
	git := &fakeGit{}
	r := NewGitRepositoryResolver(time.Minute)
	r.git = git.run

	assert.Equal(t, GitRepository{}, r.Resolve(context.Background(), dir))
	assert.Equal(t, GitRepository{}, r.Resolve(context.Background(), dir))
	assert.Equal(t, 1, git.calls)
}

// A timeout, or git failing to start, says nothing about the directory: it is kept only briefly
// and reported as unknown, instead of serving a cached "no repository" for the whole TTL.
func TestGitRepositoryResolverDoesNotCacheTransientFailures(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	failing := true
	now := time.Unix(1_700_000_000, 0)
	r := NewGitRepositoryResolver(time.Hour)
	r.now = func() time.Time { return now }
	r.git = func(ctx context.Context, d string, args ...string) (string, error) {
		calls++
		if failing {
			return "", context.DeadlineExceeded
		}
		return (&fakeGit{root: dir, remotes: "origin", url: "https://github.com/acme/web"}).run(ctx, d, args...)
	}

	repo, definitive := r.Lookup(context.Background(), dir)
	assert.Equal(t, GitRepository{}, repo)
	assert.False(t, definitive, "unknown, not no repository")
	failing = false
	_, definitive = r.Lookup(context.Background(), dir)
	assert.False(t, definitive)
	assert.Equal(t, 1, calls, "a hung directory doesn't cost every caller the whole budget")

	now = now.Add(gitRepositoryRetryAfter + time.Second)
	repo, definitive = r.Lookup(context.Background(), dir)
	assert.Equal(t, "https://github.com/acme/web", repo.URL)
	assert.True(t, definitive)
	assert.Equal(t, 4, calls, "asked again soon after")

	now = now.Add(30 * time.Minute)
	r.Resolve(context.Background(), dir)
	assert.Equal(t, 4, calls, "a definitive answer is kept for the TTL")
}

// Concurrent lookups of one directory share one run of git.
func TestGitRepositoryResolverSharesConcurrentLookups(t *testing.T) {
	dir := t.TempDir()
	release := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	r := NewGitRepositoryResolver(time.Minute)
	r.git = func(ctx context.Context, d string, args ...string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		return (&fakeGit{root: dir, remotes: "origin", url: "https://github.com/acme/web"}).run(ctx, d, args...)
	}

	var wg sync.WaitGroup
	results := make([]GitRepository, 5)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = r.Resolve(context.Background(), dir)
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	for _, got := range results {
		assert.Equal(t, "https://github.com/acme/web", got.URL)
	}
	assert.Equal(t, 3, calls, "one lookup: rev-parse, remote, get-url")
}

// A caller that gives up returns at once; the lookup it started still finishes for the others.
func TestGitRepositoryResolverWaiterGivesUp(t *testing.T) {
	dir := t.TempDir()
	release := make(chan struct{})
	r := NewGitRepositoryResolver(time.Minute)
	r.git = func(ctx context.Context, d string, args ...string) (string, error) {
		<-release
		return (&fakeGit{root: dir, remotes: "origin", url: "https://github.com/acme/web"}).run(ctx, d, args...)
	}

	first := make(chan GitRepository)
	go func() { first <- r.Resolve(context.Background(), dir) }()
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, GitRepository{}, r.Resolve(ctx, dir), "a cancelled waiter gets nothing")

	close(release)
	assert.Equal(t, "https://github.com/acme/web", (<-first).URL)
}

// A lookup blocked where its context can't reach (a hung network mount) stops being waited for
// after the budget, and the directory gets no second lookup until that one returns.
func TestGitRepositoryResolverGivesUpOnAStuckLookup(t *testing.T) {
	dir := t.TempDir()
	release := make(chan struct{})
	var calls atomic.Int32
	r := NewGitRepositoryResolver(0)
	r.timeout = 20 * time.Millisecond
	r.git = func(ctx context.Context, d string, args ...string) (string, error) {
		calls.Add(1)
		<-release // ignores ctx, like a syscall in the kernel
		return (&fakeGit{root: dir, remotes: "origin", url: "https://github.com/acme/web"}).run(ctx, d, args...)
	}

	start := time.Now()
	repo, definitive := r.Lookup(context.Background(), dir)
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, GitRepository{}, repo)
	assert.False(t, definitive)

	_, definitive = r.Lookup(context.Background(), dir)
	assert.False(t, definitive)
	assert.Equal(t, int32(1), calls.Load(), "no second lookup while the first is stuck")

	close(release)
	assert.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.stuck) == 0
	}, 2*time.Second, 5*time.Millisecond)
	r.timeout = time.Second
	repo, definitive = r.Lookup(context.Background(), dir)
	assert.True(t, definitive)
	assert.Equal(t, "https://github.com/acme/web", repo.URL)
}

// A caller whose context ends stops waiting; the lookup's answer is kept for the next caller.
func TestGitRepositoryResolverCallerGivesUp(t *testing.T) {
	dir := t.TempDir()
	release := make(chan struct{})
	r := NewGitRepositoryResolver(time.Minute)
	r.git = func(ctx context.Context, d string, args ...string) (string, error) {
		<-release
		return (&fakeGit{root: dir, remotes: "origin", url: "https://github.com/acme/web"}).run(ctx, d, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, definitive := r.Lookup(ctx, dir)
	assert.False(t, definitive)

	close(release)
	assert.Eventually(t, func() bool {
		repo, definitive := r.Lookup(context.Background(), dir)
		return definitive && repo.URL == "https://github.com/acme/web"
	}, 2*time.Second, 10*time.Millisecond)
}

// git refusing a repository it doesn't trust, or killed by a signal, says nothing about the
// directory; any other error exit does.
func TestGitExitIsAnswer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	exitOf := func(script string) *exec.ExitError {
		_, err := exec.Command("sh", "-c", script).Output()
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		return exitErr
	}
	assert.True(t, gitExitIsAnswer(exitOf(`echo "fatal: not a git repository" >&2; exit 128`)))
	assert.True(t, gitExitIsAnswer(exitOf(`echo "error: No such remote 'origin'" >&2; exit 2`)))
	assert.False(t, gitExitIsAnswer(exitOf(`echo "fatal: detected dubious ownership in repository at '/x'" >&2; exit 128`)))
	assert.False(t, gitExitIsAnswer(exitOf(`kill -9 $$`)))
}

func TestStatDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	for path, want := range map[string]dirState{dir: dirExists, file: dirMissing, filepath.Join(dir, "missing"): dirMissing} {
		assert.Equal(t, want, statDir(path, time.Second), path)
	}
}

func TestGitRepositoryResolverSkipsWhatIsNotADirectory(t *testing.T) {
	git := &fakeGit{root: "/x", remotes: "origin", url: "https://github.com/a/b"}
	r := NewGitRepositoryResolver(time.Minute)
	r.git = git.run

	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	for _, dir := range []string{"", "/", "relative/dir", filepath.Join(t.TempDir(), "missing"), file} {
		assert.Equal(t, GitRepository{}, r.Resolve(context.Background(), dir), dir)
	}
	assert.Zero(t, git.calls)
}

func TestGitRepositoryResolverWithoutRemote(t *testing.T) {
	dir := t.TempDir()
	git := &fakeGit{root: dir}
	r := NewGitRepositoryResolver(0)
	r.git = git.run

	assert.Equal(t, GitRepository{Root: dir}, r.Resolve(context.Background(), dir))

	git.remotes = "origin"
	git.url = "/srv/git/local.git"
	assert.Equal(t, GitRepository{Root: dir}, r.Resolve(context.Background(), dir), "a local remote is no repository URL")
}

func TestGitRepositoryResolverWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("init", "-q")
	run("remote", "add", "origin", "git@github.com:Acme/Web.git")
	sub := filepath.Join(root, "packages", "app")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	resolved, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	r := NewGitRepositoryResolver(time.Minute)
	got := r.Resolve(context.Background(), sub)
	assert.Equal(t, "https://github.com/Acme/Web", got.URL)
	assert.Equal(t, resolved, got.Root)

	assert.Equal(t, GitRepository{}, r.Resolve(context.Background(), t.TempDir()), "not a work tree")
}

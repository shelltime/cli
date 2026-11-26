package model

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
			return "", errors.New("not a git repository")
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

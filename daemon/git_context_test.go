package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GitContextTestSuite struct {
	suite.Suite
	dir string
}

func (s *GitContextTestSuite) SetupTest() {
	if _, err := exec.LookPath("git"); err != nil {
		s.T().Skip("git not available")
	}
	s.dir = s.T().TempDir()
}

// git runs a git command in dir with an isolated identity and config.
func (s *GitContextTestSuite) git(dir string, args ...string) string {
	base := []string{
		"-c", "user.email=test@test.com", "-c", "user.name=Test User",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main",
		"-C", dir,
	}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, "git %v: %s", args, out)
	return string(out)
}

// gitMayFail runs a git command that is expected to fail (e.g. a conflict).
func (s *GitContextTestSuite) gitMayFail(dir string, args ...string) {
	base := []string{"-c", "user.email=test@test.com", "-c", "user.name=Test User", "-c", "commit.gpgsign=false", "-C", dir}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	_ = cmd.Run()
}

func (s *GitContextTestSuite) write(dir, name, content string) {
	path := filepath.Join(dir, name)
	s.Require().NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	s.Require().NoError(os.WriteFile(path, []byte(content), 0o644))
}

func (s *GitContextTestSuite) commit(dir, name, content, msg string) {
	s.write(dir, name, content)
	s.git(dir, "add", name)
	s.git(dir, "commit", "-q", "-m", msg)
}

func (s *GitContextTestSuite) gitContext(dir string) *model.QueryGitContext {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return GetGitContext(ctx, dir)
}

func (s *GitContextTestSuite) TestNotARepository() {
	s.Nil(s.gitContext(s.dir))
	s.Nil(s.gitContext(""))
	s.Nil(s.gitContext(filepath.Join(s.dir, "missing")))
}

func (s *GitContextTestSuite) TestCleanRepoWithCommits() {
	s.git(s.dir, "init", "-q")
	for _, msg := range []string{"first", "second", "third", "fourth"} {
		s.commit(s.dir, msg+".txt", msg, "feat: "+msg)
	}
	s.Require().NoError(os.MkdirAll(filepath.Join(s.dir, "sub", "dir"), 0o755))

	gc := s.gitContext(s.dir)
	s.Require().NotNil(gc)
	s.Equal("main", gc.Branch)
	s.False(gc.Detached)
	s.Empty(gc.PathInRepo)
	s.Empty(gc.Upstream)
	s.Zero(gc.Staged + gc.Unstaged + gc.Untracked + gc.Conflicted)
	s.Empty(gc.Operation)
	s.Require().Len(gc.RecentCommits, gitContextCommits)
	s.Contains(gc.RecentCommits[0], "feat: fourth", "newest first")
	s.Empty(gc.Remotes)

	sub := s.gitContext(filepath.Join(s.dir, "sub", "dir"))
	s.Require().NotNil(sub)
	s.Equal("sub/dir", sub.PathInRepo)
}

func (s *GitContextTestSuite) TestFileCounts() {
	s.git(s.dir, "init", "-q")
	s.commit(s.dir, "a.txt", "a", "a")
	s.commit(s.dir, "b.txt", "b", "b")

	s.write(s.dir, "a.txt", "a2")
	s.git(s.dir, "add", "a.txt")
	s.write(s.dir, "b.txt", "b2")
	s.write(s.dir, "new1.txt", "")
	s.write(s.dir, "new2.txt", "")

	gc := s.gitContext(s.dir)
	s.Require().NotNil(gc)
	s.Equal(1, gc.Staged)
	s.Equal(1, gc.Unstaged)
	s.Equal(2, gc.Untracked)
}

func (s *GitContextTestSuite) TestAheadOfUpstream() {
	remote := filepath.Join(s.dir, "remote.git")
	work := filepath.Join(s.dir, "work")
	s.git(s.dir, "init", "-q", "--bare", remote)
	s.git(s.dir, "clone", "-q", remote, work)
	s.commit(work, "a.txt", "a", "a")
	s.git(work, "push", "-q", "-u", "origin", "HEAD:main")
	s.commit(work, "b.txt", "b", "b")

	gc := s.gitContext(work)
	s.Require().NotNil(gc)
	s.Equal("origin/main", gc.Upstream)
	s.Equal(1, gc.Ahead)
	s.Zero(gc.Behind)
	s.Equal([]string{"origin=local"}, gc.Remotes)
}

func (s *GitContextTestSuite) TestMergeConflict() {
	s.git(s.dir, "init", "-q")
	s.commit(s.dir, "f.txt", "base\n", "base")
	s.git(s.dir, "checkout", "-q", "-b", "other")
	s.commit(s.dir, "f.txt", "other\n", "other")
	s.git(s.dir, "checkout", "-q", "main")
	s.commit(s.dir, "f.txt", "main\n", "main")
	s.gitMayFail(s.dir, "merge", "other")

	gc := s.gitContext(s.dir)
	s.Require().NotNil(gc)
	s.Equal("merge", gc.Operation)
	s.Equal(1, gc.Conflicted)
}

func (s *GitContextTestSuite) TestDetachedHead() {
	s.git(s.dir, "init", "-q")
	s.commit(s.dir, "a.txt", "a", "a")
	s.git(s.dir, "checkout", "-q", "--detach")

	gc := s.gitContext(s.dir)
	s.Require().NotNil(gc)
	s.True(gc.Detached)
	s.Len(gc.Branch, 7, "short commit hash")
}

func (s *GitContextTestSuite) TestReadGitHEAD() {
	gitDir := s.T().TempDir()
	s.write(gitDir, "HEAD", "ref: refs/heads/feature/x\n")
	branch, detached := readGitHEAD(gitDir)
	s.Equal("feature/x", branch)
	s.False(detached)

	s.write(gitDir, "HEAD", "4b825dc642cb6eb9a060e54bf8d69288fbee4904\n")
	branch, detached = readGitHEAD(gitDir)
	s.Equal("4b825dc", branch)
	s.True(detached)

	branch, detached = readGitHEAD(filepath.Join(gitDir, "missing"))
	s.Empty(branch)
	s.False(detached)
}

func (s *GitContextTestSuite) TestDetectGitOperation() {
	gitDir := s.T().TempDir()
	s.Empty(detectGitOperation(gitDir))

	s.write(gitDir, "BISECT_LOG", "")
	s.Equal("bisect", detectGitOperation(gitDir))
	s.write(gitDir, "CHERRY_PICK_HEAD", "")
	s.Equal("cherry-pick", detectGitOperation(gitDir))
	s.write(gitDir, "rebase-apply/applying", "")
	s.Equal("am", detectGitOperation(gitDir))
	s.write(gitDir, "rebase-merge/head-name", "")
	s.Equal("rebase", detectGitOperation(gitDir))
}

func TestGitContextTestSuite(t *testing.T) {
	suite.Run(t, new(GitContextTestSuite))
}

func TestParsePorcelainV2(t *testing.T) {
	b, err := os.ReadFile("../fixtures/query_context/status_porcelain_v2.txt")
	require.NoError(t, err)

	gc := &model.QueryGitContext{}
	parsePorcelainV2(string(b), gc)
	assert.Equal(t, "feature/login", gc.Branch)
	assert.Equal(t, "origin/feature/login", gc.Upstream)
	assert.Equal(t, 2, gc.Ahead)
	assert.Equal(t, 1, gc.Behind)
	assert.Equal(t, 3, gc.Staged)
	assert.Equal(t, 2, gc.Unstaged)
	assert.Equal(t, 1, gc.Conflicted)
	assert.Equal(t, 2, gc.Untracked)
	assert.False(t, gc.Detached)

	detached := &model.QueryGitContext{}
	parsePorcelainV2("# branch.oid 4b825dc642cb6eb9a060e54bf8d69288fbee4904\n# branch.head (detached)\n", detached)
	assert.True(t, detached.Detached)
	assert.Equal(t, "4b825dc", detached.Branch)
}

func TestParseGitRemotes(t *testing.T) {
	out := "origin\thttps://user:ghp_secret@github.com/owner/repo.git (fetch)\n" +
		"origin\thttps://user:ghp_secret@github.com/owner/repo.git (push)\n" +
		"upstream\tgit@gitlab.example.com:group/repo.git (fetch)\n" +
		"backup\tssh://git@git.example.org:2222/repo.git (fetch)\n" +
		"local\t/srv/git/repo.git (fetch)\n" +
		"win\tC:\\repos\\x.git (fetch)\n" +
		"file\tfile:///srv/git/repo.git (fetch)\n"
	got := parseGitRemotes(out)
	assert.Equal(t, []string{
		"origin=github.com",
		"upstream=gitlab.example.com",
		"backup=git.example.org",
		"local=local",
		"win=local",
	}, got, "credentials and paths dropped, capped at 5")
}

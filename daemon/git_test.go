package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type GitInfoTestSuite struct {
	suite.Suite
	tempDir string
}

func (s *GitInfoTestSuite) SetupTest() {
	var err error
	s.tempDir, err = os.MkdirTemp("", "git-info-test-*")
	assert.NoError(s.T(), err)
}

func (s *GitInfoTestSuite) TearDownTest() {
	os.RemoveAll(s.tempDir)
}

func (s *GitInfoTestSuite) TestGetGitInfo_EmptyWorkingDir() {
	info := GetGitInfo("")
	assert.False(s.T(), info.IsRepo)
	assert.Empty(s.T(), info.Branch)
	assert.False(s.T(), info.Dirty)
}

func (s *GitInfoTestSuite) TestGetGitInfo_NonGitDirectory() {
	info := GetGitInfo(s.tempDir)
	assert.False(s.T(), info.IsRepo)
	assert.Empty(s.T(), info.Branch)
	assert.False(s.T(), info.Dirty)
}

func (s *GitInfoTestSuite) TestGetGitInfo_NonExistentDirectory() {
	info := GetGitInfo("/nonexistent/path/that/does/not/exist")
	assert.False(s.T(), info.IsRepo)
	assert.Empty(s.T(), info.Branch)
	assert.False(s.T(), info.Dirty)
}

func (s *GitInfoTestSuite) TestGetGitInfo_GitRepo_CleanState() {
	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = s.tempDir
	err := cmd.Run()
	if err != nil {
		s.T().Skip("git not available")
	}

	// Configure git user (required for commits)
	exec.Command("git", "-C", s.tempDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", s.tempDir, "config", "user.name", "Test User").Run()

	// Create initial commit to establish HEAD
	testFile := filepath.Join(s.tempDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	exec.Command("git", "-C", s.tempDir, "add", ".").Run()
	exec.Command("git", "-C", s.tempDir, "commit", "-m", "initial").Run()

	info := GetGitInfo(s.tempDir)
	assert.True(s.T(), info.IsRepo)
	assert.NotEmpty(s.T(), info.Branch)
	assert.False(s.T(), info.Dirty)
}

func (s *GitInfoTestSuite) TestGetGitInfo_GitRepo_DirtyState() {
	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = s.tempDir
	err := cmd.Run()
	if err != nil {
		s.T().Skip("git not available")
	}

	// Configure git user
	exec.Command("git", "-C", s.tempDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", s.tempDir, "config", "user.name", "Test User").Run()

	// Create initial commit
	testFile := filepath.Join(s.tempDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	exec.Command("git", "-C", s.tempDir, "add", ".").Run()
	exec.Command("git", "-C", s.tempDir, "commit", "-m", "initial").Run()

	// Make a change (dirty state)
	os.WriteFile(testFile, []byte("modified"), 0644)

	info := GetGitInfo(s.tempDir)
	assert.True(s.T(), info.IsRepo)
	assert.NotEmpty(s.T(), info.Branch)
	assert.True(s.T(), info.Dirty)
}

func (s *GitInfoTestSuite) TestGetGitInfo_GitRepo_UntrackedFile() {
	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = s.tempDir
	err := cmd.Run()
	if err != nil {
		s.T().Skip("git not available")
	}

	// Configure git user
	exec.Command("git", "-C", s.tempDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", s.tempDir, "config", "user.name", "Test User").Run()

	// Create initial commit
	testFile := filepath.Join(s.tempDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	exec.Command("git", "-C", s.tempDir, "add", ".").Run()
	exec.Command("git", "-C", s.tempDir, "commit", "-m", "initial").Run()

	// Add untracked file (makes repo dirty)
	untrackedFile := filepath.Join(s.tempDir, "untracked.txt")
	os.WriteFile(untrackedFile, []byte("untracked"), 0644)

	info := GetGitInfo(s.tempDir)
	assert.True(s.T(), info.IsRepo)
	assert.True(s.T(), info.Dirty, "untracked files should make repo dirty")
}

func (s *GitInfoTestSuite) TestGetGitInfo_GitRepo_BranchName() {
	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = s.tempDir
	err := cmd.Run()
	if err != nil {
		s.T().Skip("git not available")
	}

	// Configure git user
	exec.Command("git", "-C", s.tempDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", s.tempDir, "config", "user.name", "Test User").Run()

	// Create initial commit on main/master
	testFile := filepath.Join(s.tempDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	exec.Command("git", "-C", s.tempDir, "add", ".").Run()
	exec.Command("git", "-C", s.tempDir, "commit", "-m", "initial").Run()

	// Create and checkout a new branch
	exec.Command("git", "-C", s.tempDir, "checkout", "-b", "feature/test-branch").Run()

	info := GetGitInfo(s.tempDir)
	assert.True(s.T(), info.IsRepo)
	assert.Equal(s.T(), "feature/test-branch", info.Branch)
}

func (s *GitInfoTestSuite) TestGetGitInfo_Subdirectory() {
	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = s.tempDir
	err := cmd.Run()
	if err != nil {
		s.T().Skip("git not available")
	}

	// Configure git user
	exec.Command("git", "-C", s.tempDir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", s.tempDir, "config", "user.name", "Test User").Run()

	// Create initial commit
	testFile := filepath.Join(s.tempDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	exec.Command("git", "-C", s.tempDir, "add", ".").Run()
	exec.Command("git", "-C", s.tempDir, "commit", "-m", "initial").Run()

	// Create subdirectory
	subDir := filepath.Join(s.tempDir, "src", "components")
	os.MkdirAll(subDir, 0755)

	// GetGitInfo from subdirectory should still work (DetectDotGit: true)
	info := GetGitInfo(subDir)
	assert.True(s.T(), info.IsRepo)
	assert.NotEmpty(s.T(), info.Branch)
}

func TestGitInfoTestSuite(t *testing.T) {
	suite.Run(t, new(GitInfoTestSuite))
}

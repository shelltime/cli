package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	fn()
	os.Stdout = orig
	w.Close()
	out := <-done
	r.Close()
	return string(out)
}

func (s *queryTestSuite) TestQueryCommandSendsContext() {
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: "t"}, nil)
	s.mockAI.On("QueryCommandStream", mock.Anything, mock.MatchedBy(func(v model.CommandSuggestVariables) bool {
		return v.Context != nil && v.Context.Git != nil && v.Context.Git.Branch == "main" && v.Pwd != ""
	}), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(3).(func(token string))("git status")
		}).Return(nil)

	s.Require().NoError(s.app.Run([]string{"shelltime-test", "query", "show changes"}))
	s.Equal(1, s.gatherCalls)
}

func (s *queryTestSuite) TestQueryCommandShareContextDisabled() {
	disabled := false
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token: "t",
		AI:    &model.AIConfig{ShareContext: &disabled},
	}, nil)
	s.mockAI.On("QueryCommandStream", mock.Anything, mock.MatchedBy(func(v model.CommandSuggestVariables) bool {
		return v.Context == nil && v.Pwd == "" && v.Hostname == ""
	}), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(3).(func(token string))("ls")
		}).Return(nil)

	s.Require().NoError(s.app.Run([]string{"shelltime-test", "query", "list files"}))
	s.Zero(s.gatherCalls, "no context is collected when sharing is off")
}

func (s *queryTestSuite) TestQueryCommandShowContextIsDryRun() {
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: "t"}, nil)
	// No QueryCommandStream expectation: the mock fails the test if it is called.

	out := captureStdout(s.T(), func() {
		s.Require().NoError(s.app.Run([]string{"shelltime-test", "query", "--show-context", "run the tests"}))
	})

	var vars model.CommandSuggestVariables
	s.Require().NoError(json.NewDecoder(bytes.NewBufferString(out)).Decode(&vars))
	s.Equal("run the tests", vars.Query)
	s.Require().NotNil(vars.Context)
	s.Equal("main", vars.Context.Git.Branch)
}

func (s *queryTestSuite) TestQueryCommandShowContextWithoutAIService() {
	aiService = nil
	s.mockConfig.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)

	out := captureStdout(s.T(), func() {
		s.Require().NoError(s.app.Run([]string{"shelltime-test", "query", "--show-context", "x"}))
	})
	s.Contains(out, `"query": "x"`)
}

func (s *queryTestSuite) TestCurrentShell() {
	origShell, hadShell := os.LookupEnv("SHELL")
	defer func() {
		if hadShell {
			os.Setenv("SHELL", origShell)
		} else {
			os.Unsetenv("SHELL")
		}
	}()
	os.Setenv("SHELL", "/bin/zsh")

	parentProcessNameFn = func() string { return "-fish" }
	s.Equal("fish", currentShell(), "login shell dash is stripped")

	parentProcessNameFn = func() string { return "/usr/local/bin/bash" }
	s.Equal("bash", currentShell())

	parentProcessNameFn = func() string { return "go" }
	s.Equal("zsh", currentShell(), "unknown parent falls back to $SHELL")

	parentProcessNameFn = func() string { return "" }
	os.Unsetenv("SHELL")
	s.Equal("unknown", currentShell())
}

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetectMultiplexer(t *testing.T) {
	assert.Equal(t, "tmux", detectMultiplexer(envFrom(map[string]string{"TMUX": "/tmp/tmux-0/default,1,0"})))
	assert.Equal(t, "zellij", detectMultiplexer(envFrom(map[string]string{"ZELLIJ": "0"})))
	assert.Equal(t, "screen", detectMultiplexer(envFrom(map[string]string{"STY": "123.pts-0"})))
	assert.Empty(t, detectMultiplexer(envFrom(nil)))
}

func TestDetectDisplay(t *testing.T) {
	assert.Equal(t, "wayland", detectDisplay("linux", envFrom(map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"})))
	assert.Equal(t, "x11", detectDisplay("linux", envFrom(map[string]string{"DISPLAY": ":0"})))
	assert.Empty(t, detectDisplay("darwin", envFrom(map[string]string{"DISPLAY": ":0"})))
	assert.Empty(t, detectDisplay("linux", envFrom(nil)))
}

func TestLocalTimezone(t *testing.T) {
	assert.Equal(t, "Asia/Shanghai", localTimezone(envFrom(map[string]string{"TZ": "Asia/Shanghai"})))
	assert.Equal(t, "Europe/Berlin", localTimezone(envFrom(map[string]string{"TZ": ":Europe/Berlin"})))
	// A TZ that is a file path falls through to the system zone
	assert.NotEmpty(t, localTimezone(envFrom(map[string]string{"TZ": "/etc/localtime"})))
}

func TestShareContextEnabled(t *testing.T) {
	on, off := true, false
	assert.True(t, shareContextEnabled(nil))
	assert.True(t, shareContextEnabled(&model.AIConfig{}))
	assert.True(t, shareContextEnabled(&model.AIConfig{ShareContext: &on}))
	assert.False(t, shareContextEnabled(&model.AIConfig{ShareContext: &off}))
}

func TestGatherQueryContext(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"test":"vitest"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), nil, 0o644))
	cmd := exec.Command("git", "-c", "init.defaultBranch=main", "init", "-q", dir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	require.NoError(t, cmd.Run())

	qc := gatherQueryContext(context.Background(), dir)
	require.NotNil(t, qc)
	require.NotNil(t, qc.System)
	assert.NotEmpty(t, qc.System.Arch)
	assert.Positive(t, qc.System.CPUCount)
	assert.NotEmpty(t, qc.System.LocalTime)

	require.NotNil(t, qc.Git)
	assert.Equal(t, "main", qc.Git.Branch)
	require.NotNil(t, qc.Project)
	assert.Equal(t, []string{"pnpm"}, qc.Project.PackageManagers)
	assert.Equal(t, []string{"test"}, qc.Project.PackageScripts)
	require.NotNil(t, qc.Dir)
	assert.Equal(t, []string{"package.json", "pnpm-lock.yaml"}, qc.Dir.Entries)
}

func TestGatherQueryContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Returns promptly with whatever finished; never blocks or panics
	assert.NotNil(t, gatherQueryContext(ctx, t.TempDir()))
}

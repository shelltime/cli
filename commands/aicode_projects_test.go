package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func runProjects(t *testing.T, group string, token, endpoint string, args ...string) error {
	t.Helper()
	setupCCTest(t)
	original := configService
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: token, APIEndpoint: endpoint}, nil).Maybe()
	configService = mc
	t.Cleanup(func() { configService = original })

	app := &cli.App{Name: "t", Commands: []*cli.Command{{Name: group, Subcommands: []*cli.Command{AICodeProjectsCommand}}}}
	return app.Run(append([]string{"t", group, "projects"}, args...))
}

func TestAICodeProjectsMovesClones(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/acme/web.git"}} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}

	var moves int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/cc/projects/unresolved":
			_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]any{{"host": "mbp", "pwd": dir, "sessions": 2}}})
		case "/api/v1/cc/projects/repositories":
			moves++
			_ = json.NewEncoder(w).Encode(map[string]int{"updated": 2})
		}
	}))
	t.Cleanup(server.Close)

	require.NoError(t, runProjects(t, "codex", "tok", server.URL, "--dry-run"))
	assert.Zero(t, moves)
	require.NoError(t, runProjects(t, "cc", "tok", server.URL))
	assert.Equal(t, 1, moves)
}

func TestAICodeProjectsErrors(t *testing.T) {
	assert.ErrorContains(t, runProjects(t, "cc", "", "http://127.0.0.1:0"), "no ShellTime token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	assert.ErrorContains(t, runProjects(t, "cc", "tok", server.URL), "can't group projects by repository yet")
}

package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withMigrationHosts names this machine for the migration in a test.
func withMigrationHosts(t *testing.T, hosts ...string) {
	t.Helper()
	original := projectMigrationHosts
	projectMigrationHosts = func() []string { return hosts }
	t.Cleanup(func() { projectMigrationHosts = original })
}

func TestRunProjectMigrationOnce(t *testing.T) {
	withMigrationHosts(t, "mbp")
	dir := gitRepoWithOrigin(t, "git@github.com:acme/web.git")
	var requests atomic.Int32
	var moved []model.ProjectRepository
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/api/v1/cc/projects/unresolved":
			_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]any{{"host": "mbp", "pwd": dir, "sessions": 2}}})
		case "/api/v1/cc/projects/repositories":
			var body struct {
				Projects []model.ProjectRepository `json:"projects"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			moved = body.Projects
			_ = json.NewEncoder(w).Encode(map[string]int{"updated": 2})
		}
	}))
	t.Cleanup(server.Close)
	cfg := model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	marker := filepath.Join(t.TempDir(), model.ProjectMigrationMarkerFile)

	runProjectMigrationOnce(context.Background(), cfg, 0, marker)
	require.Equal(t, []model.ProjectRepository{{Host: "mbp", Pwd: dir, Repository: "https://github.com/acme/web"}}, moved)
	data, err := os.ReadFile(marker)
	require.NoError(t, err)
	var written projectMigrationMarker
	require.NoError(t, json.Unmarshal(data, &written))
	assert.Equal(t, projectMigrationMarker{MigratedAt: written.MigratedAt, Checked: 1, Linked: 1, Sessions: 2}, written)

	runProjectMigrationOnce(context.Background(), cfg, 0, marker)
	assert.Equal(t, int32(2), requests.Load(), "with the marker in place nothing is asked again")
}

// A machine with only a host name many machines share asks nothing and leaves no marker, so a
// real name (a later `cc install`, a renamed host) is picked up on another start.
func TestRunProjectMigrationOnceWithoutHosts(t *testing.T) {
	withMigrationHosts(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), model.ProjectMigrationMarkerFile)

	runProjectMigrationOnce(context.Background(), model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}, 0, marker)
	assert.NoFileExists(t, marker)
	assert.Zero(t, requests.Load())
}

func TestRunProjectMigrationOnceRetriesATruncatedList(t *testing.T) {
	withMigrationHosts(t, "mbp")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []any{}, "truncated": true})
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), model.ProjectMigrationMarkerFile)

	runProjectMigrationOnce(context.Background(), model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}, 0, marker)
	assert.NoFileExists(t, marker, "the rest of the list is checked on the next start")
}

func TestRunProjectMigrationOnceSurvivesAnUnwritableMarker(t *testing.T) {
	withMigrationHosts(t, "mbp")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
	}))
	t.Cleanup(server.Close)
	parent := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(parent, nil, 0o600))
	marker := filepath.Join(parent, model.ProjectMigrationMarkerFile)

	assert.NotPanics(t, func() {
		runProjectMigrationOnce(context.Background(), model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}, 0, marker)
	})
	assert.NoFileExists(t, marker)
}

func TestRunProjectMigrationOnceRetriesLater(t *testing.T) {
	withMigrationHosts(t, "mbp")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	marker := filepath.Join(t.TempDir(), model.ProjectMigrationMarkerFile)

	runProjectMigrationOnce(context.Background(), model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}, 0, marker)
	assert.NoFileExists(t, marker, "a server without the endpoints is asked again on the next start")

	runProjectMigrationOnce(context.Background(), model.ShellTimeConfig{APIEndpoint: server.URL}, 0, marker)
	assert.NoFileExists(t, marker, "nothing happens without a token")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runProjectMigrationOnce(ctx, model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}, projectMigrationDelay, marker)
	assert.NoFileExists(t, marker, "stopping the daemon ends the wait")
}

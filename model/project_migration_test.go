package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectsServer answers the project repository endpoints and records what it was sent.
type projectsServer struct {
	server     *httptest.Server
	unresolved []UnresolvedProject

	mu    sync.Mutex
	hosts []string
	moved [][]ProjectRepository
}

func newProjectsServer(t *testing.T, unresolved ...UnresolvedProject) *projectsServer {
	t.Helper()
	ps := &projectsServer{unresolved: unresolved}
	ps.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "CLI tok", r.Header.Get("Authorization"))
		ps.mu.Lock()
		defer ps.mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/cc/projects/unresolved":
			var body unresolvedProjectsRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			ps.hosts = body.Hosts
			_ = json.NewEncoder(w).Encode(unresolvedProjectsResponse{Projects: ps.unresolved})
		case "/api/v1/cc/projects/repositories":
			var body projectRepositoriesRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			ps.moved = append(ps.moved, body.Projects)
			_ = json.NewEncoder(w).Encode(projectRepositoriesResponse{Updated: 3 * len(body.Projects)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ps.server.Close)
	return ps
}

func TestMigrateProjectRepositories(t *testing.T) {
	clone := t.TempDir()
	plain := t.TempDir()
	missing := filepath.Join(t.TempDir(), "renamed")
	ps := newProjectsServer(t,
		UnresolvedProject{Host: "mbp", Pwd: clone, Sessions: 4},
		UnresolvedProject{Host: "mbp.local", Pwd: plain, Sessions: 1},
		UnresolvedProject{Host: "mbp", Pwd: missing, Sessions: 2},
	)
	repos := NewGitRepositoryResolver(time.Minute)
	repos.git = func(_ context.Context, dir string, args ...string) (string, error) {
		if dir != clone {
			return "", os.ErrNotExist
		}
		return (&fakeGit{root: clone, remotes: "origin", url: "git@github.com:acme/web.git"}).run(context.Background(), dir, args...)
	}
	endpoint := Endpoint{Token: "tok", APIEndpoint: ps.server.URL}

	result, err := MigrateProjectRepositories(context.Background(), endpoint, []string{"mbp", "mbp.local"}, repos, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"mbp", "mbp.local"}, ps.hosts)
	assert.Equal(t, 3, result.Checked)
	assert.Equal(t, []ProjectRepository{{Host: "mbp", Pwd: clone, Repository: "https://github.com/acme/web"}}, result.Linked)
	assert.Equal(t, 1, result.Missing)
	assert.Equal(t, 1, result.NotRepositories)
	assert.Empty(t, ps.moved, "a dry run changes nothing")

	result, err = MigrateProjectRepositories(context.Background(), endpoint, []string{"mbp"}, repos, false)
	require.NoError(t, err)
	require.Len(t, ps.moved, 1)
	assert.Equal(t, result.Linked, ps.moved[0])
	assert.Equal(t, 3, result.Sessions)
}

func TestMigrateProjectRepositoriesReportsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	t.Cleanup(server.Close)

	_, err := MigrateProjectRepositories(context.Background(), Endpoint{Token: "tok", APIEndpoint: server.URL}, []string{"mbp"}, NewGitRepositoryResolver(0), false)
	var statusErr *HTTPStatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusNotFound, statusErr.StatusCode)
}

func TestProjectMigrationHosts(t *testing.T) {
	hostname, err := os.Hostname()
	require.NoError(t, err)
	hosts := ProjectMigrationHosts("old-name")
	assert.Equal(t, hostname, hosts[0])
	assert.Equal(t, "old-name", hosts[len(hosts)-1])
	if short, _, ok := strings.Cut(hostname, "."); ok {
		assert.Contains(t, hosts, short)
	}
	assert.Equal(t, hosts[:len(hosts)-1], ProjectMigrationHosts(hostname), "names are listed once")
}

func TestResourceMachineName(t *testing.T) {
	dir := t.TempDir()
	s := &ClaudeSettingsAICodeOtelEnvService{settingsPath: filepath.Join(dir, "settings.json")}
	assert.Empty(t, s.ResourceMachineName(), "no settings file")

	require.NoError(t, os.WriteFile(s.settingsPath, []byte(`{"env":{"OTEL_RESOURCE_ATTRIBUTES":"user.name=me, machine.name=old-mbp ,team.id=shelltime"}}`), 0o600))
	assert.Equal(t, "old-mbp", s.ResourceMachineName())

	require.NoError(t, os.WriteFile(s.settingsPath, []byte(`{"env":{}}`), 0o600))
	assert.Empty(t, s.ResourceMachineName())

	require.NoError(t, os.WriteFile(s.settingsPath, []byte(`not json`), 0o600))
	assert.Empty(t, s.ResourceMachineName())
}

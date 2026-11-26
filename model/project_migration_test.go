package model

import (
	"context"
	"encoding/json"
	"errors"
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
	// pageSize pages the listing like the server, by the last project sent; 0 sends it whole.
	pageSize int

	mu     sync.Mutex
	hosts  []string
	afters []*unresolvedProjectCursor
	moved  [][]ProjectRepository
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
			ps.afters = append(ps.afters, body.After)
			start := 0
			if body.After != nil {
				for i, p := range ps.unresolved {
					if p.Host == body.After.Host && p.Pwd == body.After.Pwd {
						start = i + 1
					}
				}
			}
			end := len(ps.unresolved)
			if ps.pageSize > 0 {
				end = min(start+ps.pageSize, end)
			}
			_ = json.NewEncoder(w).Encode(unresolvedProjectsResponse{Projects: ps.unresolved[start:end], Truncated: end < len(ps.unresolved)})
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
			return "", gitExitError{errors.New("not a git repository")}
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
	assert.True(t, result.Complete())
}

// The listing comes a page at a time; folders that can't be moved stay in it, so the run pages
// by the last project of each page.
func TestMigrateProjectRepositoriesPages(t *testing.T) {
	ps := newProjectsServer(t,
		UnresolvedProject{Host: "mbp", Pwd: "/gone/a"},
		UnresolvedProject{Host: "mbp", Pwd: "/gone/b"},
		UnresolvedProject{Host: "mbp", Pwd: "/gone/c"},
	)
	ps.pageSize = 2
	result, err := MigrateProjectRepositories(context.Background(), Endpoint{Token: "tok", APIEndpoint: ps.server.URL}, []string{"mbp"}, NewGitRepositoryResolver(0), false)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Checked)
	assert.Equal(t, 3, result.Missing)
	assert.True(t, result.Complete())
	require.Len(t, ps.afters, 2)
	assert.Nil(t, ps.afters[0])
	assert.Equal(t, &unresolvedProjectCursor{Host: "mbp", Pwd: "/gone/b"}, ps.afters[1])
}

// A folder git couldn't answer for is unknown, not "no repository": the run isn't complete.
func TestMigrateProjectRepositoriesCountsUncheckedFolders(t *testing.T) {
	dir := t.TempDir()
	ps := newProjectsServer(t, UnresolvedProject{Host: "mbp", Pwd: dir})
	repos := NewGitRepositoryResolver(time.Minute)
	repos.git = func(context.Context, string, ...string) (string, error) {
		return "", context.DeadlineExceeded
	}
	result, err := MigrateProjectRepositories(context.Background(), Endpoint{Token: "tok", APIEndpoint: ps.server.URL}, []string{"mbp"}, repos, false)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Unchecked)
	assert.Zero(t, result.NotRepositories)
	assert.False(t, result.Complete())
	assert.Empty(t, ps.moved)
}

// After a few folders go unanswered, a run stops asking about the rest.
func TestMigrateProjectRepositoriesStopsAskingWhenUnanswered(t *testing.T) {
	var projects []UnresolvedProject
	for range 6 {
		projects = append(projects, UnresolvedProject{Host: "mbp", Pwd: t.TempDir()})
	}
	ps := newProjectsServer(t, projects...)
	var mu sync.Mutex
	asked := map[string]bool{}
	repos := NewGitRepositoryResolver(time.Minute)
	repos.git = func(_ context.Context, dir string, _ ...string) (string, error) {
		mu.Lock()
		asked[dir] = true
		mu.Unlock()
		return "", context.DeadlineExceeded
	}
	result, err := MigrateProjectRepositories(context.Background(), Endpoint{Token: "tok", APIEndpoint: ps.server.URL}, []string{"mbp"}, repos, false)
	require.NoError(t, err)
	assert.Equal(t, 6, result.Unchecked)
	assert.False(t, result.Complete())
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, asked, projectMigrationMaxUnanswered)
}

func TestMigrateProjectRepositoriesWithoutHosts(t *testing.T) {
	result, err := MigrateProjectRepositories(context.Background(), Endpoint{Token: "tok", APIEndpoint: "http://127.0.0.1:0"}, nil, NewGitRepositoryResolver(0), false)
	require.NoError(t, err, "no names to ask about means nothing to do, and no request")
	assert.Zero(t, result.Checked)
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
	if genericHostNames[strings.ToLower(hostname)] {
		t.Skip("this machine's host name is a generic one")
	}
	hosts := ProjectMigrationHosts("old-name")
	assert.Equal(t, hostname, hosts[0])
	assert.Equal(t, "old-name", hosts[len(hosts)-1])
	if short, _, ok := strings.Cut(hostname, "."); ok {
		assert.Contains(t, hosts, short)
	}
	assert.Equal(t, hosts[:len(hosts)-1], ProjectMigrationHosts(hostname), "names are listed once")
	assert.NotContains(t, ProjectMigrationHosts("localhost"), "localhost", "a name many machines share is left out")
	assert.NotContains(t, ProjectMigrationHosts("Ubuntu"), "Ubuntu")
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

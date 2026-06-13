package commands

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// c2SetupDotfiles swaps in a mock ConfigService for the dotfiles tests,
// restoring the original on cleanup.
func c2SetupDotfiles(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

// c2DotfilesServer returns an httptest server replying with the given GraphQL
// JSON body for the fetch-dotfiles request, closed via t.Cleanup.
func c2DotfilesServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestC2PullDotfiles_UnknownAppOnServerSkipped covers the loop branch where the
// server returns an app name with no local handler: it is warned and skipped,
// yielding "No dotfiles to process".
func TestC2PullDotfiles_UnknownAppOnServerSkipped(t *testing.T) {
	mc := c2SetupDotfiles(t)
	t.Setenv("HOME", t.TempDir())

	srv := c2DotfilesServer(t, `{
		"data": {"fetchUser": {"id": 1, "dotfiles": {"totalCount": 1, "apps": [
			{"app": "totally-unknown-app", "files": [
				{"path": "~/.whatever", "records": [
					{"id": 1, "content": "x", "contentHash": "h", "size": 1, "fileType": "text",
					 "host": {"id": 0, "hostname": ""},
					 "createdAt": "2024-01-01T00:00:00Z", "updatedAt": "2024-01-02T00:00:00Z"}
				]}
			]}
		]}}}
	}`)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	require.NoError(t, app.Run([]string{"t", "dotfiles", "pull"}))
}

// TestC2PullDotfiles_HostlessRecordPreferred exercises record selection when a
// file has both a host-specific record and a host-less ("general") record: the
// host-less one is chosen. Dry-run keeps the filesystem untouched while still
// driving IsEqual/Backup/Save and printPullResults.
func TestC2PullDotfiles_HostlessRecordPreferred(t *testing.T) {
	mc := c2SetupDotfiles(t)
	t.Setenv("HOME", t.TempDir())

	srv := c2DotfilesServer(t, `{
		"data": {"fetchUser": {"id": 7, "dotfiles": {"totalCount": 1, "apps": [
			{"app": "bash", "files": [
				{"path": "~/.bashrc", "records": [
					{"id": 1, "content": "host-specific\n", "contentHash": "h1", "size": 14, "fileType": "bash",
					 "host": {"id": 5, "hostname": "box"},
					 "createdAt": "2024-01-01T00:00:00Z", "updatedAt": "2024-01-03T00:00:00Z"},
					{"id": 2, "content": "general-config\n", "contentHash": "h2", "size": 15, "fileType": "bash",
					 "host": {"id": 0, "hostname": ""},
					 "createdAt": "2024-01-01T00:00:00Z", "updatedAt": "2024-01-02T00:00:00Z"}
				]}
			]}
		]}}}
	}`)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	require.NoError(t, app.Run([]string{"t", "dotfiles", "pull", "--apps", "bash", "--dry-run"}))
}

// TestC2PullDotfiles_AllUpToDateSkipped covers the equality branch: the local
// file already matches the server content, so it is recorded as skipped and the
// "All files are up to date" path is taken (no write attempted).
func TestC2PullDotfiles_AllUpToDateSkipped(t *testing.T) {
	mc := c2SetupDotfiles(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	content := "identical-content\n"
	// Pre-create the local ~/.bash_logout with the exact server content so IsEqual
	// reports true. (~/.bash_logout is a config path but NOT a shelltime include
	// directive, so equality is a straight content hash compare.)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bash_logout"), []byte(content), 0644))

	srv := c2DotfilesServer(t, `{
		"data": {"fetchUser": {"id": 7, "dotfiles": {"totalCount": 1, "apps": [
			{"app": "bash", "files": [
				{"path": "~/.bash_logout", "records": [
					{"id": 1, "content": "identical-content\n", "contentHash": "h", "size": 18, "fileType": "bash",
					 "host": {"id": 0, "hostname": ""},
					 "createdAt": "2024-01-01T00:00:00Z", "updatedAt": "2024-01-02T00:00:00Z"}
				]}
			]}
		]}}}
	}`)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	require.NoError(t, app.Run([]string{"t", "dotfiles", "pull", "--apps", "bash"}))

	// Content must be untouched (it was already identical).
	got, err := os.ReadFile(filepath.Join(home, ".bash_logout"))
	require.NoError(t, err)
	assert.Equal(t, content, string(got))
}

// TestC2PullDotfiles_NonDryRunWritesFile covers the real (non-dry-run) apply
// path: a non-include config file (~/.bash_logout) absent locally is written to
// disk via the diff-merge Save, producing an isSuccess result and the "Updated"
// summary branch.
func TestC2PullDotfiles_NonDryRunWritesFile(t *testing.T) {
	mc := c2SetupDotfiles(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := c2DotfilesServer(t, `{
		"data": {"fetchUser": {"id": 7, "dotfiles": {"totalCount": 1, "apps": [
			{"app": "bash", "files": [
				{"path": "~/.bash_logout", "records": [
					{"id": 1, "content": "from-server\n", "contentHash": "h", "size": 12, "fileType": "bash",
					 "host": {"id": 0, "hostname": ""},
					 "createdAt": "2024-01-01T00:00:00Z", "updatedAt": "2024-01-02T00:00:00Z"}
				]}
			]}
		]}}}
	}`)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	require.NoError(t, app.Run([]string{"t", "dotfiles", "pull", "--apps", "bash"}))

	// The file should now exist with the server content.
	got, err := os.ReadFile(filepath.Join(home, ".bash_logout"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "from-server")
}

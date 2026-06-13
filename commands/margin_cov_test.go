package commands

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// x3SetupCmdMargin installs a mock ConfigService + isolated HOME, restoring the
// original ConfigService on cleanup.
func x3SetupCmdMargin(t *testing.T) (string, *model.MockConfigService) {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return home, mc
}

// TestX3PullDotfiles_HostSpecificOnlyUsesLatest covers the record-selection
// branch where every record for a file carries a host (no general/host-less
// record), so the code falls back to the latest record overall.
func TestX3PullDotfiles_HostSpecificOnlyUsesLatest(t *testing.T) {
	_, mc := x3SetupCmdMargin(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Two records, both with a host -> selectedRecord stays nil during the
		// loop and is set to latestRecord afterwards (the 2024-01-03 one).
		_, _ = io.WriteString(w, `{
			"data": {"fetchUser": {"id": 9, "dotfiles": {"totalCount": 1, "apps": [
				{"app": "bash", "files": [
					{"path": "~/.bash_logout", "records": [
						{"id": 1, "content": "older\n", "contentHash": "h1", "size": 6, "fileType": "bash",
						 "host": {"id": 5, "hostname": "box-a"},
						 "createdAt": "2024-01-01T00:00:00Z", "updatedAt": "2024-01-02T00:00:00Z"},
						{"id": 2, "content": "newer\n", "contentHash": "h2", "size": 6, "fileType": "bash",
						 "host": {"id": 6, "hostname": "box-b"},
						 "createdAt": "2024-01-01T00:00:00Z", "updatedAt": "2024-01-03T00:00:00Z"}
					]}
				]}
			]}}}
		}`)
	}))
	t.Cleanup(srv.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	// dry-run avoids any disk writes while still exercising the selection logic.
	require.NoError(t, app.Run([]string{"t", "dotfiles", "pull", "--apps", "bash", "--dry-run"}))
}

// TestX3CodexUninstall_MalformedConfigErrors covers the uninstall error branch of
// commandCodexUninstall: a malformed ~/.codex/config.toml makes the underlying
// Uninstall fail to parse, so the command returns an error.
func TestX3CodexUninstall_MalformedConfigErrors(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)

	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0o755))
	// Invalid TOML -> toml.Unmarshal fails inside Uninstall.
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("this is = = not valid toml ]["), 0o644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{CodexCommand}}
	err := app.Run([]string{"t", "codex", "uninstall"})
	require.Error(t, err)
}

// TestX3ConfigView_TableNoToken exercises outputConfigTable with an empty Token
// (the "<empty>" string-value branch of flattenConfig) and a nil pointer field
// rendered as "<not set>".
func TestX3ConfigView_TableNoToken(t *testing.T) {
	_, mc := x3SetupCmdMargin(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "", // -> "<empty>"
		APIEndpoint: "https://api.shelltime.xyz",
		// DataMasking left nil -> "<not set>" pointer branch.
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{ConfigViewCommand}}
	require.NoError(t, app.Run([]string{"t", "view"}))
}

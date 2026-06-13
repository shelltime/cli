package commands

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func setupDotfilesTest(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

// --- printPullResults (pure-ish stdout glue) ----------------------------------

func TestPrintPullResults_Empty(t *testing.T) {
	// Empty result map -> "No dotfiles to process" branch, returns early.
	assert.NotPanics(t, func() {
		printPullResults(map[model.DotfileAppName][]dotfilePullFileResult{}, false)
	})
}

func TestPrintPullResults_AllSkipped(t *testing.T) {
	result := map[model.DotfileAppName][]dotfilePullFileResult{
		model.AppBash: {
			{path: "~/.bashrc", isSkipped: true},
		},
	}
	// Only skipped -> summary printed, "All dotfiles are up to date" branch.
	assert.NotPanics(t, func() { printPullResults(result, false) })
}

func TestPrintPullResults_UpdatedAndFailed(t *testing.T) {
	result := map[model.DotfileAppName][]dotfilePullFileResult{
		model.AppBash: {
			{path: "~/.bashrc", isSuccess: true},
			{path: "~/.bash_profile", isFailed: true},
		},
		model.AppZsh: {
			{path: "~/.zshrc", isSkipped: true},
		},
	}
	// Non-dry-run: "Updated"/"Failed" plus details table.
	assert.NotPanics(t, func() { printPullResults(result, false) })
}

func TestPrintPullResults_DryRunWouldUpdate(t *testing.T) {
	result := map[model.DotfileAppName][]dotfilePullFileResult{
		model.AppGit: {
			{path: "~/.gitconfig", isSuccess: true},
		},
	}
	// Dry-run: "Would Update" labels.
	assert.NotPanics(t, func() { printPullResults(result, true) })
}

// --- pushDotfiles action ------------------------------------------------------

func TestPushDotfiles_ConfigReadError(t *testing.T) {
	mc := setupDotfilesTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	err := app.Run([]string{"t", "dotfiles", "push"})
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestPushDotfiles_NoToken(t *testing.T) {
	mc := setupDotfilesTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: ""}, nil)
	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	err := app.Run([]string{"t", "dotfiles", "push"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no token found")
}

func TestPushDotfiles_NoDotfilesFound(t *testing.T) {
	mc := setupDotfilesTest(t)
	// Empty HOME so no app collects any dotfiles -> "No dotfiles found to push".
	t.Setenv("HOME", t.TempDir())
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: "https://example.invalid",
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	// Filter to a single app to keep collection minimal and deterministic.
	err := app.Run([]string{"t", "dotfiles", "push", "--apps", "bash"})
	require.NoError(t, err)
}

func TestPushDotfiles_UnknownAppStillSucceeds(t *testing.T) {
	mc := setupDotfilesTest(t)
	t.Setenv("HOME", t.TempDir())
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: "https://example.invalid",
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	// Unknown app name -> warned, selectedApps empty -> no dotfiles -> nil.
	err := app.Run([]string{"t", "dotfiles", "push", "--apps", "not-a-real-app"})
	require.NoError(t, err)
}

// --- pullDotfiles action ------------------------------------------------------

func TestPullDotfiles_ConfigReadError(t *testing.T) {
	mc := setupDotfilesTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	err := app.Run([]string{"t", "dotfiles", "pull"})
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestPullDotfiles_NoToken(t *testing.T) {
	mc := setupDotfilesTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: ""}, nil)
	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	err := app.Run([]string{"t", "dotfiles", "pull"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no token found")
}

func TestPullDotfiles_FetchError(t *testing.T) {
	mc := setupDotfilesTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	err := app.Run([]string{"t", "dotfiles", "pull"})
	require.Error(t, err)
}

func TestPullDotfiles_NoDotfilesOnServer(t *testing.T) {
	mc := setupDotfilesTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"fetchUser":{"id":1,"dotfiles":{"totalCount":0,"apps":[]}}}}`)
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	// Empty apps list -> "No dotfiles found on server", returns nil.
	err := app.Run([]string{"t", "dotfiles", "pull"})
	require.NoError(t, err)
}

func TestPullDotfiles_DryRunProcessesServerDotfile(t *testing.T) {
	mc := setupDotfilesTest(t)
	// Isolated HOME ensures the local ~/.bashrc doesn't exist (so it's "not
	// equal" and would be updated), and dry-run means nothing is actually
	// written to disk.
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"data": {
				"fetchUser": {
					"id": 42,
					"dotfiles": {
						"totalCount": 1,
						"apps": [
							{
								"app": "bash",
								"files": [
									{
										"path": "~/.bashrc",
										"records": [
											{
												"id": 1,
												"content": "export FROM_SERVER=1\n",
												"contentHash": "h",
												"size": 20,
												"fileType": "bash",
												"host": {"id": 0, "hostname": ""},
												"createdAt": "2024-01-01T00:00:00Z",
												"updatedAt": "2024-01-02T00:00:00Z"
											}
										]
									}
								]
							}
						]
					}
				}
			}
		}`)
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	// dry-run: exercises the deep processing path (record selection, IsEqual,
	// Backup, Save) without mutating any real files, then printPullResults.
	err := app.Run([]string{"t", "dotfiles", "pull", "--dry-run"})
	require.NoError(t, err)
}

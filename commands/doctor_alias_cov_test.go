package commands

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// x3SetupDoctor isolates HOME and installs a mock ConfigService for doctor tests.
func x3SetupDoctor(t *testing.T) (string, *model.MockConfigService) {
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

// TestX3Doctor_ShelltimeDirIsFile covers the "!info.IsDir()" branch: the
// ~/.shelltime path exists but is a regular file rather than a directory.
func TestX3Doctor_ShelltimeDirIsFile(t *testing.T) {
	home, mc := x3SetupDoctor(t)
	t.Setenv("SHELL", "/bin/bash")
	// Create a *file* named .shelltime so os.Stat succeeds but IsDir() is false.
	require.NoError(t, os.WriteFile(filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER), []byte("x"), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DoctorCommand}}
	require.NoError(t, app.Run([]string{"t", "doctor"}))
}

// TestX3Doctor_NormalLogFileAndInstalledHook covers two branches at once:
//   - the log file exists and is below the size threshold (normal-size branch);
//   - the bash hook is installed and Check() succeeds for the current ($SHELL)
//     shell (the "Hook is already installed" branch).
func TestX3Doctor_NormalLogFileAndInstalledHook(t *testing.T) {
	home, mc := x3SetupDoctor(t)
	t.Setenv("SHELL", "/bin/bash")

	base := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER)
	require.NoError(t, os.MkdirAll(base, 0755))
	// Small log.log -> "size is normal" branch.
	require.NoError(t, os.WriteFile(filepath.Join(base, "log.log"), []byte("ok\n"), 0644))

	// Seed .bashrc with the exact bash hook lines so bashHookService.Check passes.
	bashrc := filepath.Join(home, ".bashrc")
	content := "# Added by shelltime CLI\n" +
		fmt.Sprintf("export PATH=\"$HOME/%s/bin:$PATH\"\n", model.COMMAND_BASE_STORAGE_FOLDER) +
		fmt.Sprintf("source %s\n", filepath.Join(base, "hooks", "bash.bash"))
	require.NoError(t, os.WriteFile(bashrc, []byte(content), 0644))

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DoctorCommand}}
	require.NoError(t, app.Run([]string{"t", "doctor"}))
}

// --- alias import: fish path --------------------------------------------------

// TestX3ImportAliases_SendsFishAliases covers the fish-config branch of
// importAliases (the existing suite only drives the zsh branch).
func TestX3ImportAliases_SendsFishAliases(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })

	var calls int32
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		lastPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"count":1}`))
	}))
	t.Cleanup(srv.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
	}, nil)

	dir := t.TempDir()
	fishPath := filepath.Join(dir, "config.fish")
	require.NoError(t, os.WriteFile(fishPath, []byte("alias gs 'git status'\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{AliasCommand}}
	err := app.Run([]string{"t", "alias", "import",
		"--zsh-config", filepath.Join(dir, "missing-zsh"),
		"--fish-config", fishPath,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "exactly one import call for fish")
	assert.Equal(t, "/api/v1/import-alias", lastPath)
}

// TestX3ImportAliases_FishServerErrorPropagates covers the fish send-error branch.
func TestX3ImportAliases_FishServerErrorPropagates(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
	}, nil)

	dir := t.TempDir()
	fishPath := filepath.Join(dir, "config.fish")
	require.NoError(t, os.WriteFile(fishPath, []byte("alias gs 'git status'\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{AliasCommand}}
	err := app.Run([]string{"t", "alias", "import",
		"--zsh-config", filepath.Join(dir, "missing-zsh"),
		"--fish-config", fishPath,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send aliases to server")
}

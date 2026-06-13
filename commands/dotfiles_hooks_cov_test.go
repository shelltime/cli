package commands

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// x3SetupCmd swaps in a mock ConfigService and an isolated HOME, restoring the
// original ConfigService on cleanup. Returns the temp HOME and the mock.
func x3SetupCmd(t *testing.T) (string, *model.MockConfigService) {
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

// --- pushDotfiles success send path -------------------------------------------

// TestX3PushDotfiles_SendsCollectedDotfilesToServer drives pushDotfiles all the
// way through model.SendDotfilesToServer: a seeded ~/.bashrc gives the bash app
// something to collect, and the httptest backend returns a userId so the
// "Successfully pushed" / web-link formatting branch runs.
func TestX3PushDotfiles_SendsCollectedDotfilesToServer(t *testing.T) {
	home, mc := x3SetupCmd(t)

	// Seed a bash dotfile so CollectDotfiles produces at least one item.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("export FOO=1\n"), 0644))

	var calls int32
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":1,"failed":0,"userId":42,"results":[]}`)
	}))
	t.Cleanup(srv.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	// Filter to just bash to keep collection deterministic.
	require.NoError(t, app.Run([]string{"t", "dotfiles", "push", "--apps", "bash"}))

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "exactly one push call")
	assert.Equal(t, "/api/v1/dotfiles/push", gotPath)
}

// TestX3PushDotfiles_ServerErrorPropagates covers the send-failure branch: the
// backend returns 500, so SendDotfilesToServer errors and pushDotfiles returns it.
func TestX3PushDotfiles_ServerErrorPropagates(t *testing.T) {
	home, mc := x3SetupCmd(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("export FOO=1\n"), 0644))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	}))
	t.Cleanup(srv.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{DotfilesCommand}}
	err := app.Run([]string{"t", "dotfiles", "push", "--apps", "bash"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send dotfiles to server")
}

// --- hooks install happy path -------------------------------------------------

// TestX3HooksInstall_InstallsBashHook covers the install path of
// commandHooksInstall (lines past the binary-found gate). The bin folder is
// created so the binary check passes, and bash-preexec.sh is pre-seeded so the
// bash hook Install() does not attempt a network download. zsh/fish installs
// fail silently (their configs are absent), bash succeeds and writes hook lines.
func TestX3HooksInstall_InstallsBashHook(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)

	base := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER)
	// bin folder present -> "binary not found" gate is skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(base, "bin"), 0755))
	// Pre-seed bash-preexec.sh so ensureBashPreexec short-circuits (no network).
	hooksDir := filepath.Join(base, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash-preexec.sh"), []byte("# stub\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{HooksInstallCommand}}
	require.NoError(t, app.Run([]string{"t", "install"}))

	// bash config is auto-created and carries the shelltime hook marker.
	data, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "# Added by shelltime CLI")
}

// TestX3HooksInstall_BinaryFoundViaPath covers the alternate gate where the bin
// folder is absent but `shelltime` is resolvable on PATH, so installation still
// proceeds.
func TestX3HooksInstall_BinaryFoundViaPath(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a fake `shelltime` executable on a dir we put on PATH.
	binDir := t.TempDir()
	exe := filepath.Join(binDir, "shelltime")
	require.NoError(t, os.WriteFile(exe, []byte("#!/bin/sh\n"), 0755))
	t.Setenv("PATH", binDir)

	// Pre-seed bash-preexec.sh under the (otherwise absent) storage hooks dir.
	hooksDir := filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER, "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "bash-preexec.sh"), []byte("# stub\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{HooksInstallCommand}}
	require.NoError(t, app.Run([]string{"t", "install"}))

	data, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "# Added by shelltime CLI"))
}

// --- hooks uninstall happy path -----------------------------------------------

// TestX3HooksUninstall_RemovesBashHook installs then uninstalls, covering the
// success branches of commandHooksUninstall (all three Uninstall() calls
// returning nil) and verifying the bash hook lines are stripped.
func TestX3HooksUninstall_RemovesBashHook(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	// Seed a .bashrc that already contains the hook lines so Uninstall has work.
	bashrc := filepath.Join(home, ".bashrc")
	content := "# existing\n" +
		"# Added by shelltime CLI\n" +
		"export PATH=\"$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER + "/bin:$PATH\"\n" +
		"source " + filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER, "hooks", "bash.bash") + "\n"
	require.NoError(t, os.WriteFile(bashrc, []byte(content), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{HooksUninstallCommand}}
	require.NoError(t, app.Run([]string{"t", "uninstall"}))

	data, err := os.ReadFile(bashrc)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "# Added by shelltime CLI", "uninstall should strip hook lines")
}

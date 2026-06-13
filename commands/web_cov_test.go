package commands

import (
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

// TestX3CommandWeb_OpensBrowser covers the success path of commandWeb. A fake
// `xdg-open` executable is placed on PATH so github.com/pkg/browser.OpenURL
// resolves and runs it (exit 0), driving the "Opening ... in your default
// browser" branch without launching a real browser.
func TestX3CommandWeb_OpensBrowser(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	origCfg := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = origCfg })

	// Fake browser launcher: xdg-open is tried first on linux.
	binDir := t.TempDir()
	xdg := filepath.Join(binDir, "xdg-open")
	require.NoError(t, os.WriteFile(xdg, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", binDir)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{WebCommand}}
	require.NoError(t, app.Run([]string{"t", "web"}))
}

// TestX3CommandWeb_OpenURLFails covers the browser-launch failure branch: no
// browser provider is resolvable on an empty PATH, so OpenURL returns an error.
func TestX3CommandWeb_OpenURLFails(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	origCfg := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = origCfg })

	// Empty PATH -> none of xdg-open/x-www-browser/www-browser resolve.
	t.Setenv("PATH", "")

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		WebEndpoint: "https://shelltime.xyz",
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{WebCommand}}
	err := app.Run([]string{"t", "web"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open browser")
}

// --- AdjustPathForCurrentUser additional branches -----------------------------

// TestX3AdjustPathForCurrentUser_RootAndNoMatch covers the /root/ rewrite branch
// and the no-standard-pattern pass-through branch.
func TestX3AdjustPathForCurrentUser_RootAndNoMatch(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// /root/<rest> -> <home>/<rest>
	assert.Equal(t, home+"/.config/app", AdjustPathForCurrentUser("/root/.config/app"))

	// Unrecognized prefix is returned unchanged.
	assert.Equal(t, "/opt/data/file", AdjustPathForCurrentUser("/opt/data/file"))

	// /Users/<name>/<rest> -> <home>/<rest>
	assert.Equal(t, home+"/.zshrc", AdjustPathForCurrentUser("/Users/someone/.zshrc"))

	// /home/<name>/<rest> -> <home>/<rest>
	assert.Equal(t, home+"/.bashrc", AdjustPathForCurrentUser("/home/other/.bashrc"))
}

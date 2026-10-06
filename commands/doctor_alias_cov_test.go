package commands

import (
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

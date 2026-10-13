package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestCommandAuth_RestrictsConfigPermissions(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	prevSkip := SKIP_LOGGER_SETTINGS
	SKIP_LOGGER_SETTINGS = true
	t.Cleanup(func() { SKIP_LOGGER_SETTINGS = prevSkip })

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".shelltime")
	require.NoError(t, os.Mkdir(dir, 0o755))
	cfgPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("token: old\napiEndpoint: https://api.shelltime.xyz\nwebEndpoint: https://shelltime.xyz\n"), 0o644))

	orig := configService
	configService = model.NewConfigService(dir)
	t.Cleanup(func() { configService = orig })

	app := &cli.App{Name: "t", Commands: []*cli.Command{AuthCommand}}
	require.NoError(t, app.Run([]string{"t", "auth", "--token", "secret-token"}))

	info, err := os.Stat(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())

	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "secret-token")
	assert.NotContains(t, string(data), "token: old")
}

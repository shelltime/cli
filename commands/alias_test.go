package commands

import (
	"context"
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

func setupAliasTest(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

// --- pure alias helpers -------------------------------------------------------

func TestParseZshAliasLine_PassThrough(t *testing.T) {
	got, ok := parseZshAliasLine("alias gs='git status'")
	assert.True(t, ok)
	assert.Equal(t, "alias gs='git status'", got)
}

func TestParseFishAliasLine_PassThrough(t *testing.T) {
	got, ok := parseFishAliasLine("alias gs 'git status'")
	assert.True(t, ok)
	assert.Equal(t, "alias gs 'git status'", got)
}

func TestParseAliasFile_SkipsBlankAndComments(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".zshrc")
	content := "" +
		"# a comment\n" +
		"\n" +
		"   \n" +
		"alias gs='git status'\n" +
		"  alias ll='ls -la'  \n" +
		"# another comment\n"
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))

	aliases, err := parseAliasFile(p, parseZshAliasLine)
	require.NoError(t, err)
	// Two real alias lines; blanks and comments are dropped; lines are trimmed.
	require.Len(t, aliases, 2)
	assert.Equal(t, "alias gs='git status'", aliases[0])
	assert.Equal(t, "alias ll='ls -la'", aliases[1])
}

func TestParseAliasFile_MissingFile(t *testing.T) {
	_, err := parseAliasFile(filepath.Join(t.TempDir(), "nope"), parseZshAliasLine)
	require.Error(t, err)
}

func TestParseZshAliases_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".zshrc")
	require.NoError(t, os.WriteFile(p, []byte("alias a='b'\n"), 0644))
	aliases, err := parseZshAliases(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, []string{"alias a='b'"}, aliases)
}

func TestParseFishAliases_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.fish")
	require.NoError(t, os.WriteFile(p, []byte("alias a 'b'\n"), 0644))
	aliases, err := parseFishAliases(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, []string{"alias a 'b'"}, aliases)
}

// --- importAliases action -----------------------------------------------------

func TestImportAliases_ConfigReadError(t *testing.T) {
	mc := setupAliasTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)

	app := &cli.App{Name: "t", Commands: []*cli.Command{AliasCommand}}
	// Point both config files at non-existent paths so path expansion succeeds
	// but the config read fails first.
	err := app.Run([]string{"t", "alias", "import",
		"--zsh-config", filepath.Join(t.TempDir(), "nope-zsh"),
		"--fish-config", filepath.Join(t.TempDir(), "nope-fish"),
	})
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestImportAliases_NoConfigFilesPresent(t *testing.T) {
	mc := setupAliasTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: "https://example.invalid",
	}, nil)

	// Neither config file exists -> os.Stat fails for both -> no server calls,
	// action returns nil.
	app := &cli.App{Name: "t", Commands: []*cli.Command{AliasCommand}}
	err := app.Run([]string{"t", "alias", "import",
		"--zsh-config", filepath.Join(t.TempDir(), "missing-zsh"),
		"--fish-config", filepath.Join(t.TempDir(), "missing-fish"),
	})
	require.NoError(t, err)
}

func TestImportAliases_SendsZshAliasesToServer(t *testing.T) {
	mc := setupAliasTest(t)

	var calls int32
	var lastPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		lastPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"count":2}`))
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)

	dir := t.TempDir()
	zshPath := filepath.Join(dir, ".zshrc")
	require.NoError(t, os.WriteFile(zshPath, []byte("alias gs='git status'\nalias ll='ls -la'\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{AliasCommand}}
	err := app.Run([]string{"t", "alias", "import",
		"--zsh-config", zshPath,
		"--fish-config", filepath.Join(dir, "missing-fish"),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "exactly one import call for zsh")
	assert.Equal(t, "/api/v1/import-alias", lastPath)
}

func TestImportAliases_EmptyAliasFileSkipsServer(t *testing.T) {
	mc := setupAliasTest(t)

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"success":true,"count":0}`))
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)

	dir := t.TempDir()
	// A file that exists but contains only comments/blanks -> 0 aliases parsed.
	zshPath := filepath.Join(dir, ".zshrc")
	require.NoError(t, os.WriteFile(zshPath, []byte("# only comments\n\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{AliasCommand}}
	err := app.Run([]string{"t", "alias", "import",
		"--zsh-config", zshPath,
		"--fish-config", filepath.Join(dir, "missing-fish"),
	})
	require.NoError(t, err)
	// SendAliasesToServer returns early when there are no aliases, so no HTTP call.
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls), "no server call when alias list is empty")
}

func TestImportAliases_ServerErrorPropagates(t *testing.T) {
	mc := setupAliasTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`fail`))
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)

	dir := t.TempDir()
	zshPath := filepath.Join(dir, ".zshrc")
	require.NoError(t, os.WriteFile(zshPath, []byte("alias gs='git status'\n"), 0644))

	app := &cli.App{Name: "t", Commands: []*cli.Command{AliasCommand}}
	err := app.Run([]string{"t", "alias", "import",
		"--zsh-config", zshPath,
		"--fish-config", filepath.Join(dir, "missing-fish"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send aliases to server")
}

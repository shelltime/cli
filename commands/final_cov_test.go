package commands

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestX3CCStatusline_StdinTimeoutFallback covers the readStdinWithTimeout error
// branch (-> outputFallback) of commandCCStatusline: stdin never reaches EOF, so
// the 100ms operation timeout fires and the fallback line is printed.
func TestX3CCStatusline_StdinTimeoutFallback(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	// configService is not consulted on this path (it returns before reading
	// config), but commandCCStatusline references it; install a mock so a stray
	// call would be caught rather than nil-panicking. No expectations set.
	origCfg := configService
	configService = model.NewMockConfigService(t)
	t.Cleanup(func() { configService = origCfg })

	// A pipe whose writer is never closed: the reader goroutine blocks forever,
	// so the command's 100ms context timeout wins -> readStdinWithTimeout errors.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		_ = w.Close()
		_ = r.Close()
	})

	app := &cli.App{Name: "t", Commands: []*cli.Command{CCStatuslineCommand}}
	require.NoError(t, app.Run([]string{"t", "statusline"}))
}

// TestX3PullDotfiles_FileWithNoRecordsSkipped covers the dotfiles_pull branch
// where a server file entry has an empty records list: it is skipped (debug log)
// and produces no work, yielding "No dotfiles to process".
func TestX3PullDotfiles_FileWithNoRecordsSkipped(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	t.Setenv("HOME", t.TempDir())

	origCfg := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = origCfg })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// One bash file with an empty records array -> the "no records" continue.
		_, _ = io.WriteString(w, `{
			"data": {"fetchUser": {"id": 1, "dotfiles": {"totalCount": 1, "apps": [
				{"app": "bash", "files": [
					{"path": "~/.bashrc", "records": []}
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
	require.NoError(t, app.Run([]string{"t", "dotfiles", "pull", "--apps", "bash"}))
}

// TestX3CommandGrep_ServerErrorTableFormat covers the non-JSON (table) error
// output branch of commandGrep: a 500 from the server prints a red error and the
// action returns nil (table format).
func TestX3CommandGrep_ServerErrorTableFormat(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true

	origCfg := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = origCfg })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "boom")
	}))
	t.Cleanup(srv.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: srv.URL,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	// Default (table) format -> the color.Red error-print branch, returns nil.
	require.NoError(t, app.Run([]string{"t", "rg", "git"}))
}

// TestX3OutputGrepJSON_Marshalable is a tiny direct check ensuring the JSON
// output helper round-trips a representative edge set without error.
func TestX3OutputGrepJSON_Marshalable(t *testing.T) {
	edges := []model.SearchCommandEdge{{ID: 1, Shell: "bash", Command: "ls"}}
	require.NoError(t, outputGrepJSON(edges, 1))
	// Sanity: the same structure is valid JSON.
	_, err := json.Marshal(struct {
		TotalCount int                       `json:"totalCount"`
		Commands   []model.SearchCommandEdge `json:"commands"`
	}{1, edges})
	require.NoError(t, err)
}

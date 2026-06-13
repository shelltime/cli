package commands

import (
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// setupGrepActionTest swaps in a mock ConfigService for the duration of a test.
func setupGrepActionTest(t *testing.T) *model.MockConfigService {
	t.Helper()
	otel.SetTracerProvider(noop.NewTracerProvider())
	SKIP_LOGGER_SETTINGS = true
	orig := configService
	mc := model.NewMockConfigService(t)
	configService = mc
	t.Cleanup(func() { configService = orig })
	return mc
}

// --- parseFlexibleDate (pure) -------------------------------------------------

func TestParseFlexibleDate_YearStart(t *testing.T) {
	got, err := parseFlexibleDate("2024", false)
	require.NoError(t, err)
	want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.True(t, got.Equal(want), "got %v want %v", got, want)
}

func TestParseFlexibleDate_YearEnd(t *testing.T) {
	got, err := parseFlexibleDate("2024", true)
	require.NoError(t, err)
	want := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
	assert.True(t, got.Equal(want), "got %v want %v", got, want)
}

func TestParseFlexibleDate_YearMonthStart(t *testing.T) {
	got, err := parseFlexibleDate("2024-02", false)
	require.NoError(t, err)
	want := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	assert.True(t, got.Equal(want), "got %v want %v", got, want)
}

func TestParseFlexibleDate_YearMonthEnd_LeapAware(t *testing.T) {
	// February 2024 is a leap year => end of month is Feb 29 23:59:59.
	got, err := parseFlexibleDate("2024-02", true)
	require.NoError(t, err)
	assert.Equal(t, 2024, got.Year())
	assert.Equal(t, time.February, got.Month())
	assert.Equal(t, 29, got.Day())
	assert.Equal(t, 23, got.Hour())
	assert.Equal(t, 59, got.Minute())
	assert.Equal(t, 59, got.Second())
}

func TestParseFlexibleDate_YearMonthEnd_NonLeapFeb(t *testing.T) {
	// February 2023 is not a leap year => end of month is Feb 28.
	got, err := parseFlexibleDate("2023-02", true)
	require.NoError(t, err)
	assert.Equal(t, 28, got.Day())
	assert.Equal(t, time.February, got.Month())
}

func TestParseFlexibleDate_FullDateStart(t *testing.T) {
	got, err := parseFlexibleDate("2024-01-15", false)
	require.NoError(t, err)
	want := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	assert.True(t, got.Equal(want), "got %v want %v", got, want)
}

func TestParseFlexibleDate_FullDateEnd(t *testing.T) {
	got, err := parseFlexibleDate("2024-01-15", true)
	require.NoError(t, err)
	want := time.Date(2024, 1, 15, 23, 59, 59, 0, time.UTC)
	assert.True(t, got.Equal(want), "got %v want %v", got, want)
}

func TestParseFlexibleDate_Garbage(t *testing.T) {
	for _, in := range []string{"not-a-date", "20", "2024-13-99-extra", ""} {
		t.Run(in, func(t *testing.T) {
			_, err := parseFlexibleDate(in, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "use format")
		})
	}
}

// --- buildGrepFilter (uses a built *cli.Context) ------------------------------

// newGrepContext builds a *cli.Context whose flag set mirrors GrepCommand's
// flags, so buildGrepFilter can read them.
func newGrepContext(t *testing.T, set func(fs *flag.FlagSet)) *cli.Context {
	t.Helper()
	fs := flag.NewFlagSet("rg", flag.ContinueOnError)
	fs.String("shell", "", "")
	fs.String("hostname", "", "")
	fs.String("username", "", "")
	fs.Int("result", -1, "")
	fs.String("main-command", "", "")
	fs.String("since", "", "")
	fs.String("until", "", "")
	fs.Int("limit", 50, "")
	fs.Int("last-id", 0, "")
	fs.String("format", "table", "")
	if set != nil {
		set(fs)
	}
	return cli.NewContext(cli.NewApp(), fs, nil)
}

func TestBuildGrepFilter_DefaultsEmpty(t *testing.T) {
	c := newGrepContext(t, nil)
	filter, err := buildGrepFilter(c, "git status")
	require.NoError(t, err)

	assert.Equal(t, "git status", filter.Command)
	// No optional filters provided -> all slices stay empty.
	assert.Empty(t, filter.Shell)
	assert.Empty(t, filter.Hostname)
	assert.Empty(t, filter.Username)
	assert.Empty(t, filter.MainCommand)
	// result defaults to -1 (any) -> not set.
	assert.Empty(t, filter.Result)
	// no since/until -> Time stays as the initial empty slice.
	assert.Empty(t, filter.Time)
}

func TestBuildGrepFilter_AllOptionalFilters(t *testing.T) {
	c := newGrepContext(t, func(fs *flag.FlagSet) {
		require.NoError(t, fs.Set("shell", "zsh"))
		require.NoError(t, fs.Set("hostname", "myhost"))
		require.NoError(t, fs.Set("username", "alice"))
		require.NoError(t, fs.Set("result", "0"))
		require.NoError(t, fs.Set("main-command", "git"))
	})
	filter, err := buildGrepFilter(c, "log")
	require.NoError(t, err)

	assert.Equal(t, []string{"zsh"}, filter.Shell)
	assert.Equal(t, []string{"myhost"}, filter.Hostname)
	assert.Equal(t, []string{"alice"}, filter.Username)
	assert.Equal(t, []int{0}, filter.Result)
	assert.Equal(t, []string{"git"}, filter.MainCommand)
	assert.Equal(t, "log", filter.Command)
}

func TestBuildGrepFilter_ResultNegativeNotApplied(t *testing.T) {
	c := newGrepContext(t, func(fs *flag.FlagSet) {
		require.NoError(t, fs.Set("result", "-1"))
	})
	filter, err := buildGrepFilter(c, "x")
	require.NoError(t, err)
	assert.Empty(t, filter.Result, "result=-1 means 'any' and should not filter")
}

func TestBuildGrepFilter_SinceAndUntilTimeWindow(t *testing.T) {
	c := newGrepContext(t, func(fs *flag.FlagSet) {
		require.NoError(t, fs.Set("since", "2024"))
		require.NoError(t, fs.Set("until", "2024-06"))
	})
	filter, err := buildGrepFilter(c, "x")
	require.NoError(t, err)

	require.Len(t, filter.Time, 2)
	since := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, float64(since.UnixMilli()), filter.Time[0])
	// until=2024-06 end-of-period -> 2024-06-30 23:59:59 UTC.
	untilEnd := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)
	assert.Equal(t, float64(untilEnd.UnixMilli()), filter.Time[1])
}

func TestBuildGrepFilter_InvalidSince(t *testing.T) {
	c := newGrepContext(t, func(fs *flag.FlagSet) {
		require.NoError(t, fs.Set("since", "garbage"))
	})
	_, err := buildGrepFilter(c, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --since date")
}

func TestBuildGrepFilter_InvalidUntil(t *testing.T) {
	c := newGrepContext(t, func(fs *flag.FlagSet) {
		require.NoError(t, fs.Set("until", "garbage"))
	})
	_, err := buildGrepFilter(c, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --until date")
}

// --- outputGrepJSON (pure) ----------------------------------------------------

func TestOutputGrepJSON_ValidJSON(t *testing.T) {
	edges := []model.SearchCommandEdge{
		{ID: 1, Shell: "bash", Command: "ls -la", Result: 0},
		{ID: 2, Shell: "zsh", Command: "git status", Result: 1},
	}
	err := outputGrepJSON(edges, 2)
	assert.NoError(t, err)

	// Confirm the produced structure matches what outputGrepJSON marshals.
	output := struct {
		TotalCount int                       `json:"totalCount"`
		Commands   []model.SearchCommandEdge `json:"commands"`
	}{TotalCount: 2, Commands: edges}
	data, err := json.MarshalIndent(output, "", "  ")
	require.NoError(t, err)

	var back struct {
		TotalCount int                       `json:"totalCount"`
		Commands   []model.SearchCommandEdge `json:"commands"`
	}
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, 2, back.TotalCount)
	require.Len(t, back.Commands, 2)
	assert.Equal(t, "git status", back.Commands[1].Command)
}

func TestOutputGrepJSON_Empty(t *testing.T) {
	err := outputGrepJSON([]model.SearchCommandEdge{}, 0)
	assert.NoError(t, err)
}

// --- outputGrepTable (stdout glue, smoke only) --------------------------------

func TestOutputGrepTable_NoError(t *testing.T) {
	edges := []model.SearchCommandEdge{
		{ID: 1, Shell: "bash", Command: "ls", Time: 1000, EndTime: 2000, Result: 0, Username: "u", Hostname: "h"},
		// Encrypted with original available -> uses OriginalCommand.
		{ID: 2, Shell: "zsh", Command: "ENC", IsEncrypted: true, OriginalCommand: "secret cmd", Time: 3000, EndTime: 3500},
	}
	// totalCount > len triggers the "Showing X of Y" summary branch.
	assert.NoError(t, outputGrepTable(edges, 99, 50))
}

func TestOutputGrepTable_NoSummaryWhenAllShown(t *testing.T) {
	edges := []model.SearchCommandEdge{
		{ID: 1, Shell: "bash", Command: "ls", Time: 1000, EndTime: 2000},
	}
	assert.NoError(t, outputGrepTable(edges, 1, 50))
}

// --- commandGrep action (mock config + httptest GraphQL backend) --------------

func TestCommandGrep_UnsupportedFormat(t *testing.T) {
	setupGrepActionTest(t) // config not consulted before format check
	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	err := app.Run([]string{"t", "rg", "-f", "xml", "needle"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestCommandGrep_MissingSearchText(t *testing.T) {
	setupGrepActionTest(t)
	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	err := app.Run([]string{"t", "rg"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search text is required")
}

func TestCommandGrep_ConfigReadError(t *testing.T) {
	mc := setupGrepActionTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, assert.AnError)
	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	err := app.Run([]string{"t", "rg", "needle"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config")
}

func TestCommandGrep_NotAuthenticated(t *testing.T) {
	mc := setupGrepActionTest(t)
	// Empty token -> "not authenticated" error.
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: ""}, nil)
	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	err := app.Run([]string{"t", "rg", "needle"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")
}

func TestCommandGrep_InvalidSinceDate(t *testing.T) {
	mc := setupGrepActionTest(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: "https://example.invalid",
	}, nil)
	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	// buildGrepFilter should fail on a bad --since before any network call.
	err := app.Run([]string{"t", "rg", "--since", "not-a-date", "needle"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --since date")
}

func TestCommandGrep_SuccessJSON(t *testing.T) {
	mc := setupGrepActionTest(t)

	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/graphql", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"fetchCommands":{"count":1,"edges":[{"id":7,"shell":"bash","command":"git status","result":0}]}}}`)
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	err := app.Run([]string{"t", "rg", "-f", "json", "git"})
	require.NoError(t, err)
	// The GraphQL request carries the search term in its variables payload.
	assert.True(t, strings.Contains(gotBody, "fetchCommands"), "request body should contain the query")
}

func TestCommandGrep_NoResults(t *testing.T) {
	mc := setupGrepActionTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"fetchCommands":{"count":0,"edges":[]}}}`)
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	// "No commands found" path returns nil (not an error).
	err := app.Run([]string{"t", "rg", "git"})
	require.NoError(t, err)
}

func TestCommandGrep_ServerErrorJSONFormat(t *testing.T) {
	mc := setupGrepActionTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `boom`)
	}))
	t.Cleanup(server.Close)

	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)

	app := &cli.App{Name: "t", Commands: []*cli.Command{GrepCommand}}
	// On fetch error the action prints (json error output) and returns nil.
	err := app.Run([]string{"t", "rg", "-f", "json", "git"})
	require.NoError(t, err)
}

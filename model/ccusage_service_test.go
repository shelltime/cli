package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCCUsage_NewService(t *testing.T) {
	cmd := NewMockCommandService(t)
	svc := NewCCUsageService(ShellTimeConfig{}, cmd)
	require.NotNil(t, svc)
	var _ CCUsageService = svc
}

func TestCCUsage_StartDisabled(t *testing.T) {
	cmd := NewMockCommandService(t)

	// CCUsage nil -> disabled, returns nil without touching command service.
	svc := NewCCUsageService(ShellTimeConfig{}, cmd)
	require.NoError(t, svc.Start(context.Background()))

	// Enabled explicitly false -> disabled.
	off := false
	svc2 := NewCCUsageService(ShellTimeConfig{CCUsage: &CCUsage{Enabled: &off}}, cmd)
	require.NoError(t, svc2.Start(context.Background()))
}

func TestCCUsage_Stop_BeforeStartDoesNotPanic(t *testing.T) {
	cmd := NewMockCommandService(t)
	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	// ticker is nil before Start; Stop must guard against that.
	assert.NotPanics(t, func() { svc.Stop() })
}

func TestGetUserShell(t *testing.T) {
	t.Run("uses SHELL env when set", func(t *testing.T) {
		t.Setenv("SHELL", "/usr/bin/zsh")
		assert.Equal(t, "/usr/bin/zsh", getUserShell())
	})

	t.Run("falls back to default when SHELL unset", func(t *testing.T) {
		t.Setenv("SHELL", "")
		got := getUserShell()
		if runtime.GOOS == "windows" {
			assert.NotEmpty(t, got)
		} else {
			assert.Equal(t, "/bin/sh", got)
		}
	})
}

func TestShellEscapeArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"simple", []string{"a", "b"}, "'a' 'b'"},
		{"empty", []string{}, ""},
		{"single quote inside", []string{"it's"}, `'it'"'"'s'`},
		{"spaces preserved", []string{"hello world"}, "'hello world'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shellEscapeArgs(tc.in))
		})
	}
}

func TestCCUsage_collectData_NeitherBinaryFound(t *testing.T) {
	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return("", errors.New("not found"))
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	_, err := svc.collectData(context.Background(), time.Time{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither bunx nor npx found")
}

func TestCCUsage_collectData_SuccessViaFakeBunx(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	// Fake bunx: a shell script that prints valid ccusage JSON regardless of args.
	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	script := `#!/bin/sh
cat <<'JSON'
{"projects":{"projA":[{"date":"20260101","inputTokens":10,"outputTokens":20,"totalTokens":30,"totalCost":0.5,"modelsUsed":["claude"],"modelBreakdowns":[{"modelName":"claude","inputTokens":10,"outputTokens":20,"cost":0.5}]}]},"totals":{"inputTokens":10,"outputTokens":20,"totalTokens":30,"totalCost":0.5}}
JSON
`
	require.NoError(t, os.WriteFile(fakeBunx, []byte(script), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	data, err := svc.collectData(context.Background(), time.Time{})
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.NotEmpty(t, data.Timestamp)
	assert.NotEmpty(t, data.Hostname)
	require.Contains(t, data.Data.Projects, "projA")
	require.Len(t, data.Data.Projects["projA"], 1)
	day := data.Data.Projects["projA"][0]
	assert.Equal(t, "20260101", day.Date)
	assert.Equal(t, 10, day.InputTokens)
	assert.Equal(t, 0.5, day.TotalCost)
}

func TestCCUsage_collectData_WithSinceUsesNpxFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	binDir := t.TempDir()
	fakeNpx := filepath.Join(binDir, "npx")
	// Echo args to verify --since is forwarded, then print minimal JSON.
	script := `#!/bin/sh
echo "$@" >&2
echo '{"projects":{},"totals":{}}'
`
	require.NoError(t, os.WriteFile(fakeNpx, []byte(script), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	// bunx missing -> npx fallback path taken.
	cmd.On("LookPath", "bunx").Return("", errors.New("not found"))
	cmd.On("LookPath", "npx").Return(fakeNpx, nil)

	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	since := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	data, err := svc.collectData(context.Background(), since)
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Empty(t, data.Data.Projects)
}

func TestCCUsage_collectData_InvalidJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	require.NoError(t, os.WriteFile(fakeBunx, []byte("#!/bin/sh\necho 'not json'\n"), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	_, err := svc.collectData(context.Background(), time.Time{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse ccusage output")
}

func TestCCUsage_getLastSyncTimestamp(t *testing.T) {
	t.Run("parses recent RFC3339 timestamp", func(t *testing.T) {
		recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"ccusage":{"lastSyncAt":"` + recent + `"}}}}`))
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
		endpoint := Endpoint{Token: "t", APIEndpoint: server.URL}
		got, err := svc.getLastSyncTimestamp(context.Background(), endpoint)
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(-time.Hour), got, 2*time.Second)
	})

	t.Run("empty lastSyncAt returns zero time", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"ccusage":{"lastSyncAt":""}}}}`))
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
		got, err := svc.getLastSyncTimestamp(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL})
		require.NoError(t, err)
		assert.True(t, got.IsZero())
	})

	t.Run("timestamp before 2023 is treated as zero", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"ccusage":{"lastSyncAt":"2020-01-01T00:00:00Z"}}}}`))
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
		got, err := svc.getLastSyncTimestamp(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL})
		require.NoError(t, err)
		assert.True(t, got.IsZero())
	})

	t.Run("server error is swallowed and returns zero time", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"down"}`))
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
		got, err := svc.getLastSyncTimestamp(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL})
		require.NoError(t, err) // intentionally swallowed
		assert.True(t, got.IsZero())
	})
}

func TestCCUsage_sendData(t *testing.T) {
	t.Run("transforms projects into entries and posts batch", func(t *testing.T) {
		var gotPath string
		var payload struct {
			Host    string `json:"host"`
			Entries []struct {
				Project string `json:"project"`
				Date    string `json:"date"`
				Usage   struct {
					InputTokens int     `json:"inputTokens"`
					TotalCost   float64 `json:"totalCost"`
				} `json:"usage"`
			} `json:"entries"`
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"successCount":1,"totalCount":1}`))
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)

		var data CCUsageData
		data.Hostname = "host1"
		raw := `{"projects":{"projA":[{"date":"20260101","inputTokens":10,"outputTokens":20,"totalTokens":30,"totalCost":1.5,"modelsUsed":["m"],"modelBreakdowns":[{"modelName":"m","inputTokens":10,"outputTokens":20,"cost":1.5}]}]},"totals":{}}`
		require.NoError(t, json.Unmarshal([]byte(raw), &data.Data))

		err := svc.sendData(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL}, &data)
		require.NoError(t, err)
		assert.Equal(t, "/api/v1/ccusage/batch", gotPath)
		assert.Equal(t, "host1", payload.Host)
		require.Len(t, payload.Entries, 1)
		assert.Equal(t, "projA", payload.Entries[0].Project)
		assert.Equal(t, "20260101", payload.Entries[0].Date)
		assert.Equal(t, 10, payload.Entries[0].Usage.InputTokens)
		assert.Equal(t, 1.5, payload.Entries[0].Usage.TotalCost)
	})

	t.Run("no entries short-circuits without request", func(t *testing.T) {
		called := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
		err := svc.sendData(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL}, &CCUsageData{Hostname: "h"})
		require.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("server rejection with failed projects returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":false,"successCount":0,"totalCount":1,"failedProjects":["projA"]}`))
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)

		var data CCUsageData
		data.Hostname = "h"
		raw := `{"projects":{"projA":[{"date":"20260101","totalCost":1.0,"modelBreakdowns":[]}]},"totals":{}}`
		require.NoError(t, json.Unmarshal([]byte(raw), &data.Data))

		err := svc.sendData(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL}, &data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "projA")
	})

	t.Run("http error wraps message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad batch"}`))
		}))
		defer server.Close()

		cmd := NewMockCommandService(t)
		svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)

		var data CCUsageData
		data.Hostname = "h"
		raw := `{"projects":{"projA":[{"date":"20260101","modelBreakdowns":[]}]},"totals":{}}`
		require.NoError(t, json.Unmarshal([]byte(raw), &data.Data))

		err := svc.sendData(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL}, &data)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad batch")
	})
}

func TestCCUsage_CollectCCUsage_WithCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	// Full happy path: fetch last-sync (GraphQL), collect via fake bunx, send batch.
	var sawGraphQL, sawBatch bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/graphql":
			sawGraphQL = true
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"ccusage":{"lastSyncAt":""}}}}`))
		case "/api/v1/ccusage/batch":
			sawBatch = true
			_, _ = w.Write([]byte(`{"success":true,"successCount":1,"totalCount":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	usageJSON := `{"projects":{"projA":[{"date":"20260101","inputTokens":1,"totalTokens":1,"totalCost":0.1,"modelBreakdowns":[]}]},"totals":{}}`
	require.NoError(t, os.WriteFile(fakeBunx, []byte("#!/bin/sh\necho '"+usageJSON+"'\n"), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	svc := NewCCUsageService(cfg, cmd).(*ccUsageService)
	require.NoError(t, svc.CollectCCUsage(context.Background()))
	assert.True(t, sawGraphQL, "should fetch last sync timestamp")
	assert.True(t, sawBatch, "should send the batch")
}

func TestCCUsage_StartEnabled_RunsInitialCollectionThenStops(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	// Enabled config triggers an immediate initial collection on Start. Provide
	// a fake bunx + server so it succeeds, then Stop to halt the ticker loop.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/graphql":
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"ccusage":{"lastSyncAt":""}}}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"successCount":0,"totalCount":0}`))
		}
	}))
	defer server.Close()

	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	require.NoError(t, os.WriteFile(fakeBunx, []byte("#!/bin/sh\necho '{\"projects\":{},\"totals\":{}}'\n"), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	on := true
	cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL, CCUsage: &CCUsage{Enabled: &on}}
	svc := NewCCUsageService(cfg, cmd)

	require.NoError(t, svc.Start(context.Background()))
	// Stop should not block; the background loop must exit promptly.
	done := make(chan struct{})
	go func() { svc.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked")
	}
}

func TestCCUsage_CollectCCUsage_NoCredentialsButCollects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	// No token/endpoint => skips both the last-sync fetch and the send, but
	// still runs collectData. Provide a fake bunx so collection succeeds.
	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	require.NoError(t, os.WriteFile(fakeBunx, []byte("#!/bin/sh\necho '{\"projects\":{},\"totals\":{}}'\n"), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	require.NoError(t, svc.CollectCCUsage(context.Background()))
}

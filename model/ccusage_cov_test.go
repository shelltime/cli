package model

import (
	"context"
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

// TestCCUsage_collectData_ExitErrorWithStderr covers the *exec.ExitError branch:
// a fake bunx that prints to stderr and exits non-zero, surfacing the stderr in
// the error message.
func TestCCUsage_collectData_ExitErrorWithStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	require.NoError(t, os.WriteFile(fakeBunx, []byte("#!/bin/sh\necho 'boom on stderr' >&2\nexit 3\n"), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	_, err := svc.collectData(context.Background(), time.Time{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ccusage command failed")
	assert.Contains(t, err.Error(), "boom on stderr")
}

// TestCCUsage_collectData_UsernameFromUserCurrent covers the branch where USER is
// empty so the username is resolved via user.Current().
func TestCCUsage_collectData_UsernameFromUserCurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	require.NoError(t, os.WriteFile(fakeBunx, []byte("#!/bin/sh\necho '{\"projects\":{},\"totals\":{}}'\n"), 0o755))
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("USER", "") // force the user.Current() fallback branch

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	data, err := svc.collectData(context.Background(), time.Time{})
	require.NoError(t, err)
	assert.NotEmpty(t, data.Username, "username resolved via user.Current() when USER unset")
}

// TestCCUsage_getLastSyncTimestamp_ParseError covers the branch where the server
// returns a non-empty but unparseable timestamp -> returns an error.
func TestCCUsage_getLastSyncTimestamp_ParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"ccusage":{"lastSyncAt":"not-a-timestamp"}}}}`))
	}))
	defer server.Close()

	cmd := NewMockCommandService(t)
	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	_, err := svc.getLastSyncTimestamp(context.Background(), Endpoint{Token: "t", APIEndpoint: server.URL})
	require.Error(t, err, "unparseable timestamp surfaces a parse error")
}

// TestCCUsage_CollectCCUsage_CollectErrorWrapped covers CollectCCUsage's branch
// where collectData fails (no bunx/npx) and the error is wrapped.
func TestCCUsage_CollectCCUsage_CollectErrorWrapped(t *testing.T) {
	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return("", errors.New("nope"))
	cmd.On("LookPath", "npx").Return("", errors.New("nope"))

	// No credentials -> skips last-sync fetch and send; only collectData runs.
	svc := NewCCUsageService(ShellTimeConfig{}, cmd).(*ccUsageService)
	err := svc.CollectCCUsage(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to collect ccusage data")
}

// TestCCUsage_CollectCCUsage_SendErrorWrapped covers the branch where collection
// succeeds but the server rejects the batch send, wrapping the send error.
func TestCCUsage_CollectCCUsage_SendErrorWrapped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh script")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/graphql":
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"ccusage":{"lastSyncAt":""}}}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"batch rejected"}`))
		}
	}))
	defer server.Close()

	binDir := t.TempDir()
	fakeBunx := filepath.Join(binDir, "bunx")
	usageJSON := `{"projects":{"projA":[{"date":"20260101","totalCost":0.1,"modelBreakdowns":[]}]},"totals":{}}`
	require.NoError(t, os.WriteFile(fakeBunx, []byte("#!/bin/sh\necho '"+usageJSON+"'\n"), 0o755))
	t.Setenv("SHELL", "/bin/sh")

	cmd := NewMockCommandService(t)
	cmd.On("LookPath", "bunx").Return(fakeBunx, nil)
	cmd.On("LookPath", "npx").Return("", errors.New("not found"))

	cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	svc := NewCCUsageService(cfg, cmd).(*ccUsageService)
	err := svc.CollectCCUsage(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send usage data")
}

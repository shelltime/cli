package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncCodexUsage_SendsUsageToServer(t *testing.T) {
	t.Helper()

	originalLoad := loadCodexAuthFunc
	originalFetch := fetchCodexUsageFunc
	defer func() {
		loadCodexAuthFunc = originalLoad
		fetchCodexUsageFunc = originalFetch
	}()

	loadCodexAuthFunc = func() (*codexAuthData, error) {
		return &codexAuthData{AccessToken: "test-token", AccountID: "acct-1"}, nil
	}
	fetchCodexUsageFunc = func(ctx context.Context, auth *codexAuthData) (*CodexRateLimitData, error) {
		return &CodexRateLimitData{
			Plan: "pro",
			Windows: []CodexRateLimitWindow{
				{
					LimitID:               "main",
					UsagePercentage:       72.5,
					ResetAt:               1712400000,
					WindowDurationMinutes: 300,
				},
			},
		}, nil
	}

	var captured struct {
		Plan    string `json:"plan"`
		Windows []struct {
			LimitID               string  `json:"limit_id"`
			UsagePercentage       float64 `json:"usage_percentage"`
			ResetsAt              string  `json:"resets_at"`
			WindowDurationMinutes int     `json:"window_duration_minutes"`
		} `json:"windows"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/codex-usage", r.URL.Path)
		assert.Equal(t, "CLI shelltime-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := model.ShellTimeConfig{
		Token:       "shelltime-token",
		APIEndpoint: server.URL,
	}

	err := syncCodexUsage(context.Background(), cfg)
	require.NoError(t, err)

	require.Len(t, captured.Windows, 1)
	assert.Equal(t, "pro", captured.Plan)
	assert.Equal(t, "main", captured.Windows[0].LimitID)
	assert.Equal(t, 72.5, captured.Windows[0].UsagePercentage)
	assert.Equal(t, "2024-04-06T10:40:00Z", captured.Windows[0].ResetsAt)
	assert.Equal(t, 300, captured.Windows[0].WindowDurationMinutes)
}

func TestSyncCodexUsage_AuthError(t *testing.T) {
	t.Helper()

	originalLoad := loadCodexAuthFunc
	originalFetch := fetchCodexUsageFunc
	defer func() {
		loadCodexAuthFunc = originalLoad
		fetchCodexUsageFunc = originalFetch
	}()

	loadCodexAuthFunc = func() (*codexAuthData, error) {
		return nil, assert.AnError
	}
	fetchCodexUsageFunc = func(ctx context.Context, auth *codexAuthData) (*CodexRateLimitData, error) {
		t.Fatal("fetchCodexUsageFunc should not be called when auth loading fails")
		return nil, nil
	}

	cfg := model.ShellTimeConfig{Token: "shelltime-token"}
	err := syncCodexUsage(context.Background(), cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestCodexUsageSyncService_StartRunsImmediatelyAndOnTicker(t *testing.T) {
	t.Helper()

	originalInterval := CodexUsageSyncInterval
	originalLoad := loadCodexAuthFunc
	originalFetch := fetchCodexUsageFunc
	defer func() {
		CodexUsageSyncInterval = originalInterval
		loadCodexAuthFunc = originalLoad
		fetchCodexUsageFunc = originalFetch
	}()

	CodexUsageSyncInterval = 20 * time.Millisecond

	loadCodexAuthFunc = func() (*codexAuthData, error) {
		return &codexAuthData{AccessToken: "test-token"}, nil
	}
	fetchCodexUsageFunc = func(ctx context.Context, auth *codexAuthData) (*CodexRateLimitData, error) {
		return &CodexRateLimitData{
			Plan: "pro",
			Windows: []CodexRateLimitWindow{
				{LimitID: "main", UsagePercentage: 12, ResetAt: time.Now().Unix(), WindowDurationMinutes: 300},
			},
		}, nil
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewCodexUsageSyncService(model.ShellTimeConfig{
		Token:       "shelltime-token",
		APIEndpoint: server.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, service.Start(ctx))

	require.Eventually(t, func() bool {
		return calls.Load() >= 2
	}, 250*time.Millisecond, 10*time.Millisecond)

	service.Stop()
	stoppedAt := calls.Load()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, stoppedAt, calls.Load())
}

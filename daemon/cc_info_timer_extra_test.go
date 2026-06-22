package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchUserProfile_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/graphql", r.URL.Path)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"fetchUser": map[string]interface{}{
					"id":    42,
					"login": "alice",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := &model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	service := NewCCInfoTimerService(config)

	service.fetchUserProfile(context.Background())
	assert.Equal(t, "alice", service.GetCachedUserLogin())

	// Marked fetched -> a second call is a no-op (does not re-query).
	service.mu.RLock()
	assert.True(t, service.userLoginFetched)
	service.mu.RUnlock()
	service.fetchUserProfile(context.Background())
	assert.Equal(t, "alice", service.GetCachedUserLogin())
}

func TestFetchUserProfile_NoToken(t *testing.T) {
	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: ""})
	service.fetchUserProfile(context.Background())
	assert.Empty(t, service.GetCachedUserLogin())
}

func TestFetchUserProfile_APIErrorLeavesEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})
	service.fetchUserProfile(context.Background())
	assert.Empty(t, service.GetCachedUserLogin())

	service.mu.RLock()
	defer service.mu.RUnlock()
	assert.False(t, service.userLoginFetched, "failed fetch should not mark as fetched")
}

func TestSendAnthropicUsageToServer_PostsPayload(t *testing.T) {
	var hits atomic.Int32
	var captured struct {
		FiveHour struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"seven_day"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/anthropic-usage", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		_ = json.NewDecoder(r.Body).Decode(&captured)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})
	usage := &AnthropicRateLimitData{
		FiveHourUtilization: 0.5,
		FiveHourResetsAt:    "2025-01-01T00:00:00Z",
		SevenDayUtilization: 0.25,
		SevenDayResetsAt:    "2025-01-07T00:00:00Z",
	}
	service.sendAnthropicUsageToServer(context.Background(), usage)

	assert.Equal(t, int32(1), hits.Load())
	assert.Equal(t, 0.5, captured.FiveHour.Utilization)
	assert.Equal(t, "2025-01-01T00:00:00Z", captured.FiveHour.ResetsAt)
	assert.Equal(t, 0.25, captured.SevenDay.Utilization)
	assert.Equal(t, "2025-01-07T00:00:00Z", captured.SevenDay.ResetsAt)
}

func TestSendAnthropicUsageToServer_NoToken(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "", APIEndpoint: server.URL})
	service.sendAnthropicUsageToServer(context.Background(), &AnthropicRateLimitData{})
	assert.False(t, hit, "no token -> no request")
}

func TestSendAnthropicUsageToServer_ServerErrorIsSwallowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})
	// Must not panic; error is logged and swallowed.
	assert.NotPanics(t, func() {
		service.sendAnthropicUsageToServer(context.Background(), &AnthropicRateLimitData{FiveHourUtilization: 1})
	})
}

func TestFetchRateLimit_FreshCacheSkips(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("fetchRateLimit only runs on darwin/linux")
	}
	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok"})

	// Pre-populate a fresh cache so the TTL guard short-circuits before any
	// token lookup or network call.
	service.rateLimitCache.mu.Lock()
	service.rateLimitCache.usage = &AnthropicRateLimitData{FiveHourUtilization: 0.9}
	service.rateLimitCache.fetchedAt = time.Now()
	service.rateLimitCache.lastAttemptAt = time.Now()
	service.rateLimitCache.mu.Unlock()

	service.fetchRateLimit(context.Background())

	// Cache is unchanged and no error was recorded.
	assert.Equal(t, "", service.GetCachedRateLimitError())
	rl := service.GetCachedRateLimit()
	require.NotNil(t, rl)
	assert.Equal(t, 0.9, rl.FiveHourUtilization)
}

func TestFetchRateLimit_OAuthMissingSetsError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("token-from-file path is exercised on linux")
	}
	// On linux, fetchClaudeCodeOAuthToken reads ~/.claude/.credentials.json.
	// With an empty HOME that file is missing -> token lookup fails -> lastError="oauth".
	home := t.TempDir()
	t.Setenv("HOME", home)

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok"})
	service.fetchRateLimit(context.Background())

	assert.Equal(t, "oauth", service.GetCachedRateLimitError())
	assert.Nil(t, service.GetCachedRateLimit())
}

func TestFetchRateLimit_MissingScopeSkipsFetch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("token-from-file path is exercised on linux")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o700))
	// setup-token style creds: a valid token that lacks the user:profile scope.
	content := `{"claudeAiOauth":{"accessToken":"sk-setup","scopes":["user:inference"]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(content), 0o600))

	// Point the usage URL at a server that flags if it is ever called.
	var called int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	withTestUsageURL(t, server.URL)

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok"})
	service.fetchRateLimit(context.Background())

	assert.Equal(t, "api:scope", service.GetCachedRateLimitError())
	assert.Nil(t, service.GetCachedRateLimit())
	assert.Equal(t, int32(0), atomic.LoadInt32(&called), "usage endpoint must not be called when scope is missing")
}

func TestFetchRateLimit_Forbidden403SetsScopeError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("token-from-file path is exercised on linux")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o700))
	// Token claims the required scope, so the proactive check passes and we hit the API,
	// which still returns 403 (e.g. org access restriction).
	content := `{"claudeAiOauth":{"accessToken":"sk-login","scopes":["user:inference","user:profile"]}}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(content), 0o600))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	withTestUsageURL(t, server.URL)

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok"})
	service.fetchRateLimit(context.Background())

	assert.Equal(t, "api:scope", service.GetCachedRateLimitError())
	assert.Nil(t, service.GetCachedRateLimit())
	// A backoff window must be set so the daemon stops hammering the forbidden endpoint.
	service.rateLimitCache.mu.RLock()
	backoff := service.rateLimitCache.backoffUntil
	service.rateLimitCache.mu.RUnlock()
	assert.True(t, backoff.After(time.Now()), "403 should set a backoff window")
}

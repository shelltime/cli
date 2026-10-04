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
	withRunningProcesses(t, "claude")
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
	withRunningProcesses(t, "claude")
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
	withRunningProcesses(t, "claude")
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

// withStubOAuthToken replaces the Keychain / credentials-file lookup.
func withStubOAuthToken(t *testing.T, token string, scopes []string) {
	t.Helper()
	orig := fetchClaudeCodeOAuthTokenFunc
	fetchClaudeCodeOAuthTokenFunc = func() (string, []string, error) {
		return token, scopes, nil
	}
	t.Cleanup(func() { fetchClaudeCodeOAuthTokenFunc = orig })
}

// newUsageSyncServers starts a fake Anthropic usage endpoint and a fake ShellTime API that records
// the pushed usage payloads. Assertions use the per-test API server rather than the usage endpoint,
// because timers leaked by other tests may also reach the (global) usage URL.
func newUsageSyncServers(t *testing.T) (pushed chan map[string]any, apiURL string) {
	t.Helper()
	usage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":42,"resets_at":"2026-10-04T18:00:00Z"},"seven_day":{"utilization":70,"resets_at":"2026-10-08T00:00:00Z"}}`))
	}))
	t.Cleanup(usage.Close)
	withTestUsageURL(t, usage.URL)

	pushed = make(chan map[string]any, 8)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/anthropic-usage" && r.Method == http.MethodPost {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			pushed <- body
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(api.Close)

	return pushed, api.URL
}

func TestStartUsageSync_SyncsWithoutStatuslineActivity(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("fetchRateLimit only runs on darwin/linux")
	}
	withStubOAuthToken(t, "sk-login", []string{"user:profile"})
	withRunningProcesses(t, "claude")
	pushed, apiURL := newUsageSyncServers(t)

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok", APIEndpoint: apiURL})
	service.usageSyncInterval = 10 * time.Millisecond
	service.StartUsageSync()
	defer service.Stop()

	// No NotifyActivity: the statusline timer never starts, yet usage is fetched and pushed.
	select {
	case body := <-pushed:
		fiveHour, ok := body["five_hour"].(map[string]any)
		require.True(t, ok, "payload should carry five_hour")
		assert.Equal(t, float64(42), fiveHour["utilization"])
	case <-time.After(2 * time.Second):
		t.Fatal("usage was not pushed to the server")
	}

	rl := service.GetCachedRateLimit()
	require.NotNil(t, rl)
	assert.Equal(t, float64(70), rl.SevenDayUtilization)

	service.timerMu.Lock()
	assert.False(t, service.timerRunning, "statusline timer should not be started by the usage sync")
	service.timerMu.Unlock()
}

func TestStartUsageSync_HonorsCacheTTL(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("fetchRateLimit only runs on darwin/linux")
	}
	withStubOAuthToken(t, "sk-login", []string{"user:profile"})
	withRunningProcesses(t, "claude")
	pushed, apiURL := newUsageSyncServers(t)

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok", APIEndpoint: apiURL})
	service.usageSyncInterval = 10 * time.Millisecond
	service.StartUsageSync()
	defer service.Stop()

	select {
	case <-pushed:
	case <-time.After(2 * time.Second):
		t.Fatal("usage was not pushed to the server")
	}

	// Many ticks pass, but the TTL keeps the service from fetching and pushing again.
	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, pushed, "usage should be pushed only once within the TTL")
}

func TestStartUsageSync_SkipsWhenClaudeNotRunning(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("fetchRateLimit only runs on darwin/linux")
	}
	withStubOAuthToken(t, "sk-login", []string{"user:profile"})
	var claudeRunning atomic.Bool
	orig := listProcessNamesFunc
	listProcessNamesFunc = func() ([]string, error) {
		if claudeRunning.Load() {
			return []string{"/usr/sbin/sshd", "claude"}, nil
		}
		return []string{"/usr/sbin/sshd", "/bin/zsh"}, nil
	}
	t.Cleanup(func() { listProcessNamesFunc = orig })
	pushed, apiURL := newUsageSyncServers(t)

	service := NewCCInfoTimerService(&model.ShellTimeConfig{Token: "tok", APIEndpoint: apiURL})
	service.usageSyncInterval = 10 * time.Millisecond
	service.StartUsageSync()
	defer service.Stop()

	time.Sleep(100 * time.Millisecond)
	assert.Empty(t, pushed, "usage should not be fetched while Claude is not running")
	service.rateLimitCache.mu.RLock()
	lastAttempt := service.rateLimitCache.lastAttemptAt
	service.rateLimitCache.mu.RUnlock()
	assert.True(t, lastAttempt.IsZero(), "a skipped fetch must not start the TTL")

	// Once Claude starts, the next tick fetches without waiting out a TTL.
	claudeRunning.Store(true)
	select {
	case <-pushed:
	case <-time.After(2 * time.Second):
		t.Fatal("usage was not pushed after Claude started")
	}
}

func TestStartUsageSync_NoTokenDoesNothing(t *testing.T) {
	service := NewCCInfoTimerService(&model.ShellTimeConfig{})
	service.usageSyncInterval = 10 * time.Millisecond
	service.StartUsageSync()
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		service.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return")
	}

	service.rateLimitCache.mu.RLock()
	lastAttempt := service.rateLimitCache.lastAttemptAt
	service.rateLimitCache.mu.RUnlock()
	assert.True(t, lastAttempt.IsZero(), "no usage fetch should be attempted without a ShellTime token")
}

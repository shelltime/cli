package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
)

// withTestUsageURL points fetchAnthropicUsage at a test server for the duration of the test.
func withTestUsageURL(t *testing.T, url string) {
	t.Helper()
	orig := anthropicUsageURL
	anthropicUsageURL = url
	t.Cleanup(func() { anthropicUsageURL = orig })
}

func TestFetchAnthropicUsage_SetsClaudeCodeUserAgent(t *testing.T) {
	var gotUA, gotCT string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotCT = r.Header.Get("Content-Type")
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Equal(t, "oauth-2025-04-20", r.Header.Get("anthropic-beta"))
		json.NewEncoder(w).Encode(anthropicUsageResponse{})
	}))
	defer server.Close()
	withTestUsageURL(t, server.URL)

	_, err := fetchAnthropicUsage(context.Background(), "tok", "9.9.9")
	assert.NoError(t, err)
	assert.Equal(t, "claude-code/9.9.9", gotUA)
	assert.Equal(t, "application/json", gotCT)
}

func TestFetchAnthropicUsage_UserAgentFallback(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		json.NewEncoder(w).Encode(anthropicUsageResponse{})
	}))
	defer server.Close()
	withTestUsageURL(t, server.URL)

	_, err := fetchAnthropicUsage(context.Background(), "tok", "")
	assert.NoError(t, err)
	assert.Equal(t, "claude-code/"+claudeCodeFallbackVersion, gotUA)
}

func TestFetchAnthropicUsage_429ReturnsTypedErrorWithRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	withTestUsageURL(t, server.URL)

	_, err := fetchAnthropicUsage(context.Background(), "tok", "1.0.0")
	assert.Error(t, err)

	var apiErr *anthropicAPIError
	assert.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, 120*time.Second, apiErr.RetryAfter)

	// The typed error must still shorten to the historical "api:429" for the statusline.
	assert.Equal(t, "api:429", shortenAPIError(err))
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"120", 120 * time.Second},
		{" 60 ", 60 * time.Second},
		{"0", 0},
		{"-5", 0},
		{"abc", 0},
		{"Wed, 21 Oct 2026 07:28:00 GMT", 0}, // HTTP-date form is not parsed
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, parseRetryAfter(c.in), "parseRetryAfter(%q)", c.in)
	}
}

func TestSetGetClaudeCodeVersion(t *testing.T) {
	service := NewCCInfoTimerService(&model.ShellTimeConfig{})
	assert.Empty(t, service.GetClaudeCodeVersion())

	service.SetClaudeCodeVersion("3.1.4")
	assert.Equal(t, "3.1.4", service.GetClaudeCodeVersion())

	// Empty values are ignored so a known version is not clobbered.
	service.SetClaudeCodeVersion("")
	assert.Equal(t, "3.1.4", service.GetClaudeCodeVersion())
}

func TestStopTimer_PreservesRateLimitCache(t *testing.T) {
	service := NewCCInfoTimerService(&model.ShellTimeConfig{})

	// Seed a good rate-limit cache.
	service.rateLimitCache.mu.Lock()
	service.rateLimitCache.usage = &AnthropicRateLimitData{FiveHourUtilization: 0.5}
	service.rateLimitCache.fetchedAt = time.Now()
	service.rateLimitCache.lastAttemptAt = time.Now()
	service.rateLimitCache.backoffUntil = time.Now().Add(time.Hour)
	service.rateLimitCache.mu.Unlock()

	// stopTimer must be called with timerMu held and the timer "running".
	service.timerMu.Lock()
	service.timerRunning = true
	service.ticker = time.NewTicker(time.Hour)
	service.stopTimer()
	service.timerMu.Unlock()

	// The cache must survive an idle stop so the TTL/backoff hold across idle cycles.
	rl := service.GetCachedRateLimit()
	assert.NotNil(t, rl)
	assert.Equal(t, 0.5, rl.FiveHourUtilization)

	service.rateLimitCache.mu.RLock()
	assert.False(t, service.rateLimitCache.backoffUntil.IsZero())
	service.rateLimitCache.mu.RUnlock()
}

func TestFetchAnthropicUsage_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "oauth-2025-04-20", r.Header.Get("anthropic-beta"))

		resp := anthropicUsageResponse{
			FiveHour: anthropicUsageBucket{
				Utilization: 0.45,
				ResetsAt:    "2025-01-15T12:00:00Z",
			},
			SevenDay: anthropicUsageBucket{
				Utilization: 0.23,
				ResetsAt:    "2025-01-20T00:00:00Z",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// We need to test with the real function but override the URL.
	// Since fetchAnthropicUsage uses a hardcoded URL, we test the parsing logic
	// by calling the test server directly.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	assert.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	var usage anthropicUsageResponse
	err = json.NewDecoder(resp.Body).Decode(&usage)
	assert.NoError(t, err)

	assert.Equal(t, 0.45, usage.FiveHour.Utilization)
	assert.Equal(t, "2025-01-15T12:00:00Z", usage.FiveHour.ResetsAt)
	assert.Equal(t, 0.23, usage.SevenDay.Utilization)
	assert.Equal(t, "2025-01-20T00:00:00Z", usage.SevenDay.ResetsAt)
}

func TestFetchAnthropicUsage_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// Test that non-200 status is handled - we can't directly call fetchAnthropicUsage
	// with a custom URL, so we verify the error handling pattern
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	assert.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestParseKeychainJSON(t *testing.T) {
	raw := `{"claudeAiOauth":{"accessToken":"sk-ant-test-token-123"}}`

	var creds claudeCodeCredentials
	err := json.Unmarshal([]byte(raw), &creds)
	assert.NoError(t, err)
	assert.NotNil(t, creds.ClaudeAiOauth)
	assert.Equal(t, "sk-ant-test-token-123", creds.ClaudeAiOauth.AccessToken)
}

func TestParseKeychainJSON_MissingOAuth(t *testing.T) {
	raw := `{"someOtherKey":"value"}`

	var creds claudeCodeCredentials
	err := json.Unmarshal([]byte(raw), &creds)
	assert.NoError(t, err)
	assert.Nil(t, creds.ClaudeAiOauth)
}

func TestParseKeychainJSON_EmptyAccessToken(t *testing.T) {
	raw := `{"claudeAiOauth":{"accessToken":""}}`

	var creds claudeCodeCredentials
	err := json.Unmarshal([]byte(raw), &creds)
	assert.NoError(t, err)
	assert.NotNil(t, creds.ClaudeAiOauth)
	assert.Empty(t, creds.ClaudeAiOauth.AccessToken)
}

func TestShortenAPIError_HTTPStatus(t *testing.T) {
	err := fmt.Errorf("anthropic usage API returned status %d", 403)
	assert.Equal(t, "api:403", shortenAPIError(err))
}

func TestShortenAPIError_DecodeError(t *testing.T) {
	err := fmt.Errorf("failed to decode usage response: unexpected EOF")
	assert.Equal(t, "api:decode", shortenAPIError(err))
}

func TestShortenAPIError_NetworkError(t *testing.T) {
	err := fmt.Errorf("dial tcp: connection refused")
	assert.Equal(t, "network", shortenAPIError(err))
}

func TestGetCachedRateLimitError_Empty(t *testing.T) {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)
	assert.Empty(t, service.GetCachedRateLimitError())
}

func TestGetCachedRateLimitError_WithError(t *testing.T) {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	service.rateLimitCache.mu.Lock()
	service.rateLimitCache.lastError = "oauth"
	service.rateLimitCache.mu.Unlock()

	assert.Equal(t, "oauth", service.GetCachedRateLimitError())
}

func TestAnthropicRateLimitCache_GetCachedRateLimit_Nil(t *testing.T) {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)
	result := service.GetCachedRateLimit()
	assert.Nil(t, result)
}

func TestAnthropicRateLimitCache_GetCachedRateLimit_ReturnsCopy(t *testing.T) {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	service.rateLimitCache.mu.Lock()
	service.rateLimitCache.usage = &AnthropicRateLimitData{
		FiveHourUtilization: 0.5,
		SevenDayUtilization: 0.3,
	}
	service.rateLimitCache.mu.Unlock()

	result := service.GetCachedRateLimit()
	assert.NotNil(t, result)
	assert.Equal(t, 0.5, result.FiveHourUtilization)
	assert.Equal(t, 0.3, result.SevenDayUtilization)

	// Modify returned copy - original should be unchanged
	result.FiveHourUtilization = 0.99

	service.rateLimitCache.mu.RLock()
	assert.Equal(t, 0.5, service.rateLimitCache.usage.FiveHourUtilization)
	service.rateLimitCache.mu.RUnlock()
}

func TestParseOAuthTokenFromJSON_Valid(t *testing.T) {
	raw := `{"claudeAiOauth":{"accessToken":"sk-ant-test-token-123","refreshToken":"sk-ref","expiresAt":1773399176544,"scopes":["user:inference"],"subscriptionType":"max","rateLimitTier":"default_claude_max_5x"}}`
	token, err := parseOAuthTokenFromJSON([]byte(raw))
	assert.NoError(t, err)
	assert.Equal(t, "sk-ant-test-token-123", token)
}

func TestParseOAuthTokenFromJSON_MissingOAuth(t *testing.T) {
	raw := `{"someOtherKey":"value"}`
	token, err := parseOAuthTokenFromJSON([]byte(raw))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no OAuth access token found")
	assert.Empty(t, token)
}

func TestParseOAuthTokenFromJSON_EmptyToken(t *testing.T) {
	raw := `{"claudeAiOauth":{"accessToken":""}}`
	token, err := parseOAuthTokenFromJSON([]byte(raw))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no OAuth access token found")
	assert.Empty(t, token)
}

func TestParseOAuthTokenFromJSON_InvalidJSON(t *testing.T) {
	raw := `not-json`
	token, err := parseOAuthTokenFromJSON([]byte(raw))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse credentials JSON")
	assert.Empty(t, token)
}

func TestFetchOAuthTokenFromCredentialsFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	claudeDir := filepath.Join(tmpDir, ".claude")
	err := os.MkdirAll(claudeDir, 0700)
	assert.NoError(t, err)

	content := `{"claudeAiOauth":{"accessToken":"sk-test-linux-token","refreshToken":"sk-ref","expiresAt":1773399176544}}`
	err = os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte(content), 0600)
	assert.NoError(t, err)

	token, err := fetchOAuthTokenFromCredentialsFile()
	assert.NoError(t, err)
	assert.Equal(t, "sk-test-linux-token", token)
}

func TestFetchOAuthTokenFromCredentialsFile_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	token, err := fetchOAuthTokenFromCredentialsFile()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "credentials file read failed")
	assert.Empty(t, token)
}

func TestFetchOAuthTokenFromCredentialsFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	claudeDir := filepath.Join(tmpDir, ".claude")
	err := os.MkdirAll(claudeDir, 0700)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(claudeDir, ".credentials.json"), []byte("not-json"), 0600)
	assert.NoError(t, err)

	token, err := fetchOAuthTokenFromCredentialsFile()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse credentials JSON")
	assert.Empty(t, token)
}

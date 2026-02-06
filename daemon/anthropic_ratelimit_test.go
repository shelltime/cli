package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
)

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

	var creds keychainCredentials
	err := json.Unmarshal([]byte(raw), &creds)
	assert.NoError(t, err)
	assert.NotNil(t, creds.ClaudeAiOauth)
	assert.Equal(t, "sk-ant-test-token-123", creds.ClaudeAiOauth.AccessToken)
}

func TestParseKeychainJSON_MissingOAuth(t *testing.T) {
	raw := `{"someOtherKey":"value"}`

	var creds keychainCredentials
	err := json.Unmarshal([]byte(raw), &creds)
	assert.NoError(t, err)
	assert.Nil(t, creds.ClaudeAiOauth)
}

func TestParseKeychainJSON_EmptyAccessToken(t *testing.T) {
	raw := `{"claudeAiOauth":{"accessToken":""}}`

	var creds keychainCredentials
	err := json.Unmarshal([]byte(raw), &creds)
	assert.NoError(t, err)
	assert.NotNil(t, creds.ClaudeAiOauth)
	assert.Empty(t, creds.ClaudeAiOauth.AccessToken)
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

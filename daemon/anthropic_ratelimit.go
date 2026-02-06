package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

const anthropicUsageCacheTTL = 10 * time.Minute

// AnthropicRateLimitData holds the parsed rate limit utilization data
type AnthropicRateLimitData struct {
	FiveHourUtilization float64
	FiveHourResetsAt    string
	SevenDayUtilization float64
	SevenDayResetsAt    string
}

type anthropicRateLimitCache struct {
	mu            sync.RWMutex
	usage         *AnthropicRateLimitData
	fetchedAt     time.Time
	lastAttemptAt time.Time
}

// anthropicUsageResponse maps the Anthropic API response
type anthropicUsageResponse struct {
	FiveHour anthropicUsageBucket `json:"five_hour"`
	SevenDay anthropicUsageBucket `json:"seven_day"`
}

type anthropicUsageBucket struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// keychainCredentials maps the JSON stored in macOS Keychain for Claude Code
type keychainCredentials struct {
	ClaudeAiOauth *keychainOAuthEntry `json:"claudeAiOauth"`
}

type keychainOAuthEntry struct {
	AccessToken string `json:"accessToken"`
}

// fetchClaudeCodeOAuthToken reads the OAuth token from macOS Keychain.
// Returns ("", nil) on non-macOS platforms.
func fetchClaudeCodeOAuthToken() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", nil
	}

	out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return "", fmt.Errorf("keychain lookup failed: %w", err)
	}

	var creds keychainCredentials
	if err := json.Unmarshal(out, &creds); err != nil {
		return "", fmt.Errorf("failed to parse keychain JSON: %w", err)
	}

	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("no OAuth access token found in keychain")
	}

	return creds.ClaudeAiOauth.AccessToken, nil
}

// fetchAnthropicUsage calls the Anthropic OAuth usage API and returns rate limit data.
func fetchAnthropicUsage(ctx context.Context, token string) (*AnthropicRateLimitData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic usage API returned status %d", resp.StatusCode)
	}

	var usage anthropicUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("failed to decode usage response: %w", err)
	}

	return &AnthropicRateLimitData{
		FiveHourUtilization: usage.FiveHour.Utilization,
		FiveHourResetsAt:    usage.FiveHour.ResetsAt,
		SevenDayUtilization: usage.SevenDay.Utilization,
		SevenDayResetsAt:    usage.SevenDay.ResetsAt,
	}, nil
}

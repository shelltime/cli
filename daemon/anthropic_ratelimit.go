package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	lastError     string // short error description for statusline display
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

// claudeCodeCredentials maps the JSON stored in macOS Keychain or ~/.claude/.credentials.json
type claudeCodeCredentials struct {
	ClaudeAiOauth *claudeCodeOAuthEntry `json:"claudeAiOauth"`
}

type claudeCodeOAuthEntry struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"`
	Scopes           []string `json:"scopes"`
	SubscriptionType any      `json:"subscriptionType"`
	RateLimitTier    any      `json:"rateLimitTier"`
}

// fetchClaudeCodeOAuthToken reads the OAuth token from the platform-specific credential store.
// macOS: reads from Keychain via `security` command.
// Linux: reads from ~/.claude/.credentials.json file.
// Returns ("", nil) on unsupported platforms.
func fetchClaudeCodeOAuthToken() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return fetchOAuthTokenFromKeychain()
	case "linux":
		return fetchOAuthTokenFromCredentialsFile()
	default:
		return "", nil
	}
}

// fetchOAuthTokenFromKeychain reads the OAuth token from macOS Keychain.
func fetchOAuthTokenFromKeychain() (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return "", fmt.Errorf("keychain lookup failed: %w", err)
	}

	return parseOAuthTokenFromJSON(out)
}

// fetchOAuthTokenFromCredentialsFile reads the OAuth token from ~/.claude/.credentials.json.
func fetchOAuthTokenFromCredentialsFile() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".claude", ".credentials.json"))
	if err != nil {
		return "", fmt.Errorf("credentials file read failed: %w", err)
	}

	return parseOAuthTokenFromJSON(data)
}

// parseOAuthTokenFromJSON parses Claude Code credentials JSON and extracts the OAuth access token.
func parseOAuthTokenFromJSON(data []byte) (string, error) {
	var creds claudeCodeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("no OAuth access token found in credentials")
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

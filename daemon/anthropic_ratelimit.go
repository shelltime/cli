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
	"strconv"
	"strings"
	"sync"
	"time"
)

const anthropicUsageCacheTTL = 10 * time.Minute

// anthropicRateLimitBackoff is the minimum cooldown applied after a 429 from the usage API,
// used when the response carries no (or a shorter) Retry-After. It is longer than the normal
// TTL so the daemon stops poking the throttled bucket.
const anthropicRateLimitBackoff = 30 * time.Minute

// claudeCodeFallbackVersion is used in the User-Agent when the real Claude Code version is
// unknown. The usage endpoint gates on the "claude-code/" prefix, not the exact version.
const claudeCodeFallbackVersion = "2.0.0"

// anthropicUsageURL is the OAuth usage endpoint. It is a var (not a const) so tests can override it.
var anthropicUsageURL = "https://api.anthropic.com/api/oauth/usage"

// anthropicUsageRequiredScope is the OAuth scope the usage endpoint gates on. Interactive Claude Code
// login tokens carry it; tokens minted by `claude setup-token` (e.g. CLAUDE_CODE_OAUTH_TOKEN in CI) do
// not, so the endpoint authenticates them but returns 403 "does not meet scope requirement user:profile".
const anthropicUsageRequiredScope = "user:profile"

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
	backoffUntil  time.Time // when set in the future, skip fetching (e.g. after a 429)
	lastError     string    // short error description for statusline display
}

// anthropicAPIError represents a non-200 response from the Anthropic usage API.
// Error() keeps the historical message format so shortenAPIError still produces "api:<code>".
type anthropicAPIError struct {
	StatusCode int
	RetryAfter time.Duration // parsed from the Retry-After header; 0 if absent/unparseable
}

func (e *anthropicAPIError) Error() string {
	return fmt.Sprintf("anthropic usage API returned status %d", e.StatusCode)
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

// fetchClaudeCodeOAuthToken reads the OAuth token and its scopes from the platform-specific
// credential store.
// macOS: reads from Keychain via `security` command.
// Linux: reads from ~/.claude/.credentials.json file.
// Returns ("", nil, nil) on unsupported platforms.
func fetchClaudeCodeOAuthToken() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return fetchOAuthTokenFromKeychain()
	case "linux":
		return fetchOAuthTokenFromCredentialsFile()
	default:
		return "", nil, nil
	}
}

// fetchOAuthTokenFromKeychain reads the OAuth token and scopes from macOS Keychain.
func fetchOAuthTokenFromKeychain() (string, []string, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return "", nil, fmt.Errorf("keychain lookup failed: %w", err)
	}

	return parseOAuthTokenFromJSON(out)
}

// fetchOAuthTokenFromCredentialsFile reads the OAuth token and scopes from ~/.claude/.credentials.json.
func fetchOAuthTokenFromCredentialsFile() (string, []string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".claude", ".credentials.json"))
	if err != nil {
		return "", nil, fmt.Errorf("credentials file read failed: %w", err)
	}

	return parseOAuthTokenFromJSON(data)
}

// parseOAuthTokenFromJSON parses Claude Code credentials JSON and extracts the OAuth access token
// and the scopes granted to it.
func parseOAuthTokenFromJSON(data []byte) (string, []string, error) {
	var creds claudeCodeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", nil, fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	if creds.ClaudeAiOauth == nil || creds.ClaudeAiOauth.AccessToken == "" {
		return "", nil, fmt.Errorf("no OAuth access token found in credentials")
	}

	return creds.ClaudeAiOauth.AccessToken, creds.ClaudeAiOauth.Scopes, nil
}

// hasUsageScope reports whether the token can read the usage endpoint. When scopes is empty
// (unknown), returns true so we still attempt the fetch and let the reactive 403 path decide.
func hasUsageScope(scopes []string) bool {
	if len(scopes) == 0 {
		return true
	}
	for _, s := range scopes {
		if s == anthropicUsageRequiredScope {
			return true
		}
	}
	return false
}

// fetchAnthropicUsage calls the Anthropic OAuth usage API and returns rate limit data.
// version is the Claude Code version used for the User-Agent header; when empty it falls back
// to claudeCodeFallbackVersion. The endpoint requires a "claude-code/<version>" User-Agent:
// without it, requests land in an aggressively rate-limited bucket and return persistent 429s.
func fetchAnthropicUsage(ctx context.Context, token, version string) (*AnthropicRateLimitData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, anthropicUsageURL, nil)
	if err != nil {
		return nil, err
	}

	if version == "" {
		version = claudeCodeFallbackVersion
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "claude-code/"+version)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &anthropicAPIError{
			StatusCode: resp.StatusCode,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
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

// parseRetryAfter parses an HTTP Retry-After header in delay-seconds form.
// Returns 0 when the value is absent or not a positive integer (HTTP-date form is not used here).
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

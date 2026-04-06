package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const codexUsageCacheTTL = 10 * time.Minute

var (
	loadCodexAuthFunc   = loadCodexAuth
	fetchCodexUsageFunc = fetchCodexUsage
	codexPathExistsFunc = codexPathExists
)

var (
	errCodexDirMissing      = errors.New("codex directory missing")
	errCodexAuthFileMissing = errors.New("codex auth file missing")
	errCodexAuthInvalid     = errors.New("codex auth invalid")
	errCodexTokenInvalid    = errors.New("codex token invalid")
)

// CodexRateLimitData holds the parsed rate limit data from the Codex API
type CodexRateLimitData struct {
	Plan    string
	Windows []CodexRateLimitWindow
}

// CodexRateLimitWindow holds a single rate limit window from the Codex API
type CodexRateLimitWindow struct {
	LimitID               string
	UsagePercentage       float64
	ResetAt               int64 // Unix timestamp
	WindowDurationMinutes int
}

type codexRateLimitCache struct {
	mu            sync.RWMutex
	usage         *CodexRateLimitData
	fetchedAt     time.Time
	lastAttemptAt time.Time
	lastError     string // short error description for statusline display
}

// codexAuthData maps the relevant fields from ~/.codex/auth.json
type codexAuthData struct {
	AccessToken string
	AccountID   string
}

// codexAuthJSON maps the full ~/.codex/auth.json structure
type codexAuthJSON struct {
	OpenAIAPIKey *string         `json:"OPENAI_API_KEY"`
	Tokens       *codexTokenData `json:"tokens"`
	LastRefresh  string          `json:"last_refresh"`
}

type codexTokenData struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

func codexConfigDirPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".codex"), nil
}

func codexAuthFilePath() (string, error) {
	dir, err := codexConfigDirPath()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "auth.json"), nil
}

func codexPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func codexInstallationStatus() (bool, error) {
	dirPath, err := codexConfigDirPath()
	if err != nil {
		return false, err
	}
	exists, err := codexPathExistsFunc(dirPath)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, errCodexDirMissing
	}

	authPath, err := codexAuthFilePath()
	if err != nil {
		return false, err
	}
	exists, err = codexPathExistsFunc(authPath)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, errCodexAuthFileMissing
	}

	return true, nil
}

func CodexInstallationStatus() (bool, error) {
	return codexInstallationStatus()
}

// loadCodexAuth reads the Codex authentication data from ~/.codex/auth.json.
func loadCodexAuth() (*codexAuthData, error) {
	authPath, err := codexAuthFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(authPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errCodexAuthFileMissing
		}
		return nil, fmt.Errorf("codex auth file read failed: %w", err)
	}

	var auth codexAuthJSON
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("failed to parse codex auth JSON: %w", err)
	}

	if auth.Tokens == nil || auth.Tokens.AccessToken == "" {
		return nil, errCodexAuthInvalid
	}

	return &codexAuthData{
		AccessToken: auth.Tokens.AccessToken,
		AccountID:   auth.Tokens.AccountID,
	}, nil
}

// codexUsageResponse maps the Codex usage API response
type codexUsageResponse struct {
	RateLimits codexRateLimitSnapshot `json:"rateLimits"`
}

type codexRateLimitSnapshot struct {
	Plan             string                    `json:"plan"`
	RateLimitWindows []codexRateLimitWindowRaw `json:"rateLimitWindows"`
}

type codexRateLimitWindowRaw struct {
	LimitID               string  `json:"limitId"`
	UsagePercentage       float64 `json:"usagePercentage"`
	ResetAt               int64   `json:"resetAt"`
	WindowDurationMinutes int     `json:"windowDurationMinutes"`
}

// fetchCodexUsage calls the Codex usage API and returns rate limit data.
func fetchCodexUsage(ctx context.Context, auth *codexAuthData) (*CodexRateLimitData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/api/codex/usage", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	if auth.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	req.Header.Set("User-Agent", "shelltime-daemon")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, errCodexTokenInvalid
		}
		return nil, fmt.Errorf("codex usage API returned status %d", resp.StatusCode)
	}

	var usage codexUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("failed to decode codex usage response: %w", err)
	}

	windows := make([]CodexRateLimitWindow, len(usage.RateLimits.RateLimitWindows))
	for i, w := range usage.RateLimits.RateLimitWindows {
		windows[i] = CodexRateLimitWindow{
			LimitID:               w.LimitID,
			UsagePercentage:       w.UsagePercentage,
			ResetAt:               w.ResetAt,
			WindowDurationMinutes: w.WindowDurationMinutes,
		}
	}

	return &CodexRateLimitData{
		Plan:    usage.RateLimits.Plan,
		Windows: windows,
	}, nil
}

// shortenCodexAPIError converts a Codex usage API error into a short string for statusline display.
func shortenCodexAPIError(err error) string {
	msg := err.Error()

	var status int
	if _, scanErr := fmt.Sscanf(msg, "codex usage API returned status %d", &status); scanErr == nil {
		return fmt.Sprintf("api:%d", status)
	}

	if len(msg) >= 6 && msg[:6] == "failed" {
		return "api:decode"
	}

	return "network"
}

func CodexSyncSkipReason(err error) (string, bool) {
	switch {
	case errors.Is(err, errCodexDirMissing):
		return "missing_codex_dir", true
	case errors.Is(err, errCodexAuthFileMissing):
		return "missing_auth_file", true
	case errors.Is(err, errCodexAuthInvalid):
		return "invalid_auth", true
	case errors.Is(err, errCodexTokenInvalid):
		return "invalid_auth_token", true
	default:
		return "", false
	}
}

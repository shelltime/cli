package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const codexUsageCacheTTL = 10 * time.Minute

const codexUsageEndpoint = "https://chatgpt.com/backend-api/wham/usage"

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
	Credits *CodexUsageCredits
}

// CodexUsageCredits holds the extra-credit state returned by Codex.
type CodexUsageCredits struct {
	HasCredits bool   `json:"has_credits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance"`
}

// CodexRateLimitWindow holds a single rate limit window from the Codex API
type CodexRateLimitWindow struct {
	LimitID               string
	LimitName             string
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

// whamUsageResponse maps the response from chatgpt.com/backend-api/wham/usage
type whamUsageResponse struct {
	PlanType             string                 `json:"plan_type"`
	RateLimit            *whamRateLimitCategory `json:"rate_limit"`
	CodeReviewRateLimit  *whamRateLimitCategory `json:"code_review_rate_limit"`
	AdditionalRateLimits json.RawMessage        `json:"additional_rate_limits"`
	Credits              *whamCredits           `json:"credits"`
}

type whamAdditionalRateLimit struct {
	LimitName      string                 `json:"limit_name"`
	MeteredFeature string                 `json:"metered_feature"`
	RateLimit      *whamRateLimitCategory `json:"rate_limit"`
}

type whamCredits struct {
	HasCredits bool   `json:"has_credits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance"`
}

type whamRateLimitCategory struct {
	Allowed         bool                 `json:"allowed"`
	LimitReached    bool                 `json:"limit_reached"`
	PrimaryWindow   *whamRateLimitWindow `json:"primary_window"`
	SecondaryWindow *whamRateLimitWindow `json:"secondary_window"`
}

type whamRateLimitWindow struct {
	UsedPercent        int   `json:"used_percent"`
	LimitWindowSeconds int   `json:"limit_window_seconds"`
	ResetAfterSeconds  int   `json:"reset_after_seconds"`
	ResetAt            int64 `json:"reset_at"`
}

// fetchCodexUsage calls the Codex usage API and returns rate limit data.
func fetchCodexUsage(ctx context.Context, auth *codexAuthData) (*CodexRateLimitData, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	return fetchCodexUsageFromEndpoint(ctx, auth, codexUsageEndpoint, client)
}

func fetchCodexUsageFromEndpoint(ctx context.Context, auth *codexAuthData, endpoint string, client *http.Client) (*CodexRateLimitData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("User-Agent", "shelltime-daemon")
	if auth.AccountID != "" {
		req.Header.Set("ChatGPT-Account-ID", auth.AccountID)
	}

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

	var usage whamUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("failed to decode codex usage response: %w", err)
	}

	return mapWhamUsageResponse(&usage)
}

func mapWhamUsageResponse(usage *whamUsageResponse) (*CodexRateLimitData, error) {
	windows := make([]CodexRateLimitWindow, 0, 4)
	windows = appendWhamCategoryWindows(windows, "rate_limit", "", usage.RateLimit)
	windows = appendWhamCategoryWindows(windows, "code_review_rate_limit", "", usage.CodeReviewRateLimit)

	additional, legacy, err := decodeAdditionalRateLimits(usage.AdditionalRateLimits)
	if err != nil {
		return nil, fmt.Errorf("failed to decode codex additional rate limits: %w", err)
	}
	for _, item := range additional {
		identifier := normalizeAdditionalLimitID(item.MeteredFeature)
		if identifier == "" {
			identifier = normalizeAdditionalLimitID(item.LimitName)
		}
		if identifier == "" {
			identifier = "unnamed"
		}
		windows = appendWhamCategoryWindows(windows, "additional_rate_limit:"+identifier, item.LimitName, item.RateLimit)
	}

	legacyNames := make([]string, 0, len(legacy))
	for name := range legacy {
		legacyNames = append(legacyNames, name)
	}
	sort.Strings(legacyNames)
	for _, name := range legacyNames {
		windows = appendWhamCategoryWindows(windows, name, "", legacy[name])
	}

	result := &CodexRateLimitData{
		Plan:    usage.PlanType,
		Windows: windows,
	}
	if usage.Credits != nil {
		result.Credits = &CodexUsageCredits{
			HasCredits: usage.Credits.HasCredits,
			Unlimited:  usage.Credits.Unlimited,
			Balance:    usage.Credits.Balance,
		}
	}
	return result, nil
}

func decodeAdditionalRateLimits(raw json.RawMessage) ([]whamAdditionalRateLimit, map[string]*whamRateLimitCategory, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil, nil
	}

	switch trimmed[0] {
	case '[':
		var additional []whamAdditionalRateLimit
		if err := json.Unmarshal(trimmed, &additional); err != nil {
			return nil, nil, err
		}
		return additional, nil, nil
	case '{':
		var legacy map[string]*whamRateLimitCategory
		if err := json.Unmarshal(trimmed, &legacy); err != nil {
			return nil, nil, err
		}
		return nil, legacy, nil
	default:
		return nil, nil, errors.New("expected an array or object")
	}
}

func appendWhamCategoryWindows(windows []CodexRateLimitWindow, category, limitName string, rateLimit *whamRateLimitCategory) []CodexRateLimitWindow {
	if rateLimit == nil {
		return windows
	}
	if rateLimit.PrimaryWindow != nil {
		windows = append(windows, mapWhamWindow(category, limitName, "primary", rateLimit.PrimaryWindow))
	}
	if rateLimit.SecondaryWindow != nil {
		windows = append(windows, mapWhamWindow(category, limitName, "secondary", rateLimit.SecondaryWindow))
	}
	return windows
}

func normalizeAdditionalLimitID(value string) string {
	var normalized strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		isAlphaNumeric := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if isAlphaNumeric {
			normalized.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && normalized.Len() > 0 {
			normalized.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(normalized.String(), "_")
}

func mapWhamWindow(category, limitName, position string, w *whamRateLimitWindow) CodexRateLimitWindow {
	return CodexRateLimitWindow{
		LimitID:               category + ":" + position,
		LimitName:             limitName,
		UsagePercentage:       float64(w.UsedPercent),
		ResetAt:               w.ResetAt,
		WindowDurationMinutes: w.LimitWindowSeconds / 60,
	}
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

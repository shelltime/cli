package model

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const DefaultCursorAPIEndpoint = "https://api2.cursor.sh"

var (
	ErrCursorAuthMissing  = errors.New("cursor authentication not found")
	ErrCursorTokenInvalid = errors.New("cursor auth token invalid")
)

// CursorAuthData holds the resolved Cursor bearer access token and API endpoint.
type CursorAuthData struct {
	AccessToken string
	Endpoint    string
	Source      string
}

// cursorNoRedirectClient builds an HTTP client that never forwards bearer tokens across redirects.
func cursorNoRedirectClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: HTTPTransport(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ResolveCursorAPIEndpoint returns the Cursor API base URL from config, CURSOR_API_ENDPOINT, or default.
func ResolveCursorAPIEndpoint(cfg *CursorConfig) string {
	if cfg != nil {
		if ep := strings.TrimSpace(cfg.APIEndpoint); ep != "" {
			return strings.TrimRight(ep, "/")
		}
	}
	if ep := strings.TrimSpace(os.Getenv("CURSOR_API_ENDPOINT")); ep != "" {
		return strings.TrimRight(ep, "/")
	}
	return DefaultCursorAPIEndpoint
}

// ExchangeCursorAPIKey trades a Cursor user API key for the access token Cursor's RPCs accept.
func ExchangeCursorAPIKey(ctx context.Context, endpoint, apiKey string, client *http.Client) (string, error) {
	if client == nil {
		client = cursorNoRedirectClient(10 * time.Second)
	}
	ep := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if ep == "" {
		ep = DefaultCursorAPIEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep+"/auth/exchange_user_api_key", bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "shelltime-cli")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", ErrCursorTokenInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("cursor API key exchange returned status %d", resp.StatusCode)
	}

	var parsed struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode cursor API key exchange response: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", ErrCursorTokenInvalid
	}
	return strings.TrimSpace(parsed.AccessToken), nil
}

// LoadCursorAuth resolves a Cursor bearer access token from:
//  1. CURSOR_AUTH_TOKEN environment variable
//  2. CURSOR_API_KEY environment variable (exchanged via /auth/exchange_user_api_key)
//  3. cfg.APIKey from ShellTime config (exchanged via /auth/exchange_user_api_key)
//  4. Local Cursor CLI auth files (~/.cursor/auth.json, ~/.config/cursor/auth.json)
//  5. Local Cursor IDE state.vscdb (cursorAuth/accessToken)
func LoadCursorAuth(ctx context.Context, cfg *CursorConfig) (*CursorAuthData, error) {
	return loadCursorAuthWithClient(ctx, cfg, nil)
}

func loadCursorAuthWithClient(ctx context.Context, cfg *CursorConfig, client *http.Client) (*CursorAuthData, error) {
	endpoint := ResolveCursorAPIEndpoint(cfg)

	if token := strings.TrimSpace(os.Getenv("CURSOR_AUTH_TOKEN")); token != "" {
		return &CursorAuthData{
			AccessToken: token,
			Endpoint:    endpoint,
			Source:      "env:CURSOR_AUTH_TOKEN",
		}, nil
	}

	if apiKey := strings.TrimSpace(os.Getenv("CURSOR_API_KEY")); apiKey != "" {
		token, err := ExchangeCursorAPIKey(ctx, endpoint, apiKey, client)
		if err != nil {
			return nil, err
		}
		return &CursorAuthData{
			AccessToken: token,
			Endpoint:    endpoint,
			Source:      "env:CURSOR_API_KEY",
		}, nil
	}

	if cfg != nil && strings.TrimSpace(cfg.APIKey) != "" {
		token, err := ExchangeCursorAPIKey(ctx, endpoint, strings.TrimSpace(cfg.APIKey), client)
		if err != nil {
			return nil, err
		}
		return &CursorAuthData{
			AccessToken: token,
			Endpoint:    endpoint,
			Source:      "config:cursor.apiKey",
		}, nil
	}

	if token, apiKey, src := readLocalCursorAuthFile(); token != "" || apiKey != "" {
		if token != "" {
			return &CursorAuthData{
				AccessToken: token,
				Endpoint:    endpoint,
				Source:      src,
			}, nil
		}
		exchanged, err := ExchangeCursorAPIKey(ctx, endpoint, apiKey, client)
		if err != nil {
			return nil, err
		}
		return &CursorAuthData{
			AccessToken: exchanged,
			Endpoint:    endpoint,
			Source:      src,
		}, nil
	}

	if token := readCursorStateVscdbToken(); token != "" {
		return &CursorAuthData{
			AccessToken: token,
			Endpoint:    endpoint,
			Source:      "cursor:state.vscdb",
		}, nil
	}

	return nil, ErrCursorAuthMissing
}

func readLocalCursorAuthFile() (accessToken, apiKey, source string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", "", ""
	}
	candidates := []string{
		filepath.Join(home, ".cursor", "auth.json"),
		filepath.Join(home, ".config", "cursor", "auth.json"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var doc struct {
			AccessToken string `json:"accessToken"`
			AccessSnake string `json:"access_token"`
			APIKey      string `json:"apiKey"`
			APIKeySnake string `json:"api_key"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		tok := strings.TrimSpace(doc.AccessToken)
		if tok == "" {
			tok = strings.TrimSpace(doc.AccessSnake)
		}
		key := strings.TrimSpace(doc.APIKey)
		if key == "" {
			key = strings.TrimSpace(doc.APIKeySnake)
		}
		if tok != "" || key != "" {
			return tok, key, "file:" + p
		}
	}
	return "", "", ""
}

func cursorStateVscdbPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{
			filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb"),
		}
	case "linux":
		xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		return []string{
			filepath.Join(xdg, "Cursor", "User", "globalStorage", "state.vscdb"),
		}
	default:
		return nil
	}
}

func readCursorStateVscdbToken() string {
	for _, p := range cursorStateVscdbPaths() {
		if tok := readAccessTokenFromVscdb(p); tok != "" {
			return tok
		}
	}
	return ""
}

func readAccessTokenFromVscdb(dbPath string) string {
	if _, err := os.Stat(dbPath); err != nil {
		return ""
	}
	dsn := "file:" + dbPath + "?mode=ro&_pragma=busy_timeout(100)&_pragma=query_only(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return ""
	}
	defer db.Close()

	var raw string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = 'cursorAuth/accessToken'").Scan(&raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(raw)
}

package model

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeFakeJWT(sub string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]any{"sub": sub, "exp": time.Now().Add(time.Hour).Unix()})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestLoadCursorAuth_PrecedenceAndExchange(t *testing.T) {
	exchangedToken := makeFakeJWT("user-123")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/auth/exchange_user_api_key", r.URL.Path)
		assert.Equal(t, "Bearer key_secret", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]string{"accessToken": exchangedToken})
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CURSOR_AUTH_TOKEN", "")
	t.Setenv("CURSOR_API_KEY", "")
	t.Setenv("CURSOR_API_ENDPOINT", srv.URL)

	// 1. Missing everywhere
	_, err := loadCursorAuthWithClient(context.Background(), nil, srv.Client())
	require.ErrorIs(t, err, ErrCursorAuthMissing)

	// 2. Config API key exchanged
	auth, err := loadCursorAuthWithClient(context.Background(), &CursorConfig{APIKey: "key_secret"}, srv.Client())
	require.NoError(t, err)
	assert.Equal(t, exchangedToken, auth.AccessToken)
	assert.Equal(t, "config:cursor.apiKey", auth.Source)

	// 3. Env API key overrides config
	t.Setenv("CURSOR_API_KEY", "key_secret")
	auth, err = loadCursorAuthWithClient(context.Background(), &CursorConfig{APIKey: "other"}, srv.Client())
	require.NoError(t, err)
	assert.Equal(t, "env:CURSOR_API_KEY", auth.Source)

	// 4. Env auth token overrides everything without exchange
	t.Setenv("CURSOR_AUTH_TOKEN", "direct-token")
	auth, err = loadCursorAuthWithClient(context.Background(), nil, srv.Client())
	require.NoError(t, err)
	assert.Equal(t, "direct-token", auth.AccessToken)
	assert.Equal(t, "env:CURSOR_AUTH_TOKEN", auth.Source)
}

func TestReadAccessTokenFromVscdb(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.vscdb")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE ItemTable (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO ItemTable (key, value) VALUES ('cursorAuth/accessToken', 'vscdb-jwt-token')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	assert.Equal(t, "vscdb-jwt-token", readAccessTokenFromVscdb(dbPath))
	assert.Equal(t, "", readAccessTokenFromVscdb(filepath.Join(dir, "missing.vscdb")))
}

func TestFetchCursorUsageSessions_PersonalAndTeamWithOverlap(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	ts1 := now.Add(-2 * time.Hour).UnixMilli()
	ts2 := now.Add(-1 * time.Hour).UnixMilli()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "1", r.Header.Get("connect-protocol-version"))
		switch r.URL.Path {
		case "/aiserver.v1.DashboardService/GetTeams":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"teams": []map[string]any{{"id": 42}},
			})
		case "/aiserver.v1.DashboardService/GetMe":
			_ = json.NewEncoder(w).Encode(map[string]any{"userId": 7})
		case "/aiserver.v1.DashboardService/GetFilteredUsageEvents":
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			if req["teamId"] == nil {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"totalUsageEventsCount": 2,
					"usageEventsDisplay": []map[string]any{
						{
							"timestamp":      ts1,
							"model":          "claude-sonnet-4-5",
							"conversationId": "conv-personal-1",
							"tokenUsage": map[string]any{
								"inputTokens":      1000,
								"outputTokens":     250,
								"cacheReadTokens":  500,
								"cacheWriteTokens": 100,
								"totalCents":       12.5,
							},
						},
						{
							// Event without tokenUsage is skipped
							"timestamp": ts1,
							"model":     "cursor-small",
						},
					},
				})
			} else {
				assert.Equal(t, float64(42), req["teamId"])
				assert.Equal(t, float64(7), req["userId"])
				_ = json.NewEncoder(w).Encode(map[string]any{
					"totalUsageEventsCount": 1,
					"usageEventsDisplay": []map[string]any{
						{
							"timestamp":      ts2,
							"model":          "gpt-5",
							"conversationId": "",
							"tokenUsage": map[string]any{
								"inputTokens":  400,
								"outputTokens": 100,
								"totalCents":   3.0,
							},
						},
					},
				})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	auth := &CursorAuthData{
		AccessToken: makeFakeJWT("auth0|cursor-user-1"),
		Endpoint:    srv.URL,
	}
	res, err := fetchCursorUsageSessionsWithClient(context.Background(), auth, BackfillOptions{
		Since: now.Add(-24 * time.Hour),
		Until: now,
		Now:   now,
	}, srv.Client())
	require.NoError(t, err)
	require.Len(t, res.Sessions, 2)

	// Personal session
	s0 := res.Sessions[0]
	assert.Equal(t, "conv-personal-1", s0.ID)
	assert.Equal(t, AICodeClientCursor, s0.ClientType)
	require.Len(t, s0.Events, 1)
	assert.Equal(t, "claude-sonnet-4-5", s0.Events[0].Model)
	assert.Equal(t, 1000, *s0.Events[0].InputTokens)
	assert.Equal(t, 250, *s0.Events[0].OutputTokens)
	assert.Equal(t, 500, *s0.Events[0].CacheReadTokens)
	assert.Equal(t, 100, *s0.Events[0].CacheCreationTokens)
	require.NotNil(t, s0.Events[0].CostUSD)
	assert.InDelta(t, 0.125, *s0.Events[0].CostUSD, 1e-9)

	// Team session with empty conversationId falls back to daily bucket
	s1 := res.Sessions[1]
	assert.Equal(t, "cursor-day-2026-11-20", s1.ID)
	require.Len(t, s1.Events, 1)
	assert.Equal(t, "gpt-5", s1.Events[0].Model)
	assert.InDelta(t, 0.03, *s1.Events[0].CostUSD, 1e-9)
}

func TestCursorBoundaryOverlap(t *testing.T) {
	prev := []string{"a", "b", "c", "d"}
	curr := []string{"c", "d", "e", "f"}
	assert.Equal(t, 2, cursorBoundaryOverlap(prev, curr))
	assert.Equal(t, 0, cursorBoundaryOverlap(prev, []string{"x", "y"}))
}

func TestReadLocalCursorAuthFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cursor"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".cursor", "auth.json"), []byte(`{"accessToken":"file-tok"}`), 0600))

	tok, key, src := readLocalCursorAuthFile()
	assert.Equal(t, "file-tok", tok)
	assert.Equal(t, "", key)
	assert.Contains(t, src, "auth.json")
}

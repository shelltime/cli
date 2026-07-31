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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCodexAuth_Valid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0700))
	content := `{"OPENAI_API_KEY":null,"tokens":{"id_token":"id","access_token":"acc-tok","refresh_token":"ref","account_id":"acct-1"},"last_refresh":"2025-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(content), 0600))

	auth, err := loadCodexAuth()
	require.NoError(t, err)
	assert.Equal(t, "acc-tok", auth.AccessToken)
	assert.Equal(t, "acct-1", auth.AccountID)
}

func TestLoadCodexAuth_MissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	auth, err := loadCodexAuth()
	assert.Nil(t, auth)
	assert.ErrorIs(t, err, errCodexAuthFileMissing)
}

func TestLoadCodexAuth_MalformedJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte("not json"), 0600))

	auth, err := loadCodexAuth()
	assert.Nil(t, auth)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse codex auth JSON")
}

func TestLoadCodexAuth_NoTokens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0700))
	// tokens object present but empty access_token
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"tokens":{"access_token":""}}`), 0600))

	auth, err := loadCodexAuth()
	assert.Nil(t, auth)
	assert.ErrorIs(t, err, errCodexAuthInvalid)
}

func TestLoadCodexAuth_NilTokens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	codexDir := filepath.Join(home, ".codex")
	require.NoError(t, os.MkdirAll(codexDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"OPENAI_API_KEY":"sk-x"}`), 0600))

	auth, err := loadCodexAuth()
	assert.Nil(t, auth)
	assert.ErrorIs(t, err, errCodexAuthInvalid)
}

func TestCodexPathExists(t *testing.T) {
	home := t.TempDir()

	existing := filepath.Join(home, "present")
	require.NoError(t, os.WriteFile(existing, []byte("x"), 0600))

	ok, err := codexPathExists(existing)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = codexPathExists(filepath.Join(home, "absent"))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCodexInstallationStatus_RealFilesystem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No .codex dir yet.
	ok, err := codexInstallationStatus()
	assert.False(t, ok)
	assert.ErrorIs(t, err, errCodexDirMissing)

	// Create dir but no auth file.
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".codex"), 0700))
	ok, err = codexInstallationStatus()
	assert.False(t, ok)
	assert.ErrorIs(t, err, errCodexAuthFileMissing)

	// Create auth file.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte("{}"), 0600))
	ok, err = codexInstallationStatus()
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestMapWhamWindow(t *testing.T) {
	w := &whamRateLimitWindow{
		UsedPercent:        85,
		LimitWindowSeconds: 18000, // 300 minutes
		ResetAfterSeconds:  120,
		ResetAt:            1712400000,
	}
	got := mapWhamWindow("rate_limit", "", "primary", w)
	assert.Equal(t, "rate_limit:primary", got.LimitID)
	assert.Equal(t, float64(85), got.UsagePercentage)
	assert.Equal(t, int64(1712400000), got.ResetAt)
	assert.Equal(t, 300, got.WindowDurationMinutes)
}

func TestShortenCodexAPIError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected string
	}{
		{"http status", fmt.Errorf("codex usage API returned status %d", 429), "api:429"},
		{"decode error", errors.New("failed to decode codex usage response: EOF"), "api:decode"},
		{"network", errors.New("dial tcp: connection refused"), "network"},
		{"token invalid maps to network", errCodexTokenInvalid, "network"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, shortenCodexAPIError(tc.err))
		})
	}
}

func TestFetchCodexUsage_CurrentResponseShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "account-1", r.Header.Get("ChatGPT-Account-ID"))
		assert.Equal(t, "shelltime-daemon", r.Header.Get("User-Agent"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"plan_type": "prolite",
			"rate_limit": map[string]any{
				"primary_window": map[string]any{"used_percent": 12, "limit_window_seconds": 604800, "reset_at": 1712400000},
			},
			"code_review_rate_limit": nil,
			"additional_rate_limits": []any{
				map[string]any{
					"limit_name":      "GPT-5.3-Codex-Spark",
					"metered_feature": "codex_bengalfox",
					"rate_limit": map[string]any{
						"primary_window": map[string]any{"used_percent": 40, "limit_window_seconds": 18000, "reset_at": 1712400100},
					},
				},
			},
			"credits": map[string]any{"has_credits": false, "unlimited": false, "balance": "0"},
		})
	}))
	defer server.Close()

	usage, err := fetchCodexUsageFromEndpoint(context.Background(), &codexAuthData{
		AccessToken: "access-token",
		AccountID:   "account-1",
	}, server.URL, server.Client())
	require.NoError(t, err)

	assert.Equal(t, "prolite", usage.Plan)
	require.Len(t, usage.Windows, 2)
	assert.Equal(t, "rate_limit:primary", usage.Windows[0].LimitID)
	assert.Equal(t, 10080, usage.Windows[0].WindowDurationMinutes)
	assert.Equal(t, "additional_rate_limit:codex_bengalfox:primary", usage.Windows[1].LimitID)
	assert.Equal(t, "GPT-5.3-Codex-Spark", usage.Windows[1].LimitName)
	require.NotNil(t, usage.Credits)
	assert.False(t, usage.Credits.HasCredits)
	assert.False(t, usage.Credits.Unlimited)
	assert.Equal(t, "0", usage.Credits.Balance)
}

func TestMapWhamUsageResponse_LegacyAdditionalRateLimits(t *testing.T) {
	var response whamUsageResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"plan_type":"pro",
		"rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":300,"reset_at":100},"secondary_window":{"used_percent":20,"limit_window_seconds":600,"reset_at":200}},
		"code_review_rate_limit":{"primary_window":{"used_percent":30,"limit_window_seconds":1200,"reset_at":300}},
		"additional_rate_limits":{"z_extra":null,"extra":{"primary_window":{"used_percent":40,"limit_window_seconds":60,"reset_at":400}}}
	}`), &response))

	usage, err := mapWhamUsageResponse(&response)
	require.NoError(t, err)
	require.Len(t, usage.Windows, 4)
	assert.Equal(t, "rate_limit:primary", usage.Windows[0].LimitID)
	assert.Equal(t, "rate_limit:secondary", usage.Windows[1].LimitID)
	assert.Equal(t, "code_review_rate_limit:primary", usage.Windows[2].LimitID)
	assert.Equal(t, "extra:primary", usage.Windows[3].LimitID)
	assert.Empty(t, usage.Windows[3].LimitName)
}

func TestMapWhamUsageResponse_RejectsInvalidAdditionalRateLimits(t *testing.T) {
	response := &whamUsageResponse{AdditionalRateLimits: json.RawMessage(`"invalid"`)}
	_, err := mapWhamUsageResponse(response)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected an array or object")
}

func TestFetchCodexUsage_StatusHandling(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			usage, err := fetchCodexUsageFromEndpoint(context.Background(), &codexAuthData{AccessToken: "invalid"}, server.URL, server.Client())
			assert.Nil(t, usage)
			assert.ErrorIs(t, err, errCodexTokenInvalid)
		})
	}
}

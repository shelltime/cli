package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapCursorCurrentPeriodUsage(t *testing.T) {
	total := 42.5
	auto := 30.0
	api := 110.0 // clamped to 100
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	got, err := mapCursorCurrentPeriodUsage(&cursorCurrentPeriodUsageResponse{
		BillingCycleStart: float64(start),
		BillingCycleEnd:   float64(end),
		PlanUsage: &cursorPlanUsageBody{
			TotalPercentUsed: &total,
			AutoPercentUsed:  &auto,
			APIPercentUsed:   &api,
		},
	})
	require.NoError(t, err)
	require.Len(t, got.Windows, 3)

	assert.Equal(t, "totalPercentUsed", got.Windows[0].LimitID)
	assert.Equal(t, "Overall", got.Windows[0].LimitName)
	assert.InDelta(t, 42.5, got.Windows[0].UsagePercentage, 1e-6)
	assert.Equal(t, end/1000, got.Windows[0].ResetAt)
	assert.Equal(t, 30*24*60, got.Windows[0].WindowDurationMinutes)

	assert.Equal(t, "autoPercentUsed", got.Windows[1].LimitID)
	assert.Equal(t, "Cursor Models", got.Windows[1].LimitName)

	assert.Equal(t, "apiPercentUsed", got.Windows[2].LimitID)
	assert.Equal(t, "Other Models", got.Windows[2].LimitName)
	assert.Equal(t, 100.0, got.Windows[2].UsagePercentage)
}

func TestCursorUsageSyncService_SyncsQuotaAndTokens(t *testing.T) {
	origList := listProcessNamesFunc
	origAuth := loadCursorAuthFunc
	origQuota := fetchCursorQuotaFunc
	origUsage := fetchCursorUsageSessionsFunc
	t.Cleanup(func() {
		listProcessNamesFunc = origList
		loadCursorAuthFunc = origAuth
		fetchCursorQuotaFunc = origQuota
		fetchCursorUsageSessionsFunc = origUsage
	})

	var quotaHits, statusHits, backfillHits, completeHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/cursor-usage":
			quotaHits.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/cc/backfill/sessions":
			statusHits.Add(1)
			_ = json.NewEncoder(w).Encode(model.AICodeBackfillSessionsResponse{})
		case "/api/v1/cc/backfill":
			backfillHits.Add(1)
			_ = json.NewEncoder(w).Encode(model.AICodeBackfillResponse{Success: true, Accepted: 1, Summarized: 1})
		case "/api/v1/cc/backfill/complete":
			completeHits.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	listProcessNamesFunc = func() ([]string, error) { return []string{"/Applications/Cursor.app/Contents/MacOS/Cursor"}, nil }
	loadCursorAuthFunc = func(context.Context, *model.CursorConfig) (*model.CursorAuthData, error) {
		return &model.CursorAuthData{AccessToken: "tok", Endpoint: "https://api2.cursor.sh"}, nil
	}
	fetchCursorQuotaFunc = func(context.Context, *model.CursorAuthData) (*CursorRateLimitData, error) {
		return &CursorRateLimitData{
			Windows: []CursorRateLimitWindow{{
				LimitID:               "totalPercentUsed",
				LimitName:             "Overall",
				UsagePercentage:       55,
				ResetAt:               time.Now().Add(24 * time.Hour).Unix(),
				WindowDurationMinutes: 43200,
			}},
		}, nil
	}
	now := time.Now().Add(-time.Hour).UTC()
	inTok := 100
	fetchCursorUsageSessionsFunc = func(context.Context, *model.CursorAuthData, model.BackfillOptions) (*model.BackfillParseResult, error) {
		return &model.BackfillParseResult{
			Sessions: []*model.BackfillSession{{
				ID:         "cursor-sess-1",
				ClientType: model.AICodeClientCursor,
				Start:      now,
				End:        now,
				Events: []model.AICodeBackfillEvent{{
					EventID:     "bf1:0123456789abcdef0123456789abcdef01234567",
					EventType:   model.AICodeEventApiRequest,
					ClientType:  model.AICodeClientCursor,
					Timestamp:   now.Unix(),
					SessionID:   "cursor-sess-1",
					Model:       "claude-sonnet-4-5",
					InputTokens: &inTok,
				}},
			}},
		}, nil
	}

	svc := NewCursorUsageSyncService(model.ShellTimeConfig{
		Token:       "st_test",
		APIEndpoint: srv.URL,
	})
	svc.statePath = filepath.Join(t.TempDir(), "state.json")
	svc.sync()

	assert.Equal(t, int32(1), quotaHits.Load())
	assert.Equal(t, int32(1), statusHits.Load())
	assert.Equal(t, int32(1), backfillHits.Load())
	assert.Equal(t, int32(1), completeHits.Load())

	// When Cursor is not running, sync skips immediately.
	listProcessNamesFunc = func() ([]string, error) { return []string{"zsh"}, nil }
	svc.sync()
	assert.Equal(t, int32(1), quotaHits.Load())
}

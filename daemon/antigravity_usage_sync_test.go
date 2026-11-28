package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
)

func TestAntigravityUsageSyncService_SkipsUnchangedDBs(t *testing.T) {
	origList := listProcessNamesFunc
	origRoots := antigravityDataRootsFunc
	origCollect := collectAntigravityDBFilesFunc
	origParse := parseAntigravityDBFilesFunc
	origFP := antigravityDBFingerprintFunc
	t.Cleanup(func() {
		listProcessNamesFunc = origList
		antigravityDataRootsFunc = origRoots
		collectAntigravityDBFilesFunc = origCollect
		parseAntigravityDBFilesFunc = origParse
		antigravityDBFingerprintFunc = origFP
	})

	var backfillHits, completeHits, parseCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/cc/backfill/sessions":
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

	listProcessNamesFunc = func() ([]string, error) { return []string{"/usr/local/bin/agy"}, nil }
	antigravityDataRootsFunc = func() []string { return []string{"/fake/agy"} }
	collectAntigravityDBFilesFunc = func([]string, time.Time) ([]string, error) {
		return []string{"/fake/agy/sess-1.db"}, nil
	}
	fp := "100:1/-"
	antigravityDBFingerprintFunc = func(string) (string, time.Time, error) {
		return fp, time.Now(), nil
	}
	now := time.Now().Add(-time.Hour).UTC()
	inTok := 200
	parseAntigravityDBFilesFunc = func([]string, model.BackfillOptions) (*model.BackfillParseResult, error) {
		parseCalls.Add(1)
		return &model.BackfillParseResult{
			Sessions: []*model.BackfillSession{{
				ID:         "sess-1",
				ClientType: model.AICodeClientAntigravity,
				Start:      now,
				End:        now,
				Events: []model.AICodeBackfillEvent{{
					EventID:     "bf1:fedcba9876543210fedcba9876543210fedcba98",
					EventType:   model.AICodeEventApiRequest,
					ClientType:  model.AICodeClientAntigravity,
					Timestamp:   now.Unix(),
					SessionID:   "sess-1",
					Model:       "gemini-3.1-pro",
					InputTokens: &inTok,
				}},
			}},
		}, nil
	}

	svc := NewAntigravityUsageSyncService(model.ShellTimeConfig{
		Token:       "st_test",
		APIEndpoint: srv.URL,
	})

	// First tick parses and uploads.
	svc.sync(context.Background())
	assert.Equal(t, int32(1), parseCalls.Load())
	assert.Equal(t, int32(1), backfillHits.Load())
	assert.Equal(t, int32(1), completeHits.Load())

	// Second tick with unchanged fingerprint skips parsing entirely.
	svc.sync(context.Background())
	assert.Equal(t, int32(1), parseCalls.Load())

	// Third tick with updated WAL fingerprint re-parses and uploads.
	fp = "100:1/50:2"
	svc.sync(context.Background())
	assert.Equal(t, int32(2), parseCalls.Load())
	assert.Equal(t, int32(2), backfillHits.Load())
}

package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetStatuslineCache restores the package-level cache singleton to a clean
// state so tests don't leak cached values into each other.
func resetStatuslineCache(t *testing.T) {
	t.Helper()
	statuslineCache.mu.Lock()
	statuslineCache.entry = nil
	statuslineCache.fetching = false
	statuslineCache.ttl = DefaultStatuslineCacheTTL
	statuslineCache.mu.Unlock()
	t.Cleanup(func() {
		statuslineCache.mu.Lock()
		statuslineCache.entry = nil
		statuslineCache.fetching = false
		statuslineCache.ttl = DefaultStatuslineCacheTTL
		statuslineCache.mu.Unlock()
	})
}

func TestFetchDailyStats(t *testing.T) {
	t.Run("happy path maps analytics to stats", func(t *testing.T) {
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"aiCodeOtel":{"analytics":{"totalCostUsd":12.34,"totalSessionSeconds":600}}}}}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
		stats, err := FetchDailyStats(context.Background(), cfg)
		require.NoError(t, err)
		assert.Equal(t, 12.34, stats.Cost)
		assert.Equal(t, 600, stats.SessionSeconds)
		assert.Equal(t, "/api/v2/graphql", gotPath)
	})

	t.Run("error path returns zero stats and error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"server down"}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
		stats, err := FetchDailyStats(context.Background(), cfg)
		require.Error(t, err)
		assert.Equal(t, CCStatuslineDailyStats{}, stats)
	})
}

func TestFetchDailyStatsCached_ReturnsValidCache(t *testing.T) {
	resetStatuslineCache(t)

	// Seed a valid cache entry; cached path should return it without any HTTP.
	CCStatuslineCacheSet(CCStatuslineDailyStats{Cost: 5.0, SessionSeconds: 120})

	cfg := ShellTimeConfig{Token: "tok", APIEndpoint: "http://127.0.0.1:0"}
	stats := FetchDailyStatsCached(context.Background(), cfg)
	assert.Equal(t, 5.0, stats.Cost)
	assert.Equal(t, 120, stats.SessionSeconds)
}

func TestFetchDailyStatsAsync_UpdatesCacheOnSuccess(t *testing.T) {
	resetStatuslineCache(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"fetchUser":{"aiCodeOtel":{"analytics":{"totalCostUsd":7.5,"totalSessionSeconds":300}}}}}`))
	}))
	defer server.Close()

	cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	// Call the async fetch synchronously (it's just a function) to deterministically
	// exercise the cache-update path without sleeping on a goroutine.
	fetchDailyStatsAsync(context.Background(), cfg)

	stats, valid := CCStatuslineCacheGet()
	require.True(t, valid, "cache should be populated after a successful fetch")
	assert.Equal(t, 7.5, stats.Cost)
	assert.Equal(t, 300, stats.SessionSeconds)
}

func TestFetchDailyStatsAsync_NoTokenSkips(t *testing.T) {
	resetStatuslineCache(t)

	// No token -> returns early, cache stays empty, fetching flag reset.
	fetchDailyStatsAsync(context.Background(), ShellTimeConfig{Token: ""})

	_, valid := CCStatuslineCacheGet()
	assert.False(t, valid)
	assert.True(t, CCStatuslineCacheStartFetch(), "fetching flag should be reset to allow a new fetch")
}

func TestFetchDailyStatsAsync_AlreadyFetchingReturnsImmediately(t *testing.T) {
	resetStatuslineCache(t)

	// Mark a fetch in progress; the async fetch must bail out without changing
	// the cache (no token would otherwise be needed because it returns first).
	require.True(t, CCStatuslineCacheStartFetch())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP request expected when a fetch is already in progress")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
	fetchDailyStatsAsync(context.Background(), cfg)

	_, valid := CCStatuslineCacheGet()
	assert.False(t, valid)
}

func TestFetchDailyStatsCached_ConcurrentSafe(t *testing.T) {
	resetStatuslineCache(t)
	CCStatuslineCacheSet(CCStatuslineDailyStats{Cost: 1.0, SessionSeconds: 10})

	cfg := ShellTimeConfig{Token: "tok", APIEndpoint: "http://127.0.0.1:0"}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = FetchDailyStatsCached(context.Background(), cfg)
		}()
	}
	// Should complete quickly without data races (run with -race).
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent cached reads blocked")
	}
}

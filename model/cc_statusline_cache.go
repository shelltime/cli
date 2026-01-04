package model

import (
	"sync"
	"time"
)

const (
	// DefaultStatuslineCacheTTL is the default cache TTL for statusline daily cost
	DefaultStatuslineCacheTTL = 5 * time.Minute
)

// CCStatuslineDailyStats holds daily statistics for the statusline
type CCStatuslineDailyStats struct {
	Cost           float64
	SessionSeconds int
}

// ccStatuslineCacheEntry represents a cached daily cost entry
type ccStatuslineCacheEntry struct {
	Date           string
	CostUsd        float64
	SessionSeconds int
	FetchedAt      time.Time
	TTL            time.Duration
}

// IsValid returns true if the cache entry is still valid
func (e *ccStatuslineCacheEntry) IsValid() bool {
	if e == nil {
		return false
	}
	// Check if date matches today
	today := time.Now().Format("2006-01-02")
	if e.Date != today {
		return false
	}
	// Check TTL
	return time.Since(e.FetchedAt) < e.TTL
}

// ccStatuslineCache manages caching for statusline daily cost
type ccStatuslineCache struct {
	mu       sync.RWMutex
	entry    *ccStatuslineCacheEntry
	ttl      time.Duration
	fetching bool
}

// Global cache instance (package-level singleton)
var statuslineCache = &ccStatuslineCache{
	ttl: DefaultStatuslineCacheTTL,
}

// CCStatuslineCacheGet returns cached value and whether it's valid
func CCStatuslineCacheGet() (CCStatuslineDailyStats, bool) {
	statuslineCache.mu.RLock()
	defer statuslineCache.mu.RUnlock()

	if statuslineCache.entry != nil && statuslineCache.entry.IsValid() {
		return CCStatuslineDailyStats{
			Cost:           statuslineCache.entry.CostUsd,
			SessionSeconds: statuslineCache.entry.SessionSeconds,
		}, true
	}
	return CCStatuslineDailyStats{}, false
}

// CCStatuslineCacheGetLastValue returns the last cached value even if expired
func CCStatuslineCacheGetLastValue() CCStatuslineDailyStats {
	statuslineCache.mu.RLock()
	defer statuslineCache.mu.RUnlock()

	if statuslineCache.entry != nil {
		return CCStatuslineDailyStats{
			Cost:           statuslineCache.entry.CostUsd,
			SessionSeconds: statuslineCache.entry.SessionSeconds,
		}
	}
	return CCStatuslineDailyStats{}
}

// CCStatuslineCacheSet updates the cache with a new value
func CCStatuslineCacheSet(stats CCStatuslineDailyStats) {
	statuslineCache.mu.Lock()
	defer statuslineCache.mu.Unlock()

	statuslineCache.entry = &ccStatuslineCacheEntry{
		Date:           time.Now().Format("2006-01-02"),
		CostUsd:        stats.Cost,
		SessionSeconds: stats.SessionSeconds,
		FetchedAt:      time.Now(),
		TTL:            statuslineCache.ttl,
	}
	statuslineCache.fetching = false
}

// CCStatuslineCacheStartFetch marks that a fetch is in progress
// Returns true if fetch can start, false if already fetching
func CCStatuslineCacheStartFetch() bool {
	statuslineCache.mu.Lock()
	defer statuslineCache.mu.Unlock()

	if statuslineCache.fetching {
		return false
	}
	statuslineCache.fetching = true
	return true
}

// CCStatuslineCacheEndFetch marks that fetch has completed (used on error)
func CCStatuslineCacheEndFetch() {
	statuslineCache.mu.Lock()
	defer statuslineCache.mu.Unlock()
	statuslineCache.fetching = false
}

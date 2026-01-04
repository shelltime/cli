package model

import (
	"testing"
	"time"
)

func TestCCStatuslineCacheEntry_IsValid(t *testing.T) {
	t.Run("nil entry", func(t *testing.T) {
		var entry *ccStatuslineCacheEntry
		if entry.IsValid() {
			t.Error("nil entry should not be valid")
		}
	})

	t.Run("valid entry", func(t *testing.T) {
		entry := &ccStatuslineCacheEntry{
			Date:      time.Now().Format("2006-01-02"),
			CostUsd:   1.23,
			FetchedAt: time.Now(),
			TTL:       5 * time.Minute,
		}
		if !entry.IsValid() {
			t.Error("recent entry should be valid")
		}
	})

	t.Run("expired entry", func(t *testing.T) {
		entry := &ccStatuslineCacheEntry{
			Date:      time.Now().Format("2006-01-02"),
			CostUsd:   1.23,
			FetchedAt: time.Now().Add(-10 * time.Minute),
			TTL:       5 * time.Minute,
		}
		if entry.IsValid() {
			t.Error("expired entry should not be valid")
		}
	})

	t.Run("wrong date entry", func(t *testing.T) {
		entry := &ccStatuslineCacheEntry{
			Date:      "2020-01-01", // Past date
			CostUsd:   1.23,
			FetchedAt: time.Now(),
			TTL:       5 * time.Minute,
		}
		if entry.IsValid() {
			t.Error("entry from different date should not be valid")
		}
	})
}

func TestCCStatuslineCacheGetSet(t *testing.T) {
	// Reset cache state
	statuslineCache.mu.Lock()
	statuslineCache.entry = nil
	statuslineCache.fetching = false
	statuslineCache.mu.Unlock()

	// Initially cache should be empty
	cost, valid := CCStatuslineCacheGet()
	if valid {
		t.Error("cache should initially be invalid")
	}
	if cost != 0 {
		t.Errorf("Expected 0 cost, got %f", cost)
	}

	// Set a value
	CCStatuslineCacheSet(2.50)

	// Now cache should be valid
	cost, valid = CCStatuslineCacheGet()
	if !valid {
		t.Error("cache should be valid after set")
	}
	if cost != 2.50 {
		t.Errorf("Expected 2.50, got %f", cost)
	}
}

func TestCCStatuslineCacheGetLastValue(t *testing.T) {
	// Reset cache state
	statuslineCache.mu.Lock()
	statuslineCache.entry = nil
	statuslineCache.mu.Unlock()

	// No entry - should return 0
	if CCStatuslineCacheGetLastValue() != 0 {
		t.Error("Expected 0 for nil entry")
	}

	// Set a value
	CCStatuslineCacheSet(3.75)

	// Should return the value
	if CCStatuslineCacheGetLastValue() != 3.75 {
		t.Errorf("Expected 3.75, got %f", CCStatuslineCacheGetLastValue())
	}

	// Manually expire the entry but keep it
	statuslineCache.mu.Lock()
	statuslineCache.entry.FetchedAt = time.Now().Add(-1 * time.Hour)
	statuslineCache.mu.Unlock()

	// GetLastValue should still return the value even if expired
	if CCStatuslineCacheGetLastValue() != 3.75 {
		t.Errorf("Expected 3.75 even when expired, got %f", CCStatuslineCacheGetLastValue())
	}

	// But CCStatuslineCacheGet should return invalid
	_, valid := CCStatuslineCacheGet()
	if valid {
		t.Error("Cache should be invalid after expiration")
	}
}

func TestCCStatuslineCacheStartFetch(t *testing.T) {
	// Reset cache state
	statuslineCache.mu.Lock()
	statuslineCache.fetching = false
	statuslineCache.mu.Unlock()

	// First call should return true
	if !CCStatuslineCacheStartFetch() {
		t.Error("First StartFetch should return true")
	}

	// Second call should return false (already fetching)
	if CCStatuslineCacheStartFetch() {
		t.Error("Second StartFetch should return false")
	}

	// End fetch
	CCStatuslineCacheEndFetch()

	// Now should be able to start again
	if !CCStatuslineCacheStartFetch() {
		t.Error("StartFetch should return true after EndFetch")
	}

	// Cleanup
	CCStatuslineCacheEndFetch()
}

func TestCCStatuslineCacheEndFetch(t *testing.T) {
	// Reset cache state
	statuslineCache.mu.Lock()
	statuslineCache.fetching = true
	statuslineCache.mu.Unlock()

	CCStatuslineCacheEndFetch()

	// Verify fetching is false
	statuslineCache.mu.RLock()
	fetching := statuslineCache.fetching
	statuslineCache.mu.RUnlock()

	if fetching {
		t.Error("fetching should be false after EndFetch")
	}
}

func TestCCStatuslineCacheSet_ClearsFetching(t *testing.T) {
	// Start fetching
	statuslineCache.mu.Lock()
	statuslineCache.fetching = true
	statuslineCache.mu.Unlock()

	// Set value
	CCStatuslineCacheSet(1.00)

	// Verify fetching is false
	statuslineCache.mu.RLock()
	fetching := statuslineCache.fetching
	statuslineCache.mu.RUnlock()

	if fetching {
		t.Error("fetching should be false after Set")
	}
}

func TestDefaultStatuslineCacheTTL(t *testing.T) {
	expected := 5 * time.Minute
	if DefaultStatuslineCacheTTL != expected {
		t.Errorf("Expected DefaultStatuslineCacheTTL to be 5m, got %v", DefaultStatuslineCacheTTL)
	}
}

func TestCCStatuslineCache_ConcurrentAccess(t *testing.T) {
	// Reset cache state
	statuslineCache.mu.Lock()
	statuslineCache.entry = nil
	statuslineCache.fetching = false
	statuslineCache.mu.Unlock()

	done := make(chan bool, 20)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			CCStatuslineCacheGet()
			CCStatuslineCacheGetLastValue()
			done <- true
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(val float64) {
			CCStatuslineCacheSet(val)
			done <- true
		}(float64(i))
	}

	// Concurrent fetch attempts
	for i := 0; i < 5; i++ {
		go func() {
			if CCStatuslineCacheStartFetch() {
				CCStatuslineCacheEndFetch()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestCCStatuslineCacheEntry_DateComparison(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	t.Run("today matches", func(t *testing.T) {
		entry := &ccStatuslineCacheEntry{
			Date:      today,
			FetchedAt: time.Now(),
			TTL:       5 * time.Minute,
		}
		if !entry.IsValid() {
			t.Error("today's date should be valid")
		}
	})

	t.Run("yesterday doesn't match", func(t *testing.T) {
		yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
		entry := &ccStatuslineCacheEntry{
			Date:      yesterday,
			FetchedAt: time.Now(),
			TTL:       5 * time.Minute,
		}
		if entry.IsValid() {
			t.Error("yesterday's date should not be valid")
		}
	})

	t.Run("tomorrow doesn't match", func(t *testing.T) {
		tomorrow := time.Now().Add(24 * time.Hour).Format("2006-01-02")
		entry := &ccStatuslineCacheEntry{
			Date:      tomorrow,
			FetchedAt: time.Now(),
			TTL:       5 * time.Minute,
		}
		if entry.IsValid() {
			t.Error("tomorrow's date should not be valid")
		}
	})
}

func TestCCStatuslineCacheEntry_TTLBoundary(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	t.Run("just before TTL", func(t *testing.T) {
		entry := &ccStatuslineCacheEntry{
			Date:      today,
			FetchedAt: time.Now().Add(-4*time.Minute - 59*time.Second),
			TTL:       5 * time.Minute,
		}
		if !entry.IsValid() {
			t.Error("entry just before TTL should be valid")
		}
	})

	t.Run("exactly at TTL", func(t *testing.T) {
		entry := &ccStatuslineCacheEntry{
			Date:      today,
			FetchedAt: time.Now().Add(-5 * time.Minute),
			TTL:       5 * time.Minute,
		}
		// At exactly TTL, time.Since >= TTL so it should be invalid
		if entry.IsValid() {
			t.Error("entry exactly at TTL should be invalid")
		}
	})

	t.Run("just after TTL", func(t *testing.T) {
		entry := &ccStatuslineCacheEntry{
			Date:      today,
			FetchedAt: time.Now().Add(-5*time.Minute - 1*time.Second),
			TTL:       5 * time.Minute,
		}
		if entry.IsValid() {
			t.Error("entry just after TTL should be invalid")
		}
	})
}

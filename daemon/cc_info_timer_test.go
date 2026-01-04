package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CCInfoTimerTestSuite struct {
	suite.Suite
	server                   *httptest.Server
	originalFetchInterval    time.Duration
	originalInactivityTimeout time.Duration
}

func (s *CCInfoTimerTestSuite) SetupSuite() {
	// Save original values
	s.originalFetchInterval = CCInfoFetchInterval
	s.originalInactivityTimeout = CCInfoInactivityTimeout

	// Setup mock GraphQL server
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/graphql" {
			response := map[string]interface{}{
				"data": map[string]interface{}{
					"fetchUser": map[string]interface{}{
						"aiCodeOtel": map[string]interface{}{
							"analytics": map[string]interface{}{
								"totalCostUsd": 5.42,
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
}

func (s *CCInfoTimerTestSuite) TearDownSuite() {
	s.server.Close()
}

func (s *CCInfoTimerTestSuite) SetupTest() {
	// Use short intervals for fast tests
	CCInfoFetchInterval = 10 * time.Millisecond
	CCInfoInactivityTimeout = 50 * time.Millisecond
}

func (s *CCInfoTimerTestSuite) TearDownTest() {
	// Restore original values
	CCInfoFetchInterval = s.originalFetchInterval
	CCInfoInactivityTimeout = s.originalInactivityTimeout
}

// Constructor Tests

func (s *CCInfoTimerTestSuite) TestNewCCInfoTimerService() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}

	service := NewCCInfoTimerService(config)

	assert.NotNil(s.T(), service)
	assert.NotNil(s.T(), service.cache)
	assert.NotNil(s.T(), service.activeRanges)
	assert.Empty(s.T(), service.cache)
	assert.Empty(s.T(), service.activeRanges)
	assert.False(s.T(), service.timerRunning)
}

// Cache Tests

func (s *CCInfoTimerTestSuite) TestGetCachedCost_EmptyCache() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	cache := service.GetCachedCost(CCInfoTimeRangeToday)

	assert.Equal(s.T(), float64(0), cache.TotalCostUSD)
	assert.True(s.T(), cache.FetchedAt.IsZero())
}

func (s *CCInfoTimerTestSuite) TestGetCachedCost_MarksRangeAsActive() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	service.GetCachedCost(CCInfoTimeRangeToday)

	service.mu.RLock()
	defer service.mu.RUnlock()
	assert.True(s.T(), service.activeRanges[CCInfoTimeRangeToday])
}

func (s *CCInfoTimerTestSuite) TestGetCachedCost_ReturnsCachedValue() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	// Manually set cache
	expectedCost := 10.50
	expectedTime := time.Now()
	service.mu.Lock()
	service.cache[CCInfoTimeRangeToday] = CCInfoCache{
		TotalCostUSD: expectedCost,
		FetchedAt:    expectedTime,
	}
	service.mu.Unlock()

	cache := service.GetCachedCost(CCInfoTimeRangeToday)

	assert.Equal(s.T(), expectedCost, cache.TotalCostUSD)
	assert.Equal(s.T(), expectedTime, cache.FetchedAt)
}

func (s *CCInfoTimerTestSuite) TestGetCachedCost_MultipleRanges() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	// Set different values for different ranges
	service.mu.Lock()
	service.cache[CCInfoTimeRangeToday] = CCInfoCache{TotalCostUSD: 1.0}
	service.cache[CCInfoTimeRangeWeek] = CCInfoCache{TotalCostUSD: 7.0}
	service.cache[CCInfoTimeRangeMonth] = CCInfoCache{TotalCostUSD: 30.0}
	service.mu.Unlock()

	todayCache := service.GetCachedCost(CCInfoTimeRangeToday)
	weekCache := service.GetCachedCost(CCInfoTimeRangeWeek)
	monthCache := service.GetCachedCost(CCInfoTimeRangeMonth)

	assert.Equal(s.T(), 1.0, todayCache.TotalCostUSD)
	assert.Equal(s.T(), 7.0, weekCache.TotalCostUSD)
	assert.Equal(s.T(), 30.0, monthCache.TotalCostUSD)
}

// Timer Lifecycle Tests

func (s *CCInfoTimerTestSuite) TestNotifyActivity_StartsTimer() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)
	defer service.Stop()

	assert.False(s.T(), service.timerRunning)

	service.NotifyActivity()

	// Timer should start
	service.timerMu.Lock()
	running := service.timerRunning
	service.timerMu.Unlock()
	assert.True(s.T(), running)
}

func (s *CCInfoTimerTestSuite) TestNotifyActivity_UpdatesLastActivity() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)
	defer service.Stop()

	before := time.Now()
	service.NotifyActivity()
	after := time.Now()

	service.mu.RLock()
	lastActivity := service.lastActivity
	service.mu.RUnlock()

	assert.True(s.T(), lastActivity.After(before) || lastActivity.Equal(before))
	assert.True(s.T(), lastActivity.Before(after) || lastActivity.Equal(after))
}

func (s *CCInfoTimerTestSuite) TestNotifyActivity_DoesNotRestartRunningTimer() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)
	defer service.Stop()

	// Start timer
	service.NotifyActivity()

	// Get the ticker reference
	service.timerMu.Lock()
	originalTicker := service.ticker
	service.timerMu.Unlock()

	// Call again
	service.NotifyActivity()

	// Ticker should be the same instance
	service.timerMu.Lock()
	currentTicker := service.ticker
	service.timerMu.Unlock()

	assert.Same(s.T(), originalTicker, currentTicker)
}

func (s *CCInfoTimerTestSuite) TestStop_GracefulShutdown() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)

	// Start timer
	service.NotifyActivity()
	time.Sleep(20 * time.Millisecond) // Let timer loop start

	// Stop should complete without hanging
	done := make(chan struct{})
	go func() {
		service.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		s.T().Fatal("Stop() did not complete in time")
	}

	service.timerMu.Lock()
	running := service.timerRunning
	service.timerMu.Unlock()
	assert.False(s.T(), running)
}

func (s *CCInfoTimerTestSuite) TestStop_Idempotent() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	// Start and stop
	service.NotifyActivity()
	service.Stop()

	// Second stop should not panic
	assert.NotPanics(s.T(), func() {
		service.Stop()
	})
}

// Inactivity Tests

func (s *CCInfoTimerTestSuite) TestCheckInactivity_ReturnsFalseWhenActive() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	service.mu.Lock()
	service.lastActivity = time.Now()
	service.mu.Unlock()

	assert.False(s.T(), service.checkInactivity())
}

func (s *CCInfoTimerTestSuite) TestCheckInactivity_ReturnsTrueWhenInactive() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	// Set last activity to beyond timeout
	service.mu.Lock()
	service.lastActivity = time.Now().Add(-CCInfoInactivityTimeout - time.Second)
	service.mu.Unlock()

	assert.True(s.T(), service.checkInactivity())
}

func (s *CCInfoTimerTestSuite) TestTimerStopsAfterInactivity() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)

	// Start timer with activity
	service.GetCachedCost(CCInfoTimeRangeToday) // Mark range as active
	service.NotifyActivity()

	// Wait for inactivity timeout plus a buffer
	time.Sleep(CCInfoInactivityTimeout + 30*time.Millisecond)

	// Timer should have stopped
	service.timerMu.Lock()
	running := service.timerRunning
	service.timerMu.Unlock()
	assert.False(s.T(), running)
}

// Fetch Tests

func (s *CCInfoTimerTestSuite) TestFetchActiveRanges_NoToken() {
	config := &model.ShellTimeConfig{
		Token: "", // No token
	}
	service := NewCCInfoTimerService(config)

	// Mark range as active
	service.mu.Lock()
	service.activeRanges[CCInfoTimeRangeToday] = true
	service.mu.Unlock()

	service.fetchActiveRanges(context.Background())

	// Cache should remain empty
	service.mu.RLock()
	cache := service.cache[CCInfoTimeRangeToday]
	service.mu.RUnlock()
	assert.Equal(s.T(), float64(0), cache.TotalCostUSD)
}

func (s *CCInfoTimerTestSuite) TestFetchActiveRanges_UpdatesCache() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)

	// Mark range as active
	service.mu.Lock()
	service.activeRanges[CCInfoTimeRangeToday] = true
	service.mu.Unlock()

	service.fetchActiveRanges(context.Background())

	// Cache should be updated
	service.mu.RLock()
	cache := service.cache[CCInfoTimeRangeToday]
	service.mu.RUnlock()
	assert.Equal(s.T(), 5.42, cache.TotalCostUSD)
	assert.False(s.T(), cache.FetchedAt.IsZero())
}

func (s *CCInfoTimerTestSuite) TestFetchActiveRanges_APIError() {
	// Create server that returns error
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errorServer.Close()

	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: errorServer.URL,
	}
	service := NewCCInfoTimerService(config)

	// Set initial cache value
	service.mu.Lock()
	service.activeRanges[CCInfoTimeRangeToday] = true
	service.cache[CCInfoTimeRangeToday] = CCInfoCache{TotalCostUSD: 1.23}
	service.mu.Unlock()

	service.fetchActiveRanges(context.Background())

	// Original cache value should be preserved
	service.mu.RLock()
	cache := service.cache[CCInfoTimeRangeToday]
	service.mu.RUnlock()
	assert.Equal(s.T(), 1.23, cache.TotalCostUSD)
}

func (s *CCInfoTimerTestSuite) TestFetchCost_TodayRange() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)

	cost, err := service.fetchCost(context.Background(), CCInfoTimeRangeToday)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 5.42, cost)
}

func (s *CCInfoTimerTestSuite) TestFetchCost_WeekRange() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)

	cost, err := service.fetchCost(context.Background(), CCInfoTimeRangeWeek)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 5.42, cost)
}

func (s *CCInfoTimerTestSuite) TestFetchCost_MonthRange() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)

	cost, err := service.fetchCost(context.Background(), CCInfoTimeRangeMonth)

	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 5.42, cost)
}

// Concurrency Tests

func (s *CCInfoTimerTestSuite) TestConcurrentGetCachedCost() {
	config := &model.ShellTimeConfig{}
	service := NewCCInfoTimerService(config)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.GetCachedCost(CCInfoTimeRangeToday)
		}()
	}

	// Should complete without race conditions
	wg.Wait()
}

func (s *CCInfoTimerTestSuite) TestConcurrentNotifyActivity() {
	config := &model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: s.server.URL,
	}
	service := NewCCInfoTimerService(config)
	defer service.Stop()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.NotifyActivity()
		}()
	}

	// Should complete without race conditions
	wg.Wait()
}

func TestCCInfoTimerTestSuite(t *testing.T) {
	suite.Run(t, new(CCInfoTimerTestSuite))
}

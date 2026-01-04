package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

const (
	CCInfoFetchInterval     = 3 * time.Second
	CCInfoInactivityTimeout = 3 * time.Minute
)

// CCInfoCache holds the cached cost data for a time range
type CCInfoCache struct {
	TotalCostUSD float64
	FetchedAt    time.Time
}

// CCInfoTimerService manages lazy-fetching of CC info data
type CCInfoTimerService struct {
	config *model.ShellTimeConfig

	mu           sync.RWMutex
	cache        map[CCInfoTimeRange]CCInfoCache
	activeRanges map[CCInfoTimeRange]bool
	lastActivity time.Time

	timerMu      sync.Mutex
	timerRunning bool
	ticker       *time.Ticker
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// NewCCInfoTimerService creates a new CC info timer service
func NewCCInfoTimerService(config *model.ShellTimeConfig) *CCInfoTimerService {
	return &CCInfoTimerService{
		config:       config,
		cache:        make(map[CCInfoTimeRange]CCInfoCache),
		activeRanges: make(map[CCInfoTimeRange]bool),
		stopChan:     make(chan struct{}),
	}
}

// GetCachedCost returns the cached cost for the given time range
// It also marks the range as active and starts the timer if not running
func (s *CCInfoTimerService) GetCachedCost(timeRange CCInfoTimeRange) CCInfoCache {
	s.mu.Lock()
	s.activeRanges[timeRange] = true
	cache := s.cache[timeRange]
	s.mu.Unlock()

	return cache
}

// NotifyActivity signals that a client has requested data
// This starts the timer if not running, or resets the inactivity timeout
func (s *CCInfoTimerService) NotifyActivity() {
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()

	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	if !s.timerRunning {
		s.startTimer()
	}
}

// Stop gracefully stops the timer service
func (s *CCInfoTimerService) Stop() {
	s.timerMu.Lock()
	if s.timerRunning {
		s.ticker.Stop()
		s.timerRunning = false
	}
	s.timerMu.Unlock()

	select {
	case <-s.stopChan:
		// Already closed
	default:
		close(s.stopChan)
	}

	s.wg.Wait()
	slog.Info("CC info timer service stopped")
}

// startTimer starts the timer loop (must be called with timerMu held)
func (s *CCInfoTimerService) startTimer() {
	if s.timerRunning {
		return
	}

	s.timerRunning = true
	s.ticker = time.NewTicker(CCInfoFetchInterval)
	s.wg.Add(1)

	go s.timerLoop()

	slog.Info("CC info timer started")
}

// stopTimer stops the timer (must be called with timerMu held)
func (s *CCInfoTimerService) stopTimer() {
	if !s.timerRunning {
		return
	}

	s.ticker.Stop()
	s.timerRunning = false

	// Clear active ranges when stopping
	s.mu.Lock()
	s.activeRanges = make(map[CCInfoTimeRange]bool)
	s.mu.Unlock()

	slog.Info("CC info timer stopped due to inactivity")
}

// timerLoop runs the timer loop
func (s *CCInfoTimerService) timerLoop() {
	defer s.wg.Done()

	// Fetch immediately on start
	s.fetchActiveRanges(context.Background())

	for {
		select {
		case <-s.ticker.C:
			// Check inactivity before fetching
			if s.checkInactivity() {
				s.timerMu.Lock()
				s.stopTimer()
				s.timerMu.Unlock()
				return
			}
			s.fetchActiveRanges(context.Background())

		case <-s.stopChan:
			return
		}
	}
}

// checkInactivity returns true if the service has been inactive for too long
func (s *CCInfoTimerService) checkInactivity() bool {
	s.mu.RLock()
	lastActivity := s.lastActivity
	s.mu.RUnlock()

	return time.Since(lastActivity) > CCInfoInactivityTimeout
}

// fetchActiveRanges fetches data for all active time ranges
func (s *CCInfoTimerService) fetchActiveRanges(ctx context.Context) {
	if s.config.Token == "" {
		return
	}

	// Get active ranges
	s.mu.RLock()
	ranges := make([]CCInfoTimeRange, 0, len(s.activeRanges))
	for r := range s.activeRanges {
		ranges = append(ranges, r)
	}
	s.mu.RUnlock()

	// Fetch each active range
	for _, timeRange := range ranges {
		cost, err := s.fetchCost(ctx, timeRange)
		if err != nil {
			slog.Warn("Failed to fetch CC info cost",
				slog.String("timeRange", string(timeRange)),
				slog.Any("err", err))
			continue
		}

		s.mu.Lock()
		s.cache[timeRange] = CCInfoCache{
			TotalCostUSD: cost,
			FetchedAt:    time.Now(),
		}
		s.mu.Unlock()

		slog.Debug("CC info cost updated",
			slog.String("timeRange", string(timeRange)),
			slog.Float64("cost", cost))
	}
}

// fetchCost fetches the cost for a specific time range
func (s *CCInfoTimerService) fetchCost(ctx context.Context, timeRange CCInfoTimeRange) (float64, error) {
	now := time.Now()
	var since time.Time

	switch timeRange {
	case CCInfoTimeRangeToday:
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case CCInfoTimeRangeWeek:
		// Start of current week (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday is 7
		}
		since = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
	case CCInfoTimeRangeMonth:
		since = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	default:
		// Default to today
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}

	variables := map[string]interface{}{
		"filter": map[string]interface{}{
			"since":      since.Format(time.RFC3339),
			"until":      now.Format(time.RFC3339),
			"clientType": "claude_code",
		},
	}

	var result model.GraphQLResponse[model.CCStatuslineDailyCostResponse]

	err := model.SendGraphQLRequest(model.GraphQLRequestOptions[model.GraphQLResponse[model.CCStatuslineDailyCostResponse]]{
		Context: ctx,
		Endpoint: model.Endpoint{
			Token:       s.config.Token,
			APIEndpoint: s.config.APIEndpoint,
		},
		Query:     model.CCStatuslineDailyCostQuery,
		Variables: variables,
		Response:  &result,
		Timeout:   5 * time.Second,
	})

	if err != nil {
		return 0, err
	}

	return result.Data.FetchUser.AICodeOtel.Analytics.TotalCostUsd, nil
}

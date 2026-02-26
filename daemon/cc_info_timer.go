package daemon

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

var (
	CCInfoFetchInterval     = 3 * time.Second
	CCInfoInactivityTimeout = 3 * time.Minute
)

// CCInfoCache holds the cached cost data for a time range
type CCInfoCache struct {
	TotalCostUSD        float64
	TotalSessionSeconds int
	FetchedAt           time.Time
}

// GitCacheEntry holds cached git info for a single working directory
type GitCacheEntry struct {
	Info         GitInfo
	LastAccessed time.Time
	LastFetched  time.Time
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

	// Guards concurrent fetchRateLimit goroutines
	rateLimitFetchMu sync.Mutex

	// Git info cache (per working directory)
	gitCache map[string]*GitCacheEntry

	// Anthropic rate limit cache
	rateLimitCache *anthropicRateLimitCache

	// User profile cache (permanent for daemon lifetime)
	userLogin        string
	userLoginFetched bool
}

// NewCCInfoTimerService creates a new CC info timer service
func NewCCInfoTimerService(config *model.ShellTimeConfig) *CCInfoTimerService {
	return &CCInfoTimerService{
		config:         config,
		cache:          make(map[CCInfoTimeRange]CCInfoCache),
		activeRanges:   make(map[CCInfoTimeRange]bool),
		gitCache:       make(map[string]*GitCacheEntry),
		rateLimitCache: &anthropicRateLimitCache{},
		stopChan:       make(chan struct{}),
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

	// Clear active ranges, git cache, and rate limit cache when stopping
	s.mu.Lock()
	s.activeRanges = make(map[CCInfoTimeRange]bool)
	s.gitCache = make(map[string]*GitCacheEntry)
	s.mu.Unlock()

	s.rateLimitCache.mu.Lock()
	s.rateLimitCache.usage = nil
	s.rateLimitCache.fetchedAt = time.Time{}
	s.rateLimitCache.lastAttemptAt = time.Time{}
	s.rateLimitCache.mu.Unlock()

	slog.Info("CC info timer stopped due to inactivity")
}

// timerLoop runs the timer loop
func (s *CCInfoTimerService) timerLoop() {
	defer s.wg.Done()

	// Fetch immediately on start
	s.fetchActiveRanges(context.Background())
	s.fetchGitInfo()
	go func() {
		if !s.rateLimitFetchMu.TryLock() {
			return
		}
		defer s.rateLimitFetchMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.fetchRateLimit(ctx)
	}()
	go s.fetchUserProfile(context.Background())

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
			s.fetchGitInfo()
			go func() {
				if !s.rateLimitFetchMu.TryLock() {
					return
				}
				defer s.rateLimitFetchMu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				s.fetchRateLimit(ctx)
			}()

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
		info, err := s.fetchCCInfo(ctx, timeRange)
		if err != nil {
			slog.Warn("Failed to fetch CC info",
				slog.String("timeRange", string(timeRange)),
				slog.Any("err", err))
			continue
		}

		s.mu.Lock()
		s.cache[timeRange] = CCInfoCache{
			TotalCostUSD:        info.TotalCostUSD,
			TotalSessionSeconds: info.TotalSessionSeconds,
			FetchedAt:           time.Now(),
		}
		s.mu.Unlock()

		slog.Debug("CC info updated",
			slog.String("timeRange", string(timeRange)),
			slog.Float64("cost", info.TotalCostUSD),
			slog.Int("sessionSeconds", info.TotalSessionSeconds))
	}
}

// ccInfoFetchResult holds the fetched CC info data
type ccInfoFetchResult struct {
	TotalCostUSD        float64
	TotalSessionSeconds int
}

// fetchCCInfo fetches the CC info for a specific time range
func (s *CCInfoTimerService) fetchCCInfo(ctx context.Context, timeRange CCInfoTimeRange) (ccInfoFetchResult, error) {
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

	// Convert to UTC before sending to server to avoid timezone parsing issues
	sinceUTC := since.UTC()
	nowUTC := now.UTC()

	variables := map[string]interface{}{
		"filter": map[string]interface{}{
			"since":      sinceUTC.Format(time.RFC3339),
			"until":      nowUTC.Format(time.RFC3339),
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
		return ccInfoFetchResult{}, err
	}

	analytics := result.Data.FetchUser.AICodeOtel.Analytics
	return ccInfoFetchResult{
		TotalCostUSD:        analytics.TotalCostUsd,
		TotalSessionSeconds: analytics.TotalSessionSeconds,
	}, nil
}

// GetCachedGitInfo returns cached git info for the given working directory.
// It marks the directory as active for background refresh.
// Git info is fetched by the background timer, so first call may return empty.
func (s *CCInfoTimerService) GetCachedGitInfo(workingDir string) GitInfo {
	if workingDir == "" {
		return GitInfo{}
	}

	// Normalize path for consistent cache keys
	normalizedDir := filepath.Clean(workingDir)

	s.mu.Lock()
	entry, exists := s.gitCache[normalizedDir]
	if !exists {
		// Create new entry for this directory
		entry = &GitCacheEntry{
			LastAccessed: time.Now(),
		}
		s.gitCache[normalizedDir] = entry
	} else {
		// Update last accessed time
		entry.LastAccessed = time.Now()
	}
	info := entry.Info
	s.mu.Unlock()

	return info
}

// fetchGitInfo fetches git info for all recently accessed working directories
func (s *CCInfoTimerService) fetchGitInfo() {
	// Collect directories that need refresh (under read lock)
	s.mu.RLock()
	dirsToFetch := make([]string, 0, len(s.gitCache))
	for dir, entry := range s.gitCache {
		// Only fetch for recently accessed entries
		if time.Since(entry.LastAccessed) <= CCInfoInactivityTimeout {
			dirsToFetch = append(dirsToFetch, dir)
		}
	}
	s.mu.RUnlock()

	// Fetch git info for each directory (outside lock to avoid blocking)
	for _, dir := range dirsToFetch {
		info := GetGitInfo(dir)

		s.mu.Lock()
		if entry, exists := s.gitCache[dir]; exists {
			entry.Info = info
			entry.LastFetched = time.Now()
		}
		s.mu.Unlock()

		slog.Debug("Git info updated",
			slog.String("workingDir", dir),
			slog.String("branch", info.Branch),
			slog.Bool("dirty", info.Dirty))
	}

	// Cleanup stale entries
	s.cleanupStaleGitCache()
}

// cleanupStaleGitCache removes entries not accessed within the inactivity timeout
func (s *CCInfoTimerService) cleanupStaleGitCache() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for dir, entry := range s.gitCache {
		if time.Since(entry.LastAccessed) > CCInfoInactivityTimeout {
			delete(s.gitCache, dir)
			slog.Debug("Git cache entry evicted",
				slog.String("workingDir", dir))
		}
	}
}

// fetchRateLimit fetches Anthropic rate limit data if cache is stale.
// Only runs on macOS where Keychain access is available.
func (s *CCInfoTimerService) fetchRateLimit(ctx context.Context) {
	if runtime.GOOS != "darwin" {
		return
	}

	// Check cache TTL under read lock - skip if data is fresh or we attempted recently
	s.rateLimitCache.mu.RLock()
	sinceLastFetch := time.Since(s.rateLimitCache.fetchedAt)
	sinceLastAttempt := time.Since(s.rateLimitCache.lastAttemptAt)
	s.rateLimitCache.mu.RUnlock()

	if sinceLastFetch < anthropicUsageCacheTTL || sinceLastAttempt < anthropicUsageCacheTTL {
		return
	}

	// Record attempt time before fetching to avoid retrying on every tick
	s.rateLimitCache.mu.Lock()
	s.rateLimitCache.lastAttemptAt = time.Now()
	s.rateLimitCache.mu.Unlock()

	// Read token fresh from Keychain (not cached)
	token, err := fetchClaudeCodeOAuthToken()
	if err != nil || token == "" {
		slog.Debug("Failed to get Claude Code OAuth token", slog.Any("err", err))
		return
	}

	usage, err := fetchAnthropicUsage(ctx, token)
	if err != nil {
		slog.Warn("Failed to fetch Anthropic usage", slog.Any("err", err))
		return
	}

	s.rateLimitCache.mu.Lock()
	s.rateLimitCache.usage = usage
	s.rateLimitCache.fetchedAt = time.Now()
	s.rateLimitCache.mu.Unlock()

	// Send usage data to server for push notification scheduling (fire-and-forget)
	// Use a separate context so the goroutine isn't canceled when the caller returns.
	go func() {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer bgCancel()
		s.sendAnthropicUsageToServer(bgCtx, usage)
	}()

	slog.Debug("Anthropic rate limit updated",
		slog.Float64("5h", usage.FiveHourUtilization),
		slog.Float64("7d", usage.SevenDayUtilization))
}

// GetCachedUserLogin returns the cached user login, or empty string if not yet fetched.
func (s *CCInfoTimerService) GetCachedUserLogin() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userLogin
}

// fetchUserProfile fetches the current user's login once per daemon lifetime.
func (s *CCInfoTimerService) fetchUserProfile(ctx context.Context) {
	if s.config.Token == "" {
		return
	}

	s.mu.RLock()
	fetched := s.userLoginFetched
	s.mu.RUnlock()

	if fetched {
		return
	}

	profile, err := model.FetchCurrentUserProfile(ctx, *s.config)
	if err != nil {
		slog.Warn("Failed to fetch user profile", slog.Any("err", err))
		return
	}

	s.mu.Lock()
	s.userLogin = profile.FetchUser.Login
	s.userLoginFetched = true
	s.mu.Unlock()

	slog.Debug("User profile fetched", slog.String("login", profile.FetchUser.Login))
}

// sendAnthropicUsageToServer sends the Anthropic usage data to the ShellTime server
// for scheduling push notifications when rate limits reset.
func (s *CCInfoTimerService) sendAnthropicUsageToServer(ctx context.Context, usage *AnthropicRateLimitData) {
	if s.config.Token == "" {
		return
	}

	type usageBucket struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	}
	type usagePayload struct {
		FiveHour usageBucket `json:"five_hour"`
		SevenDay usageBucket `json:"seven_day"`
	}

	payload := usagePayload{
		FiveHour: usageBucket{
			Utilization: usage.FiveHourUtilization,
			ResetsAt:    usage.FiveHourResetsAt,
		},
		SevenDay: usageBucket{
			Utilization: usage.SevenDayUtilization,
			ResetsAt:    usage.SevenDayResetsAt,
		},
	}

	err := model.SendHTTPRequestJSON(model.HTTPRequestOptions[usagePayload, any]{
		Context: ctx,
		Endpoint: model.Endpoint{
			Token:       s.config.Token,
			APIEndpoint: s.config.APIEndpoint,
		},
		Method:  "POST",
		Path:    "/api/v1/anthropic-usage",
		Payload: payload,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		slog.Warn("Failed to send anthropic usage to server", slog.Any("err", err))
	}
}

// GetCachedRateLimit returns a copy of the cached rate limit data, or nil if not available.
func (s *CCInfoTimerService) GetCachedRateLimit() *AnthropicRateLimitData {
	s.rateLimitCache.mu.RLock()
	defer s.rateLimitCache.mu.RUnlock()

	if s.rateLimitCache.usage == nil {
		return nil
	}

	// Return a copy
	copy := *s.rateLimitCache.usage
	return &copy
}

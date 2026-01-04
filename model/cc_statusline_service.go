package model

import (
	"context"
	"time"
)

// FetchDailyStatsCached returns today's stats from cache or fetches from API
// This function is non-blocking - if cache is invalid, it returns cached/zero value
// and triggers a background fetch
func FetchDailyStatsCached(ctx context.Context, config ShellTimeConfig) CCStatuslineDailyStats {
	// Try cache first
	if stats, valid := CCStatuslineCacheGet(); valid {
		return stats
	}

	// Try to start background fetch
	go fetchDailyStatsAsync(context.Background(), config)

	// Return last known value or zero
	return CCStatuslineCacheGetLastValue()
}

// fetchDailyStatsAsync fetches daily stats from API in the background
func fetchDailyStatsAsync(ctx context.Context, config ShellTimeConfig) {
	// Check if already fetching
	if !CCStatuslineCacheStartFetch() {
		return
	}

	// Ensure we mark fetch as complete on any exit
	defer CCStatuslineCacheEndFetch()

	// Check if we have a token
	if config.Token == "" {
		return
	}

	// Fetch from API
	stats, err := FetchDailyStats(ctx, config)
	if err != nil {
		return
	}

	// Update cache
	CCStatuslineCacheSet(stats)
}

// FetchDailyStats fetches today's stats from the GraphQL API
func FetchDailyStats(ctx context.Context, config ShellTimeConfig) (CCStatuslineDailyStats, error) {
	ctx, span := modelTracer.Start(ctx, "statusline.fetchDailyStats")
	defer span.End()

	// Prepare time filter for today
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Convert to UTC before sending to server to avoid timezone parsing issues
	startOfDayUTC := startOfDay.UTC()
	nowUTC := now.UTC()

	variables := map[string]interface{}{
		"filter": map[string]interface{}{
			"since":      startOfDayUTC.Format(time.RFC3339),
			"until":      nowUTC.Format(time.RFC3339),
			"clientType": "claude_code",
		},
	}

	var result GraphQLResponse[CCStatuslineDailyCostResponse]

	err := SendGraphQLRequest(GraphQLRequestOptions[GraphQLResponse[CCStatuslineDailyCostResponse]]{
		Context: ctx,
		Endpoint: Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		Query:     CCStatuslineDailyCostQuery,
		Variables: variables,
		Response:  &result,
		Timeout:   5 * time.Second,
	})

	if err != nil {
		return CCStatuslineDailyStats{}, err
	}

	analytics := result.Data.FetchUser.AICodeOtel.Analytics
	return CCStatuslineDailyStats{
		Cost:           analytics.TotalCostUsd,
		SessionSeconds: analytics.TotalSessionSeconds,
	}, nil
}

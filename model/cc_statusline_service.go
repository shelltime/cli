package model

import (
	"context"
	"time"
)

// FetchDailyCostCached returns today's cost from cache or fetches from API
// This function is non-blocking - if cache is invalid, it returns cached/zero value
// and triggers a background fetch
func FetchDailyCostCached(ctx context.Context, config ShellTimeConfig) float64 {
	// Try cache first
	if cost, valid := CCStatuslineCacheGet(); valid {
		return cost
	}

	// Try to start background fetch
	go fetchDailyCostAsync(context.Background(), config)

	// Return last known value or 0
	return CCStatuslineCacheGetLastValue()
}

// fetchDailyCostAsync fetches daily cost from API in the background
func fetchDailyCostAsync(ctx context.Context, config ShellTimeConfig) {
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
	cost, err := FetchDailyCost(ctx, config)
	if err != nil {
		return
	}

	// Update cache
	CCStatuslineCacheSet(cost)
}

// FetchDailyCost fetches today's cost from the GraphQL API
func FetchDailyCost(ctx context.Context, config ShellTimeConfig) (float64, error) {
	ctx, span := modelTracer.Start(ctx, "statusline.fetchDailyCost")
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
		return 0, err
	}

	return result.Data.FetchUser.AICodeOtel.Analytics.TotalCostUsd, nil
}

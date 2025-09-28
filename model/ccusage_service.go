package model

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"time"
)

// CCUsageData represents the usage data collected from ccusage command
type CCUsageData struct {
	Timestamp string                    `json:"timestamp" msgpack:"timestamp"`
	Hostname  string                    `json:"hostname" msgpack:"hostname"`
	Username  string                    `json:"username" msgpack:"username"`
	OS        string                    `json:"os" msgpack:"os"`
	OSVersion string                    `json:"osVersion" msgpack:"osVersion"`
	Data      CCUsageProjectDailyOutput `json:"data" msgpack:"data"`
}

// CCUsageService defines the interface for CC usage collection
type CCUsageService interface {
	Start(ctx context.Context) error
	Stop()
	CollectCCUsage(ctx context.Context) error
}

// ccUsageService implements the CCUsageService interface
type ccUsageService struct {
	config   ShellTimeConfig
	ticker   *time.Ticker
	stopChan chan struct{}
}

// NewCCUsageService creates a new CCUsage service
func NewCCUsageService(config ShellTimeConfig) CCUsageService {
	return &ccUsageService{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start begins the periodic usage collection
func (s *ccUsageService) Start(ctx context.Context) error {
	// Check if CCUsage is enabled
	if s.config.CCUsage == nil || s.config.CCUsage.Enabled == nil || !*s.config.CCUsage.Enabled {
		slog.Info("CCUsage collection is disabled")
		return nil
	}

	slog.Info("Starting CCUsage collection service")

	// Create a ticker for hourly collection
	s.ticker = time.NewTicker(1 * time.Hour)

	// Run initial collection
	if err := s.CollectCCUsage(ctx); err != nil {
		slog.Warn("Initial CCUsage collection failed", "error", err)
	}

	// Start the collection loop
	go func() {
		for {
			select {
			case <-s.ticker.C:
				if err := s.CollectCCUsage(ctx); err != nil {
					slog.Warn("CCUsage collection failed", "error", err)
				}
			case <-s.stopChan:
				slog.Info("Stopping CCUsage collection service")
				return
			case <-ctx.Done():
				slog.Info("Context cancelled, stopping CCUsage collection service")
				return
			}
		}
	}()

	return nil
}

// Stop halts the usage collection
func (s *ccUsageService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
}

// CollectCCUsage collects and sends usage data to the server
func (s *ccUsageService) CollectCCUsage(ctx context.Context) error {
	ctx, span := modelTracer.Start(ctx, "ccusage.collect")
	defer span.End()

	slog.Debug("Collecting CCUsage data")

	since := time.Time{}

	// Get the last sync timestamp from server if we have credentials
	if s.config.Token != "" && s.config.APIEndpoint != "" {
		endpoint := Endpoint{
			Token:       s.config.Token,
			APIEndpoint: s.config.APIEndpoint,
		}

		// Try to get last sync timestamp, but don't fail if it doesn't work
		lastSync, err := s.getLastSyncTimestamp(ctx, endpoint)
		if err != nil {
			slog.Warn("Failed to get last sync timestamp", "error", err)
		}
		since = lastSync
		slog.Debug("Got last sync timestamp", "since", since)
	}

	// Collect data from ccusage command
	data, err := s.collectData(ctx, since)
	if err != nil {
		return fmt.Errorf("failed to collect ccusage data: %w", err)
	}

	// Send to server
	if s.config.Token != "" && s.config.APIEndpoint != "" {
		endpoint := Endpoint{
			Token:       s.config.Token,
			APIEndpoint: s.config.APIEndpoint,
		}

		err = s.sendData(ctx, endpoint, data)
		if err != nil {
			return fmt.Errorf("failed to send usage data: %w", err)
		}
	}

	slog.Debug("CCUsage data collection completed")
	return nil
}

// getLastSyncTimestamp fetches the last CCUsage sync timestamp from the server via GraphQL
func (s *ccUsageService) getLastSyncTimestamp(ctx context.Context, endpoint Endpoint) (time.Time, error) {
	// Get current hostname
	hostname, err := os.Hostname()
	if err != nil {
		slog.Warn("Failed to get hostname", "error", err)
		hostname = "unknown"
	}

	query := `query fetchUserCCUsageLastSync($hostname: String!) {
		fetchUser {
			id
			ccusage(filter: { hostname: $hostname }) {
				lastSyncAt
			}
		}
	}`

	type fetchUserResponse struct {
		FetchUser struct {
			ID      int `json:"id"`
			CCUsage struct {
				LastSyncAt string `json:"lastSyncAt"`
			} `json:"ccusage"`
		} `json:"fetchUser"`
	}

	var result GraphQLResponse[fetchUserResponse]

	variables := map[string]interface{}{
		"hostname": hostname,
	}

	slog.Debug("Fetching CCUsage last sync", "hostname", hostname)

	err = SendGraphQLRequest(GraphQLRequestOptions[GraphQLResponse[fetchUserResponse]]{
		Context:   ctx,
		Endpoint:  endpoint,
		Query:     query,
		Variables: variables,
		Response:  &result,
		Timeout:   time.Second * 10,
	})

	if err != nil {
		slog.Warn("Failed to fetch CCUsage last sync", "error", err)
		return time.Time{}, nil // Return nil to skip the since parameter
	}

	lastSyncAtStr := result.Data.FetchUser.CCUsage.LastSyncAt

	if lastSyncAtStr == "" {
		return time.Time{}, nil
	}
	lastSyncAt, err := time.Parse(time.RFC3339, lastSyncAtStr)
	if err != nil {
		slog.Warn("Failed to parse last sync timestamp", "error", err)
		return time.Time{}, err // Return nil to skip the since parameter
	}

	year2023 := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	if lastSyncAt.Before(year2023) {
		return time.Time{}, nil
	}

	return lastSyncAt, nil
}

// collectData collects usage data using bunx or npx ccusage command
func (s *ccUsageService) collectData(ctx context.Context, since time.Time) (*CCUsageData, error) {
	// Check if bunx exists
	bunxPath, bunxErr := exec.LookPath("bunx")
	npxPath, npxErr := exec.LookPath("npx")

	if bunxErr != nil && npxErr != nil {
		return nil, fmt.Errorf("neither bunx nor npx found in system PATH")
	}

	// Build command arguments
	args := []string{"ccusage", "daily", "--instances", "--json"}

	// Add since parameter if provided
	if !since.IsZero() {
		// Convert Unix timestamp (seconds) to ISO 8601 date string
		sinceDate := since.Format("20060102")
		args = append(args, "--since", sinceDate)
		slog.Debug("Using since parameter", "sinceDate", sinceDate, "since", since)
	}

	var cmd *exec.Cmd
	if bunxErr == nil {
		// Use bunx if available
		cmd = exec.CommandContext(ctx, bunxPath, args...)
		slog.Debug("Using bunx to collect ccusage data")
	} else {
		// Fall back to npx
		cmd = exec.CommandContext(ctx, npxPath, args...)
		slog.Debug("Using npx to collect ccusage data")
	}

	// Execute the command
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("ccusage command failed: %v, stderr: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to execute ccusage command: %w", err)
	}

	// Parse JSON output
	var ccusageOutput CCUsageProjectDailyOutput
	if err := json.Unmarshal(output, &ccusageOutput); err != nil {
		return nil, fmt.Errorf("failed to parse ccusage output: %w", err)
	}

	// Get system information for metadata
	hostname, err := os.Hostname()
	if err != nil {
		slog.Warn("Failed to get hostname", "error", err)
		hostname = "unknown"
	}

	username := os.Getenv("USER")
	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			slog.Warn("Failed to get username", "error", err)
			username = "unknown"
		} else {
			username = currentUser.Username
		}
	}

	sysInfo, err := GetOSAndVersion()
	if err != nil {
		slog.Warn("Failed to get OS info", "error", err)
		sysInfo = &SysInfo{
			Os:      "unknown",
			Version: "unknown",
		}
	}

	data := &CCUsageData{
		Timestamp: time.Now().Format(time.RFC3339),
		Hostname:  hostname,
		Username:  username,
		OS:        sysInfo.Os,
		OSVersion: sysInfo.Version,
		Data:      ccusageOutput,
	}

	return data, nil
}

// sendData sends the collected usage data to the server
func (s *ccUsageService) sendData(ctx context.Context, endpoint Endpoint, data *CCUsageData) error {
	// CCUsage batch request types matching server handler
	type ccUsageModelBreakdown struct {
		ModelName           string  `json:"modelName"`
		InputTokens         int     `json:"inputTokens"`
		OutputTokens        int     `json:"outputTokens"`
		CacheCreationTokens int     `json:"cacheCreationTokens"`
		CacheReadTokens     int     `json:"cacheReadTokens"`
		Cost                float64 `json:"cost"`
	}

	type ccUsageDailyData struct {
		InputTokens         int                     `json:"inputTokens"`
		OutputTokens        int                     `json:"outputTokens"`
		CacheCreationTokens int                     `json:"cacheCreationTokens"`
		CacheReadTokens     int                     `json:"cacheReadTokens"`
		TotalTokens         int                     `json:"totalTokens"`
		TotalCost           float64                 `json:"totalCost"`
		ModelsUsed          []string                `json:"modelsUsed"`
		ModelBreakdowns     []ccUsageModelBreakdown `json:"modelBreakdowns"`
	}

	type ccUsageEntry struct {
		Project string           `json:"project"`
		Date    string           `json:"date"` // YYYYMMDD format
		Usage   ccUsageDailyData `json:"usage"`
	}

	type ccUsageBatchPayload struct {
		Host    string         `json:"host"`
		Entries []ccUsageEntry `json:"entries"`
	}

	type ccUsageResponse struct {
		Success        bool     `json:"success"`
		SuccessCount   int      `json:"successCount"`
		TotalCount     int      `json:"totalCount"`
		FailedProjects []string `json:"failedProjects,omitempty"`
	}

	// Transform CCUsageData to batch format
	var entries []ccUsageEntry

	// Iterate through all projects in the collected data
	for projectName, projectDays := range data.Data.Projects {
		for _, dayData := range projectDays {
			// Convert model breakdowns
			modelBreakdowns := make([]ccUsageModelBreakdown, len(dayData.ModelBreakdowns))
			for i, mb := range dayData.ModelBreakdowns {
				modelBreakdowns[i] = ccUsageModelBreakdown{
					ModelName:           mb.ModelName,
					InputTokens:         mb.InputTokens,
					OutputTokens:        mb.OutputTokens,
					CacheCreationTokens: mb.CacheCreationTokens,
					CacheReadTokens:     mb.CacheReadTokens,
					Cost:                mb.Cost,
				}
			}

			entry := ccUsageEntry{
				Project: projectName,
				Date:    dayData.Date, // Already in YYYYMMDD format from ccusage
				Usage: ccUsageDailyData{
					InputTokens:         dayData.InputTokens,
					OutputTokens:        dayData.OutputTokens,
					CacheCreationTokens: dayData.CacheCreationTokens,
					CacheReadTokens:     dayData.CacheReadTokens,
					TotalTokens:         dayData.TotalTokens,
					TotalCost:           dayData.TotalCost,
					ModelsUsed:          dayData.ModelsUsed,
					ModelBreakdowns:     modelBreakdowns,
				},
			}
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 {
		slog.Debug("No CCUsage entries to send")
		return nil
	}

	payload := ccUsageBatchPayload{
		Host:    data.Hostname,
		Entries: entries,
	}

	var resp ccUsageResponse

	err := SendHTTPRequestJSON(HTTPRequestOptions[ccUsageBatchPayload, ccUsageResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/ccusage/batch",
		Payload:  payload,
		Response: &resp,
	})

	if err != nil {
		return fmt.Errorf("failed to send CCUsage data: %w", err)
	}

	if !resp.Success {
		if len(resp.FailedProjects) > 0 {
			return fmt.Errorf("server rejected CCUsage data for projects: %v", resp.FailedProjects)
		}
		return fmt.Errorf("server rejected CCUsage data: %d/%d entries failed", resp.TotalCount-resp.SuccessCount, resp.TotalCount)
	}

	slog.Debug("CCUsage data sent successfully", "successCount", resp.SuccessCount, "totalCount", resp.TotalCount)
	return nil
}

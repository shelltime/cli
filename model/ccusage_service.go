package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"time"

	"github.com/sirupsen/logrus"
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
		logrus.Info("CCUsage collection is disabled")
		return nil
	}

	logrus.Info("Starting CCUsage collection service")

	// Create a ticker for hourly collection
	s.ticker = time.NewTicker(1 * time.Hour)

	// Run initial collection
	if err := s.CollectCCUsage(ctx); err != nil {
		logrus.Warnf("Initial CCUsage collection failed: %v", err)
	}

	// Start the collection loop
	go func() {
		for {
			select {
			case <-s.ticker.C:
				if err := s.CollectCCUsage(ctx); err != nil {
					logrus.Warnf("CCUsage collection failed: %v", err)
				}
			case <-s.stopChan:
				logrus.Info("Stopping CCUsage collection service")
				return
			case <-ctx.Done():
				logrus.Info("Context cancelled, stopping CCUsage collection service")
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

	logrus.Debug("Collecting CCUsage data")

	// Collect data from ccusage command
	data, err := s.collectData(ctx)
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

	logrus.Debug("CCUsage data collection completed")
	return nil
}

// collectData collects usage data using bunx or npx ccusage command
func (s *ccUsageService) collectData(ctx context.Context) (*CCUsageData, error) {
	// Check if bunx exists
	bunxPath, bunxErr := exec.LookPath("bunx")
	npxPath, npxErr := exec.LookPath("npx")

	if bunxErr != nil && npxErr != nil {
		return nil, fmt.Errorf("neither bunx nor npx found in system PATH")
	}

	var cmd *exec.Cmd
	if bunxErr == nil {
		// Use bunx if available
		cmd = exec.CommandContext(ctx, bunxPath, "ccusage", "daily", "--instances", "--json")
		logrus.Debug("Using bunx to collect ccusage data")
	} else {
		// Fall back to npx
		cmd = exec.CommandContext(ctx, npxPath, "ccusage", "daily", "--instances", "--json")
		logrus.Debug("Using npx to collect ccusage data")
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
		logrus.Warnf("Failed to get hostname: %v", err)
		hostname = "unknown"
	}

	username := os.Getenv("USER")
	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			logrus.Warnf("Failed to get username: %v", err)
			username = "unknown"
		} else {
			username = currentUser.Username
		}
	}

	sysInfo, err := GetOSAndVersion()
	if err != nil {
		logrus.Warnf("Failed to get OS info: %v", err)
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
	type ccUsageRequest struct {
		Data *CCUsageData `json:"data" msgpack:"data"`
	}

	type ccUsageResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}

	payload := ccUsageRequest{
		Data: data,
	}

	var resp ccUsageResponse

	err := SendHTTPRequest(HTTPRequestOptions[ccUsageRequest, ccUsageResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/ccusage",
		Payload:  payload,
		Response: &resp,
	})

	if err != nil {
		return fmt.Errorf("failed to send CCUsage data: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("server rejected CCUsage data: %s", resp.Message)
	}

	logrus.Debugf("CCUsage data sent successfully")
	return nil
}

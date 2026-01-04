package commands

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CCStatuslineTestSuite struct {
	suite.Suite
	mockConfig *model.MockConfigService
	origConfig model.ConfigService
	socketPath string
	listener   net.Listener
}

func (s *CCStatuslineTestSuite) SetupTest() {
	s.origConfig = configService
	s.mockConfig = model.NewMockConfigService(s.T())
	configService = s.mockConfig

	s.socketPath = filepath.Join(os.TempDir(), "test-statusline.sock")
	os.Remove(s.socketPath)
}

func (s *CCStatuslineTestSuite) TearDownTest() {
	configService = s.origConfig
	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
}

// getDailyCostWithDaemonFallback Tests

func (s *CCStatuslineTestSuite) TestGetDailyStats_UsesDaemonWhenAvailable() {
	// Start mock daemon socket
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	expectedCost := 15.67
	expectedSessionSeconds := 3600
	go func() {
		conn, _ := listener.Accept()
		defer conn.Close()

		var msg daemon.SocketMessage
		json.NewDecoder(conn).Decode(&msg)

		response := daemon.CCInfoResponse{
			TotalCostUSD:        expectedCost,
			TotalSessionSeconds: expectedSessionSeconds,
			TimeRange:           "today",
			CachedAt:            time.Now(),
		}
		json.NewEncoder(conn).Encode(response)
	}()

	time.Sleep(10 * time.Millisecond)

	config := model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	stats := getDailyStatsWithDaemonFallback(context.Background(), config)

	assert.Equal(s.T(), expectedCost, stats.Cost)
	assert.Equal(s.T(), expectedSessionSeconds, stats.SessionSeconds)
}

func (s *CCStatuslineTestSuite) TestGetDailyStats_FallbackWhenDaemonUnavailable() {
	// No socket exists, should fall back to cached API
	config := model.ShellTimeConfig{
		SocketPath: "/nonexistent/socket.sock",
		Token:      "", // No token means FetchDailyStatsCached returns zero values
	}

	stats := getDailyStatsWithDaemonFallback(context.Background(), config)

	// Should return zero values (from cache fallback with no token)
	assert.Equal(s.T(), float64(0), stats.Cost)
	assert.Equal(s.T(), 0, stats.SessionSeconds)
}

func (s *CCStatuslineTestSuite) TestGetDailyStats_FallbackOnDaemonError() {
	// Start mock daemon that returns error
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	go func() {
		conn, _ := listener.Accept()
		// Close immediately to cause error
		conn.Close()
	}()

	time.Sleep(10 * time.Millisecond)

	config := model.ShellTimeConfig{
		SocketPath: s.socketPath,
		Token:      "", // No token
	}

	stats := getDailyStatsWithDaemonFallback(context.Background(), config)

	// Should fall back and return zero values
	assert.Equal(s.T(), float64(0), stats.Cost)
	assert.Equal(s.T(), 0, stats.SessionSeconds)
}

func (s *CCStatuslineTestSuite) TestGetDailyStats_UsesDefaultSocketPath() {
	// Test that default socket path is used when config is empty
	config := model.ShellTimeConfig{
		SocketPath: "", // Empty path - should use model.DefaultSocketPath
		Token:      "",
	}

	// This should use model.DefaultSocketPath internally
	// Since no daemon is running at the default path, it will fall back to cached API
	// The function should not panic and should return a valid stats struct
	stats := getDailyStatsWithDaemonFallback(context.Background(), config)

	// We can't assert on exact values since the global cache might have data
	// from previous tests. Just verify the function returns without error
	// and returns non-negative values
	assert.GreaterOrEqual(s.T(), stats.Cost, float64(0))
	assert.GreaterOrEqual(s.T(), stats.SessionSeconds, 0)
}

// formatStatuslineOutput Tests

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_AllValues() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0)

	// Should contain all components
	assert.Contains(s.T(), output, "🤖 claude-opus-4")
	assert.Contains(s.T(), output, "$1.23")
	assert.Contains(s.T(), output, "$4.56")
	assert.Contains(s.T(), output, "1h1m")    // Session time (3661 seconds = 1h 1m 1s)
	assert.Contains(s.T(), output, "75%")     // Context percentage
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_ZeroDailyCost() {
	output := formatStatuslineOutput("claude-sonnet", 0.50, 0, 300, 50.0)

	// Should show "-" for zero daily cost
	assert.Contains(s.T(), output, "📊 -")
	assert.Contains(s.T(), output, "5m0s") // Session time (300 seconds = 5m)
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_ZeroSessionSeconds() {
	output := formatStatuslineOutput("claude-sonnet", 0.50, 1.0, 0, 50.0)

	// Should show "-" for zero session seconds
	assert.Contains(s.T(), output, "⏱️ -")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_HighContextPercentage() {
	output := formatStatuslineOutput("test-model", 1.0, 1.0, 60, 85.0)

	// Should contain the percentage (color codes may vary)
	assert.Contains(s.T(), output, "85%")
	assert.Contains(s.T(), output, "1m0s")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_LowContextPercentage() {
	output := formatStatuslineOutput("test-model", 1.0, 1.0, 45, 25.0)

	// Should contain the percentage
	assert.Contains(s.T(), output, "25%")
	assert.Contains(s.T(), output, "45s")
}

// formatSessionDuration Tests

func (s *CCStatuslineTestSuite) TestFormatSessionDuration_Seconds() {
	result := formatSessionDuration(45)
	assert.Equal(s.T(), "45s", result)
}

func (s *CCStatuslineTestSuite) TestFormatSessionDuration_Minutes() {
	result := formatSessionDuration(125) // 2m 5s
	assert.Equal(s.T(), "2m5s", result)
}

func (s *CCStatuslineTestSuite) TestFormatSessionDuration_Hours() {
	result := formatSessionDuration(3665) // 1h 1m 5s
	assert.Equal(s.T(), "1h1m", result)
}

func (s *CCStatuslineTestSuite) TestFormatSessionDuration_Zero() {
	result := formatSessionDuration(0)
	assert.Equal(s.T(), "0s", result)
}

// calculateContextPercent Tests

func (s *CCStatuslineTestSuite) TestCalculateContextPercent_ZeroContextWindowSize() {
	cw := model.CCStatuslineContextWindow{
		ContextWindowSize: 0,
		TotalInputTokens:  1000,
		TotalOutputTokens: 500,
	}

	percent := calculateContextPercent(cw)

	assert.Equal(s.T(), float64(0), percent)
}

func (s *CCStatuslineTestSuite) TestCalculateContextPercent_WithCurrentUsage() {
	cw := model.CCStatuslineContextWindow{
		ContextWindowSize: 100000,
		CurrentUsage: &model.CCStatuslineContextUsage{
			InputTokens:              10000,
			OutputTokens:             5000,
			CacheCreationInputTokens: 2000,
			CacheReadInputTokens:     3000,
		},
	}

	percent := calculateContextPercent(cw)

	// (10000 + 5000 + 2000 + 3000) / 100000 * 100 = 20%
	assert.Equal(s.T(), float64(20), percent)
}

func (s *CCStatuslineTestSuite) TestCalculateContextPercent_WithoutCurrentUsage() {
	cw := model.CCStatuslineContextWindow{
		ContextWindowSize: 100000,
		TotalInputTokens:  30000,
		TotalOutputTokens: 20000,
		CurrentUsage:      nil,
	}

	percent := calculateContextPercent(cw)

	// (30000 + 20000) / 100000 * 100 = 50%
	assert.Equal(s.T(), float64(50), percent)
}

func TestCCStatuslineTestSuite(t *testing.T) {
	suite.Run(t, new(CCStatuslineTestSuite))
}

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

func (s *CCStatuslineTestSuite) TestGetDailyCost_UsesDaemonWhenAvailable() {
	// Start mock daemon socket
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	expectedCost := 15.67
	go func() {
		conn, _ := listener.Accept()
		defer conn.Close()

		var msg daemon.SocketMessage
		json.NewDecoder(conn).Decode(&msg)

		response := daemon.CCInfoResponse{
			TotalCostUSD: expectedCost,
			TimeRange:    "today",
			CachedAt:     time.Now(),
		}
		json.NewEncoder(conn).Encode(response)
	}()

	time.Sleep(10 * time.Millisecond)

	config := model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	cost := getDailyCostWithDaemonFallback(context.Background(), config)

	assert.Equal(s.T(), expectedCost, cost)
}

func (s *CCStatuslineTestSuite) TestGetDailyCost_FallbackWhenDaemonUnavailable() {
	// No socket exists, should fall back to cached API
	config := model.ShellTimeConfig{
		SocketPath: "/nonexistent/socket.sock",
		Token:      "", // No token means FetchDailyCostCached returns 0
	}

	cost := getDailyCostWithDaemonFallback(context.Background(), config)

	// Should return 0 (from cache fallback with no token)
	assert.Equal(s.T(), float64(0), cost)
}

func (s *CCStatuslineTestSuite) TestGetDailyCost_FallbackOnDaemonError() {
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

	cost := getDailyCostWithDaemonFallback(context.Background(), config)

	// Should fall back and return 0
	assert.Equal(s.T(), float64(0), cost)
}

func (s *CCStatuslineTestSuite) TestGetDailyCost_UsesDefaultSocketPath() {
	// Test that default socket path is used when config is empty
	config := model.ShellTimeConfig{
		SocketPath: "", // Empty path
		Token:      "",
	}

	// This should use model.DefaultSocketPath internally
	// Since no daemon is running, it will fall back
	cost := getDailyCostWithDaemonFallback(context.Background(), config)

	assert.Equal(s.T(), float64(0), cost)
}

// formatStatuslineOutput Tests

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_AllValues() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 75.0)

	// Should contain all components
	assert.Contains(s.T(), output, "🤖 claude-opus-4")
	assert.Contains(s.T(), output, "$1.23")
	assert.Contains(s.T(), output, "$4.56")
	assert.Contains(s.T(), output, "75%") // Context percentage
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_ZeroDailyCost() {
	output := formatStatuslineOutput("claude-sonnet", 0.50, 0, 50.0)

	// Should show "-" for zero daily cost
	assert.Contains(s.T(), output, "📊 -")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_HighContextPercentage() {
	output := formatStatuslineOutput("test-model", 1.0, 1.0, 85.0)

	// Should contain the percentage (color codes may vary)
	assert.Contains(s.T(), output, "85%")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_LowContextPercentage() {
	output := formatStatuslineOutput("test-model", 1.0, 1.0, 25.0)

	// Should contain the percentage
	assert.Contains(s.T(), output, "25%")
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

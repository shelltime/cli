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

// getDaemonInfoWithFallback Tests

func (s *CCStatuslineTestSuite) TestGetDaemonInfo_UsesDaemonWhenAvailable() {
	// Start mock daemon socket
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	expectedCost := 15.67
	expectedSessionSeconds := 3600
	expectedBranch := "main"
	expectedDirty := true
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
			GitBranch:           expectedBranch,
			GitDirty:            expectedDirty,
		}
		json.NewEncoder(conn).Encode(response)
	}()

	time.Sleep(10 * time.Millisecond)

	config := model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	result := getDaemonInfoWithFallback(context.Background(), config, "/some/path")

	assert.Equal(s.T(), expectedCost, result.Cost)
	assert.Equal(s.T(), expectedSessionSeconds, result.SessionSeconds)
	assert.Equal(s.T(), expectedBranch, result.GitBranch)
	assert.Equal(s.T(), expectedDirty, result.GitDirty)
}

func (s *CCStatuslineTestSuite) TestGetDaemonInfo_FallbackWhenDaemonUnavailable() {
	// No socket exists, should fall back to cached API
	config := model.ShellTimeConfig{
		SocketPath: "/nonexistent/socket.sock",
		Token:      "", // No token means FetchDailyStatsCached returns zero values
	}

	result := getDaemonInfoWithFallback(context.Background(), config, "")

	// Should return zero values (from cache fallback with no token)
	assert.Equal(s.T(), float64(0), result.Cost)
	assert.Equal(s.T(), 0, result.SessionSeconds)
	assert.Empty(s.T(), result.GitBranch)
	assert.False(s.T(), result.GitDirty)
}

func (s *CCStatuslineTestSuite) TestGetDaemonInfo_FallbackOnDaemonError() {
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

	result := getDaemonInfoWithFallback(context.Background(), config, "")

	// Should fall back and return zero values
	assert.Equal(s.T(), float64(0), result.Cost)
	assert.Equal(s.T(), 0, result.SessionSeconds)
	assert.Empty(s.T(), result.GitBranch)
	assert.False(s.T(), result.GitDirty)
}

func (s *CCStatuslineTestSuite) TestGetDaemonInfo_UsesDefaultSocketPath() {
	// Test that default socket path is used when config is empty
	config := model.ShellTimeConfig{
		SocketPath: "", // Empty path - should use model.DefaultSocketPath
		Token:      "",
	}

	// This should use model.DefaultSocketPath internally
	// Since no daemon is running at the default path, it will fall back to cached API
	// The function should not panic and should return a valid result struct
	result := getDaemonInfoWithFallback(context.Background(), config, "")

	// We can't assert on exact values since the global cache might have data
	// from previous tests. Just verify the function returns without error
	// and returns non-negative values
	assert.GreaterOrEqual(s.T(), result.Cost, float64(0))
	assert.GreaterOrEqual(s.T(), result.SessionSeconds, 0)
}

// formatStatuslineOutput Tests

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_AllValues() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "", "", "")

	// Should contain all components
	assert.Contains(s.T(), output, "🌿 main")
	assert.Contains(s.T(), output, "🤖 claude-opus-4")
	assert.Contains(s.T(), output, "$1.23")
	assert.Contains(s.T(), output, "$4.56")
	assert.Contains(s.T(), output, "1h1m")    // Session time (3661 seconds = 1h 1m 1s)
	assert.Contains(s.T(), output, "75%")     // Context percentage
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_WithDirtyBranch() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "feature/test", true, nil, nil, "", "", "")

	// Should contain branch with asterisk for dirty
	assert.Contains(s.T(), output, "🌿 feature/test*")
	assert.Contains(s.T(), output, "🤖 claude-opus-4")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_NoBranch() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "", false, nil, nil, "", "", "")

	// Should show "-" for no branch
	assert.Contains(s.T(), output, "🌿 -")
	assert.Contains(s.T(), output, "🤖 claude-opus-4")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_ZeroDailyCost() {
	output := formatStatuslineOutput("claude-sonnet", 0.50, 0, 300, 50.0, "main", false, nil, nil, "", "", "")

	// Should show "-" for zero daily cost
	assert.Contains(s.T(), output, "📊 -")
	assert.Contains(s.T(), output, "5m0s") // Session time (300 seconds = 5m)
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_ZeroSessionSeconds() {
	output := formatStatuslineOutput("claude-sonnet", 0.50, 1.0, 0, 50.0, "main", false, nil, nil, "", "", "")

	// Should show "-" for zero session seconds
	assert.Contains(s.T(), output, "⏱️ -")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_HighContextPercentage() {
	output := formatStatuslineOutput("test-model", 1.0, 1.0, 60, 85.0, "main", false, nil, nil, "", "", "")

	// Should contain the percentage (color codes may vary)
	assert.Contains(s.T(), output, "85%")
	assert.Contains(s.T(), output, "1m0s")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_LowContextPercentage() {
	output := formatStatuslineOutput("test-model", 1.0, 1.0, 45, 25.0, "main", false, nil, nil, "", "", "")

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

// formatQuotaPart Tests

func (s *CCStatuslineTestSuite) TestFormatQuotaPart_NilValues() {
	result := formatQuotaPart(nil, nil)
	assert.Contains(s.T(), result, "🚦 -")
}

func (s *CCStatuslineTestSuite) TestFormatQuotaPart_OnlyFiveHourNil() {
	sd := 0.23
	result := formatQuotaPart(nil, &sd)
	assert.Contains(s.T(), result, "🚦 -")
}

func (s *CCStatuslineTestSuite) TestFormatQuotaPart_LowUtilization() {
	fh := 10.0
	sd := 20.0
	result := formatQuotaPart(&fh, &sd)
	assert.Contains(s.T(), result, "5h:10%")
	assert.Contains(s.T(), result, "7d:20%")
}

func (s *CCStatuslineTestSuite) TestFormatQuotaPart_MediumUtilization() {
	fh := 55.0
	sd := 30.0
	result := formatQuotaPart(&fh, &sd)
	assert.Contains(s.T(), result, "5h:55%")
	assert.Contains(s.T(), result, "7d:30%")
}

func (s *CCStatuslineTestSuite) TestFormatQuotaPart_HighUtilization() {
	fh := 45.0
	sd := 85.0
	result := formatQuotaPart(&fh, &sd)
	assert.Contains(s.T(), result, "5h:45%")
	assert.Contains(s.T(), result, "7d:85%")
}

func (s *CCStatuslineTestSuite) TestFormatQuotaPart_ContainsLink() {
	// Nil case
	result := formatQuotaPart(nil, nil)
	assert.Contains(s.T(), result, "claude.ai/settings/usage")
	assert.Contains(s.T(), result, "\033]8;;")

	// With values
	fh := 45.0
	sd := 23.0
	result = formatQuotaPart(&fh, &sd)
	assert.Contains(s.T(), result, "claude.ai/settings/usage")
	assert.Contains(s.T(), result, "\033]8;;")
	assert.Contains(s.T(), result, "5h:45%")
	assert.Contains(s.T(), result, "7d:23%")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_SessionCostWithLink() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "testuser", "https://shelltime.xyz", "session-abc123")

	// Should contain OSC8 link wrapping session cost
	assert.Contains(s.T(), output, "shelltime.xyz/users/testuser/coding-agent/session/session-abc123")
	assert.Contains(s.T(), output, "\033]8;;")
	assert.Contains(s.T(), output, "$1.23")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_SessionCostWithoutLink() {
	// No userLogin - should not have link
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "", "https://shelltime.xyz", "session-abc123")
	assert.Contains(s.T(), output, "$1.23")
	assert.NotContains(s.T(), output, "coding-agent/session/")

	// No sessionID - should not have link
	output = formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "testuser", "https://shelltime.xyz", "")
	assert.Contains(s.T(), output, "$1.23")
	assert.NotContains(s.T(), output, "coding-agent/session/")

	// No webEndpoint - should not have link
	output = formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "testuser", "", "session-abc123")
	assert.Contains(s.T(), output, "$1.23")
	assert.NotContains(s.T(), output, "coding-agent/session/")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_TimeWithProfileLink() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "testuser", "https://shelltime.xyz", "session-abc123")

	// Should contain OSC8 link wrapping time section to user profile
	assert.Contains(s.T(), output, "shelltime.xyz/users/testuser")
	assert.Contains(s.T(), output, "1h1m")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_TimeWithoutProfileLink() {
	// No userLogin - should not have profile link on time
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "", "https://shelltime.xyz", "session-abc123")
	assert.Contains(s.T(), output, "1h1m")
	// The time section should not contain a link to users/ profile
	// Count occurrences of "shelltime.xyz/users/" - should only be in session cost and daily cost links
	assert.NotContains(s.T(), output, "shelltime.xyz/users//")

	// No webEndpoint - should not have profile link on time
	output = formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "testuser", "", "session-abc123")
	assert.Contains(s.T(), output, "1h1m")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_WithQuota() {
	fh := 45.0
	sd := 23.0
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, &fh, &sd, "", "", "")

	assert.Contains(s.T(), output, "5h:45%")
	assert.Contains(s.T(), output, "7d:23%")
	assert.Contains(s.T(), output, "🚦")
}

func (s *CCStatuslineTestSuite) TestFormatStatuslineOutput_WithoutQuota() {
	output := formatStatuslineOutput("claude-opus-4", 1.23, 4.56, 3661, 75.0, "main", false, nil, nil, "", "", "")

	assert.Contains(s.T(), output, "🚦 -")
}

func (s *CCStatuslineTestSuite) TestGetDaemonInfo_PropagatesRateLimitFields() {
	listener, err := net.Listen("unix", s.socketPath)
	assert.NoError(s.T(), err)
	s.listener = listener

	fh := 45.0
	sd := 23.0
	go func() {
		conn, _ := listener.Accept()
		defer conn.Close()

		var msg daemon.SocketMessage
		json.NewDecoder(conn).Decode(&msg)

		response := daemon.CCInfoResponse{
			TotalCostUSD:        1.23,
			TotalSessionSeconds: 100,
			TimeRange:           "today",
			CachedAt:            time.Now(),
			GitBranch:           "main",
			FiveHourUtilization: &fh,
			SevenDayUtilization: &sd,
		}
		json.NewEncoder(conn).Encode(response)
	}()

	time.Sleep(10 * time.Millisecond)

	config := model.ShellTimeConfig{
		SocketPath: s.socketPath,
	}

	result := getDaemonInfoWithFallback(context.Background(), config, "/some/path")

	assert.NotNil(s.T(), result.FiveHourUtilization)
	assert.NotNil(s.T(), result.SevenDayUtilization)
	assert.Equal(s.T(), 45.0, *result.FiveHourUtilization)
	assert.Equal(s.T(), 23.0, *result.SevenDayUtilization)
}

func TestCCStatuslineTestSuite(t *testing.T) {
	suite.Run(t, new(CCStatuslineTestSuite))
}

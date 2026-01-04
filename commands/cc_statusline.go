package commands

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var CCStatuslineCommand = &cli.Command{
	Name:   "statusline",
	Usage:  "Output statusline for Claude Code (reads JSON from stdin)",
	Action: commandCCStatusline,
}

func commandCCStatusline(c *cli.Context) error {
	// Hard timeout for entire operation - statusline must be fast
	ctx, cancel := context.WithTimeout(c.Context, 100*time.Millisecond)
	defer cancel()

	// Read from stdin
	input, err := readStdinWithTimeout(ctx)
	if err != nil {
		outputFallback()
		return nil
	}

	// Parse input
	var data model.CCStatuslineInput
	if err := json.Unmarshal(input, &data); err != nil {
		outputFallback()
		return nil
	}

	// Calculate context percentage
	contextPercent := calculateContextPercent(data.ContextWindow)

	// Get daily stats - try daemon first, fallback to direct API
	var dailyStats model.CCStatuslineDailyStats
	config, err := configService.ReadConfigFile(ctx)
	if err == nil {
		dailyStats = getDailyStatsWithDaemonFallback(ctx, config)
	}

	// Format and output
	output := formatStatuslineOutput(data.Model.DisplayName, data.Cost.TotalCostUSD, dailyStats.Cost, dailyStats.SessionSeconds, contextPercent)
	fmt.Println(output)

	return nil
}

func readStdinWithTimeout(ctx context.Context) ([]byte, error) {
	resultCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(os.Stdin)
		var data []byte
		for {
			line, err := reader.ReadBytes('\n')
			data = append(data, line...)
			if err != nil {
				if err == io.EOF {
					break
				}
				errCh <- err
				return
			}
		}
		resultCh <- data
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errCh:
		return nil, err
	case data := <-resultCh:
		return data, nil
	}
}

func calculateContextPercent(cw model.CCStatuslineContextWindow) float64 {
	if cw.ContextWindowSize == 0 {
		return 0
	}

	// Use current_usage if available for accurate context window state
	if cw.CurrentUsage != nil {
		currentTokens := cw.CurrentUsage.InputTokens +
			cw.CurrentUsage.OutputTokens +
			cw.CurrentUsage.CacheCreationInputTokens +
			cw.CurrentUsage.CacheReadInputTokens
		return float64(currentTokens) / float64(cw.ContextWindowSize) * 100
	}

	// Fallback to total tokens
	currentTokens := cw.TotalInputTokens + cw.TotalOutputTokens
	return float64(currentTokens) / float64(cw.ContextWindowSize) * 100
}

func formatStatuslineOutput(modelName string, sessionCost, dailyCost float64, sessionSeconds int, contextPercent float64) string {
	var parts []string

	// Model name
	modelStr := fmt.Sprintf("🤖 %s", modelName)
	parts = append(parts, modelStr)

	// Session cost (cyan)
	sessionStr := color.Cyan.Sprintf("💰 $%.2f", sessionCost)
	parts = append(parts, sessionStr)

	// Daily cost (yellow)
	if dailyCost > 0 {
		dailyStr := color.Yellow.Sprintf("📊 $%.2f", dailyCost)
		parts = append(parts, dailyStr)
	} else {
		parts = append(parts, color.Gray.Sprint("📊 -"))
	}

	// AI agent time (magenta)
	if sessionSeconds > 0 {
		timeStr := color.Magenta.Sprintf("⏱️ %s", formatSessionDuration(sessionSeconds))
		parts = append(parts, timeStr)
	} else {
		parts = append(parts, color.Gray.Sprint("⏱️ -"))
	}

	// Context percentage with color coding
	var contextStr string
	switch {
	case contextPercent >= 80:
		contextStr = color.Red.Sprintf("📈 %.0f%%", contextPercent)
	case contextPercent >= 50:
		contextStr = color.Yellow.Sprintf("📈 %.0f%%", contextPercent)
	default:
		contextStr = color.Green.Sprintf("📈 %.0f%%", contextPercent)
	}
	parts = append(parts, contextStr)

	return strings.Join(parts, " | ")
}

func outputFallback() {
	fmt.Println(color.Gray.Sprint("🤖 - | 💰 - | 📊 - | ⏱️ - | 📈 -%"))
}

// formatSessionDuration formats seconds into a human-readable duration
func formatSessionDuration(totalSeconds int) string {
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// getDailyStatsWithDaemonFallback tries to get daily stats from daemon first,
// falls back to direct API if daemon is unavailable
func getDailyStatsWithDaemonFallback(ctx context.Context, config model.ShellTimeConfig) model.CCStatuslineDailyStats {
	socketPath := config.SocketPath
	if socketPath == "" {
		socketPath = model.DefaultSocketPath
	}

	// Try daemon first (50ms timeout for fast path)
	if daemon.IsSocketReady(ctx, socketPath) {
		resp, err := daemon.RequestCCInfo(socketPath, daemon.CCInfoTimeRangeToday, 50*time.Millisecond)
		if err == nil && resp != nil {
			return model.CCStatuslineDailyStats{
				Cost:           resp.TotalCostUSD,
				SessionSeconds: resp.TotalSessionSeconds,
			}
		}
	}

	// Fallback to direct API (existing behavior)
	return model.FetchDailyStatsCached(ctx, config)
}

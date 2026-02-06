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

const claudeUsageURL = "https://claude.ai/settings/usage"

func wrapOSC8Link(url, text string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}

var CCStatuslineCommand = &cli.Command{
	Name:   "statusline",
	Usage:  "Output statusline for Claude Code (reads JSON from stdin)",
	Action: commandCCStatusline,
}

// ccStatuslineResult combines daily stats with git info from daemon
type ccStatuslineResult struct {
	Cost                float64
	SessionSeconds      int
	GitBranch           string
	GitDirty            bool
	FiveHourUtilization *float64
	SevenDayUtilization *float64
	UserLogin           string
	WebEndpoint         string
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

	// Get daily stats and git info - try daemon first, fallback to direct API
	var result ccStatuslineResult
	config, err := configService.ReadConfigFile(ctx)
	if err == nil {
		result = getDaemonInfoWithFallback(ctx, config, data.Cwd)
	}

	// Format and output
	output := formatStatuslineOutput(data.Model.DisplayName, data.Cost.TotalCostUSD, result.Cost, result.SessionSeconds, contextPercent, result.GitBranch, result.GitDirty, result.FiveHourUtilization, result.SevenDayUtilization, result.UserLogin, result.WebEndpoint)
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

func formatStatuslineOutput(modelName string, sessionCost, dailyCost float64, sessionSeconds int, contextPercent float64, gitBranch string, gitDirty bool, fiveHourUtil, sevenDayUtil *float64, userLogin, webEndpoint string) string {
	var parts []string

	// Git info FIRST (green)
	if gitBranch != "" {
		gitStr := gitBranch
		if gitDirty {
			gitStr += "*"
		}
		parts = append(parts, color.Green.Sprintf("🌿 %s", gitStr))
	} else {
		parts = append(parts, color.Gray.Sprint("🌿 -"))
	}

	// Model name
	modelStr := fmt.Sprintf("🤖 %s", modelName)
	parts = append(parts, modelStr)

	// Session cost (cyan)
	sessionStr := color.Cyan.Sprintf("💰 $%.2f", sessionCost)
	parts = append(parts, sessionStr)

	// Daily cost (yellow) - clickable link to coding agent page when user login is available
	if dailyCost > 0 {
		dailyStr := color.Yellow.Sprintf("📊 $%.2f", dailyCost)
		if userLogin != "" && webEndpoint != "" {
			url := fmt.Sprintf("%s/users/%s/coding-agent/claude-code", webEndpoint, userLogin)
			dailyStr = wrapOSC8Link(url, dailyStr)
		}
		parts = append(parts, dailyStr)
	} else {
		parts = append(parts, color.Gray.Sprint("📊 -"))
	}

	// Quota utilization
	parts = append(parts, formatQuotaPart(fiveHourUtil, sevenDayUtil))

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

// formatQuotaPart formats the rate limit quota section of the statusline.
// Color is based on the max utilization of both buckets.
func formatQuotaPart(fiveHourUtil, sevenDayUtil *float64) string {
	if fiveHourUtil == nil || sevenDayUtil == nil {
		return wrapOSC8Link(claudeUsageURL, color.Gray.Sprint("🚦 -"))
	}

	fh := *fiveHourUtil
	sd := *sevenDayUtil

	text := fmt.Sprintf("🚦 5h:%.0f%% 7d:%.0f%%", fh, sd)

	maxUtil := fh
	if sd > maxUtil {
		maxUtil = sd
	}

	var colored string
	switch {
	case maxUtil >= 80:
		colored = color.Red.Sprint(text)
	case maxUtil >= 50:
		colored = color.Yellow.Sprint(text)
	default:
		colored = color.Green.Sprint(text)
	}
	return wrapOSC8Link(claudeUsageURL, colored)
}

func outputFallback() {
	quotaPart := wrapOSC8Link(claudeUsageURL, "🚦 -")
	fmt.Println(color.Gray.Sprint("🌿 - | 🤖 - | 💰 - | 📊 - | " + quotaPart + " | ⏱️ - | 📈 -%"))
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

// getDaemonInfoWithFallback tries to get daily stats and git info from daemon first,
// falls back to direct API for stats if daemon is unavailable (git info only from daemon)
func getDaemonInfoWithFallback(ctx context.Context, config model.ShellTimeConfig, workingDir string) ccStatuslineResult {
	socketPath := config.SocketPath
	if socketPath == "" {
		socketPath = model.DefaultSocketPath
	}

	// Try daemon first (50ms timeout for fast path)
	if daemon.IsSocketReady(ctx, socketPath) {
		resp, err := daemon.RequestCCInfo(socketPath, daemon.CCInfoTimeRangeToday, workingDir, 50*time.Millisecond)
		if err == nil && resp != nil {
			return ccStatuslineResult{
				Cost:                resp.TotalCostUSD,
				SessionSeconds:      resp.TotalSessionSeconds,
				GitBranch:           resp.GitBranch,
				GitDirty:            resp.GitDirty,
				FiveHourUtilization: resp.FiveHourUtilization,
				SevenDayUtilization: resp.SevenDayUtilization,
				UserLogin:           resp.UserLogin,
				WebEndpoint:         config.WebEndpoint,
			}
		}
	}

	// Fallback to direct API for stats (no git info available without daemon)
	stats := model.FetchDailyStatsCached(ctx, config)
	return ccStatuslineResult{
		Cost:           stats.Cost,
		SessionSeconds: stats.SessionSeconds,
	}
}

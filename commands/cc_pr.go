package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

var CCPullRequestCommand = &cli.Command{
	Name:      "pr",
	Usage:     "Link pull requests opened in a Claude Code session to it (called by the ShellTime Claude Code mod after `gh pr create`)",
	ArgsUsage: "<pr-url> [pr-url...]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "session-id",
			Usage:    "Claude Code session id",
			Required: true,
		},
	},
	Action: commandCCPullRequest,
}

func commandCCPullRequest(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "cc.pr", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	sessionID := strings.TrimSpace(c.String("session-id"))
	urls := uniqueNonEmpty(c.Args().Slice())
	if sessionID == "" || len(urls) == 0 {
		return fmt.Errorf("usage: shelltime cc pr --session-id <id> <pr-url> [pr-url...]")
	}

	config, err := configService.ReadConfigFile(ctx)
	if err != nil {
		return err
	}
	if config.Token == "" {
		slog.Debug("cc pr: not logged in, skipping")
		return nil
	}

	socketPath := config.SocketPath
	if socketPath == "" {
		socketPath = model.DefaultSocketPath
	}
	// The daemon sends them in the background; with no daemon, send them here.
	err = daemon.SendSessionPullRequests(socketPath, sessionID, urls)
	if err == nil {
		return nil
	}
	slog.Debug("cc pr: daemon unreachable, sending directly", slog.Any("err", err))

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := model.SendSessionPullRequests(ctx, config, sessionID, urls); err != nil {
		slog.Error("cc pr: failed to send pull requests", slog.String("sessionId", sessionID), slog.Any("err", err))
		return err
	}
	return nil
}

// uniqueNonEmpty trims each value and drops blanks and repeats, keeping order.
func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}

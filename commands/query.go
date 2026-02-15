package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/malamtime/cli/stloader"
	"github.com/urfave/cli/v2"
)

var QueryCommand *cli.Command = &cli.Command{
	Name:    "query",
	Aliases: []string{"q"},
	Usage:   "Query AI for command suggestions",
	Action:  commandQuery,
	Description: `Query AI for command suggestions based on your prompt.

Examples:
  shelltime query "get the top 5 memory-using processes"
  shelltime q "find all files modified in the last 24 hours"
  shelltime q "show disk usage for current directory"`,
}

func commandQuery(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "query")
	defer span.End()

	// Check if AI service is initialized
	if aiService == nil {
		color.Red.Println("AI service is not configured")
		return fmt.Errorf("AI service is not available")
	}

	// Get the query from command arguments
	args := c.Args().Slice()
	if len(args) == 0 {
		color.Red.Println("Please provide a query")
		return fmt.Errorf("query is required")
	}

	query := strings.Join(args, " ")

	// Read config to get endpoint/token
	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		color.Red.Printf("Failed to read config: %v\n", err)
		return fmt.Errorf("failed to read config: %w", err)
	}

	endpoint := model.Endpoint{
		APIEndpoint: cfg.APIEndpoint,
		Token:       cfg.Token,
	}

	// Get system context
	systemContext, err := getSystemContext(query)
	if err != nil {
		slog.Warn("Failed to get system context", slog.Any("err", err))
	}

	l := stloader.NewLoader(stloader.LoaderConfig{
		Text:          "Querying AI...",
		EnableShining: true,
		BaseColor:     stloader.RGB{R: 100, G: 180, B: 255},
	})
	l.Start()

	var result strings.Builder
	firstToken := true

	// Stream the AI response
	err = aiService.QueryCommandStream(ctx, systemContext, endpoint, func(token string) {
		if firstToken {
			l.Stop()
			color.Green.Printf("Suggested command:\n")
			firstToken = false
		}
		fmt.Print(token)
		result.WriteString(token)
	})

	if firstToken {
		// No tokens received, stop loader
		l.Stop()
	}

	if err != nil {
		if !firstToken {
			fmt.Println()
		}
		color.Red.Printf("Failed to query AI: %v\n", err)
		return err
	}

	// Print newline after streaming
	fmt.Println()
	slog.InfoContext(ctx, "query command", "command", result.String())

	newCommand := strings.TrimSpace(result.String())

	// Check auto-run configuration
	if cfg.AI != nil && (cfg.AI.Agent.View || cfg.AI.Agent.Edit || cfg.AI.Agent.Delete) {
		// Classify the command
		actionType := model.ClassifyCommand(newCommand)

		// Check if this action type is enabled for auto-run
		canAutoRun := false
		switch actionType {
		case model.ActionView:
			canAutoRun = cfg.AI.Agent.View
		case model.ActionEdit:
			canAutoRun = cfg.AI.Agent.Edit
		case model.ActionDelete:
			canAutoRun = cfg.AI.Agent.Delete
		}

		if canAutoRun {
			// For delete commands, add an extra confirmation
			if actionType == model.ActionDelete {
				color.Yellow.Printf("This is a DELETE command. Are you sure you want to run it? (y/N): ")

				var response string
				fmt.Scanln(&response)
				if strings.ToLower(strings.TrimSpace(response)) != "y" {
					color.Yellow.Printf("Command execution cancelled.\n")
					return nil
				}
			} else {
				color.Green.Printf("Auto-running command...\n")
			}

			// Execute the command
			return executeCommand(ctx, newCommand)
		} else {
			if shouldShowTips(cfg) && actionType != model.ActionOther {
				color.Yellow.Printf("\nTip: This is a %s command. Enable 'ai.agent.%s' in your config to auto-run it.\n",
					actionType, actionType)
			}
		}
	} else {
		if shouldShowTips(cfg) {
			color.Yellow.Printf("\nTip: You can enable AI auto-run in your config file:\n")
			color.Yellow.Printf("   [ai.agent]\n")
			color.Yellow.Printf("   view = true    # Auto-run view commands\n")
			color.Yellow.Printf("   edit = true    # Auto-run edit commands\n")
			color.Yellow.Printf("   delete = true  # Auto-run delete commands\n")
		}
	}

	return nil
}

func shouldShowTips(cfg model.ShellTimeConfig) bool {
	// If ShowTips is not set (nil), default to true
	if cfg.AI == nil || cfg.AI.ShowTips == nil {
		return true
	}
	return *cfg.AI.ShowTips
}

func executeCommand(ctx context.Context, command string) error {
	// Get the shell to use
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	// Create command with shell
	cmd := exec.CommandContext(ctx, shell, "-c", command)

	// Connect stdin, stdout, stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	if err := cmd.Run(); err != nil {
		color.Red.Printf("\nCommand failed: %v\n", err)
		return err
	}

	return nil
}

func getSystemContext(query string) (model.CommandSuggestVariables, error) {
	// Get shell information
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "unknown"
	} else {
		// Extract just the shell name from path
		if idx := strings.LastIndex(shell, "/"); idx >= 0 {
			shell = shell[idx+1:]
		}
	}

	// Get OS information
	osInfo := runtime.GOOS

	return model.CommandSuggestVariables{
		Shell: shell,
		Os:    osInfo,
		Query: query,
	}, nil
}

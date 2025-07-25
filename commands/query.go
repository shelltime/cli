package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/sirupsen/logrus"
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
		color.Red.Println("❌ AI service is not configured")
		return fmt.Errorf("AI service is not available")
	}

	// Get the query from command arguments
	args := c.Args().Slice()
	if len(args) == 0 {
		color.Red.Println("❌ Please provide a query")
		return fmt.Errorf("query is required")
	}

	query := strings.Join(args, " ")

	// Get system context
	systemContext, err := getSystemContext(query)
	if err != nil {
		logrus.Warnf("Failed to get system context: %v", err)
	}

	s := spinner.New(spinner.CharSets[35], 200*time.Millisecond)
	s.Start()
	defer s.Stop()

	// skip userId for now
	userId := ""

	// Query the AI
	newCommand, err := aiService.QueryCommand(ctx, systemContext, userId)
	if err != nil {
		s.Stop()
		color.Red.Printf("❌ Failed to query AI: %v\n", err)
		return err
	}

	s.Stop()

	// Trim the command
	newCommand = strings.TrimSpace(newCommand)

	// Check auto-run configuration
	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		logrus.Warnf("Failed to read config for auto-run check: %v", err)
		// If can't read config, just display the command
		displayCommand(newCommand)
		return nil
	}

	// Check if AI auto-run is configured
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
				color.Green.Printf("💡 Suggested command:\n")
				color.Cyan.Printf("%s\n\n", newCommand)
				color.Yellow.Printf("⚠️  This is a DELETE command. Are you sure you want to run it? (y/N): ")

				var response string
				fmt.Scanln(&response)
				if strings.ToLower(strings.TrimSpace(response)) != "y" {
					color.Yellow.Printf("Command execution cancelled.\n")
					return nil
				}
			} else {
				// Display the command and auto-run it
				color.Green.Printf("💡 Auto-running command:\n")
				color.Cyan.Printf("%s\n\n", newCommand)
			}

			// Execute the command
			return executeCommand(ctx, newCommand)
		} else {
			// Display command with info about why it's not auto-running
			displayCommand(newCommand)
			if actionType != model.ActionOther {
				color.Yellow.Printf("\n💡 Tip: This is a %s command. Enable 'ai.agent.%s' in your config to auto-run it.\n",
					actionType, actionType)
			}
		}
	} else {
		// No auto-run configured, display the command and tip
		displayCommand(newCommand)
		color.Yellow.Printf("\n💡 Tip: You can enable AI auto-run in your config file:\n")
		color.Yellow.Printf("   [ai.agent]\n")
		color.Yellow.Printf("   view = true    # Auto-run view commands\n")
		color.Yellow.Printf("   edit = true    # Auto-run edit commands\n")
		color.Yellow.Printf("   delete = true  # Auto-run delete commands\n")
	}

	return nil
}

func displayCommand(command string) {
	color.Green.Printf("💡 Suggested command:\n")
	color.Cyan.Printf("%s\n", command)
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
		color.Red.Printf("\n❌ Command failed: %v\n", err)
		return err
	}

	return nil
}

func getSystemContext(query string) (model.PPPromptGuessNextPromptVariables, error) {
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

	return model.PPPromptGuessNextPromptVariables{
		Shell: shell,
		Os:    osInfo,
		Query: query,
	}, nil
}

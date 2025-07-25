package commands

import (
	"fmt"
	"os"
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
	response, err := aiService.QueryCommand(ctx, systemContext, userId)
	if err != nil {
		s.Stop()
		color.Red.Printf("❌ Failed to query AI: %v\n", err)
		return err
	}

	s.Stop()

	// Display the response
	color.Green.Printf("💡 Suggested command:\n")
	color.Cyan.Printf("%s\n", strings.TrimSpace(response))

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

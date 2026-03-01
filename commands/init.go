package commands

import (
	"github.com/gookit/color"
	"github.com/urfave/cli/v2"
)

var InitCommand *cli.Command = &cli.Command{
	Name:  "init",
	Usage: "Initialize shelltime: authenticate, install hooks, start daemon, and configure AI code integrations",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "token",
			Aliases:  []string{"t"},
			Usage:    "Authentication token",
			Required: false,
		},
	},
	Action: commandInit,
}

func commandInit(c *cli.Context) error {
	color.Yellow.Println("Initializing ShellTime...")

	// Step 1: Authenticate
	if err := commandAuth(c); err != nil {
		return err
	}

	// Step 2: Install shell hooks
	if err := commandHooksInstall(c); err != nil {
		return err
	}

	// Step 3: Install daemon service
	if err := commandDaemonInstall(c); err != nil {
		return err
	}

	// Step 4: Install Claude Code OTEL configuration
	if err := commandCCInstall(c); err != nil {
		color.Red.Printf("Failed to install Claude Code OTEL config: %v\n", err)
	}

	// Step 5: Install Codex OTEL configuration
	if err := commandCodexInstall(c); err != nil {
		color.Red.Printf("Failed to install Codex OTEL config: %v\n", err)
	}

	color.Green.Println("ShellTime is fully initialized and ready to use!")
	return nil
}

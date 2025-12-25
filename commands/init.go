package commands

import (
	"github.com/gookit/color"
	"github.com/urfave/cli/v2"
)

var InitCommand *cli.Command = &cli.Command{
	Name:  "init",
	Usage: "Initialize shelltime: authenticate, install hooks, and start daemon",
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

	color.Green.Println("ShellTime is fully initialized and ready to use!")
	return nil
}

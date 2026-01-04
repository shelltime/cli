package commands

import "github.com/urfave/cli/v2"

var ConfigCommand *cli.Command = &cli.Command{
	Name:  "config",
	Usage: "manage shelltime configuration",
	Subcommands: []*cli.Command{
		ConfigViewCommand,
	},
}

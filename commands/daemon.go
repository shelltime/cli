package commands

import "github.com/urfave/cli/v2"

var DaemonCommand *cli.Command = &cli.Command{
	Name:  "daemon",
	Usage: "shelltime daemon service",
	Subcommands: []*cli.Command{
		DaemonStatusCommand,
		DaemonInstallCommand,
		DaemonUninstallCommand,
		DaemonReinstallCommand,
	},
}

// cli/commands/hooks.go
package commands

import "github.com/urfave/cli/v2"

var HooksCommand = &cli.Command{
	Name:  "hooks",
	Usage: "shelltime hooks management",
	Subcommands: []*cli.Command{
		HooksInstallCommand,
		HooksUninstallCommand,
	},
}

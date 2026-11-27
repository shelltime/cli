package commands

import (
	"github.com/urfave/cli/v2"
)

var CursorCommand = &cli.Command{
	Name:  "cursor",
	Usage: "Cursor integration commands",
	Subcommands: []*cli.Command{
		CursorInstallCommand,
		CursorUninstallCommand,
	},
}

var CursorInstallCommand = &cli.Command{
	Name:    "install",
	Aliases: []string{"i"},
	Usage:   "Install the ShellTime hooks into the current repository's .cursor/hooks.json (Cursor IDE, cursor-agent and Cloud Agents)",
	Action:  commandCursorInstall,
}

var CursorUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove the ShellTime hooks from the current repository's .cursor/hooks.json and ~/.cursor/hooks.json",
	Action:  commandCursorUninstall,
}

func commandCursorInstall(c *cli.Context) error {
	if err := installAgentHooks(cursorHooks); err != nil {
		return err
	}
	printAgentHooksPrivacyNote("Cursor")
	return nil
}

func commandCursorUninstall(c *cli.Context) error {
	return uninstallAgentHooks(cursorHooks)
}

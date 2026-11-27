package commands

import (
	"github.com/urfave/cli/v2"
)

var AgyCommand = &cli.Command{
	Name:    "agy",
	Aliases: []string{"antigravity"},
	Usage:   "Google Antigravity integration commands",
	Subcommands: []*cli.Command{
		AgyInstallCommand,
		AgyUninstallCommand,
	},
}

var AgyInstallCommand = &cli.Command{
	Name:    "install",
	Aliases: []string{"i"},
	Usage:   "Install the ShellTime hooks into the current repository's .agents/hooks.json (Antigravity app, agy CLI and its Gemini API sandbox)",
	Action:  commandAgyInstall,
}

var AgyUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove the ShellTime hooks from the current repository's .agents/hooks.json and ~/.gemini/config/hooks.json",
	Action:  commandAgyUninstall,
}

func commandAgyInstall(c *cli.Context) error {
	if err := installAgentHooks(antigravityHooks); err != nil {
		return err
	}
	printAgentHooksPrivacyNote("Antigravity")
	return nil
}

func commandAgyUninstall(c *cli.Context) error {
	return uninstallAgentHooks(antigravityHooks)
}

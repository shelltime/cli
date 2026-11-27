package commands

import (
	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
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
	Usage:   "Install the ShellTime hooks into ~/.cursor/hooks.json (Cursor IDE and cursor-agent)",
	Action:  commandCursorInstall,
}

var CursorUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove the ShellTime hooks from ~/.cursor/hooks.json",
	Action:  commandCursorUninstall,
}

// hookBinaryPath finds the shelltime binary the hooks run; replaced in tests.
var hookBinaryPath = model.HookBinaryPath

func commandCursorInstall(c *cli.Context) error {
	if err := installCursorHooks(); err != nil {
		return err
	}
	printAgentHooksPrivacyNote("Cursor")
	return nil
}

// installCursorHooks is `cursor install` without the privacy note, which init leaves out.
func installCursorHooks() error {
	color.Yellow.Println("Installing Cursor hooks...")

	svc := model.NewCursorHooksService()
	bin, err := hookBinaryPath()
	if err != nil {
		color.Red.Printf("Failed to find the shelltime binary: %v\n", err)
		return err
	}
	if err := svc.Install(bin); err != nil {
		color.Red.Printf("Failed to install the Cursor hooks: %v\n", err)
		return err
	}

	color.Green.Printf("Cursor hooks installed in %s\n", svc.Path())
	color.Yellow.Println("Restart Cursor (the IDE and any running cursor-agent sessions) to apply them.")
	return nil
}

func commandCursorUninstall(c *cli.Context) error {
	color.Yellow.Println("Removing Cursor hooks...")

	svc := model.NewCursorHooksService()
	if err := svc.Uninstall(); err != nil {
		color.Red.Printf("Failed to remove the Cursor hooks: %v\n", err)
		return err
	}
	color.Green.Printf("Cursor hooks removed from %s\n", svc.Path())
	return nil
}

// printAgentHooksPrivacyNote says what the Cursor or Antigravity hooks send.
func printAgentHooksPrivacyNote(agent string) {
	color.Gray.Printf("Privacy: the %s hooks send your prompts, the agent's responses, tool names and inputs\n", agent)
	color.Gray.Println("(shell commands, file paths) and the workspace folder to your ShellTime account. Data masking")
	color.Gray.Println("and your exclude patterns apply. To keep prompt, response and tool input text out and send only")
	color.Gray.Println("the activity, run `shelltime config disable agentHooks.sendContent`.")
}

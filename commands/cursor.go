package commands

import (
	"os"

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
	Flags: []cli.Flag{&cli.BoolFlag{
		Name:  "project",
		Usage: "install them into the current repository's .cursor/hooks.json instead, the only hook file Cursor Cloud Agents run (commit it)",
	}},
	Action: commandCursorInstall,
}

var CursorUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove the ShellTime hooks from ~/.cursor/hooks.json",
	Flags: []cli.Flag{&cli.BoolFlag{
		Name:  "project",
		Usage: "remove them from the current repository's .cursor/hooks.json instead",
	}},
	Action: commandCursorUninstall,
}

// hookBinaryPath finds the shelltime binary the hooks run; replaced in tests.
var hookBinaryPath = model.HookBinaryPath

func commandCursorInstall(c *cli.Context) error {
	install := installCursorHooks
	if c.Bool("project") {
		install = installCursorProjectHooks
	}
	if err := install(); err != nil {
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

// installCursorProjectHooks writes the hooks into the current repository's .cursor/hooks.json.
func installCursorProjectHooks() error {
	color.Yellow.Println("Installing Cursor project hooks...")

	root, err := currentProjectRoot()
	if err != nil {
		color.Red.Printf("Failed to find the project: %v\n", err)
		return err
	}
	svc := model.NewCursorProjectHooksService(root)
	if err := svc.Install(model.ProjectHookBinary); err != nil {
		color.Red.Printf("Failed to install the Cursor project hooks: %v\n", err)
		return err
	}

	color.Green.Printf("Cursor project hooks installed in %s\n", svc.Path())
	color.Yellow.Println("Commit it: Cursor Cloud Agents run only the repository's hooks, never ~/.cursor/hooks.json.")
	printProjectHooksNote()
	return nil
}

func commandCursorUninstall(c *cli.Context) error {
	color.Yellow.Println("Removing Cursor hooks...")

	svc := model.NewCursorHooksService()
	if c.Bool("project") {
		root, err := currentProjectRoot()
		if err != nil {
			color.Red.Printf("Failed to find the project: %v\n", err)
			return err
		}
		svc = model.NewCursorProjectHooksService(root)
	}
	if err := svc.Uninstall(); err != nil {
		color.Red.Printf("Failed to remove the Cursor hooks: %v\n", err)
		return err
	}
	color.Green.Printf("Cursor hooks removed from %s\n", svc.Path())
	return nil
}

// currentProjectRoot is the root of the project the working directory is in, where agents look
// for project hook files.
func currentProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return model.ProjectHooksRoot(wd)
}

// printProjectHooksNote says what a committed project hook file does elsewhere and what a cloud
// environment needs for it.
func printProjectHooksNote() {
	color.Gray.Println("Every machine that opens the repository runs these hooks as `shelltime` from its PATH. Where")
	color.Gray.Println("your own hooks are installed too, those record the events and the project hooks stand aside.")
	color.Gray.Println("In a cloud environment, install shelltime, sign it in (`shelltime auth --token …`), keep")
	color.Gray.Println("shelltime-daemon running and set SHELLTIME_CLOUD=true, so its sessions show as cloud sessions.")
}

// printAgentHooksPrivacyNote says what the Cursor or Antigravity hooks send.
func printAgentHooksPrivacyNote(agent string) {
	color.Gray.Printf("Privacy: the %s hooks send your prompts, the agent's responses, tool names and inputs\n", agent)
	color.Gray.Println("(shell commands, file paths) and the workspace folder to your ShellTime account. Data masking")
	color.Gray.Println("and your exclude patterns apply. To keep prompt, response and tool input text out and send only")
	color.Gray.Println("the activity, run `shelltime config disable agentHooks.sendContent`.")
}

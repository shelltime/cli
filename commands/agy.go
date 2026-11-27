package commands

import (
	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
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
	Usage:   "Install the ShellTime hooks into ~/.gemini/config/hooks.json (Antigravity app and agy CLI)",
	Flags: []cli.Flag{&cli.BoolFlag{
		Name:  "project",
		Usage: "install them into the current repository's .agents/hooks.json instead, which the Antigravity agent reads in a Gemini API sandbox too (commit it)",
	}},
	Action: commandAgyInstall,
}

var AgyUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove the ShellTime hooks from ~/.gemini/config/hooks.json",
	Flags: []cli.Flag{&cli.BoolFlag{
		Name:  "project",
		Usage: "remove them from the current repository's .agents/hooks.json instead",
	}},
	Action: commandAgyUninstall,
}

func commandAgyInstall(c *cli.Context) error {
	install := installAgyHooks
	if c.Bool("project") {
		install = installAgyProjectHooks
	}
	if err := install(); err != nil {
		return err
	}
	printAgentHooksPrivacyNote("Antigravity")
	return nil
}

// installAgyHooks is `agy install` without the privacy note, which init leaves out.
func installAgyHooks() error {
	color.Yellow.Println("Installing Antigravity hooks...")

	svc := model.NewAntigravityHooksService()
	bin, err := hookBinaryPath()
	if err != nil {
		color.Red.Printf("Failed to find the shelltime binary: %v\n", err)
		return err
	}
	if err := svc.Install(bin); err != nil {
		color.Red.Printf("Failed to install the Antigravity hooks: %v\n", err)
		return err
	}

	color.Green.Printf("Antigravity hooks installed in %s\n", svc.Path())
	color.Yellow.Println("Restart Antigravity (the app and any running agy sessions) to apply them.")
	return nil
}

// installAgyProjectHooks writes the hooks into the current repository's .agents/hooks.json.
func installAgyProjectHooks() error {
	color.Yellow.Println("Installing Antigravity project hooks...")

	root, err := currentProjectRoot()
	if err != nil {
		color.Red.Printf("Failed to find the project: %v\n", err)
		return err
	}
	svc := model.NewAntigravityProjectHooksService(root)
	if err := svc.Install(model.ProjectHookBinary); err != nil {
		color.Red.Printf("Failed to install the Antigravity project hooks: %v\n", err)
		return err
	}

	color.Green.Printf("Antigravity project hooks installed in %s\n", svc.Path())
	color.Yellow.Println("Commit it: the Antigravity agent reads .agents/hooks.json from the repository its Gemini API sandbox mounts.")
	printProjectHooksNote()
	return nil
}

func commandAgyUninstall(c *cli.Context) error {
	color.Yellow.Println("Removing Antigravity hooks...")

	svc := model.NewAntigravityHooksService()
	if c.Bool("project") {
		root, err := currentProjectRoot()
		if err != nil {
			color.Red.Printf("Failed to find the project: %v\n", err)
			return err
		}
		svc = model.NewAntigravityProjectHooksService(root)
	}
	if err := svc.Uninstall(); err != nil {
		color.Red.Printf("Failed to remove the Antigravity hooks: %v\n", err)
		return err
	}
	color.Green.Printf("Antigravity hooks removed from %s\n", svc.Path())
	return nil
}

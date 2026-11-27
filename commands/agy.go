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
	Action:  commandAgyInstall,
}

var AgyUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove the ShellTime hooks from ~/.gemini/config/hooks.json",
	Action:  commandAgyUninstall,
}

func commandAgyInstall(c *cli.Context) error {
	if err := installAgyHooks(); err != nil {
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

func commandAgyUninstall(c *cli.Context) error {
	color.Yellow.Println("Removing Antigravity hooks...")

	svc := model.NewAntigravityHooksService()
	if err := svc.Uninstall(); err != nil {
		color.Red.Printf("Failed to remove the Antigravity hooks: %v\n", err)
		return err
	}
	color.Green.Printf("Antigravity hooks removed from %s\n", svc.Path())
	return nil
}

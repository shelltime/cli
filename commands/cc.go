package commands

import (
	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var CCCommand = &cli.Command{
	Name:  "cc",
	Usage: "Claude Code integration commands",
	Subcommands: []*cli.Command{
		CCInstallCommand,
		CCUninstallCommand,
	},
}

var CCInstallCommand = &cli.Command{
	Name:    "install",
	Aliases: []string{"i"},
	Usage:   "Install Claude Code OTEL environment configuration to shell config files",
	Action:  commandCCInstall,
}

var CCUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove Claude Code OTEL environment configuration from shell config files",
	Action:  commandCCUninstall,
}

func commandCCInstall(c *cli.Context) error {
	color.Yellow.Println("Installing Claude Code OTEL configuration...")

	// Create shell services
	zshService := model.NewZshCCOtelEnvService()
	fishService := model.NewFishCCOtelEnvService()
	bashService := model.NewBashCCOtelEnvService()

	// Install for all shells (non-blocking failures)
	if err := zshService.Install(); err != nil {
		color.Red.Printf("Failed to install for zsh: %v\n", err)
	}

	if err := fishService.Install(); err != nil {
		color.Red.Printf("Failed to install for fish: %v\n", err)
	}

	if err := bashService.Install(); err != nil {
		color.Red.Printf("Failed to install for bash: %v\n", err)
	}

	color.Green.Println("Claude Code OTEL configuration has been installed!")
	color.Yellow.Println("Please restart your shell or source your config file to apply changes.")

	return nil
}

func commandCCUninstall(c *cli.Context) error {
	color.Yellow.Println("Removing Claude Code OTEL configuration...")

	// Create shell services
	zshService := model.NewZshCCOtelEnvService()
	fishService := model.NewFishCCOtelEnvService()
	bashService := model.NewBashCCOtelEnvService()

	// Uninstall from all shells
	if err := zshService.Uninstall(); err != nil {
		color.Red.Printf("Failed to uninstall from zsh: %v\n", err)
	}

	if err := fishService.Uninstall(); err != nil {
		color.Red.Printf("Failed to uninstall from fish: %v\n", err)
	}

	if err := bashService.Uninstall(); err != nil {
		color.Red.Printf("Failed to uninstall from bash: %v\n", err)
	}

	color.Green.Println("Claude Code OTEL configuration has been removed!")

	return nil
}

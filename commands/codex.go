package commands

import (
	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var CodexCommand = &cli.Command{
	Name:  "codex",
	Usage: "OpenAI Codex integration commands",
	Subcommands: []*cli.Command{
		CodexInstallCommand,
		CodexUninstallCommand,
	},
}

var CodexInstallCommand = &cli.Command{
	Name:    "install",
	Aliases: []string{"i"},
	Usage:   "Install Codex OTEL configuration to ~/.codex/config.toml",
	Action:  commandCodexInstall,
}

var CodexUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove ShellTime OTEL configuration from ~/.codex/config.toml",
	Action:  commandCodexUninstall,
}

func commandCodexInstall(c *cli.Context) error {
	color.Yellow.Println("Installing Codex OTEL configuration...")

	service := model.NewCodexOtelConfigService()

	if err := service.Install(); err != nil {
		color.Red.Printf("Failed to install Codex OTEL config: %v\n", err)
		return err
	}

	color.Green.Println("Codex OTEL configuration has been installed to ~/.codex/config.toml")
	color.Yellow.Println("The Codex CLI will now send telemetry to ShellTime daemon.")

	return nil
}

func commandCodexUninstall(c *cli.Context) error {
	color.Yellow.Println("Removing Codex OTEL configuration...")

	service := model.NewCodexOtelConfigService()

	if err := service.Uninstall(); err != nil {
		color.Red.Printf("Failed to uninstall Codex OTEL config: %v\n", err)
		return err
	}

	color.Green.Println("Codex OTEL configuration has been removed from ~/.codex/config.toml")

	return nil
}

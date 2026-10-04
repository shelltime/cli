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
		CCStatuslineCommand,
	},
}

var CCInstallCommand = &cli.Command{
	Name:    "install",
	Aliases: []string{"i"},
	Usage:   "Install Claude Code OTEL environment configuration into ~/.claude/settings.json",
	Action:  commandCCInstall,
}

var CCUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Remove Claude Code OTEL environment configuration from ~/.claude/settings.json",
	Action:  commandCCUninstall,
}

// legacyAICodeOtelEnvServices returns the shell rc services that older versions of `cc install`
// wrote the OTEL block to.
func legacyAICodeOtelEnvServices() []model.AICodeOtelEnvService {
	return []model.AICodeOtelEnvService{
		model.NewZshAICodeOtelEnvService(),
		model.NewFishAICodeOtelEnvService(),
		model.NewBashAICodeOtelEnvService(),
	}
}

func commandCCInstall(c *cli.Context) error {
	color.Yellow.Println("Installing Claude Code OTEL configuration...")

	// settings.json env reaches every Claude Code session (terminal, desktop app, IDE),
	// unlike shell rc exports.
	settingsService := model.NewClaudeSettingsAICodeOtelEnvService()
	if err := settingsService.Install(); err != nil {
		color.Red.Printf("Failed to install into %s: %v\n", settingsService.SettingsPath(), err)
		return err
	}

	// Migrate: drop the shell rc blocks written by older versions. Only touch files that have one.
	for _, shellService := range legacyAICodeOtelEnvServices() {
		if shellService.Check() != nil {
			continue
		}
		if err := shellService.Uninstall(); err != nil {
			color.Red.Printf("Failed to remove legacy OTEL block for %s: %v\n", shellService.ShellName(), err)
			continue
		}
		color.Green.Printf("Removed legacy shell OTEL block for %s\n", shellService.ShellName())
	}

	color.Green.Println("Claude Code OTEL configuration has been installed!")
	color.Yellow.Println("Please restart your Claude Code sessions (terminal, desktop app, IDE) to apply changes.")

	return nil
}

func commandCCUninstall(c *cli.Context) error {
	color.Yellow.Println("Removing Claude Code OTEL configuration...")

	settingsService := model.NewClaudeSettingsAICodeOtelEnvService()
	if err := settingsService.Uninstall(); err != nil {
		color.Red.Printf("Failed to uninstall from %s: %v\n", settingsService.SettingsPath(), err)
	}

	// Also clean up shell rc blocks written by older versions
	for _, shellService := range legacyAICodeOtelEnvServices() {
		if err := shellService.Uninstall(); err != nil {
			color.Red.Printf("Failed to uninstall from %s: %v\n", shellService.ShellName(), err)
		}
	}

	color.Green.Println("Claude Code OTEL configuration has been removed!")

	return nil
}

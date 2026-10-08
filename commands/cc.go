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
		CCBackfillCommand,
		CCPullRequestCommand,
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
	printCCPrivacyNote(settingsService.SettingsPath())

	return nil
}

// printCCPrivacyNote says what the installed config sends beyond token and cost counts.
func printCCPrivacyNote(settingsPath string) {
	color.Gray.Println("Privacy: besides usage and cost, Claude Code now sends your prompts, tool details (Bash")
	color.Gray.Println("commands, file paths, truncated tool input, tool errors) and assistant responses to your")
	color.Gray.Printf("ShellTime account. To keep some of them out, set OTEL_LOG_USER_PROMPTS, OTEL_LOG_TOOL_DETAILS or\n")
	color.Gray.Printf("OTEL_LOG_ASSISTANT_RESPONSES to 0 in the env of %s (re-running install resets them).\n", settingsPath)
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

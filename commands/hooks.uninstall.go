// cli/commands/hooks.uninstall.go
package commands

import (
	"os"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var HooksUninstallCommand = &cli.Command{
	Name:   "uninstall",
	Usage:  "Uninstall shelltime shell hooks",
	Action: commandHooksUninstall,
}

func commandHooksUninstall(c *cli.Context) error {
	color.Yellow.Println("🔍 Starting hooks uninstallation...")

	shell := os.Getenv("SHELL")
	color.Blue.Println("Current shell:", shell)

	// Create shell services
	zshService := model.NewZshHookService()
	fishService := model.NewFishHookService()
	bashService := model.NewBashHookService()

	// Uninstall hooks for all shells
	if err := zshService.Uninstall(); err != nil {
		color.Red.Printf("❌ Failed to uninstall zsh hook: %v\n", err)
		return err
	}

	if err := fishService.Uninstall(); err != nil {
		color.Red.Printf("❌ Failed to uninstall fish hook: %v\n", err)
		return err
	}

	if err := bashService.Uninstall(); err != nil {
		color.Red.Printf("❌ Failed to uninstall bash hook: %v\n", err)
		return err
	}

	color.Green.Println("✅ Shell hooks have been successfully uninstalled!")
	return nil
}

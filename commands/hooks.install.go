package commands

import (
	"os"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var HooksInstallCommand = &cli.Command{
	Name:   "install",
	Usage:  "Install shelltime shell hooks",
	Action: commandHooksInstall,
}

func commandHooksInstall(c *cli.Context) error {
	binFolder := os.ExpandEnv("$HOME/.shelltime/bin")
	if _, err := os.Stat(binFolder); os.IsNotExist(err) {
		color.Red.Println("📁 cannot find bin folder at", binFolder)
		color.Red.Println("Please run 'curl -sSL https://raw.githubusercontent.com/malamtime/installation/master/install.bash | bash' first")
		return nil
	}

	color.Yellow.Println("🔍 Starting hooks installation...")

	// Create shell services
	zshService := model.NewZshHookService()
	fishService := model.NewFishHookService()

	// Install hooks for both shells
	if err := zshService.Install(); err != nil {
		color.Red.Printf("❌ Failed to install zsh hook: %v\n", err)
		// return err // Decide if one failure should stop all
	}

	if err := fishService.Install(); err != nil {
		color.Red.Printf("❌ Failed to install fish hook: %v\n", err)
		// return err // Decide if one failure should stop all
	}

	color.Green.Println("✅ Shell hooks have been successfully installed!")
	return nil
}

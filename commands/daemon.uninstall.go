package commands

import (
	"fmt"
	"os/user"
	"path/filepath"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var DaemonUninstallCommand = &cli.Command{
	Name:   "uninstall",
	Usage:  "Uninstall the shelltime daemon service",
	Action: commandDaemonUninstall,
}

func commandDaemonUninstall(c *cli.Context) error {
	color.Yellow.Println("🔍 Starting daemon service uninstallation...")

	// Get current user's home directory and username
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	baseFolder := filepath.Join(currentUser.HomeDir, ".shelltime")
	username := currentUser.Username

	installer, err := model.NewDaemonInstaller(baseFolder, username)
	if err != nil {
		return err
	}

	// Unregister and remove the service
	if err := installer.UnregisterService(); err != nil {
		return fmt.Errorf("failed to unregister service: %w", err)
	}

	// No need to remove system-wide symlink for user-level installation
	color.Yellow.Println("🗑  User-level daemon service cleanup completed...")

	color.Green.Println("✅ Daemon service has been successfully uninstalled!")
	// color.Yellow.Println("ℹ️  Note: Your commands will now be synced to shelltime.xyz on the next login")
	return nil
}

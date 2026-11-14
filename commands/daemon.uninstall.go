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
	if err := daemonUninstall(); err != nil {
		return err
	}

	color.Green.Println("✅ Daemon uninstalled")
	return nil
}

// daemonUninstall removes the daemon service and prints nothing: its callers
// report the outcome in one line.
func daemonUninstall() error {
	// Get current user's home directory and username
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	baseFolder := filepath.Join(currentUser.HomeDir, ".shelltime")
	username := currentUser.Username

	installer, err := model.NewDaemonInstaller(baseFolder, username, "")
	if err != nil {
		return err
	}

	// Unregister and remove the service
	if err := installer.UnregisterService(); err != nil {
		return fmt.Errorf("failed to unregister service: %w", err)
	}
	return nil
}

package commands

import (
	"github.com/gookit/color"
	"github.com/urfave/cli/v2"
)

// DaemonReinstallCommand is also `daemon restart`, the name people reach for
// after a config change. It stays a full reinstall rather than a plain service
// restart: rewriting the service definition is what points it at the current
// binary after an update or a move between Homebrew and the curl installer.
var DaemonReinstallCommand = &cli.Command{
	Name:    "reinstall",
	Aliases: []string{"restart"},
	Usage:   "Reinstall and restart the shelltime daemon service (uninstall then install)",
	Action:  commandDaemonReinstall,
}

func commandDaemonReinstall(c *cli.Context) error {
	color.Yellow.Println("🔄 Starting daemon service reinstallation...")

	// First, uninstall the existing service
	color.Yellow.Println("🗑  Uninstalling existing daemon service...")
	if err := commandDaemonUninstall(c); err != nil {
		return err
	}

	// Then, install the service
	color.Yellow.Println("📦 Installing daemon service...")
	if err := commandDaemonInstall(c); err != nil {
		return err
	}

	color.Green.Println("✅ Daemon service has been successfully reinstalled!")
	return nil
}

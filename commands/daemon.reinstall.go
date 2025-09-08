package commands

import (
	"github.com/gookit/color"
	"github.com/urfave/cli/v2"
)

var DaemonReinstallCommand = &cli.Command{
	Name:   "reinstall",
	Usage:  "Reinstall the shelltime daemon service (uninstall then install)",
	Action: commandDaemonReinstall,
}

func commandDaemonReinstall(c *cli.Context) error {
	color.Yellow.Println("🔄 Starting daemon service reinstallation...")

	// First, uninstall the existing service
	color.Yellow.Println("🗑 Uninstalling existing daemon service...")
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
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var DaemonInstallCommand *cli.Command = &cli.Command{
	Name:   "install",
	Usage:  "Install the shelltime daemon service",
	Action: commandDaemonInstall,
}

func commandDaemonInstall(c *cli.Context) error {
	color.Yellow.Println("⚠️ Warning: This daemon service is currently not ready for use. Please proceed with caution.")

	// Check if running as root
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (sudo shelltime daemon install)")
	}
	color.Yellow.Println("🔍 Detecting system architecture...")

	// TODO: the username is not stable in multiple user system
	baseFolder, username, err := model.SudoGetBaseFolder()
	if err != nil {
		return err
	}

	installer, err := model.NewDaemonInstaller(baseFolder, username)
	if err != nil {
		return err
	}

	installer.CheckAndStopExistingService()

	// check latest file exist or not
	if _, err := os.Stat(filepath.Join(baseFolder, "bin/shelltime-daemon.bak")); err == nil {
		color.Yellow.Println("🔄 Found latest daemon file, restoring...")
		// try to remove old file
		_ = os.Remove(filepath.Join(baseFolder, "bin/shelltime-daemon"))
		// rename .bak to original
		if err := os.Rename(
			filepath.Join(baseFolder, "bin/shelltime-daemon.bak"),
			filepath.Join(baseFolder, "bin/shelltime-daemon"),
		); err != nil {
			return fmt.Errorf("failed to restore latest daemon: %w", err)
		}
	}

	// check shelltime-daemon
	if _, err := os.Stat(filepath.Join(baseFolder, "bin/shelltime-daemon")); err != nil {
		color.Yellow.Println("⚠️ shelltime-daemon not found, please reinstall the CLI first:")
		color.Yellow.Println("curl -sSL https://raw.githubusercontent.com/malamtime/installation/master/install.bash | bash")
		return nil
	}

	// Copy to final location
	binaryPath := "/usr/local/bin/shelltime-daemon"

	if _, err := os.Stat(binaryPath); err != nil {
		color.Yellow.Println("🔍 Creating daemon symlink...")
		if err := os.Symlink(filepath.Join(baseFolder, "bin/shelltime-daemon"), binaryPath); err != nil {
			return fmt.Errorf("failed to create daemon symlink: %w", err)
		}
	}

	if err := installer.InstallService(username); err != nil {
		return err
	}

	if err := installer.RegisterService(); err != nil {
		return err
	}

	if err := installer.StartService(); err != nil {
		return err
	}

	color.Green.Println("✅ Daemon service has been installed and started successfully!")
	color.Green.Println("💡 Your commands will now be automatically synced to shelltime.xyz faster")
	return nil
}

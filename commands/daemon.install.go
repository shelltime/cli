package commands

import (
	"fmt"
	"os"
	"os/user"
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
	color.Yellow.Println("🔍 Detecting system architecture...")

	// Get current user's home directory and username
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	baseFolder := filepath.Join(currentUser.HomeDir, ".shelltime")
	username := currentUser.Username

	// Handle .bak upgrade for curl-installer users
	bakPath := filepath.Join(baseFolder, "bin/shelltime-daemon.bak")
	if _, err := os.Stat(bakPath); err == nil {
		color.Yellow.Println("🔄 Found latest daemon file, restoring...")
		_ = os.Remove(filepath.Join(baseFolder, "bin/shelltime-daemon"))
		if err := os.Rename(bakPath, filepath.Join(baseFolder, "bin/shelltime-daemon")); err != nil {
			return fmt.Errorf("failed to restore latest daemon: %w", err)
		}
	}

	// Resolve daemon binary (curl-installer, Homebrew, or PATH)
	daemonBinPath, err := model.ResolveDaemonBinaryPath()
	if err != nil {
		color.Yellow.Println("⚠️ shelltime-daemon not found.")
		color.Yellow.Println("Install via Homebrew:  brew install shelltime/tap/shelltime")
		color.Yellow.Println("Or via curl installer: curl -sSL https://shelltime.xyz/i | bash")
		return nil
	}
	color.Green.Printf("✅ Found daemon binary at: %s\n", daemonBinPath)

	installer, err := model.NewDaemonInstaller(baseFolder, username, daemonBinPath)
	if err != nil {
		return err
	}

	installer.CheckAndStopExistingService()

	// User-level installation - no system-wide symlink needed
	color.Yellow.Println("🔍 Setting up user-level daemon installation...")

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

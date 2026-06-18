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

	// Resolve daemon binary (Homebrew/PATH preferred, curl-installer fallback).
	// On miss, try to auto-download a matching daemon archive from GitHub
	// releases so curl-installer users from before the daemon-bundling fix
	// don't have to rerun the installer.
	daemonBinPath, err := model.ResolveDaemonBinaryPath()
	if err != nil {
		color.Yellow.Println("⚠️  shelltime-daemon binary not found locally.")
		cliPath, _ := model.ResolveCLIBinaryPath()
		color.Yellow.Println("⬇️  Attempting to download matching daemon from GitHub releases...")
		daemonBinPath, err = model.EnsureDaemonBinary(c.Context, cliPath, commitID)
		if err != nil {
			color.Red.Println("❌ shelltime-daemon binary not found and auto-download failed.")
			color.Yellow.Printf("   reason: %v\n", err)
			color.Yellow.Println("Install via Homebrew:  brew install shelltime/tap/shelltime")
			color.Yellow.Println("Or via curl installer: curl -sSL https://shelltime.xyz/i | bash")
			return fmt.Errorf("shelltime-daemon binary not found: %w", err)
		}
		color.Green.Printf("✅ Downloaded daemon binary to: %s\n", daemonBinPath)
	} else {
		color.Green.Printf("✅ Found daemon binary at: %s\n", daemonBinPath)
	}

	// If we picked a system-managed binary but a curl-installer copy still
	// lives under ~/.shelltime/bin, rename it to shelltime-daemon.bak rather
	// than delete it. Future resolution stays unambiguous AND the .bak
	// recovery branch above can restore it on the next `daemon install` —
	// no GitHub re-download when the user later clears the system binary.
	curlDaemonPath := model.GetCurlInstallerDaemonPath()
	if shouldPreserveCurlDaemon(daemonBinPath, curlDaemonPath) {
		if info, statErr := os.Stat(curlDaemonPath); statErr == nil && !info.IsDir() {
			preservedPath := curlDaemonPath + ".bak"
			_ = os.Remove(preservedPath)
			if rnErr := os.Rename(curlDaemonPath, preservedPath); rnErr == nil {
				color.Yellow.Printf("📦 Preserved curl-installer daemon as %s (auto-restores on next `daemon install`).\n", preservedPath)
			} else {
				color.Yellow.Printf("⚠️ Could not preserve curl-installer daemon at %s: %v\n", curlDaemonPath, rnErr)
			}
		}
	}

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

// shouldPreserveCurlDaemon reports whether the curl-installer daemon at
// curlDaemonPath should be renamed to .bak before installing the service. It is
// true only when the resolved daemon is a genuinely different on-disk file — so
// we never move away the binary the service is about to point at, even when the
// resolved path is just a symlink/alias of the curl binary (the cause of the
// "shelltime-daemon.bak but no shelltime-daemon" stuck loop on Linux).
func shouldPreserveCurlDaemon(daemonBinPath, curlDaemonPath string) bool {
	return daemonBinPath != curlDaemonPath && !model.SameDaemonFile(daemonBinPath, curlDaemonPath)
}

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
	if err := daemonInstall(c); err != nil {
		return err
	}

	color.Green.Println("✅ Daemon installed and started")
	return nil
}

// daemonInstall installs and starts the daemon service. It prints only what
// went wrong or took a detour (a daemon binary it had to download, a copy it
// could not move aside): its callers report success in one line.
func daemonInstall(c *cli.Context) error {
	// Get current user's home directory and username
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	baseFolder := filepath.Join(currentUser.HomeDir, ".shelltime")
	username := currentUser.Username

	// Bring back a curl-installer daemon preserved as .bak by a previous install
	if _, err := restoreCurlDaemonBak(filepath.Join(baseFolder, "bin", "shelltime-daemon")); err != nil {
		return err
	}

	// Resolve daemon binary (Homebrew/PATH preferred, curl-installer fallback).
	// On miss, try to auto-download a matching daemon archive from GitHub
	// releases so curl-installer users from before the daemon-bundling fix
	// don't have to rerun the installer.
	daemonBinPath, err := model.ResolveDaemonBinaryPath()
	if err != nil {
		cliPath, _ := model.ResolveCLIBinaryPath()
		color.Yellow.Println("⬇️  shelltime-daemon not found locally, downloading it from GitHub releases...")
		daemonBinPath, err = model.EnsureDaemonBinary(c.Context, cliPath, commitID)
		if err != nil {
			color.Red.Println("❌ shelltime-daemon binary not found and auto-download failed.")
			color.Yellow.Printf("   reason: %v\n", err)
			color.Yellow.Println("Install via Homebrew:  brew install shelltime/tap/shelltime")
			color.Yellow.Println("Or via curl installer: curl -sSL https://shelltime.xyz/i | bash")
			return fmt.Errorf("shelltime-daemon binary not found: %w", err)
		}
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
			if rnErr := os.Rename(curlDaemonPath, preservedPath); rnErr != nil {
				color.Yellow.Printf("⚠️ Could not preserve curl-installer daemon at %s: %v\n", curlDaemonPath, rnErr)
			}
		}
	}

	installer, err := model.NewDaemonInstaller(baseFolder, username, daemonBinPath)
	if err != nil {
		return err
	}

	installer.CheckAndStopExistingService()

	if err := installer.InstallService(username); err != nil {
		return err
	}

	if err := installer.RegisterService(); err != nil {
		return err
	}

	return installer.StartService()
}

// restoreCurlDaemonBak moves curlDaemonPath+".bak" back to curlDaemonPath, but
// only when curlDaemonPath itself is missing. A live daemon there is whatever
// the installer or `shelltime update` just put down; overwriting it with an
// older .bak is how a reinstall ended up running the previous release.
func restoreCurlDaemonBak(curlDaemonPath string) (bool, error) {
	bakPath := curlDaemonPath + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		return false, nil
	}
	if _, err := os.Stat(curlDaemonPath); err == nil {
		return false, nil
	}
	if err := os.Rename(bakPath, curlDaemonPath); err != nil {
		return false, fmt.Errorf("failed to restore daemon from %s: %w", bakPath, err)
	}
	return true, nil
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

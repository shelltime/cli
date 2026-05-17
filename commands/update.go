package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var UpdateCommand *cli.Command = &cli.Command{
	Name:  "update",
	Usage: "Download and install the latest shelltime release in place",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "check",
			Aliases: []string{"c"},
			Usage:   "Only report current vs latest version, do not install",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Proceed even if already on the latest version or running a dev build",
		},
		&cli.BoolFlag{
			Name:  "skip-daemon-reinstall",
			Usage: "Skip refreshing the daemon service after replacing binaries",
		},
	},
	Action: commandUpdate,
}

func commandUpdate(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "update")
	defer span.End()

	check := c.Bool("check")
	force := c.Bool("force")
	skipDaemonReinstall := c.Bool("skip-daemon-reinstall")

	color.Yellow.Println("🔍 Checking for updates...")

	cliPath, err := model.ResolveCLIBinaryPath()
	if err != nil {
		return fmt.Errorf("resolve running binary path: %w", err)
	}

	switch model.DetectInstallKind(cliPath) {
	case model.InstallKindHomebrew:
		color.Yellow.Println("📦 Detected Homebrew installation.")
		color.Yellow.Println("   Run: brew upgrade shelltime/tap/shelltime")
		return nil
	case model.InstallKindUnknown:
		color.Yellow.Printf("⚠️  Binary at %s is not in a known auto-updatable location.\n", cliPath)
		color.Yellow.Println("   Reinstall via the curl installer or Homebrew to enable in-place updates.")
		return nil
	}

	latest, err := model.FetchLatestVersion(ctx)
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}

	current := commitID
	if current == "" {
		current = "dev"
	}
	normalizedLatest := model.NormalizeVersion(latest)
	normalizedCurrent := model.NormalizeVersion(current)

	color.Cyan.Printf("   Current: %s\n", current)
	color.Cyan.Printf("   Latest:  %s\n", latest)

	if check {
		if normalizedLatest == normalizedCurrent {
			color.Green.Println("✅ Already on the latest version.")
		} else {
			color.Yellow.Println("⬆️  An update is available. Run `shelltime update` to install it.")
		}
		return nil
	}

	if current == "dev" && !force {
		color.Yellow.Println("⚠️  Refusing to overwrite a dev build. Use --force to proceed anyway.")
		return nil
	}

	if normalizedLatest == normalizedCurrent && !force {
		color.Green.Println("✅ Already on the latest version. Use --force to reinstall.")
		return nil
	}

	archiveName, err := model.BuildArchiveName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	downloadURL := model.BuildDownloadURL(latest, archiveName)

	expectedSum, ok, err := model.FetchChecksum(ctx, latest, archiveName)
	if err != nil {
		color.Yellow.Printf("⚠️  Could not fetch checksums.txt: %v (proceeding without verification)\n", err)
	} else if !ok {
		color.Yellow.Println("⚠️  No checksum entry for this archive — proceeding without verification.")
	}

	tmpDir, err := os.MkdirTemp("", "shelltime-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, archiveName)
	color.Yellow.Printf("⬇️  Downloading %s ...\n", archiveName)
	if err := model.DownloadAndVerify(ctx, downloadURL, expectedSum, archivePath); err != nil {
		return fmt.Errorf("download release: %w", err)
	}

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	binaries, err := model.ExtractBinaries(archivePath, extractDir)
	if err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}
	if _, ok := binaries["shelltime"]; !ok {
		return fmt.Errorf("archive %s did not contain a shelltime binary", archiveName)
	}

	color.Yellow.Println("🔄 Replacing binaries...")

	if err := model.ReplaceBinary(binaries["shelltime"], cliPath); err != nil {
		return fmt.Errorf("replace shelltime binary: %w", err)
	}
	color.Green.Printf("   shelltime -> %s\n", cliPath)

	if daemonSrc, ok := binaries["shelltime-daemon"]; ok {
		daemonDest := resolveDaemonDest()
		if err := model.ReplaceBinary(daemonSrc, daemonDest); err != nil {
			return fmt.Errorf("replace shelltime-daemon binary: %w", err)
		}
		color.Green.Printf("   shelltime-daemon -> %s\n", daemonDest)
	}

	if shouldReinstallDaemon(ctx, skipDaemonReinstall) {
		color.Yellow.Println("🔁 Refreshing daemon service...")
		if err := commandDaemonReinstall(c); err != nil {
			color.Yellow.Printf("⚠️  Daemon reinstall reported an error: %v\n", err)
			color.Yellow.Println("   You can rerun `shelltime daemon reinstall` manually.")
		}
	} else {
		color.Yellow.Println("ℹ️  Skipping daemon reinstall. Run `shelltime daemon reinstall` to pick up the new binary.")
	}

	color.Green.Printf("✅ Updated to %s. Restart your shell to use the new binary.\n", latest)
	return nil
}

// resolveDaemonDest returns the path the daemon binary should be written to —
// the existing daemon location if installed, otherwise the curl-installer default.
func resolveDaemonDest() string {
	if p, err := model.ResolveDaemonBinaryPath(); err == nil {
		return p
	}
	return filepath.Join(model.GetBinFolderPath(), "shelltime-daemon")
}

// shouldReinstallDaemon decides whether to call commandDaemonReinstall after a
// binary swap.
func shouldReinstallDaemon(_ context.Context, skipFlag bool) bool {
	if skipFlag {
		return false
	}
	if runtime.GOOS == "windows" {
		return false
	}
	if _, err := model.ResolveDaemonBinaryPath(); err != nil {
		return false
	}
	installer, err := model.NewDaemonInstaller("", "", "")
	if err != nil {
		slog.Debug("skip daemon reinstall: installer factory failed", slog.Any("err", err))
		return false
	}
	if err := installer.Check(); err != nil {
		return false
	}
	return true
}

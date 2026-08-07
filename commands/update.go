package commands

import (
	"fmt"
	"log/slog"
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
		&cli.BoolFlag{
			Name:  "allow-unverified",
			Usage: "Install even when no checksum is available (not recommended)",
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
	allowUnverified := c.Bool("allow-unverified")

	color.Yellow.Println("🔍 Checking for updates...")

	cliPath, err := model.ResolveCLIBinaryPath()
	if err != nil {
		return fmt.Errorf("resolve running binary path: %w", err)
	}

	installKind := model.DetectInstallKind(cliPath)

	// Resolve the latest tag through the server when we can, so this works in
	// regions that cannot reach github.com; fall back to the GitHub API.
	cfg, cfgErr := configService.ReadConfigFile(ctx)
	source := model.ReleaseSource{}
	if cfgErr == nil {
		source = model.NewReleaseSource(cfg.APIEndpoint)
	}

	var (
		latest      string
		assetSha    string
		archiveName string
	)
	if cfgErr == nil && cfg.Token != "" {
		if rel, relErr := model.FetchLatestCLIRelease(ctx, cfg); relErr == nil && rel.Tag != "" {
			latest = rel.Tag
			if rel.Asset != nil {
				assetSha = rel.Asset.Sha256
				archiveName = rel.Asset.Name
			}
		} else if relErr != nil {
			slog.Debug("release lookup via API failed, falling back to GitHub", slog.Any("err", relErr))
		}
	}
	if latest == "" {
		latest, err = model.FetchLatestVersion(ctx)
		if err != nil {
			return fmt.Errorf("fetch latest release: %w", err)
		}
		// Without the API we have no proxy-provided checksum, and the proxy may
		// not be reachable either; use GitHub for the download too.
		source = model.ReleaseSource{}
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

	// Homebrew and unknown locations are reported after the version check, so
	// `--check` still works there.
	switch installKind {
	case model.InstallKindHomebrew:
		color.Yellow.Println("📦 Detected Homebrew installation.")
		color.Yellow.Println("   Run: brew upgrade --cask shelltime/tap/shelltime")
		return nil
	case model.InstallKindUnknown:
		color.Yellow.Printf("⚠️  Binary at %s is not in a known auto-updatable location.\n", cliPath)
		color.Yellow.Println("   Reinstall via the curl installer or Homebrew to enable in-place updates.")
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

	manageDaemon := shouldManageDaemon(skipDaemonReinstall)
	// Snapshot this BEFORE the swap: it decides reinstall-vs-install, and the
	// swap itself makes the running service unreachable.
	daemonWasRunning := manageDaemon && daemonServiceIsRunning()

	daemonDest := ""
	if manageDaemon {
		daemonDest = resolveDaemonDest()
	}

	color.Yellow.Printf("⬇️  Downloading %s ...\n", latest)
	res, err := model.ApplyUpdate(ctx, model.UpdatePlan{
		Tag:         latest,
		ArchiveName: archiveName,
		Source:      source,
		ExpectedSha: assetSha,
		CLIDest:     cliPath,
		DaemonDest:  daemonDest,
		// Interactive users may knowingly proceed without a checksum; the
		// unattended daemon path never does.
		AllowUnverified: allowUnverified,
		BackupSuffix:    model.BackupSuffixUpdate,
	})
	if err != nil {
		return err
	}

	if !res.Verified {
		color.Yellow.Println("⚠️  Installed without checksum verification (--allow-unverified).")
	}
	if res.UsedFallback {
		color.Yellow.Println("ℹ️  Release proxy was unavailable; downloaded from GitHub directly.")
	}
	color.Green.Printf("   shelltime -> %s\n", cliPath)
	if res.ReplacedDaemon {
		color.Green.Printf("   shelltime-daemon -> %s\n", daemonDest)
	}

	if manageDaemon {
		color.Yellow.Println("🔁 Refreshing daemon service...")
		if err := ensureDaemonRunning(c, daemonWasRunning); err != nil {
			color.Yellow.Printf("⚠️  Daemon did not come back up: %v\n", err)
			color.Yellow.Println("   Run `shelltime daemon install` to start it manually.")
		} else {
			color.Green.Println("   daemon service is running")
		}
	} else {
		color.Yellow.Println("ℹ️  Skipping daemon refresh. Run `shelltime daemon reinstall` to pick up the new binary.")
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

// shouldManageDaemon reports whether this machine has a daemon we are
// responsible for after a binary swap.
//
// It deliberately does NOT consider whether the service is currently running.
// It used to, which meant a stopped daemon was left stopped forever — and since
// the daemon drives the auto-update check, that also killed auto-update. Whether
// it is running now only decides reinstall-vs-install; see ensureDaemonRunning.
func shouldManageDaemon(skipFlag bool) bool {
	if skipFlag {
		return false
	}
	if runtime.GOOS == "windows" {
		return false
	}
	if _, err := model.ResolveDaemonBinaryPath(); err != nil {
		return false
	}
	if _, err := model.NewDaemonInstaller("", "", ""); err != nil {
		slog.Debug("skip daemon management: installer factory failed", slog.Any("err", err))
		return false
	}
	return true
}

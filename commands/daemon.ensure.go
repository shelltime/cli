package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/malamtime/cli/daemon"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

// daemonReadyTimeout bounds how long ensureDaemonRunning waits for the service
// to actually come up. Package vars so tests can shrink them.
var (
	daemonReadyTimeout  = 10 * time.Second
	daemonReadyInterval = 200 * time.Millisecond
)

// Seams for tests.
var (
	ensureInstallDaemon   = commandDaemonInstall
	ensureReinstallDaemon = commandDaemonReinstall
)

// ensureDaemonRunning installs (or reinstalls) the daemon service and verifies
// it actually came up. It is safe to call whether or not the service is
// currently registered or running.
//
// Every code path that touches the daemon binary funnels through here, so a
// stopped daemon is always brought back — including when a swap or smoke test
// failed. This matters beyond convenience: the daemon is what drives the
// auto-update check, so a daemon that stays down takes auto-update with it.
func ensureDaemonRunning(c *cli.Context, wasRunning bool) error {
	if wasRunning {
		// A registered, running service needs the full unload/load cycle to pick
		// up a new binary.
		if err := ensureReinstallDaemon(c); err != nil {
			slog.Warn("daemon reinstall failed, falling back to install", slog.Any("err", err))
			if err := ensureInstallDaemon(c); err != nil {
				return fmt.Errorf("reinstall and install both failed: %w", err)
			}
		}
	} else {
		// Skip the uninstall half: `launchctl unload` / `systemctl disable` on a
		// service that was never registered just produces noise and errors.
		if err := ensureInstallDaemon(c); err != nil {
			return fmt.Errorf("install daemon service: %w", err)
		}
	}

	return waitForDaemonReady(c.Context)
}

// waitForDaemonReady polls until the service manager reports the service
// registered AND the daemon is listening on its socket.
//
// StartService() returning nil only means `launchctl load` / `systemctl start`
// was accepted — not that the process is alive and serving. If the first poll
// window expires we try one explicit StartService (covers "registered but not
// loaded") before giving up.
func waitForDaemonReady(ctx context.Context) error {
	if daemonIsReady(ctx) {
		return nil
	}
	if pollDaemonReady(ctx, daemonReadyTimeout) {
		return nil
	}

	installer, err := buildDaemonInstaller()
	if err == nil {
		if startErr := installer.StartService(); startErr != nil {
			slog.Debug("explicit StartService failed", slog.Any("err", startErr))
		}
		if pollDaemonReady(ctx, daemonReadyTimeout) {
			return nil
		}
	}

	return fmt.Errorf("daemon did not become ready within %s", daemonReadyTimeout)
}

func pollDaemonReady(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(daemonReadyInterval):
		}
		if daemonIsReady(ctx) {
			return true
		}
	}
	return false
}

// daemonIsReady reports whether the daemon is both registered with the service
// manager and accepting socket connections. The socket check is the meaningful
// one; the installer check catches "process alive but service unregistered".
func daemonIsReady(ctx context.Context) bool {
	if !daemon.IsSocketReady(ctx, resolveSocketPath(ctx)) {
		return false
	}
	installer, err := buildDaemonInstaller()
	if err != nil {
		// No installer on this platform — the socket answering is all we can check.
		return true
	}
	return installer.Check() == nil
}

// daemonServiceIsRunning reports whether the service manager currently considers
// the daemon running. Used to decide reinstall-vs-install.
func daemonServiceIsRunning() bool {
	installer, err := buildDaemonInstaller()
	if err != nil {
		return false
	}
	return installer.Check() == nil
}

func buildDaemonInstaller() (model.DaemonInstaller, error) {
	currentUser, err := user.Current()
	if err != nil {
		return model.NewDaemonInstaller("", "", "")
	}
	baseFolder := filepath.Join(currentUser.HomeDir, ".shelltime")
	return model.NewDaemonInstaller(baseFolder, currentUser.Username, "")
}

// resolveSocketPath returns the configured socket path, falling back to the
// default when config is unavailable. configService is nil until InjectVar runs,
// so guard it: this is reached from background repair paths that must never
// panic in the user's shell.
func resolveSocketPath(ctx context.Context) string {
	if configService == nil {
		return model.DefaultSocketPath
	}
	cfg, err := configService.ReadConfigFile(ctx)
	if err == nil && cfg.SocketPath != "" {
		return cfg.SocketPath
	}
	return model.DefaultSocketPath
}

// daemonServiceFileExists reports whether the daemon service definition has ever
// been installed. A clean `shelltime daemon uninstall` removes it, which is how
// we tell "the daemon crashed, restart it" apart from "the user deliberately
// removed it and we should stay out of the way".
func daemonServiceFileExists() bool {
	for _, name := range []string{"xyz.shelltime.daemon.plist", "shelltime.service"} {
		if _, err := os.Stat(model.GetStoragePath("daemon", name)); err == nil {
			return true
		}
	}
	return false
}

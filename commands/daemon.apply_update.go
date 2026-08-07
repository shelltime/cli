package commands

import (
	"log/slog"
	"os"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var DaemonApplyUpdateCommand = &cli.Command{
	Name:   "apply-update",
	Usage:  "Apply a staged daemon binary update (internal)",
	Hidden: true,
	Action: commandDaemonApplyUpdate,
}

// Seams for tests. resolveDaemonDest in particular MUST be swappable: it
// resolves through exec.LookPath and can therefore point at a real
// system-installed daemon regardless of $HOME.
var (
	applyUpdateStopService = func(inst model.DaemonInstaller) error {
		return inst.CheckAndStopExistingService()
	}
	applyUpdateEnsureRunning = ensureDaemonRunning
	applyUpdateSmokeTest     = model.SmokeTestDaemonBinary
	applyUpdateResolveDest   = resolveDaemonDest
)

// commandDaemonApplyUpdate finishes an update the daemon started: it activates
// the staged daemon binary and restarts the service.
//
// It runs detached from the shell (see launchDetachedApplyUpdate) and always
// returns nil — this is background repair, and a non-zero exit would surface as
// noise in the user's shell.
//
// INVARIANT: every exit path from step 4 onward calls ensureDaemonRunning.
// Whatever else goes wrong, this command must not return with the daemon down.
func commandDaemonApplyUpdate(c *cli.Context) error {
	ctx := c.Context

	if os.Getenv(model.DisableAutoUpdateEnv) != "" {
		return nil
	}

	// 1. Only one repair at a time: N shells can sample the drift check at once.
	release, ok := model.AcquireUpdateLock(10 * time.Minute)
	if !ok {
		slog.Debug("another process is already applying the update")
		return nil
	}
	defer release()

	// 2. Re-check the marker now that we hold the lock; another process may
	// have finished the repair while we waited.
	pendingTag, err := model.ReadDaemonUpdatePending()
	if err != nil {
		// No marker. This is the self-heal path: the daemon is down but has
		// nothing staged, so just bring it back up.
		if !daemonIsReady(ctx) && daemonServiceFileExists() {
			return finishWithRunningDaemon(c, false, "restarting a stopped daemon")
		}
		return nil
	}

	// 3. Ask the running daemon what version it is. An unreachable daemon is
	// NOT a reason to stop — it is precisely the case that needs repairing.
	socketPath := resolveSocketPath(ctx)
	wasRunning := false
	if st, _, statusErr := requestDaemonStatus(socketPath, 2*time.Second); statusErr == nil {
		wasRunning = true
		if model.NormalizeVersion(st.Version) == model.NormalizeVersion(pendingTag) {
			// Already on the new version. Make sure the service manager agrees,
			// then clear the marker.
			if !daemonIsReady(ctx) {
				_ = applyUpdateEnsureRunning(c, true)
			}
			clearPendingState()
			return nil
		}
	}

	staged := model.GetStagedDaemonPath()
	if _, statErr := os.Stat(staged); statErr != nil {
		// Nothing staged — the Homebrew path, or a staged binary already
		// consumed. Either way the daemon still runs the old inode and needs a
		// restart to pick up the new binary.
		return finishWithRunningDaemon(c, wasRunning, "no staged binary; restarting service")
	}

	// 4. Never install a daemon that cannot report its own version. This catches
	// truncated downloads, wrong-arch builds, and macOS signature failures.
	if err := applyUpdateSmokeTest(ctx, staged, pendingTag); err != nil {
		slog.Warn("staged daemon failed its smoke test; discarding", slog.Any("err", err))
		_ = os.Remove(staged)
		// A bad download must not leave the user without a daemon. Restart
		// first, then record why the update was abandoned — finishWithRunning
		// Daemon clears LastError on success, so ordering matters here.
		res := finishWithRunningDaemon(c, wasRunning, "discarded a bad staged binary")
		recordApplyError(err)
		return res
	}

	// 5. Stop the service before swapping. CheckAndStopExistingService is the
	// only stop the installer exposes; there is no Stop()/Restart().
	installer, instErr := buildDaemonInstaller()
	if instErr == nil {
		if err := applyUpdateStopService(installer); err != nil && wasRunning {
			slog.Debug("stopping the daemon reported an error", slog.Any("err", err))
		}
	}

	// 6. Activate the staged binary.
	daemonDest := applyUpdateResolveDest()
	if err := model.ReplaceBinaryWithBackupSuffix(staged, daemonDest, model.BackupSuffixUpdate); err != nil {
		slog.Error("failed to activate staged daemon binary", slog.Any("err", err))
		recordApplyError(err)
		// Leave the marker so a later shell retries, but bring the old daemon
		// back up in the meantime.
		return finishWithRunningDaemon(c, wasRunning, "swap failed; restoring previous daemon")
	}

	// 7. MANDATORY: `daemon install` treats a "<daemon>.bak" as a NEWER binary
	// and restores it. A stale one from an older CLI would undo the swap we just
	// made, and the daemon would re-download the same release forever.
	_ = os.Remove(daemonDest + model.BackupSuffixLegacy)
	_ = os.Remove(staged)

	// 8. Restart and verify.
	if err := applyUpdateEnsureRunning(c, wasRunning); err != nil {
		slog.Error("daemon did not come back up after update", slog.Any("err", err))
		recordApplyError(err)
		// Leave the marker: the next shell retries.
		return nil
	}

	// 9. Done — clear the marker and the pending state.
	clearPendingState()
	slog.Info("daemon updated", slog.String("tag", pendingTag), slog.String("path", daemonDest))
	return nil
}

// finishWithRunningDaemon restarts the service, clears the marker on success,
// and never propagates an error to the caller.
func finishWithRunningDaemon(c *cli.Context, wasRunning bool, reason string) error {
	slog.Debug("ensuring daemon is running", slog.String("reason", reason))
	if err := applyUpdateEnsureRunning(c, wasRunning); err != nil {
		slog.Error("daemon did not come back up", slog.String("reason", reason), slog.Any("err", err))
		recordApplyError(err)
		return nil
	}
	clearPendingState()
	return nil
}

func clearPendingState() {
	if err := model.ClearDaemonUpdatePending(); err != nil {
		slog.Debug("could not clear pending marker", slog.Any("err", err))
	}
	state, err := model.ReadUpdateState()
	if err != nil {
		return
	}
	state.PendingDaemonTag = ""
	state.PendingDaemonPath = ""
	state.LastError = ""
	if err := model.WriteUpdateState(state); err != nil {
		slog.Debug("could not clear pending state", slog.Any("err", err))
	}
}

func recordApplyError(cause error) {
	state, err := model.ReadUpdateState()
	if err != nil {
		return
	}
	state.LastError = cause.Error()
	if err := model.WriteUpdateState(state); err != nil {
		slog.Debug("could not record apply error", slog.Any("err", err))
	}
}

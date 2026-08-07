package commands

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
)

// updateDriftSampleRate makes 1-in-N `track` invocations pay for a single
// os.Stat. `track` runs on every command in every shell, so the common path
// must cost effectively nothing; 64 gives roughly 3 checks/day per shell at
// ~200 commands/day, which is ample since the daemon writes the marker at most
// once per release. A package var so tests can force it to 1.
var updateDriftSampleRate = 64

// noticeRepeatInterval bounds how often the same update notice is printed.
const noticeRepeatInterval = 24 * time.Hour

// daemonStartRetryInterval rate-limits self-healing restarts so a daemon that
// cannot start is not respawned by every new shell.
const daemonStartRetryInterval = time.Hour

// Seams for tests.
var (
	spawnDriftRepair   = launchDetachedApplyUpdate
	driftNow           = time.Now
	driftDaemonIsReady = daemonIsReady
)

// checkDaemonDriftSampled is called at the very top of `track`, before any
// tracing or logging setup.
//
// Cost on the ~98% path: a constant comparison plus one rand.IntN — under 100ns
// with zero syscalls. On the sampled path it adds one stat(2) of a normally
// absent file (~2-5us). Both are orders of magnitude below what `track` already
// pays for process startup and its unix-socket dial.
func checkDaemonDriftSampled() {
	if runtime.GOOS == "windows" {
		return // no daemon on Windows
	}
	// math/rand/v2's global source is seeded randomly per process and takes no
	// mutex. That matters here: every `track` is a fresh process, so a fixed
	// seed would make every process sample identically.
	if rand.IntN(updateDriftSampleRate) != 0 {
		return
	}
	if os.Getenv(model.DisableAutoUpdateEnv) != "" {
		return
	}
	if _, err := os.Stat(model.GetDaemonUpdatePendingPath()); err != nil {
		return
	}
	spawnDriftRepair()
}

// maybeRepairDaemonDrift runs once per new shell from `gc`. Unlike the sampled
// check it is deterministic, and it is the only place a user-visible notice is
// printed — `gc` runs at shell startup, so a line here cannot interleave with
// command output.
func maybeRepairDaemonDrift(ctx context.Context, cfg model.ShellTimeConfig) {
	if runtime.GOOS == "windows" {
		return
	}
	if os.Getenv(model.DisableAutoUpdateEnv) != "" {
		return
	}
	if cfg.AutoUpdate == nil || cfg.AutoUpdate.Enabled == nil || !*cfg.AutoUpdate.Enabled {
		return
	}

	// A staged update waiting to be applied.
	if _, err := os.Stat(model.GetDaemonUpdatePendingPath()); err == nil {
		spawnDriftRepair()
		return
	}

	printPendingNotice()
	maybeRestartDownDaemon(ctx)
}

// printPendingNotice shows the daemon's "update available" message at most once
// a day.
func printPendingNotice() {
	state, err := model.ReadUpdateState()
	if err != nil || state.Notice == "" {
		return
	}
	if driftNow().Sub(state.NoticeShownAt) < noticeRepeatInterval {
		return
	}
	color.Yellow.Println("💡 " + state.Notice)

	state.NoticeShownAt = driftNow()
	if err := model.WriteUpdateState(state); err != nil {
		slog.Debug("could not stamp notice", slog.Any("err", err))
	}
}

// maybeRestartDownDaemon restarts a daemon that is installed but not running.
//
// This closes a chicken-and-egg gap: the daemon is what performs the update
// check, so once it stays down, auto-update dies with it and nothing else would
// ever notice.
//
// Guarded so we never fight a user who deliberately stopped it: the service
// definition must still exist (a clean `daemon uninstall` removes it), and
// attempts are rate-limited with the same failure backoff as the update check.
func maybeRestartDownDaemon(ctx context.Context) {
	if !daemonServiceFileExists() {
		return // uninstalled on purpose; stay out of the way
	}
	if driftDaemonIsReady(ctx) {
		return
	}

	state, err := model.ReadUpdateState()
	if err != nil {
		return
	}
	if driftNow().Sub(state.LastDaemonStartAttemptAt) < daemonStartRetryInterval {
		return
	}

	state.LastDaemonStartAttemptAt = driftNow()
	if err := model.WriteUpdateState(state); err != nil {
		slog.Debug("could not stamp daemon start attempt", slog.Any("err", err))
		return
	}

	slog.Debug("daemon is installed but not running; attempting restart")
	spawnDriftRepair()
}

// launchDetachedApplyUpdate re-execs this binary as `shelltime daemon
// apply-update` in its own session.
//
// Detached rather than inline because the repair runs launchctl/systemctl, which
// takes hundreds of milliseconds to seconds and prints progress — doing that in
// the shell hook would visibly stall the prompt and pollute it with output.
// Setsid also means Ctrl-C or the shell exiting cannot kill a half-finished
// binary swap.
func launchDetachedApplyUpdate() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, "daemon", "apply-update")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	applyDetachAttrs(cmd)
	if err := cmd.Start(); err != nil {
		slog.Debug("could not spawn daemon apply-update", slog.Any("err", err))
		return
	}
	// Never Wait(): this must not outlive or block the hook.
	_ = cmd.Process.Release()
}

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

// trackDaemonStartSampleRate makes 1-in-N of track's daemon-down paths attempt
// a restart. It is only ever consulted after track has already proven no daemon
// is listening, so the sampling exists to avoid a spawn storm when the daemon
// is down and failing to start — not to save time on the happy path, which
// never reaches it. A package var so tests can force it to 1.
var trackDaemonStartSampleRate = 8

// noticeRepeatInterval bounds how often the same update notice is printed.
const noticeRepeatInterval = 24 * time.Hour

// daemonStartRetryInterval rate-limits self-healing restarts so a daemon that
// cannot start is not respawned by every new shell.
const daemonStartRetryInterval = time.Hour

// Seams for tests.
var (
	spawnDriftRepair   = launchDetachedApplyUpdate
	spawnDaemonStart   = launchDetachedDaemonStart
	driftNow           = time.Now
	driftDaemonIsReady = daemonIsReady
)

// maybeStartDaemonFromTrack restarts a daemon that has stopped.
//
// This is the full extent of what `track` is allowed to do. track runs inside
// the shell hook on every command, so it must never download, extract, verify,
// or swap a binary — all of that belongs to the daemon, with `gc` (once per new
// shell) applying anything the daemon staged. Here we only ever start a service.
//
// It is called only after track has already failed to reach a daemon on both
// the default and the configured socket, so the check itself costs nothing: a
// running daemon returns long before this point.
func maybeStartDaemonFromTrack() {
	if runtime.GOOS == "windows" {
		return // no daemon on Windows
	}
	// math/rand/v2's global source is seeded randomly per process and takes no
	// mutex. That matters here: every `track` is a fresh process, so a fixed
	// seed would make every process sample identically.
	if rand.IntN(trackDaemonStartSampleRate) != 0 {
		return
	}
	// No service definition means the user removed it deliberately; a shell hook
	// is the last place that should argue with them.
	if !daemonServiceFileExists() {
		return
	}

	// Rate limit so a daemon that cannot start is not respawned all day. This
	// touches the state file, but only on the already-slow no-daemon path.
	state, err := model.ReadUpdateState()
	if err != nil {
		return
	}
	if driftNow().Sub(state.LastDaemonStartAttemptAt) < daemonStartRetryInterval {
		return
	}
	state.LastDaemonStartAttemptAt = driftNow()
	if err := model.WriteUpdateState(state); err != nil {
		return
	}

	slog.Debug("no daemon reachable from track; attempting to start it")
	spawnDaemonStart()
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
// launchDetachedFn is a seam so tests can assert which subcommand is spawned
// without actually forking.
var launchDetachedFn = launchDetached

func launchDetachedApplyUpdate() {
	launchDetachedFn("daemon", "apply-update")
}

// launchDetachedDaemonStart starts the daemon service and nothing else. It is
// what `track` spawns: no update logic, no binary swap, no network.
func launchDetachedDaemonStart() {
	launchDetachedFn("daemon", "install")
}

// launchDetached re-execs this binary with the given args in its own session,
// discarding all output and never waiting for the result.
func launchDetached(args ...string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(self, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	applyDetachAttrs(cmd)
	if err := cmd.Start(); err != nil {
		slog.Debug("could not spawn detached command", slog.Any("args", args), slog.Any("err", err))
		return
	}
	// Never Wait(): this must not outlive or block the hook, and per design the
	// outcome is ignored — success or failure, the shell carries on.
	_ = cmd.Process.Release()
}

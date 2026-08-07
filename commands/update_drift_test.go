package commands

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// driftTestEnv isolates the state files and restores every seam.
func driftTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(model.DisableAutoUpdateEnv, "")

	prevRate := trackDaemonStartSampleRate
	prevSpawn := spawnDriftRepair
	prevStart := spawnDaemonStart
	prevNow := driftNow
	prevReady := driftDaemonIsReady
	t.Cleanup(func() {
		trackDaemonStartSampleRate = prevRate
		spawnDriftRepair = prevSpawn
		spawnDaemonStart = prevStart
		driftNow = prevNow
		driftDaemonIsReady = prevReady
	})

	// Force the sampled branch so tests are deterministic.
	trackDaemonStartSampleRate = 1
	// Don't let a real daemon on the dev machine influence the result.
	driftDaemonIsReady = func(context.Context) bool { return false }
}

// installDaemonServiceFile simulates a daemon that has been installed at some
// point (so we are allowed to restart it).
func installDaemonServiceFile(t *testing.T) {
	t.Helper()
	require.NoError(t, os.MkdirAll(model.GetStoragePath("daemon"), 0o755))
	require.NoError(t, os.WriteFile(
		model.GetStoragePath("daemon", "xyz.shelltime.daemon.plist"), []byte("<plist/>"), 0o644))
	require.NoError(t, os.WriteFile(
		model.GetStoragePath("daemon", "shelltime.service"), []byte("[Unit]"), 0o644))
}

func autoUpdateOnConfig() model.ShellTimeConfig {
	on := true
	return model.ShellTimeConfig{AutoUpdate: &model.AutoUpdate{Enabled: &on}}
}

// track's ONLY permitted daemon action is starting the service. It must never
// spawn the update-applying path, which swaps binaries.
func TestMaybeStartDaemonFromTrack_StartsDaemonNeverAppliesUpdate(t *testing.T) {
	driftTestEnv(t)
	installDaemonServiceFile(t)
	// Even with a staged update pending, track must not touch it.
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))

	var started, repaired atomic.Bool
	spawnDaemonStart = func() { started.Store(true) }
	spawnDriftRepair = func() { repaired.Store(true) }

	maybeStartDaemonFromTrack()

	assert.True(t, started.Load(), "track should start a stopped daemon")
	assert.False(t, repaired.Load(),
		"track must never apply an update; that belongs to the daemon and gc")
}

// track spawns `daemon install`, which only starts the service.
func TestLaunchDetachedDaemonStart_UsesInstallNotApplyUpdate(t *testing.T) {
	driftTestEnv(t)

	var gotArgs []string
	prev := launchDetachedFn
	t.Cleanup(func() { launchDetachedFn = prev })
	launchDetachedFn = func(args ...string) { gotArgs = args }

	launchDetachedDaemonStart()
	assert.Equal(t, []string{"daemon", "install"}, gotArgs)

	launchDetachedApplyUpdate()
	assert.Equal(t, []string{"daemon", "apply-update"}, gotArgs)
}

// A user who ran `daemon uninstall` must not have it restarted by a shell hook.
func TestMaybeStartDaemonFromTrack_SkipsWhenServiceFileAbsent(t *testing.T) {
	driftTestEnv(t)

	var started atomic.Bool
	spawnDaemonStart = func() { started.Store(true) }

	maybeStartDaemonFromTrack()
	assert.False(t, started.Load(), "no service file means the user removed it deliberately")
}

// A daemon that cannot start must not be respawned on every command.
func TestMaybeStartDaemonFromTrack_RateLimited(t *testing.T) {
	driftTestEnv(t)
	installDaemonServiceFile(t)

	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	driftNow = func() time.Time { return now }

	var starts atomic.Int32
	spawnDaemonStart = func() { starts.Add(1) }

	maybeStartDaemonFromTrack()
	require.EqualValues(t, 1, starts.Load(), "first attempt should fire")

	for range 50 {
		maybeStartDaemonFromTrack()
	}
	assert.EqualValues(t, 1, starts.Load(), "must not respawn on every command")

	driftNow = func() time.Time { return now.Add(2 * time.Hour) }
	maybeStartDaemonFromTrack()
	assert.EqualValues(t, 2, starts.Load(), "retries after the interval")
}

// Sampling keeps a spawn storm from forming while the daemon is down.
func TestMaybeStartDaemonFromTrack_Sampled(t *testing.T) {
	driftTestEnv(t)
	installDaemonServiceFile(t)
	trackDaemonStartSampleRate = 8

	// Keep the rate limiter out of the way so we measure sampling alone: each
	// call sees a clock far past the previous attempt.
	base := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	driftNow = func() time.Time {
		return base.Add(time.Duration(clock.Add(1)) * 2 * time.Hour)
	}

	var reached atomic.Int32
	spawnDaemonStart = func() { reached.Add(1) }

	const runs = 400
	for range runs {
		maybeStartDaemonFromTrack()
	}

	got := int(reached.Load())
	assert.Greater(t, got, 0, "sampling should fire occasionally")
	assert.Less(t, got, runs/2, "sampling should skip most invocations")
}

func TestMaybeRepairDaemonDrift_DisabledByConfig(t *testing.T) {
	driftTestEnv(t)
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))

	var spawned atomic.Bool
	spawnDriftRepair = func() { spawned.Store(true) }

	off := false
	maybeRepairDaemonDrift(context.Background(), model.ShellTimeConfig{
		AutoUpdate: &model.AutoUpdate{Enabled: &off},
	})
	assert.False(t, spawned.Load(), "an opted-out user must not be touched")
}

func TestMaybeRepairDaemonDrift_MarkerTriggersRepair(t *testing.T) {
	driftTestEnv(t)
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))

	var spawned atomic.Bool
	spawnDriftRepair = func() { spawned.Store(true) }

	maybeRepairDaemonDrift(context.Background(), autoUpdateOnConfig())
	assert.True(t, spawned.Load())
}

func TestPrintPendingNotice_ShownOncePerDay(t *testing.T) {
	driftTestEnv(t)

	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	driftNow = func() time.Time { return now }

	require.NoError(t, model.WriteUpdateState(model.UpdateState{
		Notice: "shelltime v0.1.90 is available.",
	}))

	printPendingNotice()

	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.True(t, st.NoticeShownAt.Equal(now), "showing the notice must stamp the time")

	// A second call within the day must not re-stamp.
	driftNow = func() time.Time { return now.Add(time.Hour) }
	printPendingNotice()

	st2, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.True(t, st2.NoticeShownAt.Equal(now), "notice must not repeat within 24h")

	// A day later it shows again.
	later := now.Add(25 * time.Hour)
	driftNow = func() time.Time { return later }
	printPendingNotice()

	st3, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.True(t, st3.NoticeShownAt.Equal(later))
}

func TestPrintPendingNotice_NoNoticeIsNoop(t *testing.T) {
	driftTestEnv(t)
	assert.NotPanics(t, func() { printPendingNotice() })
}

// A user who ran `daemon uninstall` must not have it silently reinstalled.
func TestMaybeRestartDownDaemon_SkipsWhenServiceFileAbsent(t *testing.T) {
	driftTestEnv(t)

	var spawned atomic.Bool
	spawnDriftRepair = func() { spawned.Store(true) }

	maybeRestartDownDaemon(context.Background())
	assert.False(t, spawned.Load(), "no service file means the user removed it deliberately")
}

func TestMaybeRestartDownDaemon_RateLimited(t *testing.T) {
	driftTestEnv(t)

	// Simulate an installed-but-stopped service.
	daemonDir := model.GetStoragePath("daemon")
	require.NoError(t, os.MkdirAll(daemonDir, 0o755))
	require.NoError(t, os.WriteFile(
		model.GetStoragePath("daemon", "xyz.shelltime.daemon.plist"), []byte("<plist/>"), 0o644))
	require.NoError(t, os.WriteFile(
		model.GetStoragePath("daemon", "shelltime.service"), []byte("[Unit]"), 0o644))

	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	driftNow = func() time.Time { return now }

	var spawns atomic.Int32
	spawnDriftRepair = func() { spawns.Add(1) }

	maybeRestartDownDaemon(context.Background())
	require.EqualValues(t, 1, spawns.Load(), "first attempt should fire")

	// Immediately again: rate limited.
	maybeRestartDownDaemon(context.Background())
	assert.EqualValues(t, 1, spawns.Load(), "must not respawn on every shell")

	// An hour later it retries.
	driftNow = func() time.Time { return now.Add(2 * time.Hour) }
	maybeRestartDownDaemon(context.Background())
	assert.EqualValues(t, 2, spawns.Load())
}

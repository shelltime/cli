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

	prevRate := updateDriftSampleRate
	prevSpawn := spawnDriftRepair
	prevNow := driftNow
	prevReady := driftDaemonIsReady
	t.Cleanup(func() {
		updateDriftSampleRate = prevRate
		spawnDriftRepair = prevSpawn
		driftNow = prevNow
		driftDaemonIsReady = prevReady
	})

	// Force the sampled branch so tests are deterministic.
	updateDriftSampleRate = 1
	// Don't let a real daemon on the dev machine influence the result.
	driftDaemonIsReady = func(context.Context) bool { return false }
}

func autoUpdateOnConfig() model.ShellTimeConfig {
	on := true
	return model.ShellTimeConfig{AutoUpdate: &model.AutoUpdate{Enabled: &on}}
}

func TestCheckDaemonDriftSampled_NoMarkerDoesNothing(t *testing.T) {
	driftTestEnv(t)

	var spawned atomic.Bool
	spawnDriftRepair = func() { spawned.Store(true) }

	checkDaemonDriftSampled()
	assert.False(t, spawned.Load(), "no marker means no repair")
}

func TestCheckDaemonDriftSampled_MarkerTriggersRepair(t *testing.T) {
	driftTestEnv(t)
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))

	var spawned atomic.Bool
	spawnDriftRepair = func() { spawned.Store(true) }

	checkDaemonDriftSampled()
	assert.True(t, spawned.Load())
}

func TestCheckDaemonDriftSampled_KillSwitch(t *testing.T) {
	driftTestEnv(t)
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))
	t.Setenv(model.DisableAutoUpdateEnv, "1")

	var spawned atomic.Bool
	spawnDriftRepair = func() { spawned.Store(true) }

	checkDaemonDriftSampled()
	assert.False(t, spawned.Load())
}

// The whole point of sampling is that `track` almost never does any work.
func TestCheckDaemonDriftSampled_SamplingSkipsMostInvocations(t *testing.T) {
	driftTestEnv(t)
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))
	updateDriftSampleRate = 64

	var spawns atomic.Int32
	spawnDriftRepair = func() { spawns.Add(1) }

	const runs = 2000
	for range runs {
		checkDaemonDriftSampled()
	}

	got := spawns.Load()
	// Expect ~31 of 2000. Generous bounds so this cannot flake.
	assert.Greater(t, int(got), 0, "sampling should fire occasionally")
	assert.Less(t, int(got), runs/8, "sampling should skip the vast majority of invocations")
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

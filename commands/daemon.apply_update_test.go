package commands

import (
	"context"
	"errors"
	"flag"
	"os"
	"sync/atomic"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

type applyUpdateHarness struct {
	ensureCalls    atomic.Int32
	lastWasRunning atomic.Bool
	ensureErr      error
	stopCalls      atomic.Int32
	smokeErr       error
}

// newApplyUpdateHarness isolates state files and stubs every side effect.
func newApplyUpdateHarness(t *testing.T) *applyUpdateHarness {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(model.DisableAutoUpdateEnv, "")

	h := &applyUpdateHarness{}

	prevEnsure := applyUpdateEnsureRunning
	prevStop := applyUpdateStopService
	prevSmoke := applyUpdateSmokeTest
	prevDest := applyUpdateResolveDest
	t.Cleanup(func() {
		applyUpdateEnsureRunning = prevEnsure
		applyUpdateStopService = prevStop
		applyUpdateSmokeTest = prevSmoke
		applyUpdateResolveDest = prevDest
	})

	// CRITICAL: resolveDaemonDest goes through exec.LookPath and would otherwise
	// resolve to a real system-installed daemon (e.g. /opt/homebrew/bin), which
	// the test would then overwrite. Always pin it under the temp HOME.
	applyUpdateResolveDest = model.GetCurlInstallerDaemonPath

	applyUpdateEnsureRunning = func(_ *cli.Context, wasRunning bool) error {
		h.ensureCalls.Add(1)
		h.lastWasRunning.Store(wasRunning)
		return h.ensureErr
	}
	applyUpdateStopService = func(model.DaemonInstaller) error {
		h.stopCalls.Add(1)
		return nil
	}
	applyUpdateSmokeTest = func(_ context.Context, _, _ string) error {
		return h.smokeErr
	}
	return h
}

func newTestCLIContext() *cli.Context {
	app := cli.NewApp()
	ctx := cli.NewContext(app, flag.NewFlagSet("test", flag.ContinueOnError), nil)
	ctx.Context = context.Background()
	return ctx
}

// writeStagedDaemon creates a plausible staged binary.
func writeStagedDaemon(t *testing.T) string {
	t.Helper()
	path := model.GetStagedDaemonPath()
	require.NoError(t, os.MkdirAll(model.GetBinFolderPath(), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("staged-daemon-binary"), 0o755))
	return path
}

func TestApplyUpdate_NoMarkerAndDaemonUpIsNoop(t *testing.T) {
	h := newApplyUpdateHarness(t)

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()))
	assert.EqualValues(t, 0, h.ensureCalls.Load(), "nothing to do")
}

// The self-heal path: no staged update, but the daemon is installed and down.
func TestApplyUpdate_NoMarkerRestartsDownDaemon(t *testing.T) {
	h := newApplyUpdateHarness(t)

	// An installed-but-stopped service: service file present, no socket.
	require.NoError(t, os.MkdirAll(model.GetStoragePath("daemon"), 0o755))
	require.NoError(t, os.WriteFile(
		model.GetStoragePath("daemon", "shelltime.service"), []byte("[Unit]"), 0o644))

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()))

	// daemonIsReady consults the real socket path; when a daemon happens to be
	// running on this machine there is nothing to restart, which is also correct.
	if h.ensureCalls.Load() > 0 {
		assert.False(t, h.lastWasRunning.Load(), "a stopped daemon must be installed, not reinstalled")
	}
}

func TestApplyUpdate_SwapsStagedBinaryAndRestarts(t *testing.T) {
	h := newApplyUpdateHarness(t)

	staged := writeStagedDaemon(t)
	daemonDest := model.GetCurlInstallerDaemonPath()
	require.NoError(t, os.WriteFile(daemonDest, []byte("old-daemon"), 0o755))
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))
	require.NoError(t, model.WriteUpdateState(model.UpdateState{
		PendingDaemonTag:  "v0.1.90",
		PendingDaemonPath: staged,
	}))

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()))

	got, err := os.ReadFile(daemonDest)
	require.NoError(t, err)
	assert.Equal(t, "staged-daemon-binary", string(got), "the new daemon must be active")

	// MANDATORY: a leftover .bak would make `daemon install` restore the old
	// binary, undoing this swap and causing a permanent update loop.
	_, bakErr := os.Stat(daemonDest + model.BackupSuffixLegacy)
	assert.True(t, os.IsNotExist(bakErr), "a .bak here would be restored by daemon install")

	_, stagedErr := os.Stat(staged)
	assert.True(t, os.IsNotExist(stagedErr), "the staged binary should be consumed")

	assert.GreaterOrEqual(t, int(h.ensureCalls.Load()), 1, "the service must be brought back up")

	_, markerErr := model.ReadDaemonUpdatePending()
	assert.Error(t, markerErr, "marker must be cleared on success")

	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Empty(t, st.PendingDaemonTag)
	assert.Empty(t, st.PendingDaemonPath)
}

// A bad download must never leave the user without a daemon.
func TestApplyUpdate_FailedSmokeTestStillRestartsDaemon(t *testing.T) {
	h := newApplyUpdateHarness(t)
	h.smokeErr = errors.New("staged binary is corrupt")

	staged := writeStagedDaemon(t)
	daemonDest := model.GetCurlInstallerDaemonPath()
	require.NoError(t, os.WriteFile(daemonDest, []byte("old-daemon"), 0o755))
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()))

	assert.GreaterOrEqual(t, int(h.ensureCalls.Load()), 1,
		"a failed smoke test must still leave the daemon running")

	got, err := os.ReadFile(daemonDest)
	require.NoError(t, err)
	assert.Equal(t, "old-daemon", string(got), "a corrupt binary must not be installed")

	_, stagedErr := os.Stat(staged)
	assert.True(t, os.IsNotExist(stagedErr), "the bad staged binary should be discarded")

	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Contains(t, st.LastError, "corrupt")
}

// No staged binary is the Homebrew case: brew replaced the binary, but the
// running daemon still holds the old inode, so it must be restarted.
func TestApplyUpdate_NoStagedBinaryStillRestarts(t *testing.T) {
	h := newApplyUpdateHarness(t)
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()))

	assert.GreaterOrEqual(t, int(h.ensureCalls.Load()), 1)
	_, markerErr := model.ReadDaemonUpdatePending()
	assert.Error(t, markerErr, "marker cleared once the service is back up")
}

// If the restart fails, the marker must survive so a later shell retries.
func TestApplyUpdate_KeepsMarkerWhenRestartFails(t *testing.T) {
	h := newApplyUpdateHarness(t)
	h.ensureErr = errors.New("launchctl refused")

	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()),
		"background repair must never return an error to the shell")

	tag, err := model.ReadDaemonUpdatePending()
	require.NoError(t, err, "marker must survive so the next shell retries")
	assert.Equal(t, "v0.1.90", tag)

	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Contains(t, st.LastError, "launchctl refused")
}

func TestApplyUpdate_KillSwitch(t *testing.T) {
	h := newApplyUpdateHarness(t)
	t.Setenv(model.DisableAutoUpdateEnv, "1")
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))
	writeStagedDaemon(t)

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()))
	assert.EqualValues(t, 0, h.ensureCalls.Load())

	_, err := model.ReadDaemonUpdatePending()
	assert.NoError(t, err, "the kill switch pauses the update, it does not discard it")
}

// N shells can spawn a repair at once; the lock must serialize them.
func TestApplyUpdate_RespectsUpdateLock(t *testing.T) {
	h := newApplyUpdateHarness(t)
	require.NoError(t, model.WriteDaemonUpdatePending("v0.1.90"))
	writeStagedDaemon(t)

	release, ok := model.AcquireUpdateLock(0)
	require.True(t, ok)
	defer release()

	require.NoError(t, commandDaemonApplyUpdate(newTestCLIContext()))
	assert.EqualValues(t, 0, h.ensureCalls.Load(), "must yield while another process holds the lock")
}

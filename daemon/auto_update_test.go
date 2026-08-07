package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrBool(b bool) *bool { return &b }

// autoUpdateTestHome isolates the state/marker files and restores every package
// seam the service uses.
func autoUpdateTestHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(DisableAutoUpdateEnv, "")

	prevFetch := autoUpdateFetchLatest
	prevApply := autoUpdateApply
	prevBrew := autoUpdateBrewUpgrade
	prevResolve := autoUpdateResolveCLIPath
	prevNow := autoUpdateNow
	prevJitter := autoUpdateJitter
	prevVersion := version

	t.Cleanup(func() {
		autoUpdateFetchLatest = prevFetch
		autoUpdateApply = prevApply
		autoUpdateBrewUpgrade = prevBrew
		autoUpdateResolveCLIPath = prevResolve
		autoUpdateNow = prevNow
		autoUpdateJitter = prevJitter
		version = prevVersion
	})

	// Deterministic gating in tests.
	autoUpdateJitter = time.Nanosecond
	version = "0.1.89"
}

func enabledAutoUpdateConfig() model.ShellTimeConfig {
	return model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: "https://api.example.com",
		AutoUpdate: &model.AutoUpdate{
			Enabled:       ptrBool(true),
			Homebrew:      ptrBool(false),
			IntervalHours: 24,
		},
	}
}

func TestNewAutoUpdateService(t *testing.T) {
	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NotNil(t, svc)
	assert.NotNil(t, svc.stopChan)
	assert.Equal(t, "test-token", svc.config.Token)
}

func TestAutoUpdateService_StartStop(t *testing.T) {
	autoUpdateTestHome(t)
	prevTick := AutoUpdateTickInterval
	AutoUpdateTickInterval = 10 * time.Millisecond
	defer func() { AutoUpdateTickInterval = prevTick }()

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.Start(context.Background()))
	assert.NotNil(t, svc.ticker)
	svc.Stop()
}

func TestAutoUpdateService_StopWithoutStart(t *testing.T) {
	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	assert.NotPanics(t, func() { svc.Stop() })
}

func TestAutoUpdateService_ContextCancellation(t *testing.T) {
	autoUpdateTestHome(t)
	prevTick := AutoUpdateTickInterval
	AutoUpdateTickInterval = 10 * time.Millisecond
	defer func() { AutoUpdateTickInterval = prevTick }()

	ctx, cancel := context.WithCancel(context.Background())
	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.Start(ctx))
	cancel()
	time.Sleep(30 * time.Millisecond)
	svc.Stop()
}

// A daemon that restarts more often than the check interval must still check;
// that is why the gate is a persisted timestamp rather than the ticker.
func TestAutoUpdateService_ShouldCheckNow(t *testing.T) {
	autoUpdateTestHome(t)
	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	autoUpdateNow = func() time.Time { return now }

	t.Run("never checked", func(t *testing.T) {
		assert.True(t, svc.shouldCheckNow(model.UpdateState{}))
	})

	t.Run("checked recently", func(t *testing.T) {
		assert.False(t, svc.shouldCheckNow(model.UpdateState{
			LastCheckAt: now.Add(-1 * time.Hour),
		}))
	})

	t.Run("checked long ago", func(t *testing.T) {
		assert.True(t, svc.shouldCheckNow(model.UpdateState{
			LastCheckAt: now.Add(-25 * time.Hour),
		}))
	})

	t.Run("failures back off", func(t *testing.T) {
		// 3 failures => 8x the 24h interval, so 25h ago is not yet due.
		assert.False(t, svc.shouldCheckNow(model.UpdateState{
			LastCheckAt:         now.Add(-25 * time.Hour),
			ConsecutiveFailures: 3,
		}))
		assert.True(t, svc.shouldCheckNow(model.UpdateState{
			LastCheckAt:         now.Add(-200 * time.Hour),
			ConsecutiveFailures: 3,
		}))
	})

	t.Run("future timestamp does not wedge updates", func(t *testing.T) {
		assert.True(t, svc.shouldCheckNow(model.UpdateState{
			LastCheckAt: now.Add(48 * time.Hour),
		}))
	})
}

func TestBackoffFactor(t *testing.T) {
	assert.Equal(t, 1, backoffFactor(0))
	assert.Equal(t, 1, backoffFactor(-1))
	assert.Equal(t, 2, backoffFactor(1))
	assert.Equal(t, 4, backoffFactor(2))
	assert.Equal(t, 8, backoffFactor(3))
	assert.Equal(t, 8, backoffFactor(99), "backoff is capped")
}

func TestAutoUpdateService_RunCheck_SkipsWhenNoToken(t *testing.T) {
	autoUpdateTestHome(t)
	var called atomic.Bool
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		called.Store(true)
		return model.LatestCLIRelease{}, nil
	}

	cfg := enabledAutoUpdateConfig()
	cfg.Token = ""
	svc := NewAutoUpdateService(cfg)
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))
	assert.False(t, called.Load(), "an empty token would 401; skip instead")
}

func TestAutoUpdateService_RunCheck_SkipsDevBuild(t *testing.T) {
	autoUpdateTestHome(t)
	version = "dev"

	var called atomic.Bool
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		called.Store(true)
		return model.LatestCLIRelease{}, nil
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))
	assert.False(t, called.Load(), "dev builds must never auto-update")
}

func TestAutoUpdateService_RunCheck_KillSwitch(t *testing.T) {
	autoUpdateTestHome(t)
	t.Setenv(DisableAutoUpdateEnv, "1")

	var called atomic.Bool
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		called.Store(true)
		return model.LatestCLIRelease{}, nil
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))
	assert.False(t, called.Load())
}

// A server bug reporting an old tag must never downgrade the user.
func TestAutoUpdateService_RunCheck_NeverDowngrades(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	var applied atomic.Bool
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{Tag: "v0.1.50"}, nil
	}
	autoUpdateApply = func(context.Context, model.UpdatePlan) (model.UpdateResult, error) {
		applied.Store(true)
		return model.UpdateResult{}, nil
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))
	assert.False(t, applied.Load(), "an older tag must not trigger an install")

	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Equal(t, "v0.1.50", st.LastKnownTag)
	assert.False(t, st.LastCheckAt.IsZero())
}

func TestAutoUpdateService_RunCheck_SameVersionIsNoop(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	var applied atomic.Bool
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{Tag: "v0.1.89"}, nil
	}
	autoUpdateApply = func(context.Context, model.UpdatePlan) (model.UpdateResult, error) {
		applied.Store(true)
		return model.UpdateResult{}, nil
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))
	assert.False(t, applied.Load())
}

func TestAutoUpdateService_RunCheck_StagesDaemonAndWritesMarker(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	cliPath := filepath.Join(model.GetBinFolderPath(), "shelltime")
	autoUpdateResolveCLIPath = func() (string, error) { return cliPath, nil }
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{
			Tag: "v0.1.90",
			Asset: &model.CLIReleaseAssetInfo{
				Name:   "cli_Darwin_arm64.zip",
				Sha256: "deadbeef",
			},
		}, nil
	}

	var gotPlan model.UpdatePlan
	autoUpdateApply = func(_ context.Context, plan model.UpdatePlan) (model.UpdateResult, error) {
		gotPlan = plan
		return model.UpdateResult{Tag: plan.Tag, ReplacedCLI: true, StagedDaemon: true}, nil
	}

	// The writability preflight needs the bin dir to exist.
	require.NoError(t, os.MkdirAll(model.GetBinFolderPath(), 0o755))

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))

	assert.Equal(t, "v0.1.90", gotPlan.Tag)
	assert.Equal(t, cliPath, gotPlan.CLIDest)
	assert.Equal(t, "deadbeef", gotPlan.ExpectedSha)
	assert.Equal(t, model.GetStagedDaemonPath(), gotPlan.DaemonStagePath)
	assert.Empty(t, gotPlan.DaemonDest, "the running daemon must not activate its own replacement")
	assert.False(t, gotPlan.AllowUnverified, "the unattended path must fail closed")
	assert.Equal(t, model.BackupSuffixUpdate, gotPlan.BackupSuffix)

	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Equal(t, "v0.1.90", st.LastAppliedTag)
	assert.Equal(t, "v0.1.90", st.PendingDaemonTag)
	assert.Equal(t, model.GetStagedDaemonPath(), st.PendingDaemonPath)

	tag, err := model.ReadDaemonUpdatePending()
	require.NoError(t, err)
	assert.Equal(t, "v0.1.90", tag)
}

func TestAutoUpdateService_RunCheck_NotifyOnly(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	cliPath := filepath.Join(model.GetBinFolderPath(), "shelltime")
	autoUpdateResolveCLIPath = func() (string, error) { return cliPath, nil }
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{Tag: "v0.1.90"}, nil
	}
	var applied atomic.Bool
	autoUpdateApply = func(context.Context, model.UpdatePlan) (model.UpdateResult, error) {
		applied.Store(true)
		return model.UpdateResult{}, nil
	}

	cfg := enabledAutoUpdateConfig()
	cfg.AutoUpdate.NotifyOnly = ptrBool(true)

	svc := NewAutoUpdateService(cfg)
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))

	assert.False(t, applied.Load())
	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Contains(t, st.Notice, "v0.1.90")
	_, err = model.ReadDaemonUpdatePending()
	assert.Error(t, err, "notify-only must not write a pending marker")
}

// Homebrew is opt-in: by default we only tell the user what to run.
func TestAutoUpdateService_RunCheck_HomebrewNotifiesByDefault(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	autoUpdateResolveCLIPath = func() (string, error) {
		return "/opt/homebrew/Caskroom/shelltime/0.1.89/shelltime", nil
	}
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{Tag: "v0.1.90"}, nil
	}
	var brewRan atomic.Bool
	autoUpdateBrewUpgrade = func(context.Context, model.CommandService) error {
		brewRan.Store(true)
		return nil
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))

	assert.False(t, brewRan.Load(), "brew must not run without opt-in")
	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Contains(t, st.Notice, "brew upgrade --cask")
}

func TestAutoUpdateService_RunCheck_HomebrewOptIn(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	autoUpdateResolveCLIPath = func() (string, error) {
		return "/usr/local/Caskroom/shelltime/0.1.89/shelltime", nil
	}
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{Tag: "v0.1.90"}, nil
	}
	var brewRan atomic.Bool
	autoUpdateBrewUpgrade = func(context.Context, model.CommandService) error {
		brewRan.Store(true)
		return nil
	}

	cfg := enabledAutoUpdateConfig()
	cfg.AutoUpdate.Homebrew = ptrBool(true)

	svc := NewAutoUpdateService(cfg)
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))

	assert.True(t, brewRan.Load())

	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Equal(t, "v0.1.90", st.PendingDaemonTag)
	assert.Empty(t, st.PendingDaemonPath, "brew owns the binary; only a restart is needed")

	// A brew upgrade still leaves the running daemon on the old inode, so the
	// restart marker must be written.
	tag, err := model.ReadDaemonUpdatePending()
	require.NoError(t, err)
	assert.Equal(t, "v0.1.90", tag)
}

func TestAutoUpdateService_RunCheck_UnknownInstallLocationOnlyNotifies(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	autoUpdateResolveCLIPath = func() (string, error) { return "/some/random/path/shelltime", nil }
	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{Tag: "v0.1.90"}, nil
	}
	var applied atomic.Bool
	autoUpdateApply = func(context.Context, model.UpdatePlan) (model.UpdateResult, error) {
		applied.Store(true)
		return model.UpdateResult{}, nil
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{}))

	assert.False(t, applied.Load(), "never write into an unrecognized location")
	st, err := model.ReadUpdateState()
	require.NoError(t, err)
	assert.Contains(t, st.Notice, "v0.1.90")
}

func TestAutoUpdateService_RunCheck_RecordsFailureAndBacksOff(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{}, errors.New("network is down")
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	err := svc.runCheck(context.Background(), model.UpdateState{})
	require.Error(t, err)

	st, readErr := model.ReadUpdateState()
	require.NoError(t, readErr)
	assert.Equal(t, 1, st.ConsecutiveFailures)
	assert.Contains(t, st.LastError, "network is down")
	assert.False(t, st.LastCheckAt.IsZero(), "a failed check still stamps LastCheckAt so we back off")
}

func TestAutoUpdateService_RunCheck_SkipsAlreadyAppliedTag(t *testing.T) {
	autoUpdateTestHome(t)
	version = "0.1.89"

	autoUpdateFetchLatest = func(context.Context, model.ShellTimeConfig) (model.LatestCLIRelease, error) {
		return model.LatestCLIRelease{Tag: "v0.1.90"}, nil
	}
	var applied atomic.Bool
	autoUpdateApply = func(context.Context, model.UpdatePlan) (model.UpdateResult, error) {
		applied.Store(true)
		return model.UpdateResult{}, nil
	}

	svc := NewAutoUpdateService(enabledAutoUpdateConfig())
	require.NoError(t, svc.runCheck(context.Background(), model.UpdateState{
		LastAppliedTag: "v0.1.90",
	}))
	assert.False(t, applied.Load(), "must not re-download a release already staged")
}

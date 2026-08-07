package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

// DisableAutoUpdateEnv is an emergency kill switch that needs no config edit.
// Aliased from model so the CLI-side drift check and the daemon agree.
const DisableAutoUpdateEnv = model.DisableAutoUpdateEnv

var (
	// AutoUpdateTickInterval is how often we wake up; AutoUpdateCheckInterval is
	// how often we actually check.
	//
	// A plain 24h ticker would reset on every daemon restart, so a daemon that
	// restarts more often than that would never check at all. Waking hourly and
	// gating on a persisted LastCheckAt self-corrects across restarts and makes
	// a crash loop harmless.
	AutoUpdateTickInterval  = 1 * time.Hour
	AutoUpdateCheckInterval = 24 * time.Hour

	// autoUpdateJitter spreads a fleet of daemons out so they don't all hit the
	// server in the same second after a release.
	autoUpdateJitter = 30 * time.Minute

	// autoUpdateMaxBackoff caps the exponential backoff on repeated failures.
	autoUpdateMaxBackoff = 7 * 24 * time.Hour
)

// Seams for tests, mirroring loadCodexAuthFunc/fetchCodexUsageFunc.
var (
	autoUpdateFetchLatest    = model.FetchLatestCLIRelease
	autoUpdateApply          = model.ApplyUpdate
	autoUpdateBrewUpgrade    = runBrewUpgrade
	autoUpdateNow            = time.Now
	autoUpdateResolveCLIPath = func() (string, error) {
		return model.ResolveCLIBinaryPathFrom(model.NewCommandService())
	}
)

// AutoUpdateService checks for a new CLI release once a day and installs it.
//
// It deliberately updates only the CLI binary and *stages* the daemon binary:
// the running daemon must not rewrite the binary it is executing, and staging
// means the CLI's later repair is a local rename with no network access inside
// the shell hook.
type AutoUpdateService struct {
	config   model.ShellTimeConfig
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
}

func NewAutoUpdateService(config model.ShellTimeConfig) *AutoUpdateService {
	return &AutoUpdateService{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start begins the periodic update check. Like the cleanup timer, it does NOT
// run immediately at startup: a restart loop would otherwise hammer the server
// and re-download on every boot.
func (s *AutoUpdateService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(AutoUpdateTickInterval)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.ticker.C:
				s.tick(ctx)
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	slog.Info("Auto update service started",
		slog.Duration("tick", AutoUpdateTickInterval),
		slog.Duration("checkInterval", s.checkInterval()))
	return nil
}

func (s *AutoUpdateService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Auto update service stopped")
}

func (s *AutoUpdateService) checkInterval() time.Duration {
	if s.config.AutoUpdate != nil && s.config.AutoUpdate.IntervalHours > 0 {
		return time.Duration(s.config.AutoUpdate.IntervalHours) * time.Hour
	}
	return AutoUpdateCheckInterval
}

// tick decides whether a check is due and runs it.
func (s *AutoUpdateService) tick(ctx context.Context) {
	state, err := model.ReadUpdateState()
	if err != nil {
		slog.Debug("could not read update state", slog.Any("err", err))
	}
	if !s.shouldCheckNow(state) {
		return
	}
	if err := s.runCheck(ctx, state); err != nil {
		slog.Warn("auto update check failed", slog.Any("err", err))
	}
}

// shouldCheckNow gates on the persisted LastCheckAt plus a failure backoff.
func (s *AutoUpdateService) shouldCheckNow(state model.UpdateState) bool {
	now := autoUpdateNow()
	if state.LastCheckAt.IsZero() {
		return true
	}
	// Clock skew (or a restored backup) could park LastCheckAt in the future;
	// don't let that disable updates forever.
	if state.LastCheckAt.After(now) {
		return true
	}

	gap := s.checkInterval() * time.Duration(backoffFactor(state.ConsecutiveFailures))
	if gap > autoUpdateMaxBackoff {
		gap = autoUpdateMaxBackoff
	}
	// Jitter so a fleet doesn't stampede after a release.
	gap += rand.N(autoUpdateJitter)

	return !now.Before(state.LastCheckAt.Add(gap))
}

func backoffFactor(failures int) int {
	if failures <= 0 {
		return 1
	}
	if failures > 3 {
		failures = 3
	}
	return 1 << failures
}

// runCheck performs one full check-and-maybe-install cycle.
func (s *AutoUpdateService) runCheck(ctx context.Context, state model.UpdateState) error {
	if os.Getenv(DisableAutoUpdateEnv) != "" {
		return nil
	}
	// An empty token would send `Authorization: CLI ` and get a 401, whereas no
	// header at all is treated as anonymous. Skip rather than fail.
	if s.config.Token == "" {
		return nil
	}

	current := GetVersion()
	if current == "" || current == "dev" {
		slog.Debug("skipping auto update for a dev build")
		return nil
	}

	rel, err := autoUpdateFetchLatest(ctx, s.config)
	if err != nil {
		return s.recordFailure(state, fmt.Errorf("fetch latest release: %w", err))
	}
	if rel.Tag == "" {
		return s.recordFailure(state, fmt.Errorf("server returned an empty release tag"))
	}

	state.LastCheckAt = autoUpdateNow()
	state.LastKnownTag = rel.Tag
	state.ConsecutiveFailures = 0
	state.LastError = ""

	// Only ever move forward. A server bug reporting an old tag must not
	// downgrade the user.
	if model.CompareVersions(rel.Tag, current) <= 0 {
		state.Notice = ""
		return model.WriteUpdateState(state)
	}

	// Already downloaded this release; the marker is waiting for a shell.
	if state.LastAppliedTag == rel.Tag {
		return model.WriteUpdateState(state)
	}

	cliPath, err := autoUpdateResolveCLIPath()
	if err != nil {
		state.Notice = fmt.Sprintf("shelltime %s is available, but the CLI binary could not be located.", rel.Tag)
		return model.WriteUpdateState(state)
	}

	switch model.DetectInstallKind(cliPath) {
	case model.InstallKindHomebrew:
		return s.handleHomebrew(ctx, state, rel.Tag)
	case model.InstallKindUnknown:
		// Never write into a location we don't recognize.
		state.Notice = fmt.Sprintf(
			"shelltime %s is available. Run `shelltime update` to upgrade (binary at %s).", rel.Tag, cliPath)
		return model.WriteUpdateState(state)
	}

	if s.notifyOnly() {
		state.Notice = fmt.Sprintf("shelltime %s is available. Run `shelltime update` to install it.", rel.Tag)
		return model.WriteUpdateState(state)
	}

	// Preflight: can we even write there? This is the /usr/local/bin-without-sudo
	// case. The daemon must never attempt sudo.
	if err := checkWritable(filepath.Dir(cliPath)); err != nil {
		state.Notice = fmt.Sprintf(
			"shelltime %s is available, but %s is not writable. Run `sudo shelltime update`.",
			rel.Tag, filepath.Dir(cliPath))
		return model.WriteUpdateState(state)
	}

	release, ok := model.AcquireUpdateLock(10 * time.Minute)
	if !ok {
		slog.Debug("another process holds the update lock; skipping this cycle")
		return model.WriteUpdateState(state)
	}
	defer release()

	archiveName := ""
	expectedSha := ""
	if rel.Asset != nil {
		archiveName = rel.Asset.Name
		expectedSha = rel.Asset.Sha256
	}

	res, err := autoUpdateApply(ctx, model.UpdatePlan{
		Tag:         rel.Tag,
		ArchiveName: archiveName,
		Source:      model.NewReleaseSource(s.config.APIEndpoint),
		ExpectedSha: expectedSha,
		CLIDest:     cliPath,
		// Stage, don't activate: we are the running daemon.
		DaemonStagePath: model.GetStagedDaemonPath(),
		BackupSuffix:    model.BackupSuffixUpdate,
	})
	if err != nil {
		return s.recordFailure(state, fmt.Errorf("apply update %s: %w", rel.Tag, err))
	}

	state.LastAppliedTag = rel.Tag
	state.PendingDaemonTag = rel.Tag
	state.Notice = ""
	if res.StagedDaemon {
		state.PendingDaemonPath = model.GetStagedDaemonPath()
	} else {
		state.PendingDaemonPath = ""
	}

	// State first, marker last: the CLI must never see a marker without the
	// state that explains it.
	if err := model.WriteUpdateState(state); err != nil {
		return err
	}
	if err := model.WriteDaemonUpdatePending(rel.Tag); err != nil {
		return err
	}

	slog.Info("CLI updated; daemon restart pending",
		slog.String("tag", rel.Tag),
		slog.Bool("stagedDaemon", res.StagedDaemon),
		slog.Bool("usedFallback", res.UsedFallback))
	return nil
}

// handleHomebrew either runs brew (opt-in) or records a notice.
func (s *AutoUpdateService) handleHomebrew(ctx context.Context, state model.UpdateState, tag string) error {
	if !s.homebrewEnabled() || s.notifyOnly() {
		state.Notice = fmt.Sprintf(
			"shelltime %s is available. Run: brew upgrade --cask shelltime/tap/shelltime", tag)
		return model.WriteUpdateState(state)
	}

	release, ok := model.AcquireUpdateLock(20 * time.Minute)
	if !ok {
		return model.WriteUpdateState(state)
	}
	defer release()

	if err := autoUpdateBrewUpgrade(ctx, model.NewCommandService()); err != nil {
		return s.recordFailure(state, fmt.Errorf("brew upgrade: %w", err))
	}

	state.LastAppliedTag = tag
	state.PendingDaemonTag = tag
	// brew owns the binary; the repair only needs to restart the service so the
	// running daemon stops holding the old inode.
	state.PendingDaemonPath = ""
	state.Notice = ""

	if err := model.WriteUpdateState(state); err != nil {
		return err
	}
	return model.WriteDaemonUpdatePending(tag)
}

func (s *AutoUpdateService) homebrewEnabled() bool {
	return s.config.AutoUpdate != nil &&
		s.config.AutoUpdate.Homebrew != nil &&
		*s.config.AutoUpdate.Homebrew
}

func (s *AutoUpdateService) notifyOnly() bool {
	return s.config.AutoUpdate != nil &&
		s.config.AutoUpdate.NotifyOnly != nil &&
		*s.config.AutoUpdate.NotifyOnly
}

func (s *AutoUpdateService) recordFailure(state model.UpdateState, cause error) error {
	state.LastCheckAt = autoUpdateNow()
	state.ConsecutiveFailures++
	state.LastError = cause.Error()
	if writeErr := model.WriteUpdateState(state); writeErr != nil {
		slog.Debug("could not persist update failure", slog.Any("err", writeErr))
	}
	return cause
}

// checkWritable verifies we can create a file in dir, without leaving one behind.
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".shelltime-update-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

// runBrewUpgrade upgrades the cask, falling back to the formula form.
//
// The daemon runs under launchd/systemd with a stripped PATH and no terminal, so
// brew must be located explicitly and can never be allowed to prompt.
func runBrewUpgrade(ctx context.Context, cs model.CommandService) error {
	brew, err := cs.LookPath("brew")
	if err != nil {
		return fmt.Errorf("brew not found: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	env := append(os.Environ(),
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_NO_ANALYTICS=1",
		"HOMEBREW_NO_INSTALL_CLEANUP=1",
		"NONINTERACTIVE=1",
	)

	attempts := [][]string{
		{"upgrade", "--cask", "shelltime/tap/shelltime"},
		{"upgrade", "shelltime/tap/shelltime"},
	}

	var lastErr error
	for _, args := range attempts {
		cmd := exec.CommandContext(ctx, brew, args...)
		cmd.Env = env
		cmd.Stdin = nil // never block waiting for input
		out, err := cmd.CombinedOutput()
		if err == nil {
			slog.Info("brew upgrade succeeded",
				slog.String("args", strings.Join(args, " ")),
				slog.String("output", strings.TrimSpace(string(out))))
			return nil
		}
		lastErr = fmt.Errorf("%s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		slog.Debug("brew attempt failed", slog.Any("err", lastErr))
	}
	return lastErr
}

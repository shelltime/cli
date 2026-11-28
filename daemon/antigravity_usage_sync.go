package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

const antigravityUsageInitialLookback = 24 * time.Hour

var (
	antigravityDataRootsFunc      = model.AntigravityDataRoots
	collectAntigravityDBFilesFunc = model.CollectAntigravityDBFiles
	parseAntigravityDBFilesFunc   = model.ParseAntigravityDBFiles
	antigravityDBFingerprintFunc  = model.AntigravityDBFingerprint
)

// AntigravityUsageSyncService periodically scans local Antigravity SQLite conversation DBs
// and syncs newly updated token usage to the ShellTime server while Antigravity is running.
type AntigravityUsageSyncService struct {
	config       model.ShellTimeConfig
	interval     time.Duration
	fingerprints map[string]string
	hasPending   bool
	mu           sync.Mutex
	ticker       *time.Ticker
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// NewAntigravityUsageSyncService creates a new Antigravity usage sync service.
func NewAntigravityUsageSyncService(config model.ShellTimeConfig) *AntigravityUsageSyncService {
	return &AntigravityUsageSyncService{
		config:       config,
		interval:     usageSyncInterval,
		fingerprints: make(map[string]string),
		stopChan:     make(chan struct{}),
	}
}

// Start begins the periodic Antigravity usage sync job.
func (s *AntigravityUsageSyncService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(s.interval)
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		s.sync(ctx)

		for {
			select {
			case <-s.ticker.C:
				s.sync(ctx)
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	slog.Info("Antigravity usage sync service started", slog.Duration("interval", s.interval))
	return nil
}

// Stop stops the Antigravity usage sync service.
func (s *AntigravityUsageSyncService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Antigravity usage sync service stopped")
}

func (s *AntigravityUsageSyncService) sync(parentCtx context.Context) {
	if s.config.Token == "" {
		return
	}

	s.mu.Lock()
	pending := s.hasPending
	s.mu.Unlock()

	if !isAntigravityRunning() && !pending {
		slog.Debug("Skipping antigravity usage sync", slog.String("reason", "antigravity_not_running"))
		return
	}

	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	if err := s.syncOnce(ctx); err != nil {
		slog.Warn("Failed to sync antigravity usage", slog.Any("err", err))
	}
}

func (s *AntigravityUsageSyncService) syncOnce(ctx context.Context) error {
	now := time.Now()
	since := now.Add(-antigravityUsageInitialLookback)

	roots := antigravityDataRootsFunc()
	files, err := collectAntigravityDBFilesFunc(roots, since)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		s.mu.Lock()
		s.hasPending = false
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	var changed []string
	nextFPs := make(map[string]string, len(files))
	for _, path := range files {
		fp, _, err := antigravityDBFingerprintFunc(path)
		if err != nil {
			continue
		}
		nextFPs[path] = fp
		if prev, ok := s.fingerprints[path]; !ok || prev != fp {
			changed = append(changed, path)
		}
	}
	s.mu.Unlock()

	if len(changed) == 0 {
		s.mu.Lock()
		s.hasPending = false
		s.mu.Unlock()
		return nil
	}

	parseOpts := model.BackfillOptions{
		Since: since,
		Until: now,
		Now:   now,
	}
	parsed, err := parseAntigravityDBFilesFunc(changed, parseOpts)
	if err != nil {
		return err
	}
	activeCount := 0
	if parsed != nil && len(parsed.Sessions) > 0 {
		// Files were already pruned by mtime via CollectAntigravityDBFiles; omit Since here so
		// multi-day conversations that received new events today are not dropped by their Start time.
		uploadOpts := model.BackfillOptions{
			Until: now,
			Now:   now,
		}
		var uploaded int
		uploaded, activeCount, err = uploadDaemonUsageSessions(ctx, s.config, model.AICodeClientAntigravity, parsed.Sessions, uploadOpts, false)
		if err != nil {
			return err
		}
		if uploaded > 0 {
			slog.Debug("Synced antigravity token usage",
				slog.Int("events", uploaded),
				slog.Int("sessions", len(parsed.Sessions)))
		}
	}

	failed := make(map[string]bool)
	if parsed != nil {
		for _, p := range parsed.FailedFiles {
			failed[p] = true
		}
	}
	if activeCount > 0 {
		// Re-scan changed DBs on the next tick so recently active sessions upload once settled.
		for _, p := range changed {
			failed[p] = true
		}
	}

	s.mu.Lock()
	s.hasPending = activeCount > 0 || (parsed != nil && len(parsed.FailedFiles) > 0)
	for path, fp := range nextFPs {
		if failed[path] {
			continue
		}
		s.fingerprints[path] = fp
	}
	s.mu.Unlock()
	return nil
}

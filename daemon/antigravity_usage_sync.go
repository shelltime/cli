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

		s.sync()

		for {
			select {
			case <-s.ticker.C:
				s.sync()
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

func (s *AntigravityUsageSyncService) sync() {
	if s.config.Token == "" {
		return
	}

	if !isAntigravityRunning() {
		slog.Debug("Skipping antigravity usage sync", slog.String("reason", "antigravity_not_running"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		return nil
	}

	parsed, err := parseAntigravityDBFilesFunc(changed, model.BackfillOptions{
		Since: since,
		Now:   now,
	})
	if err != nil {
		return err
	}
	if parsed != nil && len(parsed.Sessions) > 0 {
		uploaded, err := uploadDaemonUsageSessions(ctx, s.config, model.AICodeClientAntigravity, parsed.Sessions)
		if err != nil {
			return err
		}
		if uploaded > 0 {
			slog.Debug("Synced antigravity token usage",
				slog.Int("events", uploaded),
				slog.Int("sessions", len(parsed.Sessions)))
		}
	}

	s.mu.Lock()
	for path, fp := range nextFPs {
		s.fingerprints[path] = fp
	}
	s.mu.Unlock()
	return nil
}

package daemon

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

var CodexUsageSyncInterval = 10 * time.Minute

// CodexUsageSyncService periodically fetches Codex usage and syncs it to the server.
type CodexUsageSyncService struct {
	config   model.ShellTimeConfig
	ticker   *time.Ticker
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewCodexUsageSyncService creates a new Codex usage sync service.
func NewCodexUsageSyncService(config model.ShellTimeConfig) *CodexUsageSyncService {
	return &CodexUsageSyncService{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start begins the periodic Codex usage sync job.
func (s *CodexUsageSyncService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(CodexUsageSyncInterval)
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

	slog.Info("Codex usage sync service started", slog.Duration("interval", CodexUsageSyncInterval))
	return nil
}

// Stop stops the Codex usage sync service.
func (s *CodexUsageSyncService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Codex usage sync service stopped")
}

func (s *CodexUsageSyncService) sync() {
	if s.config.Token == "" {
		return
	}

	if err := syncCodexUsage(context.Background(), s.config); err != nil {
		if reason, ok := CodexSyncSkipReason(err); ok {
			slog.Info("Skipping codex usage sync", slog.String("reason", reason))
			return
		}
		slog.Warn("Failed to sync codex usage", slog.Any("err", err))
	}
}

func syncCodexUsage(ctx context.Context, config model.ShellTimeConfig) error {
	if config.Token == "" {
		return nil
	}

	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	auth, err := loadCodexAuthFunc()
	if err != nil {
		return err
	}
	if auth == nil {
		return errCodexAuthInvalid
	}

	usage, err := fetchCodexUsageFunc(runCtx, auth)
	if err != nil {
		return err
	}

	return sendCodexUsageToServer(runCtx, config, usage)
}

// sendCodexUsageToServer sends Codex usage data to the ShellTime server
// for scheduling push notifications when rate limits reset.
func sendCodexUsageToServer(ctx context.Context, config model.ShellTimeConfig, usage *CodexRateLimitData) error {
	if config.Token == "" {
		return nil
	}

	type usageWindow struct {
		LimitID               string  `json:"limit_id"`
		UsagePercentage       float64 `json:"usage_percentage"`
		ResetsAt              string  `json:"resets_at"`
		WindowDurationMinutes int     `json:"window_duration_minutes"`
	}
	type usagePayload struct {
		Plan    string        `json:"plan"`
		Windows []usageWindow `json:"windows"`
	}

	windows := make([]usageWindow, len(usage.Windows))
	for i, w := range usage.Windows {
		windows[i] = usageWindow{
			LimitID:               w.LimitID,
			UsagePercentage:       w.UsagePercentage,
			ResetsAt:              time.Unix(w.ResetAt, 0).UTC().Format(time.RFC3339),
			WindowDurationMinutes: w.WindowDurationMinutes,
		}
	}

	payload := usagePayload{
		Plan:    usage.Plan,
		Windows: windows,
	}

	return model.SendHTTPRequestJSON(model.HTTPRequestOptions[usagePayload, any]{
		Context: ctx,
		Endpoint: model.Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		Method:  "POST",
		Path:    "/api/v1/codex-usage",
		Payload: payload,
		Timeout: 5 * time.Second,
	})
}

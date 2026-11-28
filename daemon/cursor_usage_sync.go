package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/malamtime/cli/model"
)

const (
	cursorUsageInitialLookback = 24 * time.Hour
	cursorUsageSyncOverlap     = 45 * time.Minute
	cursorUsageStateFileName   = "cursor_usage_sync_state.json"
)

var fetchCursorUsageSessionsFunc = model.FetchCursorUsageSessions

// CursorUsageSyncService periodically fetches Cursor subscription quota and recent billed
// token usage and syncs them to the ShellTime server while Cursor is running.
type CursorUsageSyncService struct {
	config    model.ShellTimeConfig
	interval  time.Duration
	statePath string
	ticker    *time.Ticker
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

type cursorUsageSyncState struct {
	LastSyncedUnix int64 `json:"lastSyncedUnix"`
}

// NewCursorUsageSyncService creates a new Cursor usage sync service.
func NewCursorUsageSyncService(config model.ShellTimeConfig) *CursorUsageSyncService {
	statePath := ""
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		statePath = filepath.Join(home, model.COMMAND_BASE_STORAGE_FOLDER, cursorUsageStateFileName)
	}
	return &CursorUsageSyncService{
		config:    config,
		interval:  usageSyncInterval,
		statePath: statePath,
		stopChan:  make(chan struct{}),
	}
}

// Start begins the periodic Cursor usage sync job.
func (s *CursorUsageSyncService) Start(ctx context.Context) error {
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

	slog.Info("Cursor usage sync service started", slog.Duration("interval", s.interval))
	return nil
}

// Stop stops the Cursor usage sync service.
func (s *CursorUsageSyncService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.wg.Wait()
	slog.Info("Cursor usage sync service stopped")
}

func (s *CursorUsageSyncService) sync(parentCtx context.Context) {
	if s.config.Token == "" {
		return
	}

	if !isCursorRunning() {
		slog.Debug("Skipping cursor usage sync", slog.String("reason", "cursor_not_running"))
		return
	}

	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	auth, err := loadCursorAuthFunc(ctx, s.config.Cursor)
	if err != nil {
		if reason, ok := CursorSyncSkipReason(err); ok {
			slog.Debug("Skipping cursor usage sync", slog.String("reason", reason))
			return
		}
		slog.Warn("Failed to load cursor auth", slog.Any("err", err))
		return
	}

	if err := syncCursorQuota(ctx, s.config, auth); err != nil {
		if reason, ok := CursorSyncSkipReason(err); ok {
			slog.Info("Skipping cursor quota sync", slog.String("reason", reason))
		} else {
			slog.Warn("Failed to sync cursor quota", slog.Any("err", err))
		}
	}

	if err := s.syncTokenUsage(ctx, auth); err != nil {
		if reason, ok := CursorSyncSkipReason(err); ok {
			slog.Info("Skipping cursor token usage sync", slog.String("reason", reason))
		} else {
			slog.Warn("Failed to sync cursor token usage", slog.Any("err", err))
		}
	}
}

func syncCursorQuota(ctx context.Context, config model.ShellTimeConfig, auth *model.CursorAuthData) error {
	if config.Token == "" {
		return nil
	}

	quota, err := fetchCursorQuotaFunc(ctx, auth)
	if err != nil {
		return err
	}
	if quota == nil || len(quota.Windows) == 0 {
		return nil
	}

	return sendCursorUsageToServer(ctx, config, quota)
}

func sendCursorUsageToServer(ctx context.Context, config model.ShellTimeConfig, usage *CursorRateLimitData) error {
	if config.Token == "" || usage == nil || len(usage.Windows) == 0 {
		return nil
	}

	type usageWindow struct {
		LimitID               string  `json:"limit_id"`
		LimitName             string  `json:"limit_name"`
		UsagePercentage       float64 `json:"usage_percentage"`
		ResetsAt              string  `json:"resets_at"`
		WindowDurationMinutes int     `json:"window_duration_minutes"`
	}
	type usagePayload struct {
		SchemaVersion int           `json:"schema_version"`
		Plan          string        `json:"plan"`
		Windows       []usageWindow `json:"windows"`
	}

	windows := make([]usageWindow, len(usage.Windows))
	for i, w := range usage.Windows {
		windows[i] = usageWindow{
			LimitID:               w.LimitID,
			LimitName:             w.LimitName,
			UsagePercentage:       w.UsagePercentage,
			ResetsAt:              time.Unix(w.ResetAt, 0).UTC().Format(time.RFC3339),
			WindowDurationMinutes: w.WindowDurationMinutes,
		}
	}

	payload := usagePayload{
		SchemaVersion: 1,
		Plan:          usage.Plan,
		Windows:       windows,
	}

	return model.SendHTTPRequestJSON(model.HTTPRequestOptions[usagePayload, any]{
		Context: ctx,
		Endpoint: model.Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		Method:  "POST",
		Path:    "/api/v1/cursor-usage",
		Payload: payload,
		Timeout: 5 * time.Second,
	})
}

func (s *CursorUsageSyncService) syncTokenUsage(ctx context.Context, auth *model.CursorAuthData) error {
	now := time.Now()
	since := now.Add(-cursorUsageInitialLookback)
	if st := s.loadState(); st.LastSyncedUnix > 0 {
		candidate := time.Unix(st.LastSyncedUnix, 0).Add(-cursorUsageSyncOverlap)
		if candidate.After(since) && candidate.Before(now) {
			since = candidate
		}
	}

	opts := model.BackfillOptions{
		Since: since,
		Until: now,
		Now:   now,
	}
	parsed, err := fetchCursorUsageSessionsFunc(ctx, auth, opts)
	if err != nil {
		return err
	}
	if parsed == nil || len(parsed.Sessions) == 0 {
		s.saveState(cursorUsageSyncState{LastSyncedUnix: now.Unix()})
		return nil
	}

	uploaded, _, err := uploadDaemonUsageSessions(ctx, s.config, model.AICodeClientCursor, parsed.Sessions, opts, true)
	if err != nil {
		return err
	}
	if uploaded > 0 {
		slog.Debug("Synced cursor token usage", slog.Int("events", uploaded), slog.Int("sessions", len(parsed.Sessions)))
	}
	s.saveState(cursorUsageSyncState{LastSyncedUnix: now.Unix()})
	return nil
}

func (s *CursorUsageSyncService) loadState() cursorUsageSyncState {
	if s.statePath == "" {
		return cursorUsageSyncState{}
	}
	raw, err := os.ReadFile(s.statePath)
	if err != nil {
		return cursorUsageSyncState{}
	}
	var st cursorUsageSyncState
	if err := json.Unmarshal(raw, &st); err != nil {
		return cursorUsageSyncState{}
	}
	return st
}

func (s *CursorUsageSyncService) saveState(st cursorUsageSyncState) {
	if s.statePath == "" {
		return
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.statePath), 0700)
	_ = os.WriteFile(s.statePath, raw, 0600)
}

// uploadDaemonUsageSessions filters settled sessions within opts (respecting the backfill settle
// window so in-progress sessions are not finalized early), checks server backfill status, and
// uploads new/updated events idempotently via /api/v1/cc/backfill + /api/v1/cc/backfill/complete.
// When skipCountCheck is true (for incremental window fetches like Cursor), already-backfilled
// sessions are still uploaded so server-side event_id deduplication accepts new events.
func uploadDaemonUsageSessions(
	ctx context.Context,
	config model.ShellTimeConfig,
	clientType string,
	sessions []*model.BackfillSession,
	opts model.BackfillOptions,
	skipCountCheck bool,
) (int, int, error) {
	if config.Token == "" || len(sessions) == 0 {
		return 0, 0, nil
	}
	settled, activeCount := model.SelectBackfillSessions(sessions, opts)
	if len(settled) == 0 {
		return 0, activeCount, nil
	}
	sessions = settled
	endpoint := model.Endpoint{
		Token:       config.Token,
		APIEndpoint: config.APIEndpoint,
	}

	sessionIDs := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		if len(sess.Events) > 0 {
			sessionIDs = append(sessionIDs, sess.ID)
		}
	}
	if len(sessionIDs) == 0 {
		return 0, activeCount, nil
	}

	statuses := map[string]model.AICodeBackfillSessionStatus{}
	for start := 0; start < len(sessionIDs); start += model.AICodeBackfillMaxSessionIDs {
		end := min(start+model.AICodeBackfillMaxSessionIDs, len(sessionIDs))
		rows, err := model.FetchAICodeBackfillStatus(ctx, endpoint, clientType, sessionIDs[start:end])
		if err != nil {
			return 0, activeCount, err
		}
		for _, st := range rows {
			statuses[st.SessionID] = st
		}
	}

	var toUpload []*model.BackfillSession
	for _, sess := range sessions {
		if len(sess.Events) == 0 {
			continue
		}
		st, ok := statuses[sess.ID]
		if !ok {
			toUpload = append(toUpload, sess)
			continue
		}
		if st.Status == model.AICodeBackfillStatusLive || st.Status == model.AICodeBackfillStatusArchived {
			continue
		}
		if !skipCountCheck && st.Status == model.AICodeBackfillStatusBackfilled && st.EventCount >= len(sess.Events) {
			continue
		}
		toUpload = append(toUpload, sess)
	}
	if len(toUpload) == 0 {
		return 0, activeCount, nil
	}

	batches := model.PackBackfillBatches(
		clientType,
		toUpload,
		model.AICodeBackfillMaxEvents,
		model.AICodeBackfillMaxBatchBytes,
		model.AICodeBackfillMaxCompleted,
	)

	accepted := 0
	for _, batch := range batches {
		resp, err := model.SendAICodeBackfill(ctx, endpoint, batch)
		if err != nil {
			return accepted, activeCount, err
		}
		accepted += resp.Accepted
	}

	from := toUpload[0].Start
	to := toUpload[0].End
	for _, sess := range toUpload[1:] {
		if sess.Start.Before(from) {
			from = sess.Start
		}
		if sess.End.After(to) {
			to = sess.End
		}
	}
	if err := model.CompleteAICodeBackfill(ctx, endpoint, model.AICodeBackfillCompleteRequest{
		ClientType: clientType,
		From:       from,
		To:         to,
	}); err != nil {
		slog.Warn("Failed to finalize daemon usage sync", slog.String("clientType", clientType), slog.Any("err", err))
	}

	return accepted, activeCount, nil
}

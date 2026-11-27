package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/malamtime/cli/agenthook"
	"github.com/malamtime/cli/model"
)

const (
	// AgentHookResyncInterval is how often saved agent hook events are sent again.
	AgentHookResyncInterval = 30 * time.Minute
	// agentHookResyncChunkSize caps the saved events sent in one pass's request.
	agentHookResyncChunkSize = 500
)

// AgentHookResyncService sends again the Cursor and Antigravity hook events saved in
// ~/.shelltime/agent-hooks.pending.jsonl: by hook-ingest when no daemon took them, and by the
// daemon when the server couldn't. It runs at start and every AgentHookResyncInterval, like
// HeartbeatResyncService. Each pass readies the events as the batcher does, under today's
// config (prepareAgentHookEvents: repository, privacy). Events the server refused for what
// they hold are dropped; the rest that fail stay for the next pass, with the lines saved
// meanwhile.
type AgentHookResyncService struct {
	config        model.ShellTimeConfig
	configService model.ConfigService
	// repos resolves the repository of events saved without one (hook-ingest has no git
	// lookup); nil adds none, like the batcher's when aiCodeOtel.sendRepository is off.
	repos *model.GitRepositoryResolver
	// send is replaced in tests
	send     func(ctx context.Context, cfg model.ShellTimeConfig, events []agenthook.Event) error
	ticker   *time.Ticker
	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewAgentHookResyncService(config model.ShellTimeConfig) *AgentHookResyncService {
	return &AgentHookResyncService{
		config:   config,
		repos:    socketRepositoryResolver(config),
		send:     model.SendAgentHookEvents,
		stopChan: make(chan struct{}),
	}
}

// SetConfigService makes each pass use the config on disk instead of the one the daemon
// started with.
func (s *AgentHookResyncService) SetConfigService(cs model.ConfigService) {
	s.configService = cs
}

func (s *AgentHookResyncService) currentConfig(ctx context.Context) model.ShellTimeConfig {
	if s.configService != nil {
		if cfg, err := s.configService.ReadConfigFile(ctx, model.WithSkipCache()); err == nil {
			return cfg
		}
	}
	return s.config
}

// Start runs a pass now and then every AgentHookResyncInterval.
func (s *AgentHookResyncService) Start(ctx context.Context) error {
	s.ticker = time.NewTicker(AgentHookResyncInterval)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.resync(ctx)
		for {
			select {
			case <-s.ticker.C:
				s.resync(ctx)
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	slog.Info("Agent hook resync service started", slog.Duration("interval", AgentHookResyncInterval))
	return nil
}

// Stop stops the service and waits for a pass under way.
func (s *AgentHookResyncService) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	s.stopOnce.Do(func() { close(s.stopChan) })
	s.wg.Wait()
	slog.Info("Agent hook resync service stopped")
}

// resync sends the saved events in chunks and rewrites the file with the lines to keep.
func (s *AgentHookResyncService) resync(ctx context.Context) {
	lines, err := model.ReadAgentHookPendingLines()
	if os.IsNotExist(err) || (err == nil && len(lines) == 0) {
		return
	}
	if err != nil {
		slog.Error("Failed to read saved agent hook events", slog.Any("err", err))
		return
	}

	cfg := s.currentConfig(ctx)
	if cfg.Token == "" {
		// Every request would be refused; keep them for when the user signs in
		slog.Debug("No token configured, keeping saved agent hook events", slog.Int("count", len(lines)))
		return
	}

	// Each event next to its line: a line kept for the next pass is kept as it was saved
	var events []agenthook.Event
	var eventLines []string
	discarded := 0
	for _, line := range lines {
		var ev agenthook.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			discarded++
			continue
		}
		events = append(events, ev)
		eventLines = append(eventLines, line)
	}
	// hook-ingest applied the settings of its time and looked up no repository
	prepareAgentHookEvents(ctx, cfg, s.repos, events)

	var kept []string
	sent, dropped := 0, 0
	for start := 0; start < len(events); start += agentHookResyncChunkSize {
		end := min(start+agentHookResyncChunkSize, len(events))
		err := s.send(ctx, cfg, events[start:end])
		if err == nil {
			sent += end - start
			continue
		}
		if ctx.Err() != nil || agentHookSendCanWait(err) {
			// The server is away: keep this chunk and the ones after it untried
			kept = append(kept, eventLines[start:]...)
			slog.Warn("Failed to resend agent hook events, keeping them for the next pass", slog.Any("err", err))
			break
		}
		dropped += end - start
		slog.Warn("Server refused saved agent hook events, dropping them", slog.Int("count", end-start), slog.Any("err", err))
	}

	// Events saved while this pass was sending were appended after the lines it read. If the
	// file no longer starts with them (log cleanup removed it meanwhile), every line in it is new.
	err = model.UpdateAgentHookPending(func(current []string) []string {
		if hasPrefixLines(current, lines) {
			current = current[len(lines):]
		}
		return append(kept, current...)
	})
	if errors.Is(err, model.ErrAgentHookPendingBusy) {
		// Nothing changed: the next pass sends again what this one sent, which the server
		// recognizes by the event ids
		slog.Warn("Saved agent hook events are locked by another process, leaving them for the next pass",
			slog.Int("sent", sent), slog.Int("dropped", dropped))
		return
	}
	if err != nil {
		slog.Error("Failed to update saved agent hook events", slog.Any("err", err))
		return
	}
	slog.Info("Agent hook resync completed",
		slog.Int("sent", sent), slog.Int("kept", len(kept)), slog.Int("dropped", dropped), slog.Int("discarded", discarded))
}

package daemon

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/malamtime/cli/agenthook"
	"github.com/malamtime/cli/model"
)

const (
	// agentHookFlushInterval is how long hook events wait in the daemon before they are sent.
	agentHookFlushInterval = 2 * time.Second
	// agentHookMaxBatch sends the queued events at once when this many are waiting.
	agentHookMaxBatch = 100
	// agentHookSessionTTL is how long the last session id seen for an agent and workspace is
	// lent to events that come without one, and how long after a cloud session's last event its
	// later events are still marked as cloud.
	agentHookSessionTTL = time.Hour
	// agentHookShutdownTimeout bounds sending what is still queued when the daemon stops;
	// whatever doesn't make it is saved for the resync service.
	agentHookShutdownTimeout = 3 * time.Second
)

// AgentHookBatcher collects the Cursor and Antigravity hook events hook-ingest hands to the
// daemon and sends them in batches: every agentHookFlushInterval, or as soon as
// agentHookMaxBatch are waiting. Before sending, each event gets the git repository of its
// workspace and the user's privacy settings, read from the config on disk, and the events of a
// session an earlier event showed was a cloud run are marked as cloud too. A batch the server
// can take later (no answer, a 5xx, an expired token, a server without the endpoint yet), or
// one there is no token to send with, is saved to the pending file for
// AgentHookResyncService; one the server refused for what it holds is dropped.
type AgentHookBatcher struct {
	config        model.ShellTimeConfig
	configService model.ConfigService
	// repos resolves a workspace's repository; nil sends none (aiCodeOtel.sendRepository: false).
	repos *model.GitRepositoryResolver

	// Replaced in tests
	send          func(ctx context.Context, cfg model.ShellTimeConfig, events []agenthook.Event) error
	now           func() time.Time
	flushInterval time.Duration
	maxBatch      int

	mu       sync.Mutex
	pending  []agenthook.Event
	sessions map[agentHookSessionKey]agentHookSession
	clouds   map[agentHookCloudKey]agentHookCloud
	started  bool
	closed   bool

	// flushMu keeps one flush at a time: the loop's and the one at shutdown.
	flushMu sync.Mutex
	flushCh chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

type agentHookSessionKey struct {
	agent, workspace string
}

type agentHookSession struct {
	id   string
	seen time.Time
}

type agentHookCloudKey struct {
	agent, sessionID string
}

// agentHookCloud is a session an event showed was a cloud run, with that event's signal.
type agentHookCloud struct {
	signal string
	seen   time.Time
}

// NewAgentHookBatcher returns a batcher that sends with cfg (or, once SetConfigService is
// called, the config on disk at each batch). Its loop starts with the first event.
func NewAgentHookBatcher(cfg model.ShellTimeConfig, repos *model.GitRepositoryResolver) *AgentHookBatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &AgentHookBatcher{
		config:        cfg,
		repos:         repos,
		send:          model.SendAgentHookEvents,
		now:           time.Now,
		flushInterval: agentHookFlushInterval,
		maxBatch:      agentHookMaxBatch,
		sessions:      make(map[agentHookSessionKey]agentHookSession),
		clouds:        make(map[agentHookCloudKey]agentHookCloud),
		flushCh:       make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
}

// SetConfigService makes each batch use the config on disk, so a new token, exclude pattern
// or agentHooks.sendContent applies without a restart.
func (b *AgentHookBatcher) SetConfigService(cs model.ConfigService) {
	b.configService = cs
}

// Add queues an event. An event without a session id gets the last one an event carried for
// its agent and workspace within agentHookSessionTTL (Cursor sometimes sends tool events with
// an empty conversation_id), and one of a cloud session is marked as cloud. After Shutdown the
// event is saved for the resync service instead.
func (b *AgentHookBatcher) Add(ev agenthook.Event) {
	b.mu.Lock()
	b.fillSessionLocked(&ev)
	b.fillCloudLocked(&ev)
	if b.closed {
		b.mu.Unlock()
		// No repository lookup here: git can take seconds, and hook-ingest waits for the answer
		model.ApplyAgentHookPrivacy(&ev, b.currentConfig(context.Background()))
		b.save([]agenthook.Event{ev}, "daemon stopping")
		return
	}
	b.pending = append(b.pending, ev)
	full := len(b.pending) >= b.maxBatch
	if !b.started {
		b.started = true
		go b.loop()
	}
	b.mu.Unlock()

	if full {
		select {
		case b.flushCh <- struct{}{}:
		default:
		}
	}
}

// Shutdown sends what is queued within ctx and saves what it couldn't send. A send already
// under way is cut short and saved too.
func (b *AgentHookBatcher) Shutdown(ctx context.Context) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	started := b.started
	b.mu.Unlock()

	b.cancel()
	if started {
		<-b.done
	}
	b.flush(ctx)
}

func (b *AgentHookBatcher) loop() {
	defer close(b.done)
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.flush(b.ctx)
		case <-b.flushCh:
			b.flush(b.ctx)
		}
	}
}

// fillSessionLocked lends an event without a session id the last one seen for its agent and
// workspace. Only an event that carries its own id keeps that id lendable for longer, so a run
// of events without one never stretches it past agentHookSessionTTL. Events without a
// workspace are left alone: every agent's events without one would share a key.
func (b *AgentHookBatcher) fillSessionLocked(ev *agenthook.Event) {
	if ev.Workspace == "" {
		return
	}
	key := agentHookSessionKey{agent: ev.Agent, workspace: ev.Workspace}
	now := b.now()
	if ev.SessionID != "" {
		b.sessions[key] = agentHookSession{id: ev.SessionID, seen: now}
		return
	}
	if s, ok := b.sessions[key]; ok && now.Sub(s.seen) < agentHookSessionTTL {
		ev.SessionID = s.id
	}
}

// fillCloudLocked marks an event of a session an earlier event showed was a cloud run: Cursor
// says so only in sessionStart (is_background_agent). The session stays marked for
// agentHookSessionTTL after its last event.
func (b *AgentHookBatcher) fillCloudLocked(ev *agenthook.Event) {
	if ev.SessionID == "" {
		return
	}
	key := agentHookCloudKey{agent: ev.Agent, sessionID: ev.SessionID}
	now := b.now()
	if ev.Cloud == "" {
		c, ok := b.clouds[key]
		if !ok || now.Sub(c.seen) >= agentHookSessionTTL {
			return
		}
		ev.Cloud = c.signal
	}
	b.clouds[key] = agentHookCloud{signal: ev.Cloud, seen: now}
}

func (b *AgentHookBatcher) pruneSessionsLocked() {
	now := b.now()
	for key, s := range b.sessions {
		if now.Sub(s.seen) >= agentHookSessionTTL {
			delete(b.sessions, key)
		}
	}
	for key, c := range b.clouds {
		if now.Sub(c.seen) >= agentHookSessionTTL {
			delete(b.clouds, key)
		}
	}
}

// flush sends the queued events, chunk by chunk. After a chunk fails in a way that can work
// later, the rest is saved without trying: the server is away.
func (b *AgentHookBatcher) flush(ctx context.Context) {
	b.flushMu.Lock()
	defer b.flushMu.Unlock()

	b.mu.Lock()
	events := b.pending
	b.pending = nil
	b.pruneSessionsLocked()
	b.mu.Unlock()
	if len(events) == 0 {
		return
	}

	cfg := b.currentConfig(ctx)
	prepareAgentHookEvents(ctx, cfg, b.repos, events)
	if cfg.Token == "" {
		// Every request would be refused; keep them for when the user signs in
		b.save(events, "no token")
		slog.Debug("No token configured, agent hook events saved", slog.Int("count", len(events)))
		return
	}

	sent, dropped := 0, 0
	chunks := model.SplitAgentHookEvents(events, model.AgentHookMaxEventsPerRequest, model.AgentHookMaxRequestBytes)
	for i, chunk := range chunks {
		err := b.send(ctx, cfg, chunk)
		if err == nil {
			sent += len(chunk)
			continue
		}
		if ctx.Err() != nil || agentHookSendCanWait(err) {
			var rest []agenthook.Event
			for _, c := range chunks[i:] {
				rest = append(rest, c...)
			}
			slog.Warn("Failed to send agent hook events, saving them for the resync service",
				slog.Int("sent", sent), slog.Int("saved", len(rest)), slog.Any("err", err))
			b.save(rest, "send failed")
			return
		}
		// Refused for what the batch holds: sending it again would be refused again
		dropped += len(chunk)
		slog.Warn("Server refused agent hook events, dropping them", slog.Int("count", len(chunk)), slog.Any("err", err))
	}
	slog.Debug("Agent hook events sent", slog.Int("sent", sent), slog.Int("dropped", dropped))
}

// prepareAgentHookEvents readies events to leave the machine under cfg, for the batcher and
// the resync service alike: with aiCodeOtel.sendRepository on, an event with a workspace and
// no repository gets the workspace's (repos nil adds none); off, every repository is removed.
// Then the privacy settings apply. It can run more than once on the same events.
func prepareAgentHookEvents(ctx context.Context, cfg model.ShellTimeConfig, repos *model.GitRepositoryResolver, events []agenthook.Event) {
	sendsRepository := cfg.SendsRepository()
	repositories := map[string]string{}
	for i := range events {
		ev := &events[i]
		switch {
		case !sendsRepository:
			ev.Repository = ""
		case repos != nil && ev.Repository == "" && ev.Workspace != "":
			url, ok := repositories[ev.Workspace]
			if !ok {
				url = repos.Resolve(ctx, ev.Workspace).URL
				repositories[ev.Workspace] = url
			}
			ev.Repository = url
		}
		model.ApplyAgentHookPrivacy(ev, cfg)
	}
}

// agentHookSendCanWait reports whether events the server didn't take can be sent later, so
// they are kept: what the command sync retries (no answer, a 5xx, 429, an expired token or
// plan), and a 404 or 405, which a server without the agent hook endpoint answers (one from
// before it, or before it is deployed). Any other refusal is for what the events hold.
func agentHookSendCanWait(err error) bool {
	if isRetryableSyncError(err) {
		return true
	}
	var statusErr *model.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusNotFound || statusErr.StatusCode == http.StatusMethodNotAllowed
	}
	return false
}

// currentConfig reads the config on disk, else the last one read, else the one the daemon
// started with.
func (b *AgentHookBatcher) currentConfig(ctx context.Context) model.ShellTimeConfig {
	if b.configService != nil {
		if cfg, err := b.configService.ReadConfigFile(ctx, model.WithSkipCache()); err == nil {
			return cfg
		}
		if cfg, err := b.configService.ReadConfigFile(ctx); err == nil {
			return cfg
		}
	}
	return b.config
}

func (b *AgentHookBatcher) save(events []agenthook.Event, reason string) {
	if err := model.AppendAgentHookPending(events...); err != nil {
		slog.Error("Failed to save agent hook events", slog.String("reason", reason), slog.Int("count", len(events)), slog.Any("err", err))
	}
}

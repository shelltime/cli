package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malamtime/cli/agenthook"
	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const agentHookTestJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"

// agentHookServer records the agent hook requests it gets and answers with status.
type agentHookServer struct {
	*httptest.Server
	mu       sync.Mutex
	status   int
	requests []model.AgentHookRequest
	got      chan model.AgentHookRequest
}

func newAgentHookServer(t *testing.T, status int) *agentHookServer {
	t.Helper()
	s := &agentHookServer{status: status, got: make(chan model.AgentHookRequest, 16)}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/agent-hooks", r.URL.Path)
		var req model.AgentHookRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		s.mu.Lock()
		s.requests = append(s.requests, req)
		status := s.status
		s.mu.Unlock()
		s.got <- req
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(model.AgentHookResponse{Success: true, EventsProcessed: len(req.Events)})
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *agentHookServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func waitAgentHookRequest(t *testing.T, s *agentHookServer) model.AgentHookRequest {
	t.Helper()
	select {
	case req := <-s.got:
		return req
	case <-time.After(3 * time.Second):
		t.Fatal("no agent hook request reached the server")
		return model.AgentHookRequest{}
	}
}

// isolateAgentHookHome gives the test its own HOME, so its pending file is its own.
func isolateAgentHookHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func savedAgentHookEvents(t *testing.T) []agenthook.Event {
	t.Helper()
	lines, err := model.ReadAgentHookPendingLines()
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var events []agenthook.Event
	for _, line := range lines {
		var ev agenthook.Event
		require.NoError(t, json.Unmarshal([]byte(line), &ev))
		events = append(events, ev)
	}
	return events
}

func hookEvent(id, session, workspace string) agenthook.Event {
	return agenthook.Event{
		ID:        id,
		Agent:     agenthook.AgentCursor,
		SessionID: session,
		EventType: agenthook.EventPromptSubmit,
		Workspace: workspace,
		Prompt:    "deploy with " + agentHookTestJWT,
	}
}

func TestAgentHookBatcher_FlushesOnTimer(t *testing.T) {
	isolateAgentHookHome(t)
	srv := newAgentHookServer(t, http.StatusOK)
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}, nil)
	b.flushInterval = 20 * time.Millisecond
	t.Cleanup(func() { b.Shutdown(context.Background()) })

	b.Add(hookEvent("a", "s1", "/w"))
	b.Add(hookEvent("b", "s1", "/w"))

	req := waitAgentHookRequest(t, srv)
	require.Len(t, req.Events, 2)
	assert.Equal(t, "a", req.Events[0].ID)
	assert.NotEmpty(t, req.Host)
	assert.NotContains(t, req.Events[0].Prompt, agentHookTestJWT, "masked before it leaves")
	assert.Empty(t, savedAgentHookEvents(t))
}

func TestAgentHookBatcher_FlushesWhenFull(t *testing.T) {
	isolateAgentHookHome(t)
	srv := newAgentHookServer(t, http.StatusOK)
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}, nil)
	b.flushInterval = time.Hour
	b.maxBatch = 3
	t.Cleanup(func() { b.Shutdown(context.Background()) })

	for _, id := range []string{"a", "b", "c"} {
		b.Add(hookEvent(id, "s1", "/w"))
	}

	req := waitAgentHookRequest(t, srv)
	assert.Len(t, req.Events, 3)
}

func TestAgentHookBatcher_UsesConfigOnDisk(t *testing.T) {
	isolateAgentHookHome(t)
	srv := newAgentHookServer(t, http.StatusOK)
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: "http://127.0.0.1:1", Token: "old"}, nil)
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything, mock.Anything).Return(model.ShellTimeConfig{
		APIEndpoint: srv.URL,
		Token:       "new",
		AgentHooks:  &model.AgentHooksConfig{SendContent: new(false)},
	}, nil).Maybe()
	b.SetConfigService(mc)
	b.flushInterval = time.Hour

	b.Add(hookEvent("a", "s1", "/w"))
	b.Shutdown(context.Background())

	req := waitAgentHookRequest(t, srv)
	require.Len(t, req.Events, 1)
	assert.Empty(t, req.Events[0].Prompt, "agentHooks.sendContent read from disk")
}

func TestAgentHookBatcher_SavesOnRetryableFailure(t *testing.T) {
	isolateAgentHookHome(t)
	srv := newAgentHookServer(t, http.StatusServiceUnavailable)
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}, nil)
	b.flushInterval = time.Hour

	b.Add(hookEvent("a", "s1", "/w"))
	b.Add(hookEvent("b", "s1", "/w"))
	b.flush(context.Background())

	saved := savedAgentHookEvents(t)
	require.Len(t, saved, 2)
	assert.Equal(t, "a", saved[0].ID)
	assert.NotContains(t, saved[0].Prompt, agentHookTestJWT, "saved with the privacy settings applied")
}

func TestAgentHookBatcher_SavesWhenServerIsUnreachable(t *testing.T) {
	isolateAgentHookHome(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: url, Token: "tok"}, nil)

	b.Add(hookEvent("a", "s1", "/w"))
	b.Shutdown(context.Background())

	assert.Len(t, savedAgentHookEvents(t), 1)
}

func TestAgentHookBatcher_DropsWhatTheServerRefuses(t *testing.T) {
	isolateAgentHookHome(t)
	srv := newAgentHookServer(t, http.StatusBadRequest)
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}, nil)

	b.Add(hookEvent("a", "s1", "/w"))
	b.flush(context.Background())

	assert.Equal(t, 1, srv.requestCount())
	assert.Empty(t, savedAgentHookEvents(t))
}

// A server without the endpoint (older, or not deployed yet) answers 404 or 405: the events
// wait for one that has it.
func TestAgentHookBatcher_SavesWhenTheServerHasNoEndpoint(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			isolateAgentHookHome(t)
			srv := newAgentHookServer(t, status)
			b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}, nil)

			b.Add(hookEvent("a", "s1", "/w"))
			b.flush(context.Background())

			assert.Equal(t, 1, srv.requestCount())
			saved := savedAgentHookEvents(t)
			require.Len(t, saved, 1)
			assert.Equal(t, "a", saved[0].ID)
		})
	}
}

func TestAgentHookBatcher_SavesWithoutToken(t *testing.T) {
	isolateAgentHookHome(t)
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: "http://unused"}, nil)
	b.send = func(context.Context, model.ShellTimeConfig, []agenthook.Event) error {
		t.Error("nothing is sent without a token")
		return nil
	}

	b.Add(hookEvent("a", "s1", "/w"))
	b.Shutdown(context.Background())

	saved := savedAgentHookEvents(t)
	require.Len(t, saved, 1)
	assert.Equal(t, "a", saved[0].ID)
	assert.NotContains(t, saved[0].Prompt, agentHookTestJWT, "saved with the privacy settings applied")
}

func TestAgentHookSendCanWait(t *testing.T) {
	status := func(code int) error { return &model.HTTPStatusError{StatusCode: code} }
	for code, want := range map[int]bool{
		http.StatusNotFound:              true,
		http.StatusMethodNotAllowed:      true,
		http.StatusUnauthorized:          true,
		http.StatusTooManyRequests:       true,
		http.StatusBadGateway:            true,
		http.StatusBadRequest:            false,
		http.StatusRequestEntityTooLarge: false,
		http.StatusUnprocessableEntity:   false,
	} {
		assert.Equal(t, want, agentHookSendCanWait(status(code)), code)
	}
	assert.True(t, agentHookSendCanWait(fmt.Errorf("wrapped: %w", status(http.StatusNotFound))))
	assert.False(t, agentHookSendCanWait(errors.New("encode failed")))
}

func TestAgentHookBatcher_StopsAfterFirstRetryableChunk(t *testing.T) {
	isolateAgentHookHome(t)
	var calls int
	b := NewAgentHookBatcher(model.ShellTimeConfig{Token: "tok"}, nil)
	b.send = func(ctx context.Context, cfg model.ShellTimeConfig, events []agenthook.Event) error {
		calls++
		return &model.HTTPStatusError{StatusCode: http.StatusBadGateway, Message: "bad gateway"}
	}
	b.maxBatch = 10_000
	for i := range model.AgentHookMaxEventsPerRequest + 5 {
		b.pending = append(b.pending, hookEvent(string(rune('a'+i%26)), "s", "/w"))
	}

	b.flush(context.Background())

	assert.Equal(t, 1, calls, "the second chunk is saved without trying")
	assert.Len(t, savedAgentHookEvents(t), model.AgentHookMaxEventsPerRequest+5)
}

func TestAgentHookBatcher_FillsSessionIDs(t *testing.T) {
	b := NewAgentHookBatcher(model.ShellTimeConfig{}, nil)
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	b.flushInterval = time.Hour
	t.Cleanup(func() { b.cancel() })

	b.Add(hookEvent("1", "s1", "/w"))
	b.Add(hookEvent("2", "", "/w"))
	b.Add(hookEvent("3", "", "/other"))
	other := hookEvent("4", "", "/w")
	other.Agent = agenthook.AgentAntigravity
	b.Add(other)
	b.Add(hookEvent("5", "sX", ""))
	b.Add(hookEvent("6", "", ""))

	now = now.Add(59 * time.Minute)
	b.Add(hookEvent("7", "", "/w"))
	now = now.Add(2 * time.Minute)
	b.Add(hookEvent("8", "", "/w"))
	b.Add(hookEvent("9", "s2", "/w"))
	now = now.Add(59 * time.Minute)
	b.Add(hookEvent("10", "", "/w"))

	b.mu.Lock()
	defer b.mu.Unlock()
	sessions := map[string]string{}
	for _, ev := range b.pending {
		sessions[ev.ID] = ev.SessionID
	}
	assert.Equal(t, map[string]string{
		"1":  "s1",
		"2":  "s1", // same agent and workspace
		"3":  "",   // another workspace
		"4":  "",   // another agent
		"5":  "sX",
		"6":  "",   // no workspace: nothing is lent
		"7":  "s1", // within the hour of event 1
		"8":  "",   // after it: lending to event 7 didn't extend it
		"9":  "s2",
		"10": "s2", // within the hour of event 9
	}, sessions)
	assert.NotContains(t, b.sessions, agentHookSessionKey{agent: agenthook.AgentCursor}, "events without a workspace aren't remembered")
}

func TestAgentHookBatcher_MarksCloudSessions(t *testing.T) {
	b := NewAgentHookBatcher(model.ShellTimeConfig{}, nil)
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	b.flushInterval = time.Hour
	t.Cleanup(func() { b.cancel() })

	cloud := func(id, session, workspace string) agenthook.Event {
		ev := hookEvent(id, session, workspace)
		ev.Cloud = agenthook.CloudSignalCursorBackgroundAgent
		return ev
	}

	b.Add(cloud("1", "c1", "/w")) // Cursor's sessionStart in a Cloud Agent
	b.Add(hookEvent("2", "c1", "/w"))
	b.Add(hookEvent("3", "", "/w")) // gets c1 lent first, then its mark
	b.Add(hookEvent("4", "c2", "/w"))
	other := hookEvent("5", "c1", "/w")
	other.Agent = agenthook.AgentAntigravity
	b.Add(other)
	b.Add(cloud("6", "", "/elsewhere"))
	b.Add(hookEvent("7", "", "/elsewhere"))

	// Every event of the session keeps it marked for an hour after it
	now = now.Add(59 * time.Minute)
	b.Add(hookEvent("8", "c1", ""))
	now = now.Add(59 * time.Minute)
	b.Add(hookEvent("9", "c1", ""))
	now = now.Add(61 * time.Minute)
	b.Add(hookEvent("10", "c1", ""))

	// A signal of the event's own is kept over the session's
	env := hookEvent("11", "c3", "")
	env.Cloud = agenthook.CloudSignalEnv
	b.Add(env)

	b.mu.Lock()
	defer b.mu.Unlock()
	clouds := map[string]string{}
	for _, ev := range b.pending {
		clouds[ev.ID] = ev.Cloud
	}
	bg := agenthook.CloudSignalCursorBackgroundAgent
	assert.Equal(t, map[string]string{
		"1":  bg,
		"2":  bg,
		"3":  bg,
		"4":  "", // another session
		"5":  "", // the same id from another agent
		"6":  bg,
		"7":  "", // no session id to remember the mark by
		"8":  bg,
		"9":  bg, // within the hour of event 8
		"10": "", // more than an hour after the session's last event
		"11": agenthook.CloudSignalEnv,
	}, clouds)
}

func TestAgentHookBatcher_YieldsToUserHooksUnlessCloud(t *testing.T) {
	b := NewAgentHookBatcher(model.ShellTimeConfig{}, nil)
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	b.flushInterval = time.Hour
	t.Cleanup(func() { b.cancel() })

	yielding := func(id, session string) agenthook.Event {
		ev := hookEvent(id, session, "/w")
		ev.YieldToUserHooks = true
		return ev
	}
	start := hookEvent("1", "cloud", "/w") // a Cloud Agent's sessionStart
	start.Cloud = agenthook.CloudSignalCursorBackgroundAgent
	b.Add(start)
	b.Add(yielding("2", "cloud"))
	b.Add(yielding("3", "local"))
	b.Add(hookEvent("4", "local", "/w"))

	b.mu.Lock()
	defer b.mu.Unlock()
	var ids []string
	for _, ev := range b.pending {
		ids = append(ids, ev.ID)
		assert.False(t, ev.YieldToUserHooks, "the mark stays in the daemon")
	}
	assert.Equal(t, []string{"1", "2", "4"}, ids, "a local session's yielding event is left to the user-level hook")
}

func TestPrepareAgentHookEvents_DropsTheYieldMark(t *testing.T) {
	events := []agenthook.Event{{ID: "1", Agent: agenthook.AgentCursor, YieldToUserHooks: true}}
	prepareAgentHookEvents(context.Background(), model.ShellTimeConfig{}, nil, events)
	assert.False(t, events[0].YieldToUserHooks, "never sent to the server")
}

func TestAgentHookBatcher_PrunesCloudSessions(t *testing.T) {
	isolateAgentHookHome(t)
	b := NewAgentHookBatcher(model.ShellTimeConfig{}, nil)
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	b.send = func(context.Context, model.ShellTimeConfig, []agenthook.Event) error { return nil }
	b.flushInterval = time.Hour
	t.Cleanup(func() { b.cancel() })

	ev := hookEvent("1", "c1", "/w")
	ev.Cloud = agenthook.CloudSignalEnv
	b.Add(ev)
	now = now.Add(agentHookSessionTTL)
	b.flush(context.Background())

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.Empty(t, b.clouds)
}

func TestAgentHookBatcher_AddsRepository(t *testing.T) {
	isolateAgentHookHome(t)
	dir := gitRepoWithOrigin(t, "git@github.com:acme/web.git")
	srv := newAgentHookServer(t, http.StatusOK)
	b := NewAgentHookBatcher(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}, model.NewGitRepositoryResolver(time.Minute))

	b.Add(hookEvent("a", "s1", dir))
	b.Add(hookEvent("b", "s1", filepath.Join(t.TempDir(), "not-a-repo")))
	b.flush(context.Background())

	req := waitAgentHookRequest(t, srv)
	require.Len(t, req.Events, 2)
	assert.Equal(t, "https://github.com/acme/web", req.Events[0].Repository)
	assert.Empty(t, req.Events[1].Repository)

	// aiCodeOtel.sendRepository: false sends none
	off := model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok", AICodeOtel: &model.AICodeOtel{SendRepository: new(false)}}
	b2 := NewAgentHookBatcher(off, model.NewGitRepositoryResolver(time.Minute))
	ev := hookEvent("c", "s1", dir)
	ev.Repository = "https://github.com/acme/web"
	b2.Add(ev)
	b2.flush(context.Background())
	req = waitAgentHookRequest(t, srv)
	assert.Empty(t, req.Events[0].Repository)
	b.Shutdown(context.Background())
	b2.Shutdown(context.Background())
}

func TestAgentHookBatcher_AddAfterShutdownSaves(t *testing.T) {
	isolateAgentHookHome(t)
	b := NewAgentHookBatcher(model.ShellTimeConfig{}, nil)
	b.Shutdown(context.Background())
	b.Shutdown(context.Background())

	b.Add(hookEvent("late", "s1", "/w"))

	saved := savedAgentHookEvents(t)
	require.Len(t, saved, 1)
	assert.Equal(t, "late", saved[0].ID)
	assert.NotContains(t, saved[0].Prompt, agentHookTestJWT)
}

func TestSocketHandler_AgentHookMessage(t *testing.T) {
	isolateAgentHookHome(t)
	srv := newAgentHookServer(t, http.StatusOK)
	dir, err := os.MkdirTemp("", "sthook")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "d.sock")

	cfg := &model.ShellTimeConfig{SocketPath: socketPath, APIEndpoint: srv.URL, Token: "tok", AICloudOnly: new(true)}
	handler := NewSocketHandler(cfg, NewGoChannel(PubSubConfig{}, nil))
	handler.agentHooks.flushInterval = time.Hour
	require.NoError(t, handler.Start())

	ev := hookEvent("from-cli", "s1", "/w")
	ev.DurationMs = 1234
	ev.SetMetadata("loop_count", 3)
	require.NoError(t, SendAgentHookEvent(socketPath, ev, time.Second), "acknowledged, in AI cloud only mode too")

	// Shutdown marks the whole daemon as stopping; later tests need it running
	t.Cleanup(resetShutdown)
	handler.Shutdown(time.Second)

	req := waitAgentHookRequest(t, srv)
	require.Len(t, req.Events, 1)
	got := req.Events[0]
	assert.Equal(t, "from-cli", got.ID)
	assert.Equal(t, int64(1234), got.DurationMs)
	assert.EqualValues(t, 3, got.Metadata["loop_count"])
}

func TestSocketHandler_AgentHookBadPayload(t *testing.T) {
	isolateAgentHookHome(t)
	handler := NewSocketHandler(&model.ShellTimeConfig{}, NewGoChannel(PubSubConfig{}, nil))
	t.Cleanup(func() { handler.agentHooks.Shutdown(context.Background()) })

	client, server := net.Pipe()
	go handler.handleConnection(server)
	require.NoError(t, json.NewEncoder(client).Encode(map[string]any{"type": "agent_hook", "payload": map[string]any{"duration_ms": "soon"}}))
	var ack map[string]string
	require.NoError(t, json.NewDecoder(client).Decode(&ack))
	assert.Equal(t, "error", ack["status"])
	client.Close()
}

func TestSendAgentHookEvent_NoDaemon(t *testing.T) {
	err := SendAgentHookEvent(filepath.Join(t.TempDir(), "absent.sock"), agenthook.Event{ID: "a"}, 50*time.Millisecond)
	assert.Error(t, err)
}

func TestSendAgentHookEvent_SlowDaemonTimesOut(t *testing.T) {
	dir, err := os.MkdirTemp("", "sthook")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(500 * time.Millisecond) // never answers in time
	}()

	start := time.Now()
	err = SendAgentHookEvent(socketPath, agenthook.Event{ID: "a"}, 50*time.Millisecond)
	assert.Error(t, err)
	assert.Less(t, time.Since(start), 400*time.Millisecond)
}

// --- resync -------------------------------------------------------------------

func TestAgentHookResync_SendsAndRewrites(t *testing.T) {
	isolateAgentHookHome(t)
	require.NoError(t, model.AppendAgentHookPending(hookEvent("a", "s1", "/w"), hookEvent("b", "s1", "/w")))
	require.NoError(t, model.UpdateAgentHookPending(func(current []string) []string {
		return append(current, "not json")
	}))

	srv := newAgentHookServer(t, http.StatusOK)
	s := NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"})
	s.resync(context.Background())

	req := waitAgentHookRequest(t, srv)
	require.Len(t, req.Events, 2)
	assert.NotContains(t, req.Events[0].Prompt, agentHookTestJWT)
	assert.NoFileExists(t, model.GetAgentHooksPendingFilePath(), "sent and unreadable lines are gone")
}

func TestAgentHookResync_KeepsOnRetryableFailureAndLinesSavedMeanwhile(t *testing.T) {
	isolateAgentHookHome(t)
	require.NoError(t, model.AppendAgentHookPending(hookEvent("a", "s1", "/w")))

	s := NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: "http://unused", Token: "tok"})
	s.send = func(ctx context.Context, cfg model.ShellTimeConfig, events []agenthook.Event) error {
		// Saved while this pass sends
		require.NoError(t, model.AppendAgentHookPending(hookEvent("meanwhile", "s1", "/w")))
		return &model.HTTPStatusError{StatusCode: http.StatusServiceUnavailable, Message: "down"}
	}
	s.resync(context.Background())

	saved := savedAgentHookEvents(t)
	require.Len(t, saved, 2)
	assert.Equal(t, "a", saved[0].ID)
	assert.Equal(t, "meanwhile", saved[1].ID)
}

func TestAgentHookResync_DropsRefusedChunks(t *testing.T) {
	isolateAgentHookHome(t)
	require.NoError(t, model.AppendAgentHookPending(hookEvent("a", "s1", "/w")))

	srv := newAgentHookServer(t, http.StatusUnprocessableEntity)
	s := NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"})
	s.resync(context.Background())

	assert.Equal(t, 1, srv.requestCount())
	assert.NoFileExists(t, model.GetAgentHooksPendingFilePath())
}

func TestAgentHookResync_ChunksAndKeepsWithoutToken(t *testing.T) {
	isolateAgentHookHome(t)
	events := make([]agenthook.Event, agentHookResyncChunkSize+1)
	for i := range events {
		events[i] = agenthook.Event{ID: strings.Repeat("x", 1+i%5), Agent: agenthook.AgentCursor}
	}
	require.NoError(t, model.AppendAgentHookPending(events...))

	s := NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: "http://unused"})
	s.send = func(context.Context, model.ShellTimeConfig, []agenthook.Event) error {
		t.Fatal("nothing is sent without a token")
		return nil
	}
	s.resync(context.Background())
	assert.Len(t, savedAgentHookEvents(t), len(events))

	var sizes []int
	s.config.Token = "tok"
	s.send = func(_ context.Context, _ model.ShellTimeConfig, chunk []agenthook.Event) error {
		sizes = append(sizes, len(chunk))
		return nil
	}
	s.resync(context.Background())
	assert.Equal(t, []int{agentHookResyncChunkSize, 1}, sizes)
	assert.NoFileExists(t, model.GetAgentHooksPendingFilePath())
}

func TestAgentHookResync_KeepsWhenTheServerHasNoEndpoint(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			isolateAgentHookHome(t)
			require.NoError(t, model.AppendAgentHookPending(hookEvent("a", "s1", "/w")))

			srv := newAgentHookServer(t, status)
			s := NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"})
			s.resync(context.Background())

			assert.Equal(t, 1, srv.requestCount())
			saved := savedAgentHookEvents(t)
			require.Len(t, saved, 1)
			assert.Equal(t, "a", saved[0].ID)
		})
	}
}

// Saved events are readied under today's config, like the batcher's: hook-ingest looked up no
// repository, and aiCodeOtel.sendRepository may have been turned off since an event was saved.
func TestAgentHookResync_RepositoryFollowsTheSwitch(t *testing.T) {
	isolateAgentHookHome(t)
	dir := gitRepoWithOrigin(t, "git@github.com:acme/web.git")
	fromHookIngest := hookEvent("a", "s1", dir)
	fromBatcher := hookEvent("b", "s1", filepath.Join(t.TempDir(), "elsewhere"))
	fromBatcher.Repository = "https://github.com/acme/api"
	require.NoError(t, model.AppendAgentHookPending(fromHookIngest, fromBatcher))

	var got []agenthook.Event
	send := func(_ context.Context, _ model.ShellTimeConfig, events []agenthook.Event) error {
		got = append(got, events...)
		return &model.HTTPStatusError{StatusCode: http.StatusServiceUnavailable}
	}
	s := NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: "http://unused", Token: "tok"})
	s.send = send
	s.resync(context.Background())

	require.Len(t, got, 2)
	assert.Equal(t, "https://github.com/acme/web", got[0].Repository, "resolved from the workspace")
	assert.Equal(t, "https://github.com/acme/api", got[1].Repository, "kept")
	saved := savedAgentHookEvents(t)
	require.Len(t, saved, 2)
	assert.Empty(t, saved[0].Repository, "a kept line stays as it was saved")

	got = nil
	off := model.ShellTimeConfig{APIEndpoint: "http://unused", Token: "tok", AICodeOtel: &model.AICodeOtel{SendRepository: new(false)}}
	s = NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: "http://unused", Token: "tok"})
	s.send = send
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything, mock.Anything).Return(off, nil)
	s.SetConfigService(mc)
	s.resync(context.Background())

	require.Len(t, got, 2)
	assert.Empty(t, got[0].Repository)
	assert.Empty(t, got[1].Repository, "the switch read from disk removes it")
}

func TestAgentHookResync_StartStop(t *testing.T) {
	isolateAgentHookHome(t)
	require.NoError(t, model.AppendAgentHookPending(hookEvent("a", "s1", "/w")))
	srv := newAgentHookServer(t, http.StatusOK)
	s := NewAgentHookResyncService(model.ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"})

	require.NoError(t, s.Start(context.Background()))
	waitAgentHookRequest(t, srv)
	s.Stop()
	s.Stop()
}

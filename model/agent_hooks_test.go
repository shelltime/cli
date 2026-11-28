package model

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malamtime/cli/agenthook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"

func privacyTestEvent() agenthook.Event {
	return agenthook.Event{
		ID:        "e1",
		Agent:     agenthook.AgentCursor,
		EventType: agenthook.EventToolUse,
		Prompt:    "use " + testJWT,
		Response:  "ran it with " + testJWT,
		Command:   "curl -H 'Authorization: Bearer " + testJWT + "' https://api.example.com",
		ToolInput: `{"command":"curl ` + testJWT + `"}`,
		Error:     "401 for " + testJWT,
		FilePath:  "/repo/main.go",
	}
}

func TestApplyAgentHookPrivacy_MasksByDefault(t *testing.T) {
	ev := privacyTestEvent()
	ApplyAgentHookPrivacy(&ev, ShellTimeConfig{})

	for name, field := range map[string]string{"prompt": ev.Prompt, "response": ev.Response, "command": ev.Command, "tool_input": ev.ToolInput, "error": ev.Error} {
		assert.NotContains(t, field, testJWT, name)
		assert.Contains(t, field, "***", name)
	}
	assert.Equal(t, "/repo/main.go", ev.FilePath)
	assert.Nil(t, ev.Metadata)
}

func TestApplyAgentHookPrivacy_MaskingOff(t *testing.T) {
	ev := privacyTestEvent()
	ApplyAgentHookPrivacy(&ev, ShellTimeConfig{DataMasking: new(false)})
	assert.Contains(t, ev.Prompt, testJWT)
	assert.Contains(t, ev.Command, testJWT)
}

func TestApplyAgentHookPrivacy_Exclude(t *testing.T) {
	ev := privacyTestEvent()
	ev.PullRequestURLs = []string{"https://github.com/acme/web/pull/1"}
	ApplyAgentHookPrivacy(&ev, ShellTimeConfig{Exclude: []string{`^curl `}})

	assert.Empty(t, ev.Command)
	assert.Empty(t, ev.ToolInput)
	assert.Empty(t, ev.Response)
	assert.Empty(t, ev.Error, "an error may repeat the command")
	assert.Empty(t, ev.Prompt, "so may the prompt")
	assert.Empty(t, ev.PullRequestURLs, "excluded command drops PR links")
	assert.Equal(t, true, ev.Metadata["excluded"])
	assert.Equal(t, "/repo/main.go", ev.FilePath)

	// An event without a command is never excluded, even by a pattern matching anything
	other := agenthook.Event{Prompt: "hello"}
	ApplyAgentHookPrivacy(&other, ShellTimeConfig{Exclude: []string{`.*`}})
	assert.Equal(t, "hello", other.Prompt)
	assert.Nil(t, other.Metadata)
}

func TestApplyAgentHookPrivacy_NoContent(t *testing.T) {
	ev := privacyTestEvent()
	ev.PullRequestURLs = []string{"https://github.com/acme/web/pull/2"}
	ev.SetMetadata("attachments", []string{"/repo/secret-plan.md"})
	ev.SetMetadata("loop_count", 2)
	ApplyAgentHookPrivacy(&ev, ShellTimeConfig{AgentHooks: &AgentHooksConfig{SendContent: new(false)}})

	assert.Empty(t, ev.Prompt)
	assert.Empty(t, ev.Response)
	assert.Empty(t, ev.ToolInput)
	assert.Empty(t, ev.Error)
	assert.Equal(t, []string{"https://github.com/acme/web/pull/2"}, ev.PullRequestURLs, "PR links stay when sendContent is off")
	assert.NotContains(t, ev.Metadata, "attachments")
	assert.Equal(t, 2, ev.Metadata["loop_count"], "other metadata stays")
	assert.NotEmpty(t, ev.Command, "activity is still sent")
	assert.NotContains(t, ev.Command, testJWT, "and masked")
	assert.Equal(t, "/repo/main.go", ev.FilePath)

	ApplyAgentHookPrivacy(nil, ShellTimeConfig{})
}

func TestAgentHooksSendContent(t *testing.T) {
	assert.True(t, ShellTimeConfig{}.AgentHooksSendContent())
	assert.True(t, ShellTimeConfig{AgentHooks: &AgentHooksConfig{}}.AgentHooksSendContent())
	assert.True(t, ShellTimeConfig{AgentHooks: &AgentHooksConfig{SendContent: new(true)}}.AgentHooksSendContent())
	assert.False(t, ShellTimeConfig{AgentHooks: &AgentHooksConfig{SendContent: new(false)}}.AgentHooksSendContent())
}

func TestMergeConfig_AgentHooks(t *testing.T) {
	base := &ShellTimeConfig{}
	mergeConfig(base, &ShellTimeConfig{AgentHooks: &AgentHooksConfig{SendContent: new(false)}})
	assert.False(t, base.AgentHooksSendContent())

	mergeConfig(base, &ShellTimeConfig{AgentHooks: &AgentHooksConfig{}})
	assert.False(t, base.AgentHooksSendContent(), "a local file without the field keeps the base's")

	mergeConfig(base, &ShellTimeConfig{})
	assert.False(t, base.AgentHooksSendContent())
}

func TestReadConfigFile_AgentHooks(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"yaml": {"config.yaml": "token: t\nagentHooks:\n  sendContent: false\n"},
		"toml": {"config.toml": "Token = \"t\"\n[agentHooks]\nsendContent = false\n"},
		"local": {
			"config.yaml":       "token: t\n",
			"config.local.yaml": "agentHooks:\n  sendContent: false\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for file, content := range files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0644))
			}
			cfg, err := NewConfigService(dir).ReadConfigFile(context.Background())
			require.NoError(t, err)
			assert.False(t, cfg.AgentHooksSendContent())
		})
	}
}

func TestShellTimeConfigLogValue_AgentHooks(t *testing.T) {
	var buf bytes.Buffer
	cfg := ShellTimeConfig{AgentHooks: &AgentHooksConfig{SendContent: new(false)}}
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", slog.Any("config", cfg))
	assert.Contains(t, buf.String(), "config.agentHooks.sendContent=false")
}

func TestAgentHookPending_AppendReadUpdate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := GetAgentHooksPendingFilePath()

	_, err := ReadAgentHookPendingLines()
	assert.True(t, os.IsNotExist(err))

	require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: "a", Prompt: "line\nbreak"}, agenthook.Event{ID: "b"}))
	require.NoError(t, AppendAgentHookPending())
	require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: "c"}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	lines, err := ReadAgentHookPendingLines()
	require.NoError(t, err)
	require.Len(t, lines, 3)
	var ev agenthook.Event
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &ev))
	assert.Equal(t, "line\nbreak", ev.Prompt, "one event per line")

	require.NoError(t, UpdateAgentHookPending(func(current []string) []string {
		assert.Equal(t, lines, current)
		return current[1:2]
	}))
	lines, err = ReadAgentHookPendingLines()
	require.NoError(t, err)
	assert.Len(t, lines, 1)
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	require.NoError(t, UpdateAgentHookPending(func([]string) []string { return nil }))
	assert.NoFileExists(t, path)
	require.NoError(t, UpdateAgentHookPending(func(current []string) []string {
		assert.Empty(t, current)
		return nil
	}))
}

func TestAgentHookPending_ConcurrentAppends(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	big := strings.Repeat("x", 100_000)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			assert.NoError(t, AppendAgentHookPending(agenthook.Event{ID: string(rune('a' + i)), Prompt: big}))
		})
	}
	wg.Wait()
	lines, err := ReadAgentHookPendingLines()
	require.NoError(t, err)
	require.Len(t, lines, 20)
	for _, line := range lines {
		assert.True(t, json.Valid([]byte(line)))
	}
}

func setAgentHookPendingLockWait(t *testing.T, wait time.Duration) {
	t.Helper()
	prev := agentHookPendingLockWait
	agentHookPendingLockWait = wait
	t.Cleanup(func() { agentHookPendingLockWait = prev })
}

// holdAgentHookPendingLock takes the pending file's lock the way another process does (flock
// locks of two descriptors conflict within a process too) until the returned func is called.
// It skips the test where there is no lock.
func holdAgentHookPendingLock(t *testing.T, exclusive bool) (unlock func()) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(agentHookPendingLockPath()), 0700))
	unlock, state := acquireStoreLock(agentHookPendingLockPath(), exclusive, time.Second)
	if state != storeLockHeld {
		unlock()
		t.Skip("no advisory lock on this platform")
	}
	t.Cleanup(unlock)
	return unlock
}

func pendingIDs(t *testing.T) []string {
	t.Helper()
	lines, err := ReadAgentHookPendingLines()
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		var ev agenthook.Event
		require.NoError(t, json.Unmarshal([]byte(line), &ev))
		ids = append(ids, ev.ID)
	}
	return ids
}

// A rewrite that can't get the lock (hook-ingest appending, or another rewrite) changes
// nothing and says so; appends share the lock.
func TestAgentHookPending_RewriteGivesUpWhenBusy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setAgentHookPendingLockWait(t, 20*time.Millisecond)
	require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: "a"}))

	for _, exclusive := range []bool{false, true} {
		unlock := holdAgentHookPendingLock(t, exclusive)
		called := false
		err := UpdateAgentHookPending(func(current []string) []string {
			called = true
			return nil
		})
		assert.ErrorIs(t, err, ErrAgentHookPendingBusy)
		assert.False(t, called)
		assert.Equal(t, []string{"a"}, pendingIDs(t))
		unlock()
	}

	// An append waiting for the lock would take the whole wait
	setAgentHookPendingLockWait(t, 10*time.Second)
	unlock := holdAgentHookPendingLock(t, false)
	start := time.Now()
	require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: "b"}))
	assert.Less(t, time.Since(start), time.Second, "an append doesn't wait for another append")
	unlock()

	require.NoError(t, UpdateAgentHookPending(func([]string) []string { return nil }))
	assert.NoFileExists(t, GetAgentHooksPendingFilePath())
}

// hook-ingest never hangs on the lock: an append that can't get it in time goes ahead.
func TestAgentHookPending_AppendGoesAheadAfterTheWait(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setAgentHookPendingLockWait(t, 30*time.Millisecond)
	holdAgentHookPendingLock(t, true)

	start := time.Now()
	require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: "a"}))
	waited := time.Since(start)

	assert.GreaterOrEqual(t, waited, 30*time.Millisecond, "it waits for the lock first")
	assert.Less(t, waited, 3*time.Second, "and gives up after the bounded wait")
	assert.Equal(t, []string{"a"}, pendingIDs(t))
}

// An append waits for a rewrite that holds the lock, and then lands in the new file.
func TestAgentHookPending_AppendWaitsForARewrite(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	setAgentHookPendingLockWait(t, 10*time.Second)
	require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: "a"}, agenthook.Event{ID: "b"}))

	appended := make(chan error, 1)
	agentHookPendingAfterRead = func() {
		go func() {
			// Another process: the in-process mutex would order it anyway
			unlock, _ := acquireStoreLock(agentHookPendingLockPath(), false, 10*time.Second)
			defer unlock()
			f, err := os.OpenFile(GetAgentHooksPendingFilePath(), os.O_APPEND|os.O_WRONLY, 0600)
			if err == nil {
				_, err = f.WriteString(`{"id":"c"}` + "\n")
				f.Close()
			}
			appended <- err
		}()
		select {
		case <-appended:
			t.Error("the append went ahead while the file was locked exclusively")
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Cleanup(func() { agentHookPendingAfterRead = nil })

	require.NoError(t, UpdateAgentHookPending(func(current []string) []string { return current[1:] }))
	select {
	case err := <-appended:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the append never got the lock after it was released")
	}
	assert.Equal(t, []string{"b", "c"}, pendingIDs(t))
}

// Lines appended between a rewrite's read and its rename by a process that went ahead without
// the lock are carried over, also into a file that log cleanup replaced meanwhile.
func TestAgentHookPending_RewriteKeepsLinesAppendedSinceItsRead(t *testing.T) {
	appendUnlocked := func(t *testing.T, data string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(GetAgentHooksPendingFilePath()), 0700))
		f, err := os.OpenFile(GetAgentHooksPendingFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		require.NoError(t, err)
		_, err = f.WriteString(data)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}
	tests := []struct {
		name     string
		initial  []string
		keep     func([]string) []string
		meantime func(t *testing.T)
		want     []string
	}{
		{
			name:     "appended",
			initial:  []string{"a", "b"},
			keep:     func(c []string) []string { return c[1:] },
			meantime: func(t *testing.T) { appendUnlocked(t, `{"id":"c"}`+"\n"+`{"id":"half`) },
			want:     []string{"b", "c"},
		},
		{
			name:     "appended, nothing else kept",
			initial:  []string{"a"},
			keep:     func([]string) []string { return nil },
			meantime: func(t *testing.T) { appendUnlocked(t, `{"id":"c"}`+"\n") },
			want:     []string{"c"},
		},
		{
			name:    "file replaced",
			initial: []string{"a", "b"},
			keep:    func(c []string) []string { return c[1:] },
			meantime: func(t *testing.T) {
				require.NoError(t, os.Remove(GetAgentHooksPendingFilePath()))
				appendUnlocked(t, `{"id":"c"}`+"\n")
			},
			want: []string{"b", "c"},
		},
		{
			name:    "file replaced by a longer one",
			initial: []string{"a"},
			keep:    func(c []string) []string { return nil },
			meantime: func(t *testing.T) {
				require.NoError(t, os.Remove(GetAgentHooksPendingFilePath()))
				appendUnlocked(t, `{"id":"c"}`+"\n"+`{"id":"d"}`+"\n")
			},
			want: []string{"c", "d"},
		},
		{
			name:     "file created",
			keep:     func(c []string) []string { return c },
			meantime: func(t *testing.T) { appendUnlocked(t, `{"id":"c"}`+"\n") },
			want:     []string{"c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			for _, id := range tt.initial {
				require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: id}))
			}
			agentHookPendingAfterRead = func() { tt.meantime(t) }
			t.Cleanup(func() { agentHookPendingAfterRead = nil })

			require.NoError(t, UpdateAgentHookPending(tt.keep))
			assert.Equal(t, tt.want, pendingIDs(t))
		})
	}
}

func TestCleanLargeLogFiles_IncludesAgentHooksPending(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, AppendAgentHookPending(agenthook.Event{ID: "a", Prompt: strings.Repeat("x", 4096)}))

	freed, err := CleanLargeLogFiles(1024, false)
	require.NoError(t, err)
	assert.Positive(t, freed)
	assert.NoFileExists(t, GetAgentHooksPendingFilePath())
}

func TestSplitAgentHookEvents(t *testing.T) {
	events := make([]agenthook.Event, 7)
	for i := range events {
		events[i] = agenthook.Event{ID: string(rune('a' + i)), Prompt: strings.Repeat("p", 1000)}
	}

	assert.Nil(t, SplitAgentHookEvents(nil, 3, 1<<20))

	byCount := SplitAgentHookEvents(events, 3, 1<<20)
	require.Len(t, byCount, 3)
	assert.Len(t, byCount[0], 3)
	assert.Len(t, byCount[2], 1)

	bySize := SplitAgentHookEvents(events, 100, 2500)
	assert.Greater(t, len(bySize), 1)
	var flat []agenthook.Event
	for _, chunk := range bySize {
		assert.NotEmpty(t, chunk)
		flat = append(flat, chunk...)
	}
	assert.Equal(t, events, flat, "order and every event kept")

	huge := []agenthook.Event{{ID: "big", Prompt: strings.Repeat("p", 5000)}, {ID: "small"}}
	assert.Len(t, SplitAgentHookEvents(huge, 100, 1000), 2, "an event over the budget goes alone")
}

func TestSendAgentHookEvents(t *testing.T) {
	var mu sync.Mutex
	var requests []AgentHookRequest
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/agent-hooks", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		var req AgentHookRequest
		assert.NoError(t, json.Unmarshal(body, &req))
		mu.Lock()
		requests = append(requests, req)
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(AgentHookResponse{Success: true, EventsProcessed: len(req.Events)})
	}))
	defer srv.Close()

	events := make([]agenthook.Event, AgentHookMaxEventsPerRequest+2)
	for i := range events {
		events[i] = agenthook.Event{ID: "id", Agent: agenthook.AgentCursor, EventType: agenthook.EventStop}
	}
	events[0].PullRequestURLs = []string{"https://github.com/acme/web/pull/1"}
	events[0].YieldToUserHooks = true
	cfg := ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}
	require.NoError(t, SendAgentHookEvents(context.Background(), cfg, events))

	require.Len(t, requests, 2)
	assert.Len(t, requests[0].Events, AgentHookMaxEventsPerRequest)
	assert.Len(t, requests[1].Events, 2)
	assert.Empty(t, requests[0].Events[0].PullRequestURLs, "stripped on wire")
	assert.False(t, requests[0].Events[0].YieldToUserHooks, "stripped on wire")
	assert.Equal(t, []string{"https://github.com/acme/web/pull/1"}, events[0].PullRequestURLs, "caller slice is untouched")
	assert.NotEmpty(t, requests[0].Host)
	assert.Equal(t, "CLI tok", auth[0])
}

func TestSendAgentHookEvents_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer srv.Close()

	err := SendAgentHookEvents(context.Background(), ShellTimeConfig{APIEndpoint: srv.URL, Token: "t"}, []agenthook.Event{{ID: "a"}})
	var statusErr *HTTPStatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusServiceUnavailable, statusErr.StatusCode)
}

func TestAgentHooksPendingPath(t *testing.T) {
	assert.Equal(t, GetStoragePath("agent-hooks.pending.jsonl"), GetAgentHooksPendingFilePath())
	assert.True(t, strings.HasSuffix(AGENT_HOOKS_PENDING_FILE, "/agent-hooks.pending.jsonl"))
}

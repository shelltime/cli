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
	ApplyAgentHookPrivacy(&ev, ShellTimeConfig{Exclude: []string{`^curl `}})

	assert.Empty(t, ev.Command)
	assert.Empty(t, ev.ToolInput)
	assert.Empty(t, ev.Response)
	assert.Equal(t, true, ev.Metadata["excluded"])
	assert.NotEmpty(t, ev.Prompt, "the prompt isn't the command")
	assert.NotContains(t, ev.Prompt, testJWT)

	// An event without a command is never excluded, even by a pattern matching anything
	other := agenthook.Event{Prompt: "hello"}
	ApplyAgentHookPrivacy(&other, ShellTimeConfig{Exclude: []string{`.*`}})
	assert.Equal(t, "hello", other.Prompt)
	assert.Nil(t, other.Metadata)
}

func TestApplyAgentHookPrivacy_NoContent(t *testing.T) {
	ev := privacyTestEvent()
	ApplyAgentHookPrivacy(&ev, ShellTimeConfig{AgentHooks: &AgentHooksConfig{SendContent: new(false)}})

	assert.Empty(t, ev.Prompt)
	assert.Empty(t, ev.Response)
	assert.Empty(t, ev.ToolInput)
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
	cfg := ShellTimeConfig{APIEndpoint: srv.URL, Token: "tok"}
	require.NoError(t, SendAgentHookEvents(context.Background(), cfg, events))

	require.Len(t, requests, 2)
	assert.Len(t, requests[0].Events, AgentHookMaxEventsPerRequest)
	assert.Len(t, requests[1].Events, 2)
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

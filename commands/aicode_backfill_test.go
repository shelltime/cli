package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// backfillServer fakes the server's backfill endpoints.
type backfillServer struct {
	t        *testing.T
	mu       sync.Mutex
	statuses map[string]model.AICodeBackfillSessionStatus
	// failures are returned, in order, before backfill uploads succeed.
	failures      []int
	statusCode    int
	statusCalls   int
	uploads       []model.AICodeBackfillRequest
	uploadCalls   int
	completeCalls []model.AICodeBackfillCompleteRequest
	server        *httptest.Server
}

func newBackfillServer(t *testing.T) *backfillServer {
	bs := &backfillServer{t: t, statuses: map[string]model.AICodeBackfillSessionStatus{}}
	bs.server = httptest.NewServer(http.HandlerFunc(bs.handle))
	t.Cleanup(bs.server.Close)
	return bs
}

func (bs *backfillServer) handle(w http.ResponseWriter, r *http.Request) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	assert.Equal(bs.t, "CLI test-token", r.Header.Get("Authorization"))

	switch r.URL.Path {
	case "/api/v1/cc/backfill/sessions":
		bs.statusCalls++
		if bs.statusCode != 0 {
			w.WriteHeader(bs.statusCode)
			_, _ = w.Write([]byte("404 page not found"))
			return
		}
		var req model.AICodeBackfillSessionsRequest
		require.NoError(bs.t, json.NewDecoder(r.Body).Decode(&req))
		resp := model.AICodeBackfillSessionsResponse{Sessions: []model.AICodeBackfillSessionStatus{}}
		for _, id := range req.SessionIDs {
			if st, ok := bs.statuses[id]; ok {
				resp.Sessions = append(resp.Sessions, st)
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
	case "/api/v1/cc/backfill":
		bs.uploadCalls++
		if len(bs.failures) > 0 {
			code := bs.failures[0]
			bs.failures = bs.failures[1:]
			w.WriteHeader(code)
			_, _ = fmt.Fprintf(w, `{"code":%d,"error":"backfill rejected"}`, code)
			return
		}
		var req model.AICodeBackfillRequest
		require.NoError(bs.t, json.NewDecoder(r.Body).Decode(&req))
		bs.uploads = append(bs.uploads, req)
		_ = json.NewEncoder(w).Encode(model.AICodeBackfillResponse{
			Success:    true,
			Accepted:   len(req.Events),
			Summarized: len(req.CompletedSessionIDs),
		})
	case "/api/v1/cc/backfill/complete":
		var req model.AICodeBackfillCompleteRequest
		require.NoError(bs.t, json.NewDecoder(r.Body).Decode(&req))
		bs.completeCalls = append(bs.completeCalls, req)
		_, _ = w.Write([]byte(`{"success":true}`))
	default:
		bs.t.Errorf("unexpected request to %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func setupBackfillTest(t *testing.T, token string) *backfillServer {
	t.Helper()
	setupCCTest(t)
	bs := newBackfillServer(t)

	original := configService
	mc := model.NewMockConfigService(t)
	mc.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{Token: token, APIEndpoint: bs.server.URL}, nil).Maybe()
	configService = mc
	t.Cleanup(func() { configService = original })

	originalBackoff := backfillBackoff
	backfillBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { backfillBackoff = originalBackoff })
	return bs
}

func writeLines(t *testing.T, path string, lines ...map[string]any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	var b strings.Builder
	for _, l := range lines {
		buf, err := json.Marshal(l)
		require.NoError(t, err)
		b.Write(buf)
		b.WriteString("\n")
	}
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o644))
}

// writeClaudeSession writes a finished Claude Code session with one prompt
// and the given number of API responses.
func writeClaudeSession(t *testing.T, projects, sessionID string, start time.Time, responses int) {
	t.Helper()
	ts := func(d time.Duration) string { return start.Add(d).UTC().Format(time.RFC3339Nano) }
	lines := []map[string]any{{
		"type": "user", "uuid": sessionID + "-u", "sessionId": sessionID, "timestamp": ts(0), "cwd": "/tmp/p",
		"origin":  map[string]any{"kind": "human"},
		"message": map[string]any{"role": "user", "content": "prompt for " + sessionID},
	}}
	for i := 0; i < responses; i++ {
		lines = append(lines, map[string]any{
			"type": "assistant", "uuid": sessionID + "-a" + string(rune('0'+i)), "sessionId": sessionID,
			"timestamp": ts(time.Duration(i+1) * time.Second), "requestId": sessionID + "-req" + string(rune('0'+i)),
			"message": map[string]any{
				"id": sessionID + "-msg" + string(rune('0'+i)), "model": "claude-sonnet-4-5",
				"content": []any{map[string]any{"type": "text", "text": "ok"}},
				"usage":   map[string]any{"input_tokens": 10, "output_tokens": 20, "cache_read_input_tokens": 100},
			},
		})
	}
	writeLines(t, filepath.Join(projects, "-tmp-p", sessionID+".jsonl"), lines...)
}

func runBackfill(t *testing.T, command *cli.Command, group string, args ...string) error {
	t.Helper()
	app := &cli.App{Name: "t", Commands: []*cli.Command{{Name: group, Subcommands: []*cli.Command{command}}}}
	return app.Run(append([]string{"t", group, "backfill"}, args...))
}

func TestCCBackfill_UploadsOnlyNewSessions(t *testing.T) {
	bs := setupBackfillTest(t, "test-token")
	root := t.TempDir()
	projects := filepath.Join(root, "projects")
	start := time.Now().AddDate(0, 0, -3)
	writeClaudeSession(t, projects, "sess-new", start, 2)
	writeClaudeSession(t, projects, "sess-live", start.Add(time.Hour), 1)
	writeClaudeSession(t, projects, "sess-done", start.Add(2*time.Hour), 1)
	writeClaudeSession(t, projects, "sess-partial", start.Add(3*time.Hour), 2)
	writeClaudeSession(t, projects, "sess-active", time.Now().Add(-5*time.Minute), 1)
	t.Setenv("CLAUDE_CONFIG_DIR", root)

	bs.statuses["sess-live"] = model.AICodeBackfillSessionStatus{SessionID: "sess-live", Status: model.AICodeBackfillStatusLive}
	bs.statuses["sess-done"] = model.AICodeBackfillSessionStatus{SessionID: "sess-done", Status: model.AICodeBackfillStatusBackfilled, EventCount: 2}
	bs.statuses["sess-partial"] = model.AICodeBackfillSessionStatus{SessionID: "sess-partial", Status: model.AICodeBackfillStatusBackfilled, EventCount: 1}

	require.NoError(t, runBackfill(t, CCBackfillCommand, "cc"))

	require.Len(t, bs.uploads, 1)
	upload := bs.uploads[0]
	assert.Equal(t, model.AICodeClientClaudeCode, upload.ClientType)
	assert.False(t, upload.AISummary)
	assert.Equal(t, []string{"sess-new", "sess-partial"}, upload.CompletedSessionIDs)
	assert.Len(t, upload.Events, 6)
	for _, e := range upload.Events {
		assert.Contains(t, []string{"sess-new", "sess-partial"}, e.SessionID)
		assert.Equal(t, model.AICodeClientClaudeCode, e.ClientType)
	}

	require.Len(t, bs.completeCalls, 1)
	assert.Equal(t, model.AICodeClientClaudeCode, bs.completeCalls[0].ClientType)
	assert.False(t, bs.completeCalls[0].From.After(bs.completeCalls[0].To))
}

func TestCCBackfill_SplitsLargeBackfills(t *testing.T) {
	bs := setupBackfillTest(t, "test-token")
	root := t.TempDir()
	start := time.Now().AddDate(0, 0, -5)
	for i := 0; i < model.AICodeBackfillMaxCompleted+5; i++ {
		writeClaudeSession(t, filepath.Join(root, "projects"), "sess-"+strings.Repeat("x", i+1), start.Add(time.Duration(i)*time.Minute), 1)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", root)

	require.NoError(t, runBackfill(t, CCBackfillCommand, "cc", "--ai-summary"))

	require.Len(t, bs.uploads, 2)
	assert.Len(t, bs.uploads[0].CompletedSessionIDs, model.AICodeBackfillMaxCompleted)
	assert.Len(t, bs.uploads[1].CompletedSessionIDs, 5)
	for _, u := range bs.uploads {
		assert.True(t, u.AISummary)
		assert.LessOrEqual(t, len(u.Events), model.AICodeBackfillMaxEvents)
	}
}

func TestCCBackfill_DryRunOnlyChecksStatus(t *testing.T) {
	bs := setupBackfillTest(t, "test-token")
	root := t.TempDir()
	writeClaudeSession(t, filepath.Join(root, "projects"), "sess-1", time.Now().AddDate(0, 0, -2), 1)
	t.Setenv("CLAUDE_CONFIG_DIR", root)

	require.NoError(t, runBackfill(t, CCBackfillCommand, "cc", "--dry-run"))

	assert.Equal(t, 1, bs.statusCalls)
	assert.Equal(t, 0, bs.uploadCalls)
	assert.Empty(t, bs.completeCalls)
}

func TestCCBackfill_RetriesServerErrors(t *testing.T) {
	bs := setupBackfillTest(t, "test-token")
	root := t.TempDir()
	writeClaudeSession(t, filepath.Join(root, "projects"), "sess-1", time.Now().AddDate(0, 0, -2), 1)
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	bs.failures = []int{http.StatusServiceUnavailable, http.StatusTooManyRequests}

	require.NoError(t, runBackfill(t, CCBackfillCommand, "cc"))

	assert.Equal(t, 3, bs.uploadCalls)
	assert.Len(t, bs.uploads, 1)
	assert.Len(t, bs.completeCalls, 1)
}

func TestCCBackfill_AbortsOnClientError(t *testing.T) {
	bs := setupBackfillTest(t, "test-token")
	root := t.TempDir()
	writeClaudeSession(t, filepath.Join(root, "projects"), "sess-1", time.Now().AddDate(0, 0, -2), 1)
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	bs.failures = []int{http.StatusBadRequest}

	err := runBackfill(t, CCBackfillCommand, "cc")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "backfill rejected")
	assert.Equal(t, 1, bs.uploadCalls)
	assert.Empty(t, bs.completeCalls)
}

func TestCCBackfill_ExplainsOldServer(t *testing.T) {
	bs := setupBackfillTest(t, "test-token")
	root := t.TempDir()
	writeClaudeSession(t, filepath.Join(root, "projects"), "sess-1", time.Now().AddDate(0, 0, -2), 1)
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	bs.statusCode = http.StatusNotFound

	err := runBackfill(t, CCBackfillCommand, "cc")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support backfill yet")
	assert.Equal(t, 1, bs.statusCalls, "client errors are not retried")
}

func TestCCBackfill_RequiresToken(t *testing.T) {
	setupBackfillTest(t, "")

	err := runBackfill(t, CCBackfillCommand, "cc")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "shelltime init")
}

func TestCodexBackfill_UploadsSessions(t *testing.T) {
	bs := setupBackfillTest(t, "test-token")
	home := t.TempDir()
	sessionID := "44444444-4444-4444-8444-444444444444"
	start := time.Now().AddDate(0, 0, -1).UTC()
	ts := func(d time.Duration) string { return start.Add(d).Format(time.RFC3339Nano) }
	usage := map[string]any{"input_tokens": 1000, "cached_input_tokens": 600, "output_tokens": 50, "reasoning_output_tokens": 10, "total_tokens": 1050}
	writeLines(t, filepath.Join(home, "sessions", "2026", "10", "04", "rollout-2026-10-04T10-00-00-"+sessionID+".jsonl"),
		map[string]any{"timestamp": ts(0), "type": "session_meta", "payload": map[string]any{"id": sessionID, "cwd": "/tmp/c", "cli_version": "0.42.0"}},
		map[string]any{"timestamp": ts(time.Second), "type": "turn_context", "payload": map[string]any{"model": "gpt-5-codex"}},
		map[string]any{"timestamp": ts(2 * time.Second), "type": "event_msg", "payload": map[string]any{"type": "user_message", "message": "hello"}},
		map[string]any{"timestamp": ts(3 * time.Second), "type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": usage, "last_token_usage": usage}}},
	)
	t.Setenv("CODEX_HOME", home)

	require.NoError(t, runBackfill(t, CodexBackfillCommand, "codex", "--no-prompts"))

	require.Len(t, bs.uploads, 1)
	upload := bs.uploads[0]
	assert.Equal(t, model.AICodeClientCodex, upload.ClientType)
	assert.Equal(t, []string{sessionID}, upload.CompletedSessionIDs)
	types := map[string]model.AICodeBackfillEvent{}
	for _, e := range upload.Events {
		types[e.EventType] = e
	}
	assert.Contains(t, types, model.AICodeEventConversationStarts)
	assert.Empty(t, types[model.AICodeEventUserPrompt].Prompt)
	assert.Equal(t, 5, *types[model.AICodeEventUserPrompt].PromptLength)
	sse := types[model.AICodeEventSSEEvent]
	assert.Equal(t, "response.completed", sse.EventKind)
	assert.Equal(t, 1000, *sse.InputTokens)
	assert.Equal(t, 600, *sse.CacheReadTokens)
	require.Len(t, bs.completeCalls, 1)
	assert.Equal(t, model.AICodeClientCodex, bs.completeCalls[0].ClientType)
}

func TestBackfillOptionsFromFlags(t *testing.T) {
	parse := func(args ...string) (model.BackfillOptions, error) {
		var opts model.BackfillOptions
		var err error
		app := &cli.App{Flags: aiCodeBackfillFlags(), Action: func(c *cli.Context) error {
			opts, err = backfillOptionsFromFlags(c)
			return nil
		}}
		require.NoError(t, app.Run(append([]string{"t"}, args...)))
		return opts, err
	}

	opts, err := parse("--since", "2026-09-01", "--until", "2026-09-30", "--no-prompts")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), opts.Since)
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), opts.Until, "until includes the whole day")
	assert.True(t, opts.NoPrompts)

	_, err = parse("--since", "09/01/2026")
	assert.ErrorContains(t, err, "invalid --since")

	_, err = parse("--since", "2026-09-30", "--until", "2026-09-01")
	assert.ErrorContains(t, err, "must not be after")
}

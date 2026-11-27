package agenthook

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, 11, 27, 8, 30, 0, 123456789, time.FixedZone("CET", 3600))

func envOf(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestParseAgent(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"cursor", AgentCursor, true},
		{" Cursor ", AgentCursor, true},
		{"agy", AgentAntigravity, true},
		{"antigravity", AgentAntigravity, true},
		{"AGY", AgentAntigravity, true},
		{"claude", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := ParseAgent(tt.in)
		assert.Equal(t, tt.want, got, tt.in)
		assert.Equal(t, tt.ok, ok, tt.in)
	}
}

func TestNewID(t *testing.T) {
	a, b := NewID(), NewID()
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{32}$`), a)
	assert.NotEqual(t, a, b)
}

func TestNormalize_Cursor(t *testing.T) {
	common := `"conversation_id":"conv-1","generation_id":"gen-1","model":"claude-4.5-sonnet","cursor_version":"2.1.0","workspace_roots":["/Users/me/proj"],"user_email":"me@example.com","transcript_path":"/Users/me/.cursor/t.json"`
	tests := []struct {
		name  string
		event string
		body  string
		check func(t *testing.T, ev Event)
	}{
		{
			name:  "beforeSubmitPrompt",
			event: "beforeSubmitPrompt",
			body:  `{` + common + `,"hook_event_name":"beforeSubmitPrompt","prompt":"fix the bug","attachments":[{"type":"file","file_path":"/Users/me/proj/a.go"},{"type":"rule"}]}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventPromptSubmit, ev.EventType)
				assert.Equal(t, "fix the bug", ev.Prompt)
				assert.Equal(t, []string{"/Users/me/proj/a.go"}, ev.Metadata["attachments"])
			},
		},
		{
			name:  "postToolUse with an object input",
			event: "postToolUse",
			body:  `{` + common + `,"tool_name":"Shell","tool_input":{"command":"go test ./...","cwd":"/Users/me/proj"},"tool_output":"SECRET OUTPUT","tool_use_id":"tu-1","cwd":"/Users/me/proj","duration":1234.6}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventToolUse, ev.EventType)
				assert.Equal(t, StatusOK, ev.Status)
				assert.Equal(t, "Shell", ev.ToolName)
				assert.Equal(t, "go test ./...", ev.Command)
				assert.Equal(t, int64(1235), ev.DurationMs)
				assert.Equal(t, `{"command":"go test ./...","cwd":"/Users/me/proj"}`, ev.ToolInput)
				assert.Equal(t, "tu-1", ev.Metadata["tool_use_id"])
			},
		},
		{
			name:  "postToolUse with a string input",
			event: "postToolUse",
			body:  `{` + common + `,"tool_name":"Edit","tool_input":"{\"file_path\": \"/Users/me/proj/main.go\", \"old_string\": \"a\"}","duration":5}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "/Users/me/proj/main.go", ev.FilePath)
				assert.Equal(t, `{"file_path":"/Users/me/proj/main.go","old_string":"a"}`, ev.ToolInput)
				assert.Empty(t, ev.Command)
			},
		},
		{
			name:  "postToolUse with plain text input",
			event: "postToolUse",
			body:  `{` + common + `,"tool_name":"Search","tool_input":"needle"}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "needle", ev.ToolInput)
				assert.Empty(t, ev.FilePath)
			},
		},
		{
			name:  "postToolUse of an MCP tool",
			event: "postToolUse",
			body:  `{` + common + `,"tool_name":"MCP:github","tool_input":{"owner":"o"}}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "github", ev.MCPServer)
			},
		},
		{
			name:  "postToolUseFailure",
			event: "postToolUseFailure",
			body:  `{` + common + `,"tool_name":"Shell","tool_input":{"command":"make"},"tool_use_id":"tu-2","cwd":"/Users/me/proj","error_message":"exit status 2","failure_type":"timeout","duration":30000,"is_interrupt":false}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventToolFailure, ev.EventType)
				assert.Equal(t, "timeout", ev.Status)
				assert.Equal(t, "exit status 2", ev.Error)
				assert.Equal(t, "make", ev.Command)
				assert.Equal(t, int64(30000), ev.DurationMs)
				assert.Equal(t, "timeout", ev.Metadata["failure_type"])
				assert.Equal(t, false, ev.Metadata["is_interrupt"])
			},
		},
		{
			name:  "postToolUseFailure without a failure type",
			event: "postToolUseFailure",
			body:  `{` + common + `,"tool_name":"Read","tool_input":{"target_file":"x"},"error_message":"no such file"}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, StatusError, ev.Status)
				assert.Equal(t, "x", ev.FilePath)
			},
		},
		{
			name:  "afterAgentResponse",
			event: "afterAgentResponse",
			body:  `{` + common + `,"text":"Done: the test passes."}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventAgentResponse, ev.EventType)
				assert.Equal(t, "Done: the test passes.", ev.Response)
			},
		},
		{
			name:  "stop",
			event: "stop",
			body:  `{` + common + `,"status":"aborted","loop_count":3}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventStop, ev.EventType)
				assert.Equal(t, "aborted", ev.Status)
				assert.Equal(t, int64(3), ev.Metadata["loop_count"])
			},
		},
		{
			name:  "sessionStart",
			event: "sessionStart",
			body:  `{"session_id":"sess-9","model":"gpt-5","workspace_roots":["/w"],"is_background_agent":true,"composer_mode":"agent"}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventSessionStart, ev.EventType)
				assert.Equal(t, "sess-9", ev.SessionID, "session_id stands in for conversation_id")
				assert.Equal(t, true, ev.Metadata["is_background_agent"])
				assert.Equal(t, "agent", ev.Metadata["composer_mode"])
			},
		},
		{
			name:  "sessionEnd",
			event: "sessionEnd",
			body:  `{"session_id":"sess-9","reason":"window_closed","duration_ms":60000,"final_status":"completed","error_message":""}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventSessionEnd, ev.EventType)
				assert.Equal(t, "completed", ev.Status)
				assert.Empty(t, ev.Error)
				assert.Equal(t, "window_closed", ev.Metadata["reason"])
				assert.Equal(t, int64(60000), ev.Metadata["duration_ms"])
				assert.Zero(t, ev.DurationMs)
			},
		},
		{
			name:  "unknown event",
			event: "beforeReadFile",
			body:  `{` + common + `}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventOther, ev.EventType)
				assert.Equal(t, "beforeReadFile", ev.HookEventName)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := Normalize("cursor", tt.event, []byte(tt.body), testNow, nil)
			require.NoError(t, err)
			assert.Equal(t, AgentCursor, ev.Agent)
			assert.Equal(t, tt.event, ev.HookEventName)
			assert.Equal(t, "2026-11-27T07:30:00.123456789Z", ev.Timestamp)
			assert.Regexp(t, `^[0-9a-f]{32}$`, ev.ID)
			assert.Equal(t, SurfaceCLI, ev.Surface)
			if strings.Contains(tt.body, common) {
				assert.Equal(t, "conv-1", ev.SessionID)
				assert.Equal(t, "claude-4.5-sonnet", ev.Model)
				assert.Equal(t, "/Users/me/proj", ev.Workspace)
				assert.Equal(t, "2.1.0", ev.Metadata["cursor_version"])
			}
			assert.NotContains(t, mustJSON(t, ev), "me@example.com", "the account email is never collected")
			assert.NotContains(t, mustJSON(t, ev), "SECRET OUTPUT", "tool output is never collected")
			tt.check(t, ev)
		})
	}
}

func TestNormalize_CursorDetails(t *testing.T) {
	t.Run("BOM and whitespace are stripped", func(t *testing.T) {
		ev, err := Normalize("cursor", "beforeSubmitPrompt", []byte("\n \xEF\xBB\xBF{\"conversation_id\":\"c1\",\"prompt\":\"hi\"}\n"), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, "c1", ev.SessionID)
		assert.Equal(t, "hi", ev.Prompt)
	})

	t.Run("empty conversation id on a tool event", func(t *testing.T) {
		ev, err := Normalize("cursor", "postToolUse", []byte(`{"conversation_id":"","tool_name":"Shell","tool_input":{"command":"ls"},"cwd":"/repo"}`), testNow, nil)
		require.NoError(t, err)
		assert.Empty(t, ev.SessionID)
		assert.Equal(t, "/repo", ev.Workspace, "cwd stands in for the workspace roots")
	})

	t.Run("workspace falls back to CURSOR_PROJECT_DIR", func(t *testing.T) {
		ev, err := Normalize("cursor", "stop", []byte(`{"status":"completed"}`), testNow, envOf(map[string]string{"CURSOR_PROJECT_DIR": "/env/proj"}))
		require.NoError(t, err)
		assert.Equal(t, "/env/proj", ev.Workspace)
	})

	t.Run("file URI workspace roots", func(t *testing.T) {
		ev, err := Normalize("cursor", "stop", []byte(`{"workspace_roots":["file:///Users/me/my%20proj"]}`), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, "/Users/me/my proj", ev.Workspace)
	})

	t.Run("event name from the payload without --event", func(t *testing.T) {
		ev, err := Normalize("cursor", "", []byte(`{"hook_event_name":"afterAgentResponse","text":"ok"}`), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, EventAgentResponse, ev.EventType)
		assert.Equal(t, "afterAgentResponse", ev.HookEventName)
	})

	t.Run("a field of an unexpected type is passed over", func(t *testing.T) {
		ev, err := Normalize("cursor", "postToolUse", []byte(`{"conversation_id":"c1","model":{"name":"x"},"duration":"250","tool_name":"Shell","tool_input":{"command":42}}`), testNow, nil)
		require.NoError(t, err)
		assert.Empty(t, ev.Model)
		assert.Empty(t, ev.Command)
		assert.Equal(t, int64(250), ev.DurationMs)
	})

	t.Run("text is capped", func(t *testing.T) {
		long := strings.Repeat("é", MaxTextBytes) // two bytes each
		body, _ := json.Marshal(map[string]any{"prompt": long})
		ev, err := Normalize("cursor", "beforeSubmitPrompt", body, testNow, nil)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(ev.Prompt), MaxTextBytes)
		assert.True(t, strings.HasPrefix(long, ev.Prompt))
		assert.True(t, json.Valid([]byte(`"`+ev.Prompt+`"`)), "cut at a rune boundary")
	})
}

func TestCursorSurface(t *testing.T) {
	assert.Equal(t, SurfaceCLI, cursorSurface(envOf(nil)))
	for _, key := range []string{"VSCODE_PID", "VSCODE_IPC_HOOK", "VSCODE_CWD"} {
		assert.Equal(t, SurfaceIDE, cursorSurface(envOf(map[string]string{key: "1"})), key)
	}
	ev, err := Normalize("cursor", "stop", []byte(`{}`), testNow, envOf(map[string]string{"VSCODE_PID": "42"}))
	require.NoError(t, err)
	assert.Equal(t, SurfaceIDE, ev.Surface)
}

func TestNormalize_Antigravity(t *testing.T) {
	common := `"conversationId":"agy-conv","workspacePaths":["/home/me/app"],"transcriptPath":"/home/me/.gemini/antigravity-cli/conversations/agy-conv.pb","artifactDirectoryPath":"/home/me/.gemini/antigravity-cli/brain/agy-conv"`
	tests := []struct {
		name  string
		event string
		body  string
		check func(t *testing.T, ev Event)
	}{
		{
			name:  "SessionStart",
			event: "SessionStart",
			body:  `{` + common + `,"modelName":"gemini-3-pro"}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventSessionStart, ev.EventType)
				assert.Equal(t, "gemini-3-pro", ev.Model)
			},
		},
		{
			name:  "PreInvocation",
			event: "PreInvocation",
			body:  `{` + common + `,"lastUserInput":"add a login page","invocationNum":2}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventPromptSubmit, ev.EventType)
				assert.Equal(t, "add a login page", ev.Prompt)
				assert.Equal(t, int64(2), ev.Metadata["invocation_num"])
			},
		},
		{
			name:  "PreInvocation without lastUserInput",
			event: "PreInvocation",
			body:  `{` + common + `}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventPromptSubmit, ev.EventType)
				assert.Empty(t, ev.Prompt)
			},
		},
		{
			name:  "PostToolUse of run_command",
			event: "PostToolUse",
			body:  `{` + common + `,"toolCall":{"id":"call-1","name":"run_command","args":{"CommandLine":"npm test","Cwd":"/home/me/app"}},"stepIdx":7}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventToolUse, ev.EventType)
				assert.Equal(t, StatusOK, ev.Status)
				assert.Equal(t, "run_command", ev.ToolName)
				assert.Equal(t, "npm test", ev.Command)
				assert.Equal(t, `{"CommandLine":"npm test","Cwd":"/home/me/app"}`, ev.ToolInput)
				assert.Equal(t, int64(7), ev.Metadata["step_idx"])
				assert.Equal(t, "call-1", ev.Metadata["tool_call_id"])
			},
		},
		{
			name:  "PostToolUse of a file edit",
			event: "PostToolUse",
			body:  `{` + common + `,"toolCall":{"name":"write_to_file","args":{"TargetFile":"/home/me/app/index.ts","CodeContent":"x"}}}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "/home/me/app/index.ts", ev.FilePath)
			},
		},
		{
			name:  "PostToolUse of an MCP tool",
			event: "PostToolUse",
			body:  `{` + common + `,"toolCall":{"name":"mcp__linear__create_issue","args":{"title":"t"}}}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, "linear", ev.MCPServer)
			},
		},
		{
			name:  "PostToolUse with a string error",
			event: "PostToolUse",
			body:  `{` + common + `,"toolCall":{"name":"view_file","args":{"AbsolutePath":"/nope"}},"error":"file not found"}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventToolFailure, ev.EventType)
				assert.Equal(t, StatusError, ev.Status)
				assert.Equal(t, "file not found", ev.Error)
				assert.Equal(t, "/nope", ev.FilePath)
			},
		},
		{
			name:  "PostToolUse with an object error",
			event: "PostToolUse",
			body:  `{` + common + `,"toolCall":{"name":"run_command","args":{}},"error":{"code": 1, "message": "boom"}}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventToolFailure, ev.EventType)
				assert.Equal(t, `{"code":1,"message":"boom"}`, ev.Error)
			},
		},
		{
			name:  "PostToolUse with an empty error",
			event: "PostToolUse",
			body:  `{` + common + `,"toolCall":{"name":"run_command","args":{}},"error":""}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventToolUse, ev.EventType)
			},
		},
		{
			name:  "PostInvocation",
			event: "PostInvocation",
			body:  `{` + common + `,"content":"I added the page.","invocationNum":2}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventAgentResponse, ev.EventType)
				assert.Equal(t, "I added the page.", ev.Response)
			},
		},
		{
			name:  "Stop",
			event: "Stop",
			body:  `{` + common + `,"executionNum":4,"terminationReason":"user_cancelled","error":null,"fullyIdle":true}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventStop, ev.EventType)
				assert.Equal(t, "user_cancelled", ev.Status)
				assert.Empty(t, ev.Error)
				assert.Equal(t, "user_cancelled", ev.Metadata["termination_reason"])
				assert.Equal(t, int64(4), ev.Metadata["execution_num"])
				assert.Equal(t, true, ev.Metadata["fully_idle"])
			},
		},
		{
			name:  "unknown event",
			event: "PreToolUse",
			body:  `{` + common + `}`,
			check: func(t *testing.T, ev Event) {
				assert.Equal(t, EventOther, ev.EventType)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := Normalize("agy", tt.event, []byte(tt.body), testNow, nil)
			require.NoError(t, err)
			assert.Equal(t, AgentAntigravity, ev.Agent)
			assert.Equal(t, tt.event, ev.HookEventName)
			assert.Equal(t, "agy-conv", ev.SessionID)
			assert.Equal(t, "/home/me/app", ev.Workspace)
			assert.Equal(t, SurfaceCLI, ev.Surface)
			tt.check(t, ev)
		})
	}
}

func TestNormalize_AntigravityDetails(t *testing.T) {
	t.Run("newer builds' cwd and hookEventName", func(t *testing.T) {
		ev, err := Normalize("antigravity", "", []byte(`{"conversationId":"c","hookEventName":"PostInvocation","cwd":"/srv/x","content":"hi"}`), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, "PostInvocation", ev.HookEventName)
		assert.Equal(t, "/srv/x", ev.Workspace)
		assert.Empty(t, ev.Surface, "no transcript path, no surface")
	})

	t.Run("BOM", func(t *testing.T) {
		ev, err := Normalize("agy", "Stop", []byte("\xEF\xBB\xBF{\"conversationId\":\"c\",\"terminationReason\":\"completed\"}"), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, "completed", ev.Status)
	})
}

func TestAntigravitySurface(t *testing.T) {
	assert.Equal(t, SurfaceCLI, antigravitySurface("/Users/me/.gemini/antigravity-cli/conversations/x.pb"))
	assert.Equal(t, SurfaceIDE, antigravitySurface("/Users/me/.gemini/antigravity/conversations/x.pb"))
	assert.Equal(t, SurfaceIDE, antigravitySurface(`C:\Users\me\.gemini\antigravity\conversations\x.pb`))
	assert.Equal(t, "", antigravitySurface("/tmp/x.pb"))
	assert.Equal(t, "", antigravitySurface(""))
}

func TestNormalize_Errors(t *testing.T) {
	tests := []struct {
		name, agent, event, body string
		want                     error
	}{
		{"unknown agent", "claude", "stop", `{}`, ErrUnknownAgent},
		{"empty stdin", "cursor", "stop", "", ErrEmptyPayload},
		{"only a BOM", "cursor", "stop", "\xEF\xBB\xBF \n", ErrEmptyPayload},
		{"no event name", "agy", "", `{"conversationId":"c"}`, ErrNoEventName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Normalize(tt.agent, tt.event, []byte(tt.body), testNow, nil)
			assert.ErrorIs(t, err, tt.want)
		})
	}

	for _, body := range []string{`not json`, `{"prompt":`, `[1,2]`, `"text"`, `null`, `{"a":1} trailing`} {
		_, err := Normalize("cursor", "stop", []byte(body), testNow, nil)
		assert.Error(t, err, body)
	}
}

func TestResolveEventName(t *testing.T) {
	assert.Equal(t, "stop", ResolveEventName(" stop ", []byte(`{"hook_event_name":"x"}`)))
	assert.Equal(t, "beforeSubmitPrompt", ResolveEventName("", []byte(`{"hook_event_name":"beforeSubmitPrompt"}`)))
	assert.Equal(t, "Stop", ResolveEventName("", []byte(`{"hookEventName":"Stop"}`)))
	assert.Equal(t, "", ResolveEventName("", []byte(`garbage`)))
	assert.Equal(t, "postToolUse", ResolveEventName("", []byte(`{"hook_event_name":"postToolUse","tool_output":"cut sh`)), "a payload cut short")
}

func TestNormalizeTruncated(t *testing.T) {
	// Cut inside tool_output: the fields before it are read, the ones after are lost
	prefix := "\xEF\xBB\xBF" + `{"conversation_id":"c1","model":"gpt-5","workspace_roots":["/repo"],"tool_name":"Read",` +
		`"tool_input":{"path":"/repo/big.log"},"tool_output":"` + strings.Repeat("x", 1000)
	ev, err := NormalizeTruncated("cursor", "postToolUse", []byte(prefix), testNow, envOf(map[string]string{"VSCODE_PID": "1"}))
	require.NoError(t, err)
	assert.Equal(t, AgentCursor, ev.Agent)
	assert.Equal(t, SurfaceIDE, ev.Surface)
	assert.Equal(t, EventToolUse, ev.EventType)
	assert.Equal(t, "postToolUse", ev.HookEventName)
	assert.Equal(t, "c1", ev.SessionID)
	assert.Equal(t, "gpt-5", ev.Model)
	assert.Equal(t, "/repo", ev.Workspace)
	assert.Equal(t, "Read", ev.ToolName)
	assert.Equal(t, "/repo/big.log", ev.FilePath)
	assert.Equal(t, "2026-11-27T07:30:00.123456789Z", ev.Timestamp)
	assert.Regexp(t, `^[0-9a-f]{32}$`, ev.ID)
	assert.Equal(t, true, ev.Metadata["truncated"])

	// Nothing usable before the cut: the agent and the --event name are enough
	for name, prefix := range map[string]string{
		"cut in the first value": `{"lastUserInput":"` + strings.Repeat("y", 100),
		"not an object":          `garbage`,
		"empty":                  ``,
	} {
		ev, err := NormalizeTruncated("agy", "PreInvocation", []byte(prefix), testNow, nil)
		require.NoError(t, err, name)
		assert.Equal(t, AgentAntigravity, ev.Agent, name)
		assert.Equal(t, EventPromptSubmit, ev.EventType, name)
		assert.Empty(t, ev.Prompt, name)
		assert.Empty(t, ev.SessionID, name)
		assert.Equal(t, true, ev.Metadata["truncated"], name)
	}

	// The event name from a field before the cut, else no event
	ev, err = NormalizeTruncated("cursor", "", []byte(`{"hook_event_name":"stop","status":"comp`), testNow, nil)
	require.NoError(t, err)
	assert.Equal(t, EventStop, ev.EventType)
	assert.Empty(t, ev.Status, "cut before its end")
	_, err = NormalizeTruncated("cursor", "", []byte(`{"prompt":"x`), testNow, nil)
	assert.ErrorIs(t, err, ErrNoEventName)
	_, err = NormalizeTruncated("vim", "stop", []byte(`{}`), testNow, nil)
	assert.ErrorIs(t, err, ErrUnknownAgent)
}

func TestMCPServerFromToolName(t *testing.T) {
	assert.Equal(t, "github", mcpServerFromToolName("mcp__github__create_issue"))
	assert.Equal(t, "github", mcpServerFromToolName("MCP:github"))
	assert.Equal(t, "github", mcpServerFromToolName("mcp: github:create_issue"))
	assert.Equal(t, "", mcpServerFromToolName("Shell"))
	assert.Equal(t, "", mcpServerFromToolName("MCP:"))
}

func TestApplyToolInput_ServerKey(t *testing.T) {
	var ev Event
	applyToolInput(&ev, "call_mcp_tool", json.RawMessage(`{"server_name":"sentry","path":"/a"}`))
	assert.Equal(t, "sentry", ev.MCPServer)
	assert.Equal(t, "/a", ev.FilePath)
}

func TestEvent_JSONShape(t *testing.T) {
	ev := Event{ID: "id1", Agent: AgentCursor, EventType: EventStop, Timestamp: "2026-01-01T00:00:00Z"}
	assert.JSONEq(t, `{"id":"id1","agent":"cursor","event_type":"stop","timestamp":"2026-01-01T00:00:00Z"}`, mustJSON(t, ev))

	ev.DurationMs = 5
	ev.SetMetadata("excluded", true)
	assert.JSONEq(t, `{"id":"id1","agent":"cursor","event_type":"stop","timestamp":"2026-01-01T00:00:00Z","duration_ms":5,"metadata":{"excluded":true}}`, mustJSON(t, ev))
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

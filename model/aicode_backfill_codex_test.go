package model

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	codexSessionA = "11111111-1111-4111-8111-111111111111"
	codexSessionB = "22222222-2222-4222-8222-222222222222"
	codexSessionC = "33333333-3333-4333-8333-333333333333"
)

func codexTS(hour, min, sec int) string {
	return time.Date(2026, 9, 10, hour, min, sec, 0, time.UTC).Format("2006-01-02T15:04:05.000Z")
}

func codexLine(ts, typ string, payload any) obj {
	return obj{"timestamp": ts, "type": typ, "payload": payload}
}

func codexTokens(ts string, total, last obj) obj {
	info := obj{"model_context_window": 272000}
	if total != nil {
		info["total_token_usage"] = total
	}
	if last != nil {
		info["last_token_usage"] = last
	}
	return codexLine(ts, "event_msg", obj{"type": "token_count", "info": info})
}

func codexUsageObj(in, cached, out, reasoning, total int) obj {
	return obj{"input_tokens": in, "cached_input_tokens": cached, "output_tokens": out, "reasoning_output_tokens": reasoning, "total_tokens": total}
}

func codexUserItem(ts, text string) obj {
	return codexLine(ts, "response_item", obj{"type": "message", "role": "user", "content": []any{obj{"type": "input_text", "text": text}}})
}

func codexOutput(ts, callID, output string) obj {
	return codexLine(ts, "response_item", obj{"type": "function_call_output", "call_id": callID, "output": output})
}

func mustJSON(t *testing.T, v any) string {
	buf, err := json.Marshal(v)
	require.NoError(t, err)
	return string(buf)
}

func writeCodexFixtures(t *testing.T) string {
	home := t.TempDir()
	day := filepath.Join(home, "sessions", "2026", "09", "10")
	first := codexUsageObj(1000, 800, 100, 40, 1100)
	shellArgs := mustJSON(t, obj{"command": []any{"bash", "-lc", "go test ./..."}, "workdir": "/tmp/codex"})

	writeJSONL(t, filepath.Join(day, "rollout-2026-09-10T10-00-00-"+codexSessionA+".jsonl"),
		codexLine(codexTS(10, 0, 0), "session_meta", obj{"id": codexSessionA, "cwd": "/tmp/codex", "cli_version": "0.40.0", "model_provider": "openai", "originator": "codex_cli_rs"}),
		codexUserItem(codexTS(10, 0, 1), "<environment_context>\n  <cwd>/tmp/codex</cwd>\n</environment_context>"),
		codexUserItem(codexTS(10, 0, 1), "fix the tests"),
		codexLine(codexTS(10, 0, 1), "event_msg", obj{"type": "user_message", "message": "fix the tests", "images": []any{}}),
		codexLine(codexTS(10, 0, 2), "turn_context", obj{"cwd": "/tmp/codex", "model": "gpt-5-codex", "effort": "medium", "approval_policy": "on-request", "sandbox_policy": obj{"mode": "workspace-write"}}),
		codexLine(codexTS(10, 0, 3), "event_msg", obj{"type": "token_count", "info": nil}),
		codexLine(codexTS(10, 0, 4), "response_item", obj{"type": "function_call", "name": "shell", "arguments": shellArgs, "call_id": "call_1"}),
		codexTokens(codexTS(10, 0, 5), first, first),
		// Re-emitted count with unchanged totals.
		codexTokens(codexTS(10, 0, 6), first, first),
		codexOutput(codexTS(10, 0, 7), "call_1", mustJSON(t, obj{"output": "FAIL", "metadata": obj{"exit_code": 1, "duration_seconds": 2.5}})),
		codexLine(codexTS(10, 0, 8), "response_item", obj{"type": "custom_tool_call", "name": "apply_patch", "call_id": "call_2", "input": "*** Begin Patch\n" + strings.Repeat("+line\n", 1000)}),
		codexLine(codexTS(10, 0, 9), "response_item", obj{"type": "custom_tool_call_output", "call_id": "call_2", "output": "Exit code: 0\nWall time: 0.3 seconds\nOutput:\nSuccess"}),
		// Only the cumulative total: the turn's usage is the difference.
		codexTokens(codexTS(10, 0, 10), codexUsageObj(2500, 1800, 300, 90, 2800), nil),
		codexLine(codexTS(10, 0, 11), "response_item", obj{"type": "function_call", "name": "docs__search", "arguments": `{"q":"retry"}`, "call_id": "call_3"}),
		codexOutput(codexTS(10, 0, 12), "call_3", "found 3 results"),
		`{"timestamp":"broken`,
	)

	// A fork of session A copies its history before continuing.
	writeJSONL(t, filepath.Join(day, "rollout-2026-09-10T11-00-00-"+codexSessionB+".jsonl"),
		codexLine(codexTS(11, 0, 0), "session_meta", obj{"id": codexSessionB, "cwd": "/tmp/codex", "cli_version": "0.41.0", "model_provider": "openai"}),
		codexLine(codexTS(10, 0, 1), "event_msg", obj{"type": "user_message", "message": "fix the tests"}),
		codexTokens(codexTS(10, 0, 5), first, first),
		codexLine(codexTS(11, 0, 1), "turn_context", obj{"cwd": "/tmp/codex", "model": "gpt-5", "approval_policy": "never", "sandbox_policy": "read-only"}),
		codexLine(codexTS(11, 0, 5), "event_msg", obj{"type": "user_message", "message": "and run the linter"}),
		codexTokens(codexTS(11, 0, 6), codexUsageObj(3000, 900, 350, 90, 3350), codexUsageObj(500, 100, 50, 0, 550)),
	)

	// An old rollout: no session id in the metadata, no user_message events
	// and no turn context.
	writeJSONL(t, filepath.Join(home, "archived_sessions", "rollout-2025-08-01T09-00-00-"+codexSessionC+".jsonl"),
		codexLine("2025-08-01T09:00:00.000Z", "session_meta", obj{"cwd": "/tmp/old", "cli_version": "0.20.0"}),
		codexUserItem("2025-08-01T09:00:01.000Z", "old prompt"),
		codexTokens("2025-08-01T09:00:02.000Z", nil, codexUsageObj(10, 0, 5, 0, 15)),
	)
	return home
}

func TestParseCodexRollouts(t *testing.T) {
	home := writeCodexFixtures(t)
	t.Setenv("CODEX_HOME", home)

	res, err := ParseCodexRollouts(CodexSessionRoots(), BackfillOptions{})
	require.NoError(t, err)

	assert.Equal(t, 3, res.Files)
	assert.Equal(t, 1, res.BadLines)
	require.Len(t, res.Sessions, 3)

	a := sessionByID(t, res, codexSessionA)
	starts := eventsOfType(a, AICodeEventConversationStarts)
	require.Len(t, starts, 1)
	assert.Equal(t, "gpt-5-codex", starts[0].Model)
	assert.Equal(t, "openai", starts[0].Provider)
	assert.Equal(t, "medium", starts[0].ReasoningEffort)
	assert.Equal(t, "on-request", starts[0].ApprovalPolicy)
	assert.Equal(t, "workspace-write", starts[0].SandboxPolicy)

	prompts := eventsOfType(a, AICodeEventUserPrompt)
	require.Len(t, prompts, 1, "environment context and the duplicated response item are not prompts")
	assert.Equal(t, "fix the tests", prompts[0].Prompt)
	assert.Equal(t, 13, *prompts[0].PromptLength)

	tokens := eventsOfType(a, AICodeEventSSEEvent)
	require.Len(t, tokens, 2, "null info and re-emitted totals are skipped")
	for _, e := range tokens {
		assert.Equal(t, codexResponseCompleted, e.EventKind)
		assert.Equal(t, "gpt-5-codex", e.Model)
	}
	assert.Equal(t, []int{1000, 800, 100, 40}, []int{*tokens[0].InputTokens, *tokens[0].CacheReadTokens, *tokens[0].OutputTokens, *tokens[0].ReasoningTokens})
	assert.Equal(t, []int{1500, 1000, 200, 50}, []int{*tokens[1].InputTokens, *tokens[1].CacheReadTokens, *tokens[1].OutputTokens, *tokens[1].ReasoningTokens})

	tools := eventsOfType(a, AICodeEventToolResult)
	require.Len(t, tools, 3)
	assert.Equal(t, "shell", tools[0].ToolName)
	assert.Equal(t, "call_1", tools[0].CallID)
	assert.Equal(t, false, *tools[0].Success)
	assert.Equal(t, 2500, *tools[0].DurationMs)
	assert.Equal(t, "/tmp/codex", tools[0].ToolArguments["workdir"])
	assert.Equal(t, "apply_patch", tools[1].ToolName)
	assert.Equal(t, true, *tools[1].Success)
	assert.Equal(t, 300, *tools[1].DurationMs)
	assert.Nil(t, tools[1].ToolArguments, "large patches are not uploaded")
	assert.Equal(t, "docs__search", tools[2].ToolName)
	assert.Nil(t, tools[2].Success)
	assert.Nil(t, tools[2].DurationMs)

	for _, e := range a.Events {
		assert.Equal(t, AICodeClientCodex, e.ClientType)
		assert.Equal(t, codexSessionA, e.SessionID)
		assert.Equal(t, codexSessionA, e.ConversationID)
		assert.Equal(t, "/tmp/codex", e.Pwd)
		assert.Equal(t, "0.40.0", e.AppVersion)
	}

	b := sessionByID(t, res, codexSessionB)
	bPrompts := eventsOfType(b, AICodeEventUserPrompt)
	require.Len(t, bPrompts, 1, "copied history belongs to the original session")
	assert.Equal(t, "and run the linter", bPrompts[0].Prompt)
	bTokens := eventsOfType(b, AICodeEventSSEEvent)
	require.Len(t, bTokens, 1)
	assert.Equal(t, 500, *bTokens[0].InputTokens)
	assert.Equal(t, "gpt-5", bTokens[0].Model)
	assert.Equal(t, "read-only", eventsOfType(b, AICodeEventConversationStarts)[0].SandboxPolicy)

	c := sessionByID(t, res, codexSessionC)
	cPrompts := eventsOfType(c, AICodeEventUserPrompt)
	require.Len(t, cPrompts, 1)
	assert.Equal(t, "old prompt", cPrompts[0].Prompt)
	cTokens := eventsOfType(c, AICodeEventSSEEvent)
	require.Len(t, cTokens, 1)
	assert.Equal(t, codexFallbackModel, cTokens[0].Model)
	assert.Equal(t, "/tmp/old", cTokens[0].Pwd)
}

func TestParseCodexRolloutsIsDeterministic(t *testing.T) {
	home := writeCodexFixtures(t)
	roots := []string{filepath.Join(home, "sessions"), filepath.Join(home, "archived_sessions")}

	first, err := ParseCodexRollouts(roots, BackfillOptions{})
	require.NoError(t, err)
	second, err := ParseCodexRollouts(roots, BackfillOptions{})
	require.NoError(t, err)

	ids := func(res *BackfillParseResult) map[string]bool {
		out := map[string]bool{}
		for _, s := range res.Sessions {
			for _, e := range s.Events {
				out[e.EventID] = true
			}
		}
		return out
	}
	assert.Equal(t, ids(first), ids(second))
}

func TestCodexToolOutcome(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantSuccess  *bool
		wantDuration *int
	}{
		{name: "json metadata", output: `"{\"output\":\"ok\",\"metadata\":{\"exit_code\":0,\"duration_seconds\":0.25}}"`, wantSuccess: BoolRef(true), wantDuration: IntRef(250)},
		{name: "text form", output: `"Exit code: 2\nWall time: 1.5 seconds\nOutput:\nboom"`, wantSuccess: BoolRef(false), wantDuration: IntRef(1500)},
		{name: "plain output", output: `"hello"`},
		{name: "object output", output: `{"metadata":{"exit_code":0}}`, wantSuccess: BoolRef(true)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			success, duration := codexToolOutcome(json.RawMessage(tt.output))
			assert.Equal(t, tt.wantSuccess, success)
			assert.Equal(t, tt.wantDuration, duration)
		})
	}
}

func TestCodexSessionRootsDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	assert.Equal(t, []string{
		filepath.Join(home, ".codex", "sessions"),
		filepath.Join(home, ".codex", "archived_sessions"),
	}, CodexSessionRoots())
}

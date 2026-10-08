package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collmetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

// Fixtures shaped after the Claude Code monitoring reference (monitoring-usage.md) and Codex's
// codex-rs/otel event macros: which attributes exist and how each one is encoded.

const v2TestTimeNano = 1_791_446_400_123_456_789 // 2026-10-08T08:00:00.123456789Z

func claudeTestResource() *resourcev1.Resource {
	return serviceResource("claude-code",
		kv("service.version", strVal("2.1.300")),
		kv("os.type", strVal("darwin")),
		kv("os.version", strVal("25.0.0")),
		kv("host.arch", strVal("arm64")),
		kv("vcs.repository.url.full", strVal("https://github.com/example-org/example-repo")),
		kv("vcs.provider.name", strVal("github")),
		kv("user.name", strVal("alice")),
		kv("machine.name", strVal("mbp")),
		kv("team.id", strVal("shelltime")),
	)
}

// claudeRecord builds a Claude Code log record. Claude Code puts the unprefixed name in the
// event.name attribute and the prefixed one in the record's event name.
func claudeRecord(name string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	base := []*commonv1.KeyValue{
		kv("event.name", strVal(name)),
		kv("event.timestamp", strVal("2026-10-08T08:00:00.123Z")),
		kv("event.sequence", intVal(41)),
		kv("prompt.id", strVal("6f1c1d6e-prompt")),
		kv("session.id", strVal("sess-claude")),
		kv("app.version", strVal("2.1.300")),
		kv("organization.id", strVal("org-1")),
		kv("user.account_uuid", strVal("acct-uuid")),
		kv("user.account_id", strVal("user_01ABC")),
		kv("user.id", strVal("anon-1")),
		kv("user.email", strVal("dev@example.com")),
		kv("terminal.type", strVal("iTerm.app")),
		kv("app.entrypoint", strVal("cli")),
	}
	return &logsv1.LogRecord{
		TimeUnixNano: v2TestTimeNano,
		EventName:    "claude_code." + name,
		Body:         strVal("claude_code." + name),
		Attributes:   append(base, attrs...),
	}
}

func codexTestResource() *resourcev1.Resource {
	return serviceResource("codex_cli_rs",
		kv("service.version", strVal("0.50.0")),
		kv("env", strVal("prod")),
		kv("host.name", strVal("dev-box")),
	)
}

// codexRecord builds a Codex log record the way log_event! orders it: event.name and the
// event's own fields first, then the shared metadata.
func codexRecord(name string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	all := []*commonv1.KeyValue{kv("event.name", strVal("codex."+name))}
	all = append(all, attrs...)
	all = append(all,
		kv("event.timestamp", strVal("2026-10-08T08:00:00.123Z")),
		kv("conversation.id", strVal("conv-1")),
		kv("app.version", strVal("0.50.0")),
		kv("auth_mode", strVal("Chatgpt")),
		kv("originator", strVal("codex_cli_rs")),
		kv("user.account_id", strVal("acct-codex")),
		kv("user.email", strVal("dev@example.com")),
		kv("terminal.type", strVal("iTerm.app")),
		kv("model", strVal("gpt-5-codex")),
		kv("slug", strVal("gpt-5-codex")),
	)
	return &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, Attributes: all}
}

func parseTestRecord(t *testing.T, resource *resourcev1.Resource, lr *logsv1.LogRecord) *model.AICodeOtelEvent {
	t.Helper()
	source := detectOtelSource(resource)
	require.NotEmpty(t, source)
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	return p.parseLogRecord(lr, newOtelResource(resource, source), "test-scope")
}

func TestParseLogRecord_ClaudeSpecEvents(t *testing.T) {
	testCases := []struct {
		name   string
		record *logsv1.LogRecord
		check  func(t *testing.T, ev *model.AICodeOtelEvent)
	}{
		{
			name: "user_prompt",
			record: claudeRecord("user_prompt",
				kv("prompt_length", intVal(18)),
				kv("prompt", strVal("fix the failing test")),
				kv("prompt_text", strVal("fix the failing test")),
				kv("message.uuid", strVal("msg-uuid-1")),
				kv("command_name", strVal("compact")),
				kv("command_source", strVal("builtin")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventUserPrompt, ev.EventType)
				assert.Equal(t, model.IntRef(18), ev.PromptLength)
				assert.Equal(t, "fix the failing test", ev.Prompt)
				assert.Equal(t, "msg-uuid-1", ev.Attributes["message.uuid"])
				assert.Equal(t, "compact", ev.Attributes["command_name"])
				assert.Equal(t, "builtin", ev.Attributes["command_source"])
				assert.NotContains(t, ev.Attributes, "prompt_text", "mapped content isn't duplicated")
			},
		},
		{
			name: "user_prompt redacted",
			record: claudeRecord("user_prompt",
				kv("prompt_length", strVal("18")),
				kv("prompt", strVal("<REDACTED>")),
				kv("prompt_text", strVal("<REDACTED>")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Empty(t, ev.Prompt)
				assert.Equal(t, model.IntRef(18), ev.PromptLength)
				assert.NotContains(t, ev.Attributes, "prompt")
			},
		},
		{
			name:   "user_prompt from prompt_text",
			record: claudeRecord("user_prompt", kv("prompt_text", strVal("only in prompt_text"))),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, "only in prompt_text", ev.Prompt)
			},
		},
		{
			name: "assistant_response",
			record: claudeRecord("assistant_response",
				kv("response_length", intVal(17)),
				kv("response", strVal("Done! I fixed it.")),
				kv("model", strVal("claude-sonnet-5")),
				kv("request_id", strVal("req_011CXabc")),
				kv("message.uuid", strVal("msg-uuid-2")),
				kv("query_source", strVal("repl_main_thread")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventAssistantResponse, ev.EventType)
				assert.Equal(t, "Done! I fixed it.", ev.Response)
				assert.Equal(t, model.IntRef(17), ev.ResponseLength)
				assert.Equal(t, "claude-sonnet-5", ev.Model)
				assert.Equal(t, "req_011CXabc", ev.RequestID)
				assert.Equal(t, "repl_main_thread", ev.QuerySource)
			},
		},
		{
			name:   "assistant_response redacted",
			record: claudeRecord("assistant_response", kv("response_length", intVal(17)), kv("response", strVal("<REDACTED>"))),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Empty(t, ev.Response)
				assert.Equal(t, model.IntRef(17), ev.ResponseLength)
			},
		},
		{
			name: "tool_result failed",
			record: claudeRecord("tool_result",
				kv("tool_name", strVal("Bash")),
				kv("tool_use_id", strVal("toolu_01")),
				kv("success", strVal("false")),
				kv("duration_ms", intVal(1234)),
				kv("error_type", strVal("ShellError")),
				kv("error", strVal("Command failed with exit code 1")),
				kv("decision_type", strVal("accept")),
				kv("decision_source", strVal("config")),
				kv("tool_input_size_bytes", intVal(64)),
				kv("tool_result_size_bytes", intVal(512)),
				kv("tool_parameters", strVal(`{"bash_command":"go","full_command":"go test ./...","timeout":120000,"description":"Run tests"}`)),
				kv("tool_input", strVal(`{"command":"go test ./...","description":"Run tests"}`)),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventToolResult, ev.EventType)
				assert.Equal(t, "Bash", ev.ToolName)
				assert.Equal(t, "toolu_01", ev.CallID)
				assert.Equal(t, model.BoolRef(false), ev.Success)
				assert.Equal(t, model.IntRef(1234), ev.DurationMs)
				assert.Equal(t, "Command failed with exit code 1", ev.Error, "error wins over error_type")
				assert.Equal(t, "ShellError", ev.Attributes["error_type"])
				assert.Equal(t, `{"command":"go test ./...","description":"Run tests"}`, ev.ToolInput)
				assert.Nil(t, ev.ToolArguments)
				require.NotNil(t, ev.ToolParameters)
				assert.Equal(t, "go test ./...", ev.ToolParameters["full_command"])
				assert.Equal(t, "accept", ev.Attributes["decision_type"])
				assert.Equal(t, "config", ev.Attributes["decision_source"])
				assert.Equal(t, int64(64), ev.Attributes["tool_input_size_bytes"])
				assert.Equal(t, int64(512), ev.Attributes["tool_result_size_bytes"])
			},
		},
		{
			name: "tool_result error_type only",
			record: claudeRecord("tool_result",
				kv("tool_name", strVal("Read")),
				kv("success", strVal("false")),
				kv("error_type", strVal("Error:ENOENT")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, "Error:ENOENT", ev.Error)
				assert.Equal(t, "Error:ENOENT", ev.Attributes["error_type"])
			},
		},
		{
			name: "tool_decision",
			record: claudeRecord("tool_decision",
				kv("tool_name", strVal("Edit")),
				kv("tool_use_id", strVal("toolu_02")),
				kv("decision", strVal("reject")),
				kv("source", strVal("user_reject")),
				kv("tool_source", strVal("builtin")),
				kv("tool_parameters", strVal(`{"file_path":"/repo/main.go"}`)),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventToolDecision, ev.EventType)
				assert.Equal(t, "Edit", ev.ToolName)
				assert.Equal(t, "toolu_02", ev.CallID)
				assert.Equal(t, "reject", ev.Decision)
				assert.Equal(t, "user_reject", ev.Source)
				assert.Equal(t, "builtin", ev.Attributes["tool_source"])
				assert.Equal(t, "/repo/main.go", ev.ToolParameters["file_path"])
			},
		},
		{
			name: "api_request with zero cost",
			record: claudeRecord("api_request",
				kv("model", strVal("claude-sonnet-5")),
				kv("cost_usd", intVal(0)),
				kv("cost_usd_micros", intVal(0)),
				kv("duration_ms", intVal(2100)),
				kv("input_tokens", intVal(0)),
				kv("output_tokens", intVal(350)),
				kv("cache_read_tokens", intVal(12000)),
				kv("cache_creation_tokens", intVal(0)),
				kv("request_id", strVal("req_011CXdef")),
				kv("client_request_id", strVal("client-uuid")),
				kv("speed", strVal("fast")),
				kv("query_source", strVal("repl_main_thread")),
				kv("effort", strVal("high")),
				kv("agent.name", strVal("Explore")),
				kv("skill.name", strVal("custom")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventApiRequest, ev.EventType)
				assert.Equal(t, model.Float64Ref(0), ev.CostUSD, "an int-encoded 0 is kept")
				assert.Equal(t, model.IntRef(0), ev.InputTokens)
				assert.Equal(t, model.IntRef(350), ev.OutputTokens)
				assert.Equal(t, model.IntRef(12000), ev.CacheReadTokens)
				assert.Equal(t, model.IntRef(0), ev.CacheCreationTokens)
				assert.Equal(t, model.IntRef(2100), ev.DurationMs)
				assert.Equal(t, "req_011CXdef", ev.RequestID)
				assert.Equal(t, "fast", ev.Speed)
				assert.Equal(t, "repl_main_thread", ev.QuerySource)
				assert.Equal(t, "high", ev.ReasoningEffort)
				assert.Equal(t, "client-uuid", ev.Attributes["client_request_id"])
				assert.Equal(t, "Explore", ev.Attributes["agent.name"])
				assert.NotContains(t, ev.Attributes, "cost_usd_micros")
			},
		},
		{
			name:   "api_request with int cost",
			record: claudeRecord("api_request", kv("cost_usd", intVal(1)), kv("cost_usd_micros", intVal(1_000_000))),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.Float64Ref(1), ev.CostUSD)
			},
		},
		{
			name:   "api_request cost from micros",
			record: claudeRecord("api_request", kv("cost_usd_micros", intVal(1_234_567))),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				require.NotNil(t, ev.CostUSD)
				assert.InDelta(t, 1.234567, *ev.CostUSD, 1e-12)
			},
		},
		{
			name: "api_error",
			record: claudeRecord("api_error",
				kv("model", strVal("claude-sonnet-5")),
				kv("error", strVal("Overloaded")),
				kv("status_code", intVal(529)),
				kv("attempt", intVal(3)),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventApiError, ev.EventType)
				assert.Equal(t, "Overloaded", ev.Error)
				assert.Equal(t, model.IntRef(529), ev.StatusCode)
				assert.Equal(t, model.IntRef(3), ev.Attempt)
			},
		},
		{
			name: "compaction",
			record: claudeRecord("compaction",
				kv("trigger", strVal("auto")),
				kv("success", strVal("true")),
				kv("duration_ms", intVal(5000)),
				kv("pre_tokens", intVal(150000)),
				kv("post_tokens", intVal(20000)),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, "compaction", ev.EventType)
				assert.Equal(t, model.BoolRef(true), ev.Success)
				assert.Equal(t, model.IntRef(5000), ev.DurationMs)
				assert.Equal(t, "auto", ev.Attributes["trigger"])
				assert.Equal(t, int64(150000), ev.Attributes["pre_tokens"])
				assert.Equal(t, int64(20000), ev.Attributes["post_tokens"])
			},
		},
		{
			name: "hook_execution_complete",
			record: claudeRecord("hook_execution_complete",
				kv("hook_event", strVal("PreToolUse")),
				kv("hook_name", strVal("PreToolUse:Write")),
				kv("num_hooks", intVal(2)),
				kv("num_success", intVal(2)),
				kv("num_blocking", intVal(0)),
				kv("num_non_blocking_error", intVal(0)),
				kv("num_cancelled", intVal(0)),
				kv("total_duration_ms", intVal(45)),
				kv("managed_only", strVal("false")),
				kv("hook_source", strVal("merged")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, "hook_execution_complete", ev.EventType)
				assert.Equal(t, "PreToolUse:Write", ev.Attributes["hook_name"])
				assert.Equal(t, int64(2), ev.Attributes["num_success"])
				assert.Equal(t, int64(0), ev.Attributes["num_blocking"], "zero counts are kept")
				assert.Equal(t, int64(45), ev.Attributes["total_duration_ms"])
				assert.Equal(t, "false", ev.Attributes["managed_only"])
			},
		},
		{
			name: "subagent_completed",
			record: claudeRecord("subagent_completed",
				kv("agent_type", strVal("Explore")),
				kv("agent.source", strVal("built-in")),
				kv("is_built_in", boolVal(true)),
				kv("is_async", boolVal(false)),
				kv("total_tokens", intVal(12345)),
				kv("total_tool_uses", intVal(7)),
				kv("duration_ms", intVal(60000)),
				kv("model", strVal("claude-haiku-5")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, "subagent_completed", ev.EventType)
				assert.Equal(t, model.IntRef(60000), ev.DurationMs)
				assert.Equal(t, "claude-haiku-5", ev.Model)
				assert.Equal(t, "Explore", ev.Attributes["agent_type"])
				assert.Equal(t, true, ev.Attributes["is_built_in"])
				assert.Equal(t, false, ev.Attributes["is_async"], "false flags are kept")
				assert.Equal(t, int64(12345), ev.Attributes["total_tokens"])
				assert.Nil(t, ev.InputTokens, "total_tokens isn't a token count of this event")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ev := parseTestRecord(t, claudeTestResource(), tc.record)
			require.NotNil(t, ev)

			// Shared by every Claude Code event.
			assert.True(t, strings.HasPrefix(ev.EventID, model.AICodeOtelIDPrefix), ev.EventID)
			assert.Equal(t, model.AICodeOtelSourceClaudeCode, ev.ClientType)
			assert.Equal(t, int64(1_791_446_400), ev.Timestamp)
			assert.Equal(t, int64(1_791_446_400_123), ev.TimestampMs)
			assert.Equal(t, "2026-10-08T08:00:00.123Z", ev.EventTimestamp)
			assert.Equal(t, "6f1c1d6e-prompt", ev.PromptID)
			assert.Equal(t, model.Int64Ref(41), ev.Sequence)
			assert.Equal(t, "sess-claude", ev.SessionID)
			assert.Equal(t, "acct-uuid", ev.UserAccountUUID, "user.account_uuid wins over user.account_id")
			assert.Equal(t, "cli", ev.Entrypoint)
			assert.Equal(t, "alice", ev.UserName)
			assert.Equal(t, "mbp", ev.MachineName)
			assert.Equal(t, "darwin", ev.OSType)
			// Resource attributes without a field
			assert.Equal(t, "claude-code", ev.Attributes["service.name"])
			assert.Equal(t, "2.1.300", ev.Attributes["service.version"])
			assert.Equal(t, "https://github.com/example-org/example-repo", ev.Attributes["vcs.repository.url.full"])
			assert.Equal(t, "github", ev.Attributes["vcs.provider.name"])
			// Identity attributes have fields and stay out of the catch-all.
			for _, key := range []string{"session.id", "user.email", "prompt.id", "event.sequence", "event.name", "user.name", "machine.name", "team.id"} {
				assert.NotContains(t, ev.Attributes, key)
			}

			tc.check(t, ev)
		})
	}
}

func TestParseLogRecord_CodexSpecEvents(t *testing.T) {
	testCases := []struct {
		name   string
		record *logsv1.LogRecord
		check  func(t *testing.T, ev *model.AICodeOtelEvent)
	}{
		{
			name: "conversation_starts",
			record: codexRecord("conversation_starts",
				kv("provider_name", strVal("openai")),
				kv("auth.env_openai_api_key_present", boolVal(false)),
				kv("reasoning_effort", strVal("high")),
				kv("reasoning_summary", strVal("auto")),
				kv("context_window", intVal(272000)),
				kv("auto_compact_token_limit", intVal(244800)),
				kv("approval_policy", strVal("on-request")),
				kv("sandbox_policy", strVal("workspace-write")),
				kv("mcp_servers", strVal("filesystem, github")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventConversationStarts, ev.EventType)
				assert.Equal(t, "openai", ev.Provider)
				assert.Equal(t, "high", ev.ReasoningEffort)
				assert.Equal(t, "auto", ev.ReasoningSummary)
				assert.Equal(t, model.IntRef(272000), ev.ContextWindow)
				assert.Equal(t, model.IntRef(244800), ev.AutoCompactTokenLimit)
				assert.Equal(t, "on-request", ev.ApprovalPolicy)
				assert.Equal(t, "workspace-write", ev.SandboxPolicy)
				assert.Equal(t, []string{"filesystem", "github"}, ev.MCPServers)
				assert.Equal(t, false, ev.Attributes["auth.env_openai_api_key_present"])
			},
		},
		{
			name: "sse_event response.completed",
			record: codexRecord("sse_event",
				kv("event.kind", strVal("response.completed")),
				kv("input_token_count", strVal("1000")),
				kv("output_token_count", strVal("200")),
				kv("cached_token_count", intVal(800)),
				kv("cache_write_token_count", intVal(0)),
				kv("reasoning_token_count", intVal(64)),
				kv("tool_token_count", strVal("1200")),
				kv("ttft_ms", intVal(350)),
				kv("service_tier", strVal("flex")),
				kv("model_reasoning_effort", strVal("medium")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventSSEEvent, ev.EventType)
				assert.Equal(t, "response.completed", ev.EventKind)
				assert.Equal(t, model.IntRef(1000), ev.InputTokens)
				assert.Equal(t, model.IntRef(200), ev.OutputTokens)
				assert.Equal(t, model.IntRef(800), ev.CacheReadTokens)
				assert.Equal(t, model.IntRef(0), ev.CacheCreationTokens)
				assert.Equal(t, model.IntRef(64), ev.ReasoningTokens)
				assert.Nil(t, ev.ToolTokens, "tool_token_count is total_tokens, not tool tokens")
				assert.Equal(t, int64(1200), ev.Attributes["total_token_count"])
				assert.Equal(t, int64(350), ev.Attributes["ttft_ms"])
				assert.Equal(t, "flex", ev.Attributes["service_tier"])
				assert.Equal(t, "medium", ev.ReasoningEffort)
			},
		},
		{
			name: "sse_event failed",
			record: codexRecord("sse_event",
				kv("duration_ms", strVal("12")),
				kv("error.message", strVal("stream disconnected before completion")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, "stream disconnected before completion", ev.Error)
				assert.Equal(t, model.IntRef(12), ev.DurationMs)
			},
		},
		{
			name: "tool_result apply_patch",
			record: codexRecord("tool_result",
				kv("tool_result_seq", intVal(5)),
				kv("tool_name", strVal("apply_patch")),
				kv("tool_namespace", strVal("functions")),
				kv("call_id", strVal("call_abc")),
				kv("duration_ms", strVal("37")),
				kv("success", strVal("true")),
				kv("output_truncated", boolVal(false)),
				kv("agent_name", strVal("main")),
				kv("arguments", strVal("*** Begin Patch\n*** Update File: main.go\n@@\n-old\n+new\n*** End Patch")),
				kv("output", strVal("Success. Updated the following files:\nM main.go")),
				kv("mcp_server", strVal("")),
				kv("mcp_server_origin", strVal("")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventToolResult, ev.EventType)
				assert.Equal(t, "apply_patch", ev.ToolName)
				assert.Equal(t, "call_abc", ev.CallID)
				assert.Equal(t, model.IntRef(37), ev.DurationMs)
				assert.Equal(t, model.BoolRef(true), ev.Success)
				assert.Equal(t, "*** Begin Patch\n*** Update File: main.go\n@@\n-old\n+new\n*** End Patch", ev.ToolInput, "freeform arguments are kept raw")
				assert.Nil(t, ev.ToolArguments)
				assert.Equal(t, "Success. Updated the following files:\nM main.go", ev.ToolOutput)
				assert.Equal(t, int64(5), ev.Attributes["tool_result_seq"])
				assert.Equal(t, "functions", ev.Attributes["tool_namespace"])
				assert.Equal(t, false, ev.Attributes["output_truncated"])
				assert.Equal(t, "main", ev.Attributes["agent_name"])
				assert.NotContains(t, ev.Attributes, "mcp_server", "empty values are skipped")
				assert.NotContains(t, ev.Attributes, "output")
			},
		},
		{
			name: "tool_result shell JSON arguments",
			record: codexRecord("tool_result",
				kv("tool_name", strVal("shell")),
				kv("call_id", strVal("call_def")),
				kv("success", strVal("false")),
				kv("arguments", strVal(`{"command":["bash","-lc","go test ./..."],"workdir":"/repo"}`)),
				kv("output", strVal("exit status 1")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.BoolRef(false), ev.Success)
				assert.Equal(t, `{"command":["bash","-lc","go test ./..."],"workdir":"/repo"}`, ev.ToolInput)
				assert.Nil(t, ev.ToolArguments, "arguments no longer fill toolArguments")
			},
		},
		{
			name: "tool_decision",
			record: codexRecord("tool_decision",
				kv("tool_name", strVal("shell")),
				kv("tool_namespace", strVal("functions")),
				kv("call_id", strVal("call_xyz")),
				kv("decision", strVal("approved_for_session")),
				kv("source", strVal("User")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventToolDecision, ev.EventType)
				assert.Equal(t, "call_xyz", ev.CallID)
				assert.Equal(t, "approved_for_session", ev.Decision)
				assert.Equal(t, "User", ev.Source)
			},
		},
		{
			name: "turn_cost",
			record: codexRecord("turn_cost",
				kv("turn.id", strVal("turn-1")),
				kv("usage.estimated_usd", strVal("0.0123")),
				kv("turn.interrupted", boolVal(false)),
				kv("speed", strVal("fast")),
				kv("reasoning_effort", strVal("high")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventTurnCost, ev.EventType)
				assert.Nil(t, ev.CostUSD, "the turn's cost is already counted per request")
				assert.Equal(t, "0.0123", ev.Attributes["usage.estimated_usd"])
				assert.Equal(t, "turn-1", ev.Attributes["turn.id"])
				assert.Equal(t, false, ev.Attributes["turn.interrupted"])
				assert.Equal(t, "fast", ev.Speed)
				assert.Equal(t, "high", ev.ReasoningEffort)
			},
		},
		{
			name: "agent_response",
			record: codexRecord("agent_response",
				kv("agent.type", strVal("main")),
				kv("turn.id", strVal("turn-2")),
				kv("item.id", strVal("msg_1")),
				kv("response", strVal("Here is the summary")),
				kv("response_length", intVal(19)),
				kv("response_truncated", boolVal(false)),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventAgentResponse, ev.EventType)
				assert.Equal(t, "Here is the summary", ev.Response)
				assert.Equal(t, model.IntRef(19), ev.ResponseLength)
				assert.Equal(t, "main", ev.Attributes["agent.type"])
				assert.Equal(t, "msg_1", ev.Attributes["item.id"])
			},
		},
		{
			name:   "user_prompt redacted",
			record: codexRecord("user_prompt", kv("prompt_length", strVal("15")), kv("prompt", strVal("[REDACTED]"))),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.AICodeEventUserPrompt, ev.EventType)
				assert.Empty(t, ev.Prompt)
				assert.Equal(t, model.IntRef(15), ev.PromptLength)
			},
		},
		{
			name: "api_request",
			record: codexRecord("api_request",
				kv("duration_ms", strVal("830")),
				kv("http.response.status_code", intVal(200)),
				kv("attempt", intVal(1)),
				kv("endpoint", strVal("/responses")),
			),
			check: func(t *testing.T, ev *model.AICodeOtelEvent) {
				assert.Equal(t, model.IntRef(830), ev.DurationMs)
				assert.Equal(t, model.IntRef(200), ev.StatusCode)
				assert.Equal(t, model.IntRef(1), ev.Attempt)
				assert.Equal(t, "/responses", ev.Attributes["endpoint"])
				assert.Empty(t, ev.Error)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ev := parseTestRecord(t, codexTestResource(), tc.record)
			require.NotNil(t, ev)

			// Shared by every Codex event.
			assert.True(t, strings.HasPrefix(ev.EventID, model.AICodeOtelIDPrefix), ev.EventID)
			assert.Equal(t, model.AICodeOtelSourceCodex, ev.ClientType)
			assert.Equal(t, int64(1_791_446_400_123), ev.TimestampMs)
			assert.Equal(t, "conv-1", ev.ConversationID)
			assert.Equal(t, "conv-1", ev.SessionID)
			assert.Equal(t, "acct-codex", ev.UserAccountUUID)
			assert.Equal(t, "Chatgpt", ev.AuthMode)
			assert.Equal(t, "gpt-5-codex", ev.Model)
			assert.Equal(t, "codex_cli_rs", ev.Entrypoint, "from originator")
			assert.Equal(t, "dev-box", ev.MachineName, "host.name is the machine name fallback")
			assert.Equal(t, "prod", ev.Attributes["env"])
			assert.Equal(t, "dev-box", ev.Attributes["host.name"])
			assert.Equal(t, "0.50.0", ev.Attributes["service.version"])
			assert.NotContains(t, ev.Attributes, "originator")

			tc.check(t, ev)
		})
	}
}

func TestParseLogRecord_EntrypointFallsBackToServiceName(t *testing.T) {
	resource := serviceResource("claude-code-desktop")
	lr := &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, Attributes: []*commonv1.KeyValue{kv("event.name", strVal("user_prompt"))}}
	ev := parseTestRecord(t, resource, lr)
	require.NotNil(t, ev)
	assert.Equal(t, "claude-code-desktop", ev.Entrypoint)
}

func TestParseLogRecord_RecordAttributeOverridesResourceAttribute(t *testing.T) {
	resource := serviceResource("claude-code", kv("vcs.repository.name", strVal("from-resource")), kv("department", strVal("eng")))
	lr := &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, Attributes: []*commonv1.KeyValue{
		kv("event.name", strVal("user_prompt")),
		kv("vcs.repository.name", strVal("from-record")),
	}}
	ev := parseTestRecord(t, resource, lr)
	require.NotNil(t, ev)
	assert.Equal(t, "from-record", ev.Attributes["vcs.repository.name"])
	assert.Equal(t, "eng", ev.Attributes["department"], "custom OTEL_RESOURCE_ATTRIBUTES keys are kept")
}

func TestParseLogRecord_EventNameAndBodyFallback(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	res := newOtelResource(claudeTestResource(), model.AICodeOtelSourceClaudeCode)

	byEventName := &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, EventName: "claude_code.user_prompt", Attributes: []*commonv1.KeyValue{kv("prompt_length", intVal(3))}}
	ev := p.parseLogRecord(byEventName, res, "")
	require.NotNil(t, ev)
	assert.Equal(t, model.AICodeEventUserPrompt, ev.EventType)

	byBody := &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, Body: strVal("claude_code.tool_result")}
	ev = p.parseLogRecord(byBody, res, "")
	require.NotNil(t, ev)
	assert.Equal(t, model.AICodeEventToolResult, ev.EventType)

	plainLog := &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, Body: strVal("an ordinary log line")}
	assert.Nil(t, p.parseLogRecord(plainLog, res, ""))
}

func TestProcessLogs_DropListIsNotForwarded(t *testing.T) {
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	req := &collogsv1.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{{
		Resource: claudeTestResource(),
		ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: []*logsv1.LogRecord{
			claudeRecord("api_request_body", kv("body", strVal(`{"messages":[]}`))),
			claudeRecord("api_response_body", kv("body", strVal(`{"content":[]}`))),
			claudeRecord("system_prompt", kv("system_prompt", strVal("You are Claude Code"))),
			claudeRecord("user_prompt", kv("prompt_length", intVal(1))),
		}}},
	}}}

	_, err := cp.processor.ProcessLogs(context.Background(), req)
	require.NoError(t, err)
	reqs := cp.captured()
	require.Len(t, reqs, 1)
	require.Len(t, reqs[0].Events, 1)
	assert.Equal(t, model.AICodeEventUserPrompt, reqs[0].Events[0].EventType)

	// Only dropped events -> no request at all.
	onlyDropped := &collogsv1.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{{
		Resource:  claudeTestResource(),
		ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: []*logsv1.LogRecord{claudeRecord("api_request_body")}}},
	}}}
	_, err = cp.processor.ProcessLogs(context.Background(), onlyDropped)
	require.NoError(t, err)
	assert.Len(t, cp.captured(), 1)
}

func TestStableOtelIDs(t *testing.T) {
	logsRequest := func() *collogsv1.ExportLogsServiceRequest {
		return &collogsv1.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{{
			Resource: claudeTestResource(),
			ScopeLogs: []*logsv1.ScopeLogs{{
				Scope: &commonv1.InstrumentationScope{Name: "com.anthropic.claude_code.events"},
				LogRecords: []*logsv1.LogRecord{
					claudeRecord("user_prompt", kv("prompt_length", intVal(1))),
					claudeRecord("api_request", kv("cost_usd", dblVal(0.5))),
				},
			}},
		}}}
	}
	eventIDs := func(t *testing.T, req *collogsv1.ExportLogsServiceRequest) []string {
		cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})
		_, err := cp.processor.ProcessLogs(context.Background(), req)
		require.NoError(t, err)
		reqs := cp.captured()
		require.Len(t, reqs, 1)
		var ids []string
		for _, ev := range reqs[0].Events {
			ids = append(ids, ev.EventID)
		}
		return ids
	}

	first := eventIDs(t, logsRequest())
	require.Len(t, first, 2)
	assert.NotEqual(t, first[0], first[1], "different records get different ids")
	for _, id := range first {
		assert.Regexp(t, `^ot1:[0-9a-f]{40}$`, id)
	}

	// An exporter retry sends the same payload again: same ids, so the server de-duplicates.
	retry := eventIDs(t, proto.Clone(logsRequest()).(*collogsv1.ExportLogsServiceRequest))
	assert.Equal(t, first, retry)

	changedRecord := logsRequest()
	changedRecord.ResourceLogs[0].ScopeLogs[0].LogRecords[1].Attributes = append(changedRecord.ResourceLogs[0].ScopeLogs[0].LogRecords[1].Attributes, kv("speed", strVal("fast")))
	changed := eventIDs(t, changedRecord)
	assert.Equal(t, first[0], changed[0])
	assert.NotEqual(t, first[1], changed[1], "a different record gets a different id")

	changedResource := logsRequest()
	changedResource.ResourceLogs[0].Resource.Attributes = append(changedResource.ResourceLogs[0].Resource.Attributes, kv("host.name", strVal("other")))
	assert.NotEqual(t, first, eventIDs(t, changedResource), "the resource is part of the id")

	changedScope := logsRequest()
	changedScope.ResourceLogs[0].ScopeLogs[0].Scope.Name = "other.scope"
	assert.NotEqual(t, first, eventIDs(t, changedScope), "the scope is part of the id")

	// Metrics: same data point, same id; a later data point, another id.
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	res := newOtelResource(claudeTestResource(), model.AICodeOtelSourceClaudeCode)
	metric := func(ts uint64) *metricsv1.Metric {
		return &metricsv1.Metric{Name: "claude_code.cost.usage", Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
			AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
			DataPoints:             []*metricsv1.NumberDataPoint{{TimeUnixNano: ts, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 0.1}}},
		}}}
	}
	m1 := p.parseMetric(metric(v2TestTimeNano), res, "com.anthropic.claude_code")
	m2 := p.parseMetric(metric(v2TestTimeNano), res, "com.anthropic.claude_code")
	m3 := p.parseMetric(metric(v2TestTimeNano+10_000_000_000), res, "com.anthropic.claude_code")
	require.Len(t, m1, 1)
	assert.Regexp(t, `^ot1:[0-9a-f]{40}$`, m1[0].MetricID)
	assert.Equal(t, m1[0].MetricID, m2[0].MetricID)
	assert.NotEqual(t, m1[0].MetricID, m3[0].MetricID)
}

func sumMetric(name string, temporality metricsv1.AggregationTemporality, value int64, attrs ...*commonv1.KeyValue) *metricsv1.Metric {
	return &metricsv1.Metric{Name: name, Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
		AggregationTemporality: temporality,
		IsMonotonic:            true,
		DataPoints: []*metricsv1.NumberDataPoint{{
			TimeUnixNano: v2TestTimeNano,
			Value:        &metricsv1.NumberDataPoint_AsInt{AsInt: value},
			Attributes:   attrs,
		}},
	}}}
}

func TestParseMetric_TypeRoutingAndAttributes(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	res := newOtelResource(claudeTestResource(), model.AICodeOtelSourceClaudeCode)
	delta := metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
	parse := func(m *metricsv1.Metric) model.AICodeOtelMetric {
		t.Helper()
		got := p.parseMetric(m, res, "com.anthropic.claude_code")
		require.Len(t, got, 1)
		return got[0]
	}

	token := parse(sumMetric("claude_code.token.usage", delta, 1200,
		kv("type", strVal("cacheRead")),
		kv("model", strVal("claude-sonnet-5")),
		kv("query_source", strVal("main")),
		kv("effort", strVal("high")),
		kv("session.id", strVal("sess-m")),
		kv("user.name", strVal("alice")),
		kv("machine.name", strVal("mbp")),
		kv("team.id", strVal("shelltime")),
	))
	assert.Equal(t, model.AICodeMetricTokenUsage, token.MetricType)
	assert.Equal(t, "cacheRead", token.TokenType)
	assert.Empty(t, token.LinesType)
	assert.Equal(t, "claude-sonnet-5", token.Model)
	assert.Equal(t, int64(1_791_446_400_123), token.TimestampMs)
	assert.Equal(t, "sess-m", token.SessionID)
	assert.Equal(t, map[string]any{"query_source": "main", "effort": "high"}, token.Attributes, "identity keys have fields")

	lines := parse(sumMetric("claude_code.lines_of_code.count", delta, 10, kv("type", strVal("added"))))
	assert.Equal(t, "added", lines.LinesType)
	assert.Empty(t, lines.TokenType)

	active := parse(sumMetric("claude_code.active_time.total", delta, 30, kv("type", strVal("cli"))))
	assert.Empty(t, active.TokenType, "active time's type isn't a token type")
	assert.Empty(t, active.LinesType)
	assert.Equal(t, "cli", active.Attributes["type"])

	cost := parse(sumMetric("claude_code.cost.usage", delta, 1, kv("speed", strVal("fast"))))
	assert.Empty(t, cost.TokenType)
	assert.Equal(t, "fast", cost.Attributes["speed"])

	decision := parse(sumMetric("claude_code.code_edit_tool.decision", delta, 1,
		kv("tool_name", strVal("Edit")),
		kv("decision", strVal("accept")),
		kv("source", strVal("user_temporary")),
		kv("language", strVal("Go")),
	))
	assert.Equal(t, "Edit", decision.Tool, "tool_name is read")
	assert.Equal(t, "accept", decision.Decision)
	assert.Equal(t, "Go", decision.Language)
	assert.Equal(t, "user_temporary", decision.Attributes["source"])

	legacyTool := parse(sumMetric("claude_code.code_edit_tool.decision", delta, 1, kv("tool", strVal("Write"))))
	assert.Equal(t, "Write", legacyTool.Tool)
}

func TestParseMetric_CumulativeTemporalityWarnsOnce(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&lockedWriter{w: &buf, mu: &mu}, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	res := newOtelResource(claudeTestResource(), model.AICodeOtelSourceClaudeCode)
	cumulative := metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE
	for i := 0; i < 3; i++ {
		got := p.parseMetric(sumMetric("claude_code.session.count", cumulative, 1), res, "")
		require.Len(t, got, 1, "cumulative data is still forwarded")
	}
	p.parseMetric(sumMetric("claude_code.session.count", metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, 1), res, "")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, strings.Count(buf.String(), "cumulative"), buf.String())
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func TestParseLogRecord_AttributeCaps(t *testing.T) {
	attrs := []*commonv1.KeyValue{kv("event.name", strVal("tool_result"))}
	for i := 0; i < 100; i++ {
		attrs = append(attrs, kv(fmt.Sprintf("custom.attr_%03d", i), intVal(int64(i))))
	}
	long := strings.Repeat("é", 4000) // 8000 bytes
	attrs = append(attrs,
		kv("tool_input", strVal(strings.Repeat("i", 100<<10))),
		kv("output", strVal(strings.Repeat("o", 100<<10))),
		kv("response", strVal(strings.Repeat("r", 100<<10))),
	)
	lr := &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, Attributes: attrs}

	ev := parseTestRecord(t, serviceResource("codex_cli_rs", kv("vcs.repository.url.full", strVal("https://github.com/o/r"))), lr)
	require.NotNil(t, ev)
	assert.Len(t, ev.Attributes, model.AICodeOtelMaxAttributes)
	assert.Equal(t, "https://github.com/o/r", ev.Attributes["vcs.repository.url.full"], "resource attributes are kept first")
	assert.Equal(t, "codex_cli_rs", ev.Attributes["service.name"])
	assert.NotContains(t, ev.Attributes, "custom.attr_099", "attributes past the cap are dropped")
	assert.Len(t, ev.ToolInput, model.AICodeOtelMaxTextBytes)
	assert.Len(t, ev.ToolOutput, model.AICodeOtelMaxTextBytes)
	assert.Len(t, ev.Response, model.AICodeOtelMaxTextBytes)

	// A long value is capped without splitting a UTF-8 character.
	lr = &logsv1.LogRecord{TimeUnixNano: v2TestTimeNano, Attributes: []*commonv1.KeyValue{
		kv("event.name", strVal("tool_result")),
		kv("hook_definitions", strVal(long)),
		kv("workspace.host_paths", arrVal(strVal(long), strVal("/b"))),
		kv("tool_parameters", strVal(long)), // not a JSON object: kept raw, capped
	}}
	ev = parseTestRecord(t, serviceResource("claude-code"), lr)
	require.NotNil(t, ev)
	capped, _ := ev.Attributes["hook_definitions"].(string)
	assert.Len(t, capped, model.AICodeOtelMaxAttributeBytes)
	assert.True(t, strings.HasPrefix(long, capped))
	paths, _ := ev.Attributes["workspace.host_paths"].([]any)
	require.Len(t, paths, 2)
	assert.Len(t, paths[0], model.AICodeOtelMaxAttributeBytes)
	assert.Equal(t, "/b", paths[1])
	assert.Len(t, ev.Attributes["tool_parameters"], model.AICodeOtelMaxAttributeBytes)

	// Everything still encodes as JSON.
	_, err := json.Marshal(ev)
	require.NoError(t, err)
}

func TestProcessLogs_ChunkedSending(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		mu.Lock()
		bodies = append(bodies, buf.Bytes())
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(model.AICodeOtelResponse{Success: true})
	}))
	t.Cleanup(server.Close)

	p := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})
	const limit = 16 << 10
	p.maxRequestBytes = limit

	var records []*logsv1.LogRecord
	for i := 0; i < 40; i++ {
		records = append(records, claudeRecord("tool_result",
			kv("tool_use_id", strVal(fmt.Sprintf("toolu_%02d", i))),
			kv("tool_input", strVal(strings.Repeat("x", 1500))),
		))
	}
	req := &collogsv1.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{{
		Resource:  claudeTestResource(),
		ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: records}},
	}}}
	_, err := p.ProcessLogs(context.Background(), req)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Greater(t, len(bodies), 1, "the batch is split")
	var callIDs []string
	for _, body := range bodies {
		assert.LessOrEqual(t, len(body), limit)
		var got model.AICodeOtelRequest
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, model.AICodeOtelSourceClaudeCode, got.Source)
		for _, ev := range got.Events {
			callIDs = append(callIDs, ev.CallID)
		}
	}
	require.Len(t, callIDs, 40)
	for i, id := range callIDs {
		assert.Equal(t, fmt.Sprintf("toolu_%02d", i), id, "order is kept")
	}
}

func TestProcessMetrics_ChunkedSending(t *testing.T) {
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})
	cp.processor.maxRequestBytes = 4 << 10

	var metrics []*metricsv1.Metric
	for i := 0; i < 30; i++ {
		metrics = append(metrics, sumMetric("claude_code.token.usage", metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, int64(i), kv("type", strVal("input"))))
	}
	req := &collmetricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricsv1.ResourceMetrics{{
		Resource:     claudeTestResource(),
		ScopeMetrics: []*metricsv1.ScopeMetrics{{Metrics: metrics}},
	}}}
	_, err := cp.processor.ProcessMetrics(context.Background(), req)
	require.NoError(t, err)

	reqs := cp.captured()
	require.Greater(t, len(reqs), 1)
	total := 0
	for _, r := range reqs {
		for _, m := range r.Metrics {
			assert.Equal(t, float64(total), m.Value, "order is kept")
			total++
		}
	}
	assert.Equal(t, 30, total)
}

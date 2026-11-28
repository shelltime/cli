// Package agenthook turns the native hook payloads of coding agents that expose no OTEL
// (Cursor and Google Antigravity) into one normalized event, and answers each hook with
// the JSON the agent needs to carry on. It depends on nothing else in the module, so the
// hook-ingest command, the daemon and the model share it.
package agenthook

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Agents, as the server names them.
const (
	AgentCursor      = "cursor"
	AgentAntigravity = "antigravity"
)

// Surfaces an agent runs on. Empty when it can't be told.
const (
	SurfaceIDE = "ide"
	SurfaceCLI = "cli"
)

// Event types, the agent-independent kind of a hook event.
const (
	EventSessionStart  = "session_start"
	EventPromptSubmit  = "prompt_submit"
	EventToolUse       = "tool_use"
	EventToolFailure   = "tool_failure"
	EventAgentResponse = "agent_response"
	EventStop          = "stop"
	EventSessionEnd    = "session_end"
	EventOther         = "other"
)

// Statuses set by the mappers beyond the ones the agents send themselves.
const (
	StatusOK    = "ok"
	StatusError = "error"
)

// MaxTextBytes caps every text field of an event (prompts, responses, tool input, errors).
const MaxTextBytes = 64 * 1024

// Event is one hook event, normalized. The CLI, the daemon and the server share this shape;
// every string field is left out of the JSON when empty.
type Event struct {
	// ID is a random 128-bit hex id minted once by hook-ingest, so resends are deduplicated.
	ID      string `json:"id,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Surface string `json:"surface,omitempty"`
	// Cloud is set when the hook ran in a cloud agent's VM rather than on one of the user's
	// machines, to the signal that showed it (CloudSignal*).
	Cloud string `json:"cloud,omitempty"`
	// SessionID is the agent's conversation id.
	SessionID     string `json:"session_id,omitempty"`
	EventType     string `json:"event_type,omitempty"`
	HookEventName string `json:"hook_event_name,omitempty"`
	// Timestamp is RFC 3339 with nanoseconds, in UTC.
	Timestamp string `json:"timestamp,omitempty"`
	Workspace string `json:"workspace,omitempty"`

	Repository string `json:"repository,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
	Model      string `json:"model,omitempty"`

	ToolName   string `json:"tool_name,omitempty"`
	MCPServer  string `json:"mcp_server,omitempty"`
	Command    string `json:"command,omitempty"`
	FilePath   string `json:"file_path,omitempty"`
	Status     string `json:"status,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`

	Prompt    string `json:"prompt,omitempty"`
	Response  string `json:"response,omitempty"`
	ToolInput string `json:"tool_input,omitempty"`

	// Token counts for a model turn/invocation when reported. InputTokens excludes cache
	// reads and cache writes (uncached input).
	InputTokens         *int `json:"input_tokens,omitempty"`
	OutputTokens        *int `json:"output_tokens,omitempty"`
	CacheReadTokens     *int `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens *int `json:"cache_creation_tokens,omitempty"`
	ReasoningTokens     *int `json:"reasoning_tokens,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`

	// YieldToUserHooks travels from hook-ingest to the daemon only, never to the server: the
	// event came from a project's hook while an older version's user-level hook records it too,
	// so the daemon drops it unless the session is a cloud run (whose agent runs no user-level
	// hooks), which only the daemon remembers across events.
	YieldToUserHooks bool `json:"yield_to_user_hooks,omitempty"`

	// PullRequestURLs travels from hook-ingest to the daemon (and the pending file) only,
	// never to POST /api/v1/agent-hooks: it holds normalized pull request URLs opened by a
	// successful `gh pr create` or GitHub MCP `create_pull_request` tool call so the daemon
	// can link them via POST /api/v1/cc/session-pull-requests.
	PullRequestURLs []string `json:"pull_request_urls,omitempty"`
}

// SetMetadata sets one metadata key, creating the map when needed.
func (e *Event) SetMetadata(key string, value any) {
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	e.Metadata[key] = value
}

// ParseAgent returns the canonical agent name for a --agent value: cursor, or antigravity
// for agy and antigravity. ok is false for anything else.
func ParseAgent(s string) (agent string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cursor":
		return AgentCursor, true
	case "agy", "antigravity":
		return AgentAntigravity, true
	}
	return "", false
}

// NewID returns a random 128-bit id in hex.
func NewID() string {
	var b [16]byte
	// crypto/rand.Read never returns an error (it panics if the system has no randomness)
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

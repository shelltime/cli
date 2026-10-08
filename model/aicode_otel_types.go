package model

import "unicode/utf8"

// AICodeOtelRequest is the main request to POST /api/v1/cc/otel
// Flat structure without session - resource attributes are embedded in each metric/event
type AICodeOtelRequest struct {
	Host    string             `json:"host"`
	Project string             `json:"project"`
	Source  string             `json:"source,omitempty"` // "claude-code" or "codex" - identifies the CLI source
	Events  []AICodeOtelEvent  `json:"events,omitempty"`
	Metrics []AICodeOtelMetric `json:"metrics,omitempty"`
}

// AICodeOtelResourceAttributes contains common resource-level attributes
// extracted from OTEL resources and embedded into each metric/event
type AICodeOtelResourceAttributes struct {
	// Standard resource attributes
	SessionID       string
	EventKind       string
	ConversationID  string // Codex uses conversation.id instead of session.id
	UserAccountUUID string
	OrganizationID  string
	TerminalType    string
	AppVersion      string
	ServiceVersion  string
	OSType          string
	OSVersion       string
	HostArch        string
	WSLVersion      string

	// ServiceName is the resource service.name (claude-code, claude-code-desktop, codex_cli_rs, ...).
	// It is the Entrypoint fallback.
	ServiceName string
	// HostName is the resource host.name (sent by Codex). It is the MachineName fallback.
	HostName string
	// Entrypoint is app.entrypoint when it is set on the resource.
	Entrypoint string

	// Additional attributes from data points
	UserID    string // from user.id (hashed identifier)
	UserEmail string // from user.email

	// Custom resource attributes (from OTEL_RESOURCE_ATTRIBUTES)
	UserName    string // from user.name
	MachineName string // from machine.name
	TeamID      string // from team.id
	Pwd         string // from pwd

	// Attributes holds the resource attributes that have no dedicated field (service.name,
	// service.version, wsl.version, env, vcs.*, host.name, custom OTEL_RESOURCE_ATTRIBUTES keys).
	// They are merged into each event's attributes.
	Attributes map[string]any
}

// AICodeOtelEvent represents an event from Claude Code or Codex (api_request, tool_result, etc.)
// with embedded resource attributes for a flat, session-less structure.
//
// Optional counts, costs and flags are pointers so that false and zero values survive omitempty
// (a failed tool call must be sent as "success": false).
type AICodeOtelEvent struct {
	EventID        string `json:"eventId"`
	EventType      string `json:"eventType"`
	Timestamp      int64  `json:"timestamp"`                // unix seconds, kept for older servers
	TimestampMs    int64  `json:"timestampMs,omitempty"`    // unix milliseconds
	EventTimestamp string `json:"eventTimestamp,omitempty"` // ISO 8601 timestamp

	Model               string                 `json:"model,omitempty"`
	CostUSD             *float64               `json:"costUsd,omitempty"`
	DurationMs          *int                   `json:"durationMs,omitempty"`
	InputTokens         *int                   `json:"inputTokens,omitempty"`
	OutputTokens        *int                   `json:"outputTokens,omitempty"`
	CacheReadTokens     *int                   `json:"cacheReadTokens,omitempty"`
	CacheCreationTokens *int                   `json:"cacheCreationTokens,omitempty"`
	ReasoningTokens     *int                   `json:"reasoningTokens,omitempty"` // Codex: reasoning output tokens
	ToolName            string                 `json:"toolName,omitempty"`
	Success             *bool                  `json:"success,omitempty"`
	Decision            string                 `json:"decision,omitempty"`
	Source              string                 `json:"source,omitempty"`
	Error               string                 `json:"error,omitempty"`
	PromptLength        *int                   `json:"promptLength,omitempty"`
	Prompt              string                 `json:"prompt,omitempty"`
	PromptEncrypted     bool                   `json:"promptEncrypted,omitempty"` // Whether prompt is encrypted
	ToolParameters      map[string]interface{} `json:"toolParameters,omitempty"`
	StatusCode          *int                   `json:"statusCode,omitempty"`
	Attempt             *int                   `json:"attempt,omitempty"`
	Language            string                 `json:"language,omitempty"`
	Provider            string                 `json:"provider,omitempty"` // Codex: provider (e.g., "openai")

	// Correlation and request details (payload v2)
	PromptID       string `json:"promptId,omitempty"`    // Claude prompt.id
	Sequence       *int64 `json:"sequence,omitempty"`    // Claude event.sequence (0-based)
	RequestID      string `json:"requestId,omitempty"`   // API request id
	Speed          string `json:"speed,omitempty"`       // "fast" / "normal"
	QuerySource    string `json:"querySource,omitempty"` // subsystem that issued the request
	Response       string `json:"response,omitempty"`    // assistant / agent response text
	ResponseLength *int   `json:"responseLength,omitempty"`
	ToolInput      string `json:"toolInput,omitempty"`  // raw tool input (Claude JSON, Codex arguments)
	Entrypoint     string `json:"entrypoint,omitempty"` // app.entrypoint, Codex originator, or service.name

	// Attributes carries every attribute without a dedicated field, so nothing is dropped silently.
	Attributes map[string]any `json:"attributes,omitempty"`

	// Tool call id: Codex call_id, Claude tool_use_id
	CallID string `json:"callId,omitempty"`

	// Codex-specific fields for sse_event
	EventKind  string `json:"eventKind,omitempty"`
	ToolTokens *int   `json:"toolTokens,omitempty"`

	// Codex-specific fields for conversation_starts
	AuthMode              string   `json:"authMode,omitempty"`
	Slug                  string   `json:"slug,omitempty"`
	ContextWindow         *int     `json:"contextWindow,omitempty"`
	ApprovalPolicy        string   `json:"approvalPolicy,omitempty"`
	SandboxPolicy         string   `json:"sandboxPolicy,omitempty"`
	MCPServers            []string `json:"mcpServers,omitempty"`
	Profile               string   `json:"profile,omitempty"`
	ReasoningEnabled      *bool    `json:"reasoningEnabled,omitempty"`
	ReasoningEffort       string   `json:"reasoningEffort,omitempty"`
	ReasoningSummary      string   `json:"reasoningSummary,omitempty"`
	MaxOutputTokens       *int     `json:"maxOutputTokens,omitempty"`
	AutoCompactTokenLimit *int     `json:"autoCompactTokenLimit,omitempty"`

	// Codex-specific fields for tool_result
	ToolArguments map[string]interface{} `json:"toolArguments,omitempty"` // legacy tool_arguments JSON object
	ToolOutput    string                 `json:"toolOutput,omitempty"`

	// Embedded resource attributes (previously in session)
	SessionID       string `json:"sessionId,omitempty"`
	ConversationID  string `json:"conversationId,omitempty"` // Codex uses conversationId instead of sessionId
	UserAccountUUID string `json:"userAccountUuid,omitempty"`
	OrganizationID  string `json:"organizationId,omitempty"`
	TerminalType    string `json:"terminalType,omitempty"`
	AppVersion      string `json:"appVersion,omitempty"`
	OSType          string `json:"osType,omitempty"`
	OSVersion       string `json:"osVersion,omitempty"`
	HostArch        string `json:"hostArch,omitempty"`

	// Additional identifiers
	UserID    string `json:"userId,omitempty"`
	UserEmail string `json:"userEmail,omitempty"`

	// Custom resource attributes
	UserName    string `json:"userName,omitempty"`
	MachineName string `json:"machineName,omitempty"`
	TeamID      string `json:"teamId,omitempty"`
	Pwd         string `json:"pwd,omitempty"`

	ClientType string `json:"clientType"` // claude_code, codex (defaults to claude_code)
}

// AICodeOtelMetric represents a metric data point from Claude Code or Codex
// with embedded resource attributes for a flat, session-less structure
type AICodeOtelMetric struct {
	MetricID    string  `json:"metricId"`
	MetricType  string  `json:"metricType"`
	Timestamp   int64   `json:"timestamp"`             // unix seconds, kept for older servers
	TimestampMs int64   `json:"timestampMs,omitempty"` // unix milliseconds
	Value       float64 `json:"value"`
	Model       string  `json:"model,omitempty"`
	TokenType   string  `json:"tokenType,omitempty"`
	LinesType   string  `json:"linesType,omitempty"`
	Tool        string  `json:"tool,omitempty"`
	Decision    string  `json:"decision,omitempty"`
	Language    string  `json:"language,omitempty"`

	// Attributes carries every data point attribute without a dedicated field.
	Attributes map[string]any `json:"attributes,omitempty"`

	// Embedded resource attributes (previously in session)
	SessionID       string `json:"sessionId,omitempty"`
	ConversationID  string `json:"conversationId,omitempty"` // Codex uses conversationId instead of sessionId
	UserAccountUUID string `json:"userAccountUuid,omitempty"`
	OrganizationID  string `json:"organizationId,omitempty"`
	TerminalType    string `json:"terminalType,omitempty"`
	AppVersion      string `json:"appVersion,omitempty"`
	OSType          string `json:"osType,omitempty"`
	OSVersion       string `json:"osVersion,omitempty"`
	HostArch        string `json:"hostArch,omitempty"`

	// Additional identifiers
	UserID    string `json:"userId,omitempty"`
	UserEmail string `json:"userEmail,omitempty"`

	// Custom resource attributes
	UserName    string `json:"userName,omitempty"`
	MachineName string `json:"machineName,omitempty"`
	TeamID      string `json:"teamId,omitempty"`
	Pwd         string `json:"pwd,omitempty"`

	ClientType string `json:"clientType"` // claude_code, codex (defaults to claude_code)
}

// AICodeOtelResponse is the response from POST /api/v1/cc/otel
type AICodeOtelResponse struct {
	Success          bool   `json:"success"`
	EventsProcessed  int    `json:"eventsProcessed"`
	MetricsProcessed int    `json:"metricsProcessed"`
	Message          string `json:"message,omitempty"`
}

// AICodeOtelIDPrefix marks event and metric ids derived from the OTLP payload itself, so an
// exporter retry produces the same id and the server can de-duplicate it (like backfill's bf1:).
const AICodeOtelIDPrefix = "ot1:"

// Limits applied to what the daemon forwards.
const (
	// AICodeOtelMaxTextBytes caps large text fields (tool input/output, responses).
	AICodeOtelMaxTextBytes = 64 << 10
	// AICodeOtelMaxAttributeBytes caps each string value in the attributes catch-all.
	AICodeOtelMaxAttributeBytes = 2 << 10
	// AICodeOtelMaxAttributes caps the number of keys in the attributes catch-all.
	AICodeOtelMaxAttributes = 64
	// AICodeOtelMaxRequestBytes bounds the JSON body of one POST /api/v1/cc/otel request.
	AICodeOtelMaxRequestBytes = 8 << 20
)

// AICodeOtelDroppedEvents lists event names (without the claude_code./codex. prefix) that the
// daemon never forwards: opt-in, very large payloads with no use on ShellTime.
var AICodeOtelDroppedEvents = []string{
	"api_request_body",
	"api_response_body",
	"system_prompt",
}

// IsDroppedAICodeOtelEvent reports whether an event name (prefix already stripped) is dropped.
func IsDroppedAICodeOtelEvent(name string) bool {
	for _, dropped := range AICodeOtelDroppedEvents {
		if name == dropped {
			return true
		}
	}
	return false
}

// OTEL source identifiers
const (
	AICodeOtelSourceClaudeCode = "claude-code"
	AICodeOtelSourceCodex      = "codex"
)

// AI Code OTEL metric types (shared between Claude Code and Codex)
const (
	AICodeMetricSessionCount         = "session_count"
	AICodeMetricLinesOfCodeCount     = "lines_of_code_count"
	AICodeMetricPullRequestCount     = "pull_request_count"
	AICodeMetricCommitCount          = "commit_count"
	AICodeMetricCostUsage            = "cost_usage"
	AICodeMetricTokenUsage           = "token_usage"
	AICodeMetricCodeEditToolDecision = "code_edit_tool_decision"
	AICodeMetricActiveTimeTotal      = "active_time_total"
)

// AI Code OTEL event types (shared between Claude Code and Codex)
const (
	AICodeEventUserPrompt         = "user_prompt"
	AICodeEventToolResult         = "tool_result"
	AICodeEventApiRequest         = "api_request"
	AICodeEventApiError           = "api_error"
	AICodeEventToolDecision       = "tool_decision"
	AICodeEventExecCommand        = "exec_command"        // Codex: shell command execution
	AICodeEventConversationStarts = "conversation_starts" // Codex: conversation/session start
	AICodeEventSSEEvent           = "sse_event"           // Codex: SSE streaming event

	AICodeEventAssistantResponse = "assistant_response" // Claude: assistant response text
	AICodeEventAgentResponse     = "agent_response"     // Codex: final agent response text
	AICodeEventTurnCost          = "turn_cost"          // Codex: estimated cost of a turn
)

// Token types for AICodeMetricTokenUsage
const (
	AICodeTokenTypeInput         = "input"
	AICodeTokenTypeOutput        = "output"
	AICodeTokenTypeCacheRead     = "cacheRead"
	AICodeTokenTypeCacheCreation = "cacheCreation"
)

// Lines types for AICodeMetricLinesOfCodeCount
const (
	AICodeLinesTypeAdded   = "added"
	AICodeLinesTypeRemoved = "removed"
)

// IntRef returns a pointer to v.
func IntRef(v int) *int { return &v }

// Int64Ref returns a pointer to v.
func Int64Ref(v int64) *int64 { return &v }

// Float64Ref returns a pointer to v.
func Float64Ref(v float64) *float64 { return &v }

// BoolRef returns a pointer to v.
func BoolRef(v bool) *bool { return &v }

// CapAICodeText truncates s to at most maxBytes without splitting a UTF-8 character.
func CapAICodeText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

package model

import "time"

// Client types as stored by the server. The live OTEL processor sends
// "claude-code", which the server maps to claude_code; backfill sends the
// canonical values directly.
const (
	AICodeClientClaudeCode = "claude_code"
	AICodeClientCodex      = "codex"
)

// Backfill session statuses reported by the server.
const (
	AICodeBackfillStatusBackfilled = "backfilled"
	AICodeBackfillStatusLive       = "live"
	AICodeBackfillStatusArchived   = "archived"
)

// Server limits for one backfill request. The body limit is 16 MiB; batches
// are packed to half of it to leave room for the JSON envelope.
const (
	AICodeBackfillMaxEvents     = 500
	AICodeBackfillMaxBatchBytes = 8 << 20
	AICodeBackfillMaxCompleted  = 50
	AICodeBackfillMaxSessionIDs = 1000
)

// AICodeBackfillEvent is one historical event sent to POST /api/v1/cc/backfill.
// It mirrors the server's CCEventData. Unlike AICodeOtelEvent, flags, counts
// and costs are pointers so that false and zero values survive omitempty
// (a failed tool call must be sent as "success": false).
type AICodeBackfillEvent struct {
	EventID    string `json:"eventId"`
	EventType  string `json:"eventType"`
	ClientType string `json:"clientType"`
	Timestamp  int64  `json:"timestamp"` // unix seconds

	Model string `json:"model,omitempty"`

	Prompt       string `json:"prompt,omitempty"`
	PromptLength *int   `json:"promptLength,omitempty"`

	ToolName       string         `json:"toolName,omitempty"`
	Success        *bool          `json:"success,omitempty"`
	DurationMs     *int           `json:"durationMs,omitempty"`
	Error          string         `json:"error,omitempty"`
	ToolParameters map[string]any `json:"toolParameters,omitempty"`
	ToolArguments  map[string]any `json:"toolArguments,omitempty"`
	CallID         string         `json:"callId,omitempty"`

	CostUSD             *float64 `json:"costUsd,omitempty"`
	InputTokens         *int     `json:"inputTokens,omitempty"`
	OutputTokens        *int     `json:"outputTokens,omitempty"`
	CacheReadTokens     *int     `json:"cacheReadTokens,omitempty"`
	CacheCreationTokens *int     `json:"cacheCreationTokens,omitempty"`
	ReasoningTokens     *int     `json:"reasoningTokens,omitempty"`

	EventKind       string `json:"eventKind,omitempty"`
	Provider        string `json:"provider,omitempty"`
	ApprovalPolicy  string `json:"approvalPolicy,omitempty"`
	SandboxPolicy   string `json:"sandboxPolicy,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`

	SessionID      string `json:"sessionId"`
	ConversationID string `json:"conversationId,omitempty"`
	AppVersion     string `json:"appVersion,omitempty"`
	OSType         string `json:"osType,omitempty"`
	HostArch       string `json:"hostArch,omitempty"`
	Pwd            string `json:"pwd,omitempty"`
	UserName       string `json:"userName,omitempty"`
	MachineName    string `json:"machineName,omitempty"`
	TeamID         string `json:"teamId,omitempty"`
}

// AICodeBackfillRequest is the body of POST /api/v1/cc/backfill.
type AICodeBackfillRequest struct {
	ClientType          string                `json:"clientType"`
	Events              []AICodeBackfillEvent `json:"events"`
	CompletedSessionIDs []string              `json:"completedSessionIds,omitempty"`
	AISummary           bool                  `json:"aiSummary,omitempty"`
}

type AICodeBackfillSessionStatus struct {
	SessionID  string `json:"sessionId"`
	Status     string `json:"status"`
	EventCount int    `json:"eventCount,omitempty"`
}

type AICodeBackfillResponse struct {
	Success         bool                          `json:"success"`
	Accepted        int                           `json:"accepted"`
	Summarized      int                           `json:"summarized"`
	SkippedSessions []AICodeBackfillSessionStatus `json:"skippedSessions"`
}

type AICodeBackfillSessionsRequest struct {
	ClientType string   `json:"clientType"`
	SessionIDs []string `json:"sessionIds"`
}

type AICodeBackfillSessionsResponse struct {
	Sessions []AICodeBackfillSessionStatus `json:"sessions"`
}

type AICodeBackfillCompleteRequest struct {
	ClientType string    `json:"clientType"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
}

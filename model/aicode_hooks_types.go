package model

// AICodeHooksRequest is the main request to POST /api/v1/cc/aicode-hooks
type AICodeHooksRequest struct {
	Host   string                 `json:"host"`
	Events []AICodeHooksEventData `json:"events"`
}

// AICodeHooksEventData represents a single hook event from Claude Code, Codex, or Cursor
type AICodeHooksEventData struct {
	EventID        string         `json:"eventId"`
	ClientType     string         `json:"clientType"`
	HookEventName  string         `json:"hookEventName"`
	Timestamp      int64          `json:"timestamp"`
	SessionID      string         `json:"sessionId,omitempty"`
	Cwd            string         `json:"cwd,omitempty"`
	PermissionMode string         `json:"permissionMode,omitempty"`
	Model          string         `json:"model,omitempty"`
	ToolName       string         `json:"toolName,omitempty"`
	ToolInput      map[string]any `json:"toolInput,omitempty"`
	ToolResponse   map[string]any `json:"toolResponse,omitempty"`
	ToolUseID      string         `json:"toolUseId,omitempty"`
	Prompt         string         `json:"prompt,omitempty"`
	Error          string         `json:"error,omitempty"`
	IsInterrupt    bool           `json:"isInterrupt,omitempty"`
	AgentID        string         `json:"agentId,omitempty"`
	AgentType      string         `json:"agentType,omitempty"`
	LastMessage    string         `json:"lastMessage,omitempty"`
	StopHookActive      bool           `json:"stopHookActive,omitempty"`
	NotificationType    string         `json:"notificationType,omitempty"`
	NotificationMessage string         `json:"notificationMessage,omitempty"`
	SessionEndReason    string         `json:"sessionEndReason,omitempty"`
	TranscriptPath      string         `json:"transcriptPath,omitempty"`
	RawPayload          map[string]any `json:"rawPayload"`
}

// AICodeHooksResponse is the response from POST /api/v1/cc/aicode-hooks
type AICodeHooksResponse struct {
	Success         bool   `json:"success"`
	EventsProcessed int    `json:"eventsProcessed"`
	Message         string `json:"message,omitempty"`
}

// AICode Hooks source identifiers
const (
	AICodeHooksSourceClaudeCode = "claude-code"
	AICodeHooksSourceCodex      = "codex"
	AICodeHooksSourceCursor     = "cursor"
)

// AICode Hooks client type constants (for DB storage)
const (
	AICodeHooksClientClaudeCode = "claude_code"
	AICodeHooksClientCodex      = "codex"
	AICodeHooksClientCursor     = "cursor"
)

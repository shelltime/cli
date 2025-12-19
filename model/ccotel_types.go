package model

// CCOtelRequest is the main request to POST /api/v1/cc/otel
// Flat structure without session - resource attributes are embedded in each metric/event
type CCOtelRequest struct {
	Host    string         `json:"host"`
	Project string         `json:"project"`
	Events  []CCOtelEvent  `json:"events,omitempty"`
	Metrics []CCOtelMetric `json:"metrics,omitempty"`
}

// CCOtelResourceAttributes contains common resource-level attributes
// extracted from OTEL resources and embedded into each metric/event
type CCOtelResourceAttributes struct {
	// Standard resource attributes
	SessionID       string
	UserAccountUUID string
	OrganizationID  string
	TerminalType    string
	AppVersion      string
	ServiceVersion  string
	OSType          string
	OSVersion       string
	HostArch        string
	WSLVersion      string

	// Additional attributes from data points
	UserID    string // from user.id (hashed identifier)
	UserEmail string // from user.email

	// Custom resource attributes (from OTEL_RESOURCE_ATTRIBUTES)
	UserName    string // from user.name
	MachineName string // from machine.name
	TeamID      string // from team.id
}

// CCOtelEvent represents an event from Claude Code (api_request, tool_result, etc.)
// with embedded resource attributes for a flat, session-less structure
type CCOtelEvent struct {
	EventID             string                 `json:"eventId"`
	EventType           string                 `json:"eventType"`
	Timestamp           int64                  `json:"timestamp"`
	EventTimestamp      string                 `json:"eventTimestamp,omitempty"` // ISO 8601 timestamp
	Model               string                 `json:"model,omitempty"`
	CostUSD             float64                `json:"costUsd,omitempty"`
	DurationMs          int                    `json:"durationMs,omitempty"`
	InputTokens         int                    `json:"inputTokens,omitempty"`
	OutputTokens        int                    `json:"outputTokens,omitempty"`
	CacheReadTokens     int                    `json:"cacheReadTokens,omitempty"`
	CacheCreationTokens int                    `json:"cacheCreationTokens,omitempty"`
	ToolName            string                 `json:"toolName,omitempty"`
	Success             bool                   `json:"success,omitempty"`
	Decision            string                 `json:"decision,omitempty"`
	Source              string                 `json:"source,omitempty"`
	Error               string                 `json:"error,omitempty"`
	PromptLength        int                    `json:"promptLength,omitempty"`
	Prompt              string                 `json:"prompt,omitempty"`
	ToolParameters      map[string]interface{} `json:"toolParameters,omitempty"`
	StatusCode          int                    `json:"statusCode,omitempty"`
	Attempt             int                    `json:"attempt,omitempty"`
	Language            string                 `json:"language,omitempty"`

	// Embedded resource attributes (previously in session)
	SessionID       string `json:"sessionId,omitempty"`
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
}

// CCOtelMetric represents a metric data point from Claude Code
// with embedded resource attributes for a flat, session-less structure
type CCOtelMetric struct {
	MetricID   string  `json:"metricId"`
	MetricType string  `json:"metricType"`
	Timestamp  int64   `json:"timestamp"`
	Value      float64 `json:"value"`
	Model      string  `json:"model,omitempty"`
	TokenType  string  `json:"tokenType,omitempty"`
	LinesType  string  `json:"linesType,omitempty"`
	Tool       string  `json:"tool,omitempty"`
	Decision   string  `json:"decision,omitempty"`
	Language   string  `json:"language,omitempty"`

	// Embedded resource attributes (previously in session)
	SessionID       string `json:"sessionId,omitempty"`
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
}

// CCOtelResponse is the response from POST /api/v1/cc/otel
type CCOtelResponse struct {
	Success          bool   `json:"success"`
	EventsProcessed  int    `json:"eventsProcessed"`
	MetricsProcessed int    `json:"metricsProcessed"`
	Message          string `json:"message,omitempty"`
}

// Claude Code OTEL metric types
const (
	CCMetricSessionCount         = "session_count"
	CCMetricLinesOfCodeCount     = "lines_of_code_count"
	CCMetricPullRequestCount     = "pull_request_count"
	CCMetricCommitCount          = "commit_count"
	CCMetricCostUsage            = "cost_usage"
	CCMetricTokenUsage           = "token_usage"
	CCMetricCodeEditToolDecision = "code_edit_tool_decision"
	CCMetricActiveTimeTotal      = "active_time_total"
)

// Claude Code OTEL event types
const (
	CCEventUserPrompt   = "user_prompt"
	CCEventToolResult   = "tool_result"
	CCEventApiRequest   = "api_request"
	CCEventApiError     = "api_error"
	CCEventToolDecision = "tool_decision"
)

// Token types for CCMetricTokenUsage
const (
	CCTokenTypeInput         = "input"
	CCTokenTypeOutput        = "output"
	CCTokenTypeCacheRead     = "cacheRead"
	CCTokenTypeCacheCreation = "cacheCreation"
)

// Lines types for CCMetricLinesOfCodeCount
const (
	CCLinesTypeAdded   = "added"
	CCLinesTypeRemoved = "removed"
)

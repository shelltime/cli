package model

// CCOtelRequest is the main request to POST /api/v1/cc/otel
type CCOtelRequest struct {
	Host    string         `json:"host"`
	Project string         `json:"project"`
	Session *CCOtelSession `json:"session"`
	Events  []CCOtelEvent  `json:"events,omitempty"`
	Metrics []CCOtelMetric `json:"metrics,omitempty"`
}

// CCOtelSession represents session data for Claude Code OTEL tracking
type CCOtelSession struct {
	SessionID                string  `json:"sessionId"`
	AppVersion               string  `json:"appVersion"`
	OrganizationID           string  `json:"organizationId,omitempty"`
	UserAccountUUID          string  `json:"userAccountUuid,omitempty"`
	TerminalType             string  `json:"terminalType"`
	ServiceVersion           string  `json:"serviceVersion"`
	OSType                   string  `json:"osType"`
	OSVersion                string  `json:"osVersion"`
	HostArch                 string  `json:"hostArch"`
	WSLVersion               string  `json:"wslVersion,omitempty"`
	StartedAt                int64   `json:"startedAt"`
	EndedAt                  int64   `json:"endedAt,omitempty"`
	ActiveTimeSeconds        int     `json:"activeTimeSeconds,omitempty"`
	TotalPrompts             int     `json:"totalPrompts,omitempty"`
	TotalToolCalls           int     `json:"totalToolCalls,omitempty"`
	TotalApiRequests         int     `json:"totalApiRequests,omitempty"`
	TotalCostUSD             float64 `json:"totalCostUsd,omitempty"`
	LinesAdded               int     `json:"linesAdded,omitempty"`
	LinesRemoved             int     `json:"linesRemoved,omitempty"`
	CommitsCreated           int     `json:"commitsCreated,omitempty"`
	PRsCreated               int     `json:"prsCreated,omitempty"`
	TotalInputTokens         int64   `json:"totalInputTokens,omitempty"`
	TotalOutputTokens        int64   `json:"totalOutputTokens,omitempty"`
	TotalCacheReadTokens     int64   `json:"totalCacheReadTokens,omitempty"`
	TotalCacheCreationTokens int64   `json:"totalCacheCreationTokens,omitempty"`
}

// CCOtelEvent represents an event from Claude Code (api_request, tool_result, etc.)
type CCOtelEvent struct {
	EventID             string                 `json:"eventId"`
	EventType           string                 `json:"eventType"`
	Timestamp           int64                  `json:"timestamp"`
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
}

// CCOtelMetric represents a metric data point from Claude Code
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
}

// CCOtelResponse is the response from POST /api/v1/cc/otel
type CCOtelResponse struct {
	Success          bool   `json:"success"`
	SessionID        int64  `json:"sessionId,omitempty"`
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

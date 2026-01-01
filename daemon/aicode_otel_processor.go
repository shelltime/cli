package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/malamtime/cli/model"
	collogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collmetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

// AICodeOtelProcessor handles OTEL data parsing and forwarding to the backend
type AICodeOtelProcessor struct {
	config   model.ShellTimeConfig
	endpoint model.Endpoint
	hostname string
	debug    bool
}

// NewAICodeOtelProcessor creates a new AICodeOtel processor
func NewAICodeOtelProcessor(config model.ShellTimeConfig) *AICodeOtelProcessor {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	debug := config.AICodeOtel != nil && config.AICodeOtel.Debug != nil && *config.AICodeOtel.Debug

	return &AICodeOtelProcessor{
		config: config,
		endpoint: model.Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		hostname: hostname,
		debug:    debug,
	}
}

// writeDebugFile appends JSON-formatted data to a debug file
func (p *AICodeOtelProcessor) writeDebugFile(filename string, data interface{}) {
	debugDir := filepath.Join(os.TempDir(), "shelltime")
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		slog.Error("AICodeOtel: Failed to create debug directory", "error", err)
		return
	}

	filePath := filepath.Join(debugDir, filename)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("AICodeOtel: Failed to open debug file", "error", err, "path", filePath)
		return
	}
	defer f.Close()

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		slog.Error("AICodeOtel: Failed to marshal debug data", "error", err)
		return
	}

	timestamp := time.Now().Format(time.RFC3339)
	if _, err := f.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", timestamp, jsonData)); err != nil {
		slog.Error("AICodeOtel: Failed to write debug data", "error", err)
	}
	slog.Debug("AICodeOtel: Wrote debug data", "path", filePath)
}

// ProcessMetrics receives OTEL metrics and forwards to backend immediately
func (p *AICodeOtelProcessor) ProcessMetrics(ctx context.Context, req *collmetricsv1.ExportMetricsServiceRequest) (*collmetricsv1.ExportMetricsServiceResponse, error) {
	slog.Debug("AICodeOtel: Processing metrics request", "resourceMetricsCount", len(req.GetResourceMetrics()))

	if p.debug {
		p.writeDebugFile("aicode-otel-debug-metrics.txt", req)
	}

	for _, rm := range req.GetResourceMetrics() {
		resource := rm.GetResource()

		// Check if this is from Claude Code or Codex
		source := detectOtelSource(resource)
		if source == "" {
			slog.Debug("AICodeOtel: Skipping unknown resource")
			continue
		}

		// Extract resource attributes once for all metrics in this resource
		resourceAttrs := extractResourceAttributes(resource)
		project := p.detectProject(resource, source)

		var metrics []model.AICodeOtelMetric

		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				parsedMetrics := p.parseMetric(m, resourceAttrs, source)
				metrics = append(metrics, parsedMetrics...)
			}
		}

		if len(metrics) == 0 {
			continue
		}

		// Build and send request immediately - flat structure without session
		aiCodeReq := &model.AICodeOtelRequest{
			Host:    p.hostname,
			Project: project,
			Source:  source,
			Metrics: metrics,
		}

		resp, err := model.SendAICodeOtelData(ctx, aiCodeReq, p.endpoint)
		if err != nil {
			slog.Error("AICodeOtel: Failed to send metrics to backend", "error", err)
			// Continue processing - passthrough mode, we don't retry
		} else {
			slog.Debug("AICodeOtel: Metrics sent to backend", "metricsProcessed", resp.MetricsProcessed)
		}
	}

	return &collmetricsv1.ExportMetricsServiceResponse{}, nil
}

// ProcessLogs receives OTEL logs/events and forwards to backend immediately
func (p *AICodeOtelProcessor) ProcessLogs(ctx context.Context, req *collogsv1.ExportLogsServiceRequest) (*collogsv1.ExportLogsServiceResponse, error) {
	slog.Debug("AICodeOtel: Processing logs request", "resourceLogsCount", len(req.GetResourceLogs()), slog.Bool("debug", p.debug))

	if p.debug {
		p.writeDebugFile("aicode-otel-debug-logs.txt", req)
	}

	for _, rl := range req.GetResourceLogs() {
		resource := rl.GetResource()

		// Check if this is from Claude Code or Codex
		source := detectOtelSource(resource)
		if source == "" {
			slog.Debug("AICodeOtel: Skipping unknown resource")
			continue
		}

		// Extract resource attributes once for all events in this resource
		resourceAttrs := extractResourceAttributes(resource)
		project := p.detectProject(resource, source)

		var events []model.AICodeOtelEvent

		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				event := p.parseLogRecord(lr, resourceAttrs, source)
				if event != nil {
					events = append(events, *event)
				}
			}
		}

		if len(events) == 0 {
			continue
		}

		// Build and send request immediately - flat structure without session
		aiCodeReq := &model.AICodeOtelRequest{
			Host:    p.hostname,
			Project: project,
			Source:  source,
			Events:  events,
		}

		resp, err := model.SendAICodeOtelData(ctx, aiCodeReq, p.endpoint)
		if err != nil {
			slog.Error("AICodeOtel: Failed to send events to backend", "error", err)
			// Continue processing - passthrough mode, we don't retry
		} else {
			slog.Debug("AICodeOtel: Events sent to backend", "eventsProcessed", resp.EventsProcessed)
		}
	}

	return &collogsv1.ExportLogsServiceResponse{}, nil
}

// detectOtelSource checks the resource and returns the source type (claude-code, codex, or empty if unknown)
func detectOtelSource(resource *resourcev1.Resource) string {
	if resource == nil {
		return ""
	}

	for _, attr := range resource.GetAttributes() {
		if attr.GetKey() == "service.name" {
			serviceName := attr.GetValue().GetStringValue()
			if strings.Contains(serviceName, "claude") {
				return model.AICodeOtelSourceClaudeCode
			}
			if strings.Contains(serviceName, "codex") {
				return model.AICodeOtelSourceCodex
			}
		}
	}
	return ""
}

// extractResourceAttributes extracts resource-level attributes from OTEL resource
// Returns a struct that can be used to populate metrics and events
func extractResourceAttributes(resource *resourcev1.Resource) *model.AICodeOtelResourceAttributes {
	attrs := &model.AICodeOtelResourceAttributes{}

	if resource == nil {
		return attrs
	}

	for _, attr := range resource.GetAttributes() {
		key := attr.GetKey()
		value := attr.GetValue()

		switch key {
		// Standard resource attributes
		case "session.id":
			attrs.SessionID = value.GetStringValue()
		case "conversation.id":
			attrs.ConversationID = value.GetStringValue()
		case "app.version":
			attrs.AppVersion = value.GetStringValue()
		case "organization.id":
			attrs.OrganizationID = value.GetStringValue()
		case "user.account_uuid", "user.account_id":
			attrs.UserAccountUUID = value.GetStringValue()
		case "terminal.type":
			attrs.TerminalType = value.GetStringValue()
		case "service.version":
			attrs.ServiceVersion = value.GetStringValue()
		case "os.type":
			attrs.OSType = value.GetStringValue()
		case "os.version":
			attrs.OSVersion = value.GetStringValue()
		case "host.arch":
			attrs.HostArch = value.GetStringValue()
		case "wsl.version":
			attrs.WSLVersion = value.GetStringValue()
		// Additional identifiers
		case "user.id":
			attrs.UserID = value.GetStringValue()
		case "user.email":
			attrs.UserEmail = value.GetStringValue()
		// Custom resource attributes (from OTEL_RESOURCE_ATTRIBUTES)
		case "user.name":
			attrs.UserName = value.GetStringValue()
		case "machine.name":
			attrs.MachineName = value.GetStringValue()
		case "team.id":
			attrs.TeamID = value.GetStringValue()
		case "pwd":
			attrs.Pwd = value.GetStringValue()
		}
	}

	return attrs
}

// applyResourceAttributesToMetric copies resource attributes into a metric
func applyResourceAttributesToMetric(metric *model.AICodeOtelMetric, attrs *model.AICodeOtelResourceAttributes) {
	// Standard resource attributes
	metric.SessionID = attrs.SessionID
	metric.ConversationID = attrs.ConversationID
	metric.UserAccountUUID = attrs.UserAccountUUID
	metric.OrganizationID = attrs.OrganizationID
	metric.TerminalType = attrs.TerminalType
	metric.AppVersion = attrs.AppVersion
	metric.OSType = attrs.OSType
	metric.OSVersion = attrs.OSVersion
	metric.HostArch = attrs.HostArch

	// Additional identifiers
	metric.UserID = attrs.UserID
	metric.UserEmail = attrs.UserEmail

	// Custom resource attributes
	metric.UserName = attrs.UserName
	metric.MachineName = attrs.MachineName
	metric.TeamID = attrs.TeamID
	metric.Pwd = attrs.Pwd
}

// applyResourceAttributesToEvent copies resource attributes into an event
func applyResourceAttributesToEvent(event *model.AICodeOtelEvent, attrs *model.AICodeOtelResourceAttributes) {
	// Standard resource attributes
	event.SessionID = attrs.SessionID
	event.ConversationID = attrs.ConversationID
	event.UserAccountUUID = attrs.UserAccountUUID
	event.OrganizationID = attrs.OrganizationID
	event.TerminalType = attrs.TerminalType
	event.AppVersion = attrs.AppVersion
	event.OSType = attrs.OSType
	event.OSVersion = attrs.OSVersion
	event.HostArch = attrs.HostArch

	// Additional identifiers
	event.UserID = attrs.UserID
	event.UserEmail = attrs.UserEmail

	// Custom resource attributes
	event.UserName = attrs.UserName
	event.MachineName = attrs.MachineName
	event.TeamID = attrs.TeamID
	event.Pwd = attrs.Pwd
}

// detectProject extracts project from resource attributes or environment
func (p *AICodeOtelProcessor) detectProject(resource *resourcev1.Resource, source string) string {
	// First check resource attributes
	if resource != nil {
		for _, attr := range resource.GetAttributes() {
			if attr.GetKey() == "project" || attr.GetKey() == "project.path" {
				return attr.GetValue().GetStringValue()
			}
		}
	}

	// Fall back to environment variables based on source
	if source == model.AICodeOtelSourceClaudeCode {
		if project := os.Getenv("CLAUDE_CODE_PROJECT"); project != "" {
			return project
		}
	} else if source == model.AICodeOtelSourceCodex {
		if project := os.Getenv("CODEX_PROJECT"); project != "" {
			return project
		}
	}

	if pwd := os.Getenv("PWD"); pwd != "" {
		return pwd
	}

	return "unknown"
}

// parseMetric parses an OTEL metric into AICodeOtelMetric(s)
func (p *AICodeOtelProcessor) parseMetric(m *metricsv1.Metric, resourceAttrs *model.AICodeOtelResourceAttributes, source string) []model.AICodeOtelMetric {
	var metrics []model.AICodeOtelMetric

	name := m.GetName()
	metricType := mapMetricName(name, source)
	if metricType == "" {
		return metrics // Unknown metric, skip
	}

	// Handle different metric data types
	switch data := m.GetData().(type) {
	case *metricsv1.Metric_Sum:
		for _, dp := range data.Sum.GetDataPoints() {
			metric := model.AICodeOtelMetric{
				MetricID:   uuid.New().String(),
				MetricType: metricType,
				Timestamp:  int64(dp.GetTimeUnixNano() / 1e9), // Convert to seconds
				Value:      getDataPointValue(dp),
				ClientType: source,
			}
			// Apply resource attributes first
			applyResourceAttributesToMetric(&metric, resourceAttrs)
			// Then extract data point attributes (can override resource attrs)
			for _, attr := range dp.GetAttributes() {
				applyMetricAttribute(&metric, attr, metricType)
			}
			metrics = append(metrics, metric)
		}
	case *metricsv1.Metric_Gauge:
		for _, dp := range data.Gauge.GetDataPoints() {
			metric := model.AICodeOtelMetric{
				MetricID:   uuid.New().String(),
				MetricType: metricType,
				Timestamp:  int64(dp.GetTimeUnixNano() / 1e9),
				Value:      getDataPointValue(dp),
				ClientType: source,
			}
			// Apply resource attributes first
			applyResourceAttributesToMetric(&metric, resourceAttrs)
			// Then extract data point attributes (can override resource attrs)
			for _, attr := range dp.GetAttributes() {
				applyMetricAttribute(&metric, attr, metricType)
			}
			metrics = append(metrics, metric)
		}
	}

	return metrics
}

// parseLogRecord parses an OTEL log record into a AICodeOtelEvent
func (p *AICodeOtelProcessor) parseLogRecord(lr *logsv1.LogRecord, resourceAttrs *model.AICodeOtelResourceAttributes, source string) *model.AICodeOtelEvent {
	event := &model.AICodeOtelEvent{
		EventID:   uuid.New().String(),
		Timestamp: int64(lr.GetTimeUnixNano() / 1e9), // Convert to seconds

		ClientType: source,
	}

	if event.Timestamp == 0 {
		event.Timestamp = int64(lr.GetObservedTimeUnixNano() / 1e9) // Convert to seconds
	}

	// Apply resource attributes first
	applyResourceAttributesToEvent(event, resourceAttrs)

	// Extract event type and other attributes from log record
	for _, attr := range lr.GetAttributes() {
		key := attr.GetKey()
		value := attr.GetValue()

		switch key {
		case "event.name":
			event.EventType = mapEventName(value.GetStringValue(), source)
		case "event.timestamp":
			event.EventTimestamp = value.GetStringValue()
		case "model":
			event.Model = value.GetStringValue()
		case "cost_usd":
			event.CostUSD = getFloatFromValue(value)
		case "duration_ms":
			event.DurationMs = getIntFromValue(value)
		case "input_tokens":
			event.InputTokens = getIntFromValue(value)
		case "output_tokens":
			event.OutputTokens = getIntFromValue(value)
		case "cache_read_tokens":
			event.CacheReadTokens = getIntFromValue(value)
		case "cache_creation_tokens":
			event.CacheCreationTokens = getIntFromValue(value)
		case "tool_name":
			event.ToolName = value.GetStringValue()
		case "success":
			event.Success = getBoolFromValue(value)
		case "decision":
			event.Decision = value.GetStringValue()
		case "source":
			event.Source = value.GetStringValue()
		case "error":
			event.Error = value.GetStringValue()
		case "prompt_length":
			event.PromptLength = getIntFromValue(value)
		case "prompt":
			event.Prompt = value.GetStringValue()
		case "tool_parameters":
			// tool_parameters comes as a JSON string, parse it into map
			if jsonStr := value.GetStringValue(); jsonStr != "" {
				var params map[string]interface{}
				if err := json.Unmarshal([]byte(jsonStr), &params); err == nil {
					event.ToolParameters = params
				} else {
					slog.Debug("AICodeOtel: Failed to parse tool_parameters", "error", err)
				}
			}
		case "status_code", "http.response.status_code":
			event.StatusCode = getIntFromValue(value)
		case "attempt":
			event.Attempt = getIntFromValue(value)
		case "error.message":
			event.Error = value.GetStringValue()
		case "language":
			event.Language = value.GetStringValue()
		// Codex-specific fields
		case "reasoning_tokens":
			event.ReasoningTokens = getIntFromValue(value)
		case "provider":
			event.Provider = value.GetStringValue()
		// Codex-specific fields for tool_decision
		case "call_id", "callId":
			event.CallID = value.GetStringValue()
		// Codex-specific fields for sse_event
		case "event_kind", "eventKind":
			event.EventKind = value.GetStringValue()
		case "tool_tokens", "toolTokens":
			event.ToolTokens = getIntFromValue(value)
		// Codex-specific fields for conversation_starts
		case "auth_mode", "authMode":
			event.AuthMode = value.GetStringValue()
		case "slug":
			event.Slug = value.GetStringValue()
		case "context_window", "contextWindow":
			event.ContextWindow = getIntFromValue(value)
		case "approval_policy", "approvalPolicy":
			event.ApprovalPolicy = value.GetStringValue()
		case "sandbox_policy", "sandboxPolicy":
			event.SandboxPolicy = value.GetStringValue()
		case "mcp_servers", "mcpServers":
			event.MCPServers = getStringArrayFromValue(value)
		case "profile":
			event.Profile = value.GetStringValue()
		case "reasoning_enabled", "reasoningEnabled":
			event.ReasoningEnabled = getBoolFromValue(value)
		// Codex-specific fields for tool_result
		case "tool_arguments", "toolArguments":
			if jsonStr := value.GetStringValue(); jsonStr != "" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(jsonStr), &args); err == nil {
					event.ToolArguments = args
				} else {
					slog.Debug("AICodeOtel: Failed to parse tool_arguments", "error", err)
				}
			}
		case "tool_output", "toolOutput":
			event.ToolOutput = value.GetStringValue()
		case "prompt_encrypted", "promptEncrypted":
			event.PromptEncrypted = getBoolFromValue(value)
		// Codex uses conversation.id instead of session.id
		case "conversation.id", "conversationId":
			event.ConversationID = value.GetStringValue()
		// Log record level attributes that override resource attrs
		case "user.id":
			event.UserID = value.GetStringValue()
		case "user.email":
			event.UserEmail = value.GetStringValue()
		case "session.id":
			event.SessionID = value.GetStringValue()
		case "app.version":
			event.AppVersion = value.GetStringValue()
		case "organization.id":
			event.OrganizationID = value.GetStringValue()
		case "user.account_uuid", "user.account_id":
			event.UserAccountUUID = value.GetStringValue()
		case "terminal.type":
			event.TerminalType = value.GetStringValue()
		}
	}

	// Skip if no event type was extracted
	if event.EventType == "" {
		return nil
	}

	return event
}

// mapMetricName maps OTEL metric names to our internal types
// Supports both Claude Code (claude_code.*) and Codex (codex.*) prefixes
func mapMetricName(name string, source string) string {
	switch name {
	// Claude Code metrics
	case "claude_code.session.count":
		return model.AICodeMetricSessionCount
	case "claude_code.token.usage":
		return model.AICodeMetricTokenUsage
	case "claude_code.cost.usage":
		return model.AICodeMetricCostUsage
	case "claude_code.lines_of_code.count":
		return model.AICodeMetricLinesOfCodeCount
	case "claude_code.commit.count":
		return model.AICodeMetricCommitCount
	case "claude_code.pull_request.count":
		return model.AICodeMetricPullRequestCount
	case "claude_code.active_time.total":
		return model.AICodeMetricActiveTimeTotal
	case "claude_code.code_edit_tool.decision":
		return model.AICodeMetricCodeEditToolDecision
	// Codex metrics (same internal types, different prefix)
	case "codex.session.count":
		return model.AICodeMetricSessionCount
	case "codex.token.usage":
		return model.AICodeMetricTokenUsage
	case "codex.cost.usage":
		return model.AICodeMetricCostUsage
	case "codex.lines_of_code.count":
		return model.AICodeMetricLinesOfCodeCount
	case "codex.commit.count":
		return model.AICodeMetricCommitCount
	case "codex.pull_request.count":
		return model.AICodeMetricPullRequestCount
	case "codex.active_time.total":
		return model.AICodeMetricActiveTimeTotal
	default:
		return ""
	}
}

// mapEventName maps OTEL event names to our internal types
// Supports both Claude Code (claude_code.*) and Codex (codex.*) prefixes
func mapEventName(name string, source string) string {
	switch name {
	// Claude Code events
	case "claude_code.user_prompt":
		return model.AICodeEventUserPrompt
	case "claude_code.tool_result":
		return model.AICodeEventToolResult
	case "claude_code.api_request":
		return model.AICodeEventApiRequest
	case "claude_code.api_error":
		return model.AICodeEventApiError
	case "claude_code.tool_decision":
		return model.AICodeEventToolDecision
	// Codex events (same internal types, different prefix)
	case "codex.user_prompt":
		return model.AICodeEventUserPrompt
	case "codex.tool_result":
		return model.AICodeEventToolResult
	case "codex.api_request":
		return model.AICodeEventApiRequest
	case "codex.api_error":
		return model.AICodeEventApiError
	case "codex.exec_command":
		return model.AICodeEventExecCommand
	case "codex.conversation_starts":
		return model.AICodeEventConversationStarts
	case "codex.sse_event":
		return model.AICodeEventSSEEvent
	default:
		return name // Return as-is if not in our map
	}
}

// getDataPointValue extracts the numeric value from a data point
func getDataPointValue(dp *metricsv1.NumberDataPoint) float64 {
	switch v := dp.GetValue().(type) {
	case *metricsv1.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricsv1.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	default:
		return 0
	}
}

// getIntFromValue extracts an int from an OTEL value, handling both int and string formats
func getIntFromValue(value *commonv1.AnyValue) int {
	// First try to get as int
	if intVal := value.GetIntValue(); intVal != 0 {
		return int(intVal)
	}
	// Try to parse from string (Claude Code sends some values as strings)
	if strVal := value.GetStringValue(); strVal != "" {
		if parsed, err := strconv.Atoi(strVal); err == nil {
			return parsed
		}
	}
	return 0
}

func getBoolFromValue(value *commonv1.AnyValue) bool {
	// First try to get as bool
	if boolVal := value.GetBoolValue(); boolVal {
		return boolVal
	}
	// Try to parse from string (Claude Code sends some values as strings)
	if strVal := value.GetStringValue(); strVal != "" {
		if parsed, err := strconv.ParseBool(strVal); err == nil {
			return parsed
		}
	}
	return false
}

// getFloatFromValue extracts a float64 from an OTEL value, handling both double and string formats
func getFloatFromValue(value *commonv1.AnyValue) float64 {
	// First try to get as double
	if doubleVal := value.GetDoubleValue(); doubleVal != 0 {
		return doubleVal
	}
	// Try to parse from string (Claude Code sends some values as strings)
	if strVal := value.GetStringValue(); strVal != "" {
		if parsed, err := strconv.ParseFloat(strVal, 64); err == nil {
			return parsed
		}
	}
	return 0
}

// getStringArrayFromValue extracts a string array from an OTEL value
func getStringArrayFromValue(value *commonv1.AnyValue) []string {
	if arr := value.GetArrayValue(); arr != nil {
		var result []string
		for _, v := range arr.GetValues() {
			if s := v.GetStringValue(); s != "" {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// applyMetricAttribute applies an attribute to a metric
func applyMetricAttribute(metric *model.AICodeOtelMetric, attr *commonv1.KeyValue, metricType string) {
	key := attr.GetKey()
	value := attr.GetValue()

	switch key {
	case "type":
		if metricType == model.AICodeMetricLinesOfCodeCount {
			metric.LinesType = value.GetStringValue()
		} else {
			metric.TokenType = value.GetStringValue()
		}
	case "model":
		metric.Model = value.GetStringValue()
	case "tool":
		metric.Tool = value.GetStringValue()
	case "decision":
		metric.Decision = value.GetStringValue()
	case "language":
		metric.Language = value.GetStringValue()
	// Resource attributes at data point level - apply them (override if already set from resource)
	case "session.id":
		metric.SessionID = value.GetStringValue()
	case "user.account_uuid":
		metric.UserAccountUUID = value.GetStringValue()
	case "organization.id":
		metric.OrganizationID = value.GetStringValue()
	case "terminal.type":
		metric.TerminalType = value.GetStringValue()
	case "app.version":
		metric.AppVersion = value.GetStringValue()
	case "os.type":
		metric.OSType = value.GetStringValue()
	case "os.version":
		metric.OSVersion = value.GetStringValue()
	case "host.arch":
		metric.HostArch = value.GetStringValue()
	// Additional identifiers at data point level
	case "user.id":
		metric.UserID = value.GetStringValue()
	case "user.email":
		metric.UserEmail = value.GetStringValue()
	}
}

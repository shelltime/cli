package daemon

import (
	"context"
	"log/slog"
	"os"
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

// CCOtelProcessor handles OTEL data parsing and forwarding to the backend
type CCOtelProcessor struct {
	config   model.ShellTimeConfig
	endpoint model.Endpoint
	hostname string
}

// NewCCOtelProcessor creates a new CCOtel processor
func NewCCOtelProcessor(config model.ShellTimeConfig) *CCOtelProcessor {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	return &CCOtelProcessor{
		config: config,
		endpoint: model.Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		},
		hostname: hostname,
	}
}

// ProcessMetrics receives OTEL metrics and forwards to backend immediately
func (p *CCOtelProcessor) ProcessMetrics(ctx context.Context, req *collmetricsv1.ExportMetricsServiceRequest) (*collmetricsv1.ExportMetricsServiceResponse, error) {
	slog.Debug("CCOtel: Processing metrics request", "resourceMetricsCount", len(req.GetResourceMetrics()))

	for _, rm := range req.GetResourceMetrics() {
		resource := rm.GetResource()

		// Check if this is from Claude Code
		if !isClaudeCodeResource(resource) {
			slog.Debug("CCOtel: Skipping non-Claude Code resource")
			continue
		}

		session := extractSessionFromResource(resource)
		project := p.detectProject(resource)

		var metrics []model.CCOtelMetric

		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				parsedMetrics := p.parseMetric(m)
				metrics = append(metrics, parsedMetrics...)
			}
		}

		if len(metrics) == 0 {
			continue
		}

		// Build and send request immediately
		ccReq := &model.CCOtelRequest{
			Host:    p.hostname,
			Project: project,
			Session: session,
			Metrics: metrics,
		}

		resp, err := model.SendCCOtelData(ctx, ccReq, p.endpoint)
		if err != nil {
			slog.Error("CCOtel: Failed to send metrics to backend", "error", err)
			// Continue processing - passthrough mode, we don't retry
		} else {
			slog.Debug("CCOtel: Metrics sent to backend", "metricsProcessed", resp.MetricsProcessed)
		}
	}

	return &collmetricsv1.ExportMetricsServiceResponse{}, nil
}

// ProcessLogs receives OTEL logs/events and forwards to backend immediately
func (p *CCOtelProcessor) ProcessLogs(ctx context.Context, req *collogsv1.ExportLogsServiceRequest) (*collogsv1.ExportLogsServiceResponse, error) {
	slog.Debug("CCOtel: Processing logs request", "resourceLogsCount", len(req.GetResourceLogs()))

	for _, rl := range req.GetResourceLogs() {
		resource := rl.GetResource()

		// Check if this is from Claude Code
		if !isClaudeCodeResource(resource) {
			slog.Debug("CCOtel: Skipping non-Claude Code resource")
			continue
		}

		session := extractSessionFromResource(resource)
		project := p.detectProject(resource)

		var events []model.CCOtelEvent

		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				event := p.parseLogRecord(lr)
				if event != nil {
					events = append(events, *event)
				}
			}
		}

		if len(events) == 0 {
			continue
		}

		// Build and send request immediately
		ccReq := &model.CCOtelRequest{
			Host:    p.hostname,
			Project: project,
			Session: session,
			Events:  events,
		}

		resp, err := model.SendCCOtelData(ctx, ccReq, p.endpoint)
		if err != nil {
			slog.Error("CCOtel: Failed to send events to backend", "error", err)
			// Continue processing - passthrough mode, we don't retry
		} else {
			slog.Debug("CCOtel: Events sent to backend", "eventsProcessed", resp.EventsProcessed)
		}
	}

	return &collogsv1.ExportLogsServiceResponse{}, nil
}

// isClaudeCodeResource checks if the resource is from Claude Code
func isClaudeCodeResource(resource *resourcev1.Resource) bool {
	if resource == nil {
		return false
	}

	for _, attr := range resource.GetAttributes() {
		if attr.GetKey() == "service.name" {
			return attr.GetValue().GetStringValue() == "claude-code"
		}
	}
	return false
}

// extractSessionFromResource extracts session info from resource attributes
func extractSessionFromResource(resource *resourcev1.Resource) *model.CCOtelSession {
	session := &model.CCOtelSession{
		StartedAt: time.Now().Unix(),
	}

	if resource == nil {
		return session
	}

	for _, attr := range resource.GetAttributes() {
		key := attr.GetKey()
		value := attr.GetValue()

		switch key {
		case "session.id":
			session.SessionID = value.GetStringValue()
		case "app.version":
			session.AppVersion = value.GetStringValue()
		case "organization.id":
			session.OrganizationID = value.GetStringValue()
		case "user.account_uuid":
			session.UserAccountUUID = value.GetStringValue()
		case "terminal.type":
			session.TerminalType = value.GetStringValue()
		case "service.version":
			session.ServiceVersion = value.GetStringValue()
		case "os.type":
			session.OSType = value.GetStringValue()
		case "os.version":
			session.OSVersion = value.GetStringValue()
		case "host.arch":
			session.HostArch = value.GetStringValue()
		}
	}

	// Generate session ID if not present
	if session.SessionID == "" {
		session.SessionID = uuid.New().String()
	}

	return session
}

// detectProject extracts project from resource attributes or environment
func (p *CCOtelProcessor) detectProject(resource *resourcev1.Resource) string {
	// First check resource attributes
	if resource != nil {
		for _, attr := range resource.GetAttributes() {
			if attr.GetKey() == "project" || attr.GetKey() == "project.path" {
				return attr.GetValue().GetStringValue()
			}
		}
	}

	// Fall back to environment variables
	if project := os.Getenv("CLAUDE_CODE_PROJECT"); project != "" {
		return project
	}
	if pwd := os.Getenv("PWD"); pwd != "" {
		return pwd
	}

	return "unknown"
}

// parseMetric parses an OTEL metric into CCOtelMetric(s)
func (p *CCOtelProcessor) parseMetric(m *metricsv1.Metric) []model.CCOtelMetric {
	var metrics []model.CCOtelMetric

	name := m.GetName()
	metricType := mapMetricName(name)
	if metricType == "" {
		return metrics // Unknown metric, skip
	}

	// Handle different metric data types
	switch data := m.GetData().(type) {
	case *metricsv1.Metric_Sum:
		for _, dp := range data.Sum.GetDataPoints() {
			metric := model.CCOtelMetric{
				MetricID:   uuid.New().String(),
				MetricType: metricType,
				Timestamp:  int64(dp.GetTimeUnixNano() / 1e9), // Convert to seconds
				Value:      getDataPointValue(dp),
			}
			// Extract attributes
			for _, attr := range dp.GetAttributes() {
				applyMetricAttribute(&metric, attr)
			}
			metrics = append(metrics, metric)
		}
	case *metricsv1.Metric_Gauge:
		for _, dp := range data.Gauge.GetDataPoints() {
			metric := model.CCOtelMetric{
				MetricID:   uuid.New().String(),
				MetricType: metricType,
				Timestamp:  int64(dp.GetTimeUnixNano() / 1e9),
				Value:      getDataPointValue(dp),
			}
			for _, attr := range dp.GetAttributes() {
				applyMetricAttribute(&metric, attr)
			}
			metrics = append(metrics, metric)
		}
	}

	return metrics
}

// parseLogRecord parses an OTEL log record into a CCOtelEvent
func (p *CCOtelProcessor) parseLogRecord(lr *logsv1.LogRecord) *model.CCOtelEvent {
	event := &model.CCOtelEvent{
		EventID:   uuid.New().String(),
		Timestamp: int64(lr.GetTimeUnixNano() / 1e9), // Convert to seconds
	}

	// Extract event type and other attributes
	for _, attr := range lr.GetAttributes() {
		key := attr.GetKey()
		value := attr.GetValue()

		switch key {
		case "event.name":
			event.EventType = mapEventName(value.GetStringValue())
		case "model":
			event.Model = value.GetStringValue()
		case "cost_usd":
			event.CostUSD = value.GetDoubleValue()
		case "duration_ms":
			event.DurationMs = int(value.GetIntValue())
		case "input_tokens":
			event.InputTokens = int(value.GetIntValue())
		case "output_tokens":
			event.OutputTokens = int(value.GetIntValue())
		case "cache_read_tokens":
			event.CacheReadTokens = int(value.GetIntValue())
		case "cache_creation_tokens":
			event.CacheCreationTokens = int(value.GetIntValue())
		case "tool_name":
			event.ToolName = value.GetStringValue()
		case "success":
			event.Success = value.GetBoolValue()
		case "decision":
			event.Decision = value.GetStringValue()
		case "source":
			event.Source = value.GetStringValue()
		case "error":
			event.Error = value.GetStringValue()
		case "prompt_length":
			event.PromptLength = int(value.GetIntValue())
		case "status_code":
			event.StatusCode = int(value.GetIntValue())
		case "attempt":
			event.Attempt = int(value.GetIntValue())
		case "language":
			event.Language = value.GetStringValue()
		}
	}

	// Skip if no event type was extracted
	if event.EventType == "" {
		return nil
	}

	return event
}

// mapMetricName maps OTEL metric names to our internal types
func mapMetricName(name string) string {
	switch name {
	case "claude_code.session.count":
		return model.CCMetricSessionCount
	case "claude_code.token.usage":
		return model.CCMetricTokenUsage
	case "claude_code.cost.usage":
		return model.CCMetricCostUsage
	case "claude_code.lines_of_code.count":
		return model.CCMetricLinesOfCodeCount
	case "claude_code.commit.count":
		return model.CCMetricCommitCount
	case "claude_code.pull_request.count":
		return model.CCMetricPullRequestCount
	case "claude_code.active_time.total":
		return model.CCMetricActiveTimeTotal
	case "claude_code.code_edit_tool.decision":
		return model.CCMetricCodeEditToolDecision
	default:
		return ""
	}
}

// mapEventName maps OTEL event names to our internal types
func mapEventName(name string) string {
	switch name {
	case "claude_code.user_prompt":
		return model.CCEventUserPrompt
	case "claude_code.tool_result":
		return model.CCEventToolResult
	case "claude_code.api_request":
		return model.CCEventApiRequest
	case "claude_code.api_error":
		return model.CCEventApiError
	case "claude_code.tool_decision":
		return model.CCEventToolDecision
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

// applyMetricAttribute applies an attribute to a metric
func applyMetricAttribute(metric *model.CCOtelMetric, attr *commonv1.KeyValue) {
	key := attr.GetKey()
	value := attr.GetValue()

	switch key {
	case "type":
		metric.TokenType = value.GetStringValue()
	case "model":
		metric.Model = value.GetStringValue()
	case "tool":
		metric.Tool = value.GetStringValue()
	case "decision":
		metric.Decision = value.GetStringValue()
	case "language":
		metric.Language = value.GetStringValue()
	}
}

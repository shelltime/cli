package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/malamtime/cli/model"
	collogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collmetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

// AICodeOtelProcessor handles OTEL data parsing and forwarding to the backend
type AICodeOtelProcessor struct {
	config   model.ShellTimeConfig
	endpoint model.Endpoint
	hostname string
	debug    bool

	// maxRequestBytes bounds the JSON body of one backend request; larger batches are split.
	maxRequestBytes int
	// cumulativeWarning makes sure the cumulative-temporality warning is logged once.
	cumulativeWarning sync.Once
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
		hostname:        hostname,
		debug:           debug,
		maxRequestBytes: model.AICodeOtelMaxRequestBytes,
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

// otelResource is one OTLP resource together with what every record under it shares.
type otelResource struct {
	source string
	attrs  *model.AICodeOtelResourceAttributes
	// digest identifies the resource in stable ids. It is computed once per resource.
	digest [sha256.Size]byte
}

// deterministicMarshal encodes OTLP messages byte-for-byte reproducibly, so an exporter retry of
// the same record hashes to the same id.
var deterministicMarshal = proto.MarshalOptions{Deterministic: true}

func newOtelResource(resource *resourcev1.Resource, source string) *otelResource {
	res := &otelResource{source: source, attrs: extractResourceAttributes(resource)}
	if resource != nil {
		if buf, err := deterministicMarshal.Marshal(resource); err == nil {
			res.digest = sha256.Sum256(buf)
		}
	}
	return res
}

// stableOtelID derives an event or metric id from its OTLP payload: the source, the resource,
// the instrumentation scope and the encoded record (or metric name and data point). Each part
// is length-prefixed so different splits never collide.
func stableOtelID(source string, resourceDigest [sha256.Size]byte, scope string, parts ...[]byte) string {
	h := sha256.New()
	write := func(b []byte) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(b)))
		h.Write(n[:])
		h.Write(b)
	}
	write([]byte(source))
	write(resourceDigest[:])
	write([]byte(scope))
	for _, part := range parts {
		write(part)
	}
	sum := h.Sum(nil)
	return model.AICodeOtelIDPrefix + hex.EncodeToString(sum[:20])
}

// stableMessageID is stableOtelID over a proto message. If the message can't be encoded, which
// doesn't happen for valid OTLP, it falls back to a random id.
func stableMessageID(res *otelResource, scope string, prefix string, msg proto.Message) string {
	buf, err := deterministicMarshal.Marshal(msg)
	if err != nil {
		slog.Debug("AICodeOtel: Failed to encode record for its id", "error", err)
		return uuid.New().String()
	}
	return stableOtelID(res.source, res.digest, scope, []byte(prefix), buf)
}

// ProcessMetrics receives OTEL metrics and forwards to backend immediately
func (p *AICodeOtelProcessor) ProcessMetrics(ctx context.Context, req *collmetricsv1.ExportMetricsServiceRequest) (*collmetricsv1.ExportMetricsServiceResponse, error) {
	slog.Debug("AICodeOtel: Processing metrics request", "resourceMetricsCount", len(req.GetResourceMetrics()), slog.Bool("debug", p.debug))

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
		res := newOtelResource(resource, source)
		project := p.detectProject(resource, source)

		var metrics []model.AICodeOtelMetric
		for _, sm := range rm.GetScopeMetrics() {
			scope := sm.GetScope().GetName()
			for _, m := range sm.GetMetrics() {
				metrics = append(metrics, p.parseMetric(m, res, scope)...)
			}
		}

		if len(metrics) == 0 {
			continue
		}

		// Build and send request immediately - flat structure without session
		p.send(ctx, &model.AICodeOtelRequest{
			Host:    p.hostname,
			Project: project,
			Source:  source,
			Metrics: metrics,
		})
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
		res := newOtelResource(resource, source)
		project := p.detectProject(resource, source)

		var events []model.AICodeOtelEvent
		for _, sl := range rl.GetScopeLogs() {
			scope := sl.GetScope().GetName()
			for _, lr := range sl.GetLogRecords() {
				if event := p.parseLogRecord(lr, res, scope); event != nil {
					events = append(events, *event)
				}
			}
		}

		if len(events) == 0 {
			continue
		}

		// Build and send request immediately - flat structure without session
		p.send(ctx, &model.AICodeOtelRequest{
			Host:    p.hostname,
			Project: project,
			Source:  source,
			Events:  events,
		})
	}

	return &collogsv1.ExportLogsServiceResponse{}, nil
}

// send forwards a request to the backend, split so that each body stays within maxRequestBytes.
// Passthrough mode: failures are logged, not retried.
func (p *AICodeOtelProcessor) send(ctx context.Context, req *model.AICodeOtelRequest) {
	maxBytes := p.maxRequestBytes
	if maxBytes <= 0 {
		maxBytes = model.AICodeOtelMaxRequestBytes
	}
	for _, chunk := range model.SplitAICodeOtelRequest(req, maxBytes) {
		resp, err := model.SendAICodeOtelData(ctx, chunk, p.endpoint)
		if err != nil {
			slog.Error("AICodeOtel: Failed to send data to backend", "error", err, "events", len(chunk.Events), "metrics", len(chunk.Metrics))
			continue
		}
		slog.Debug("AICodeOtel: Data sent to backend", "eventsProcessed", resp.EventsProcessed, "metricsProcessed", resp.MetricsProcessed)
	}
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

// extractResourceAttributes extracts resource-level attributes from OTEL resource.
// Attributes without a dedicated field are kept in Attributes for the events.
func extractResourceAttributes(resource *resourcev1.Resource) *model.AICodeOtelResourceAttributes {
	attrs := &model.AICodeOtelResourceAttributes{}

	if resource == nil {
		return attrs
	}

	var accountID string
	for _, attr := range resource.GetAttributes() {
		key := attr.GetKey()
		value := attr.GetValue()

		switch key {
		// Standard resource attributes
		case "session.id":
			attrs.SessionID = anyString(value)
		case "event.kind":
			attrs.EventKind = anyString(value)
		case "conversation.id":
			attrs.ConversationID = anyString(value)
		case "app.version":
			attrs.AppVersion = anyString(value)
		case "app.entrypoint":
			attrs.Entrypoint = anyString(value)
		case "organization.id":
			attrs.OrganizationID = anyString(value)
		case "user.account_uuid":
			attrs.UserAccountUUID = anyString(value)
		case "user.account_id":
			accountID = anyString(value)
		case "terminal.type":
			attrs.TerminalType = anyString(value)
		case "os.type":
			attrs.OSType = anyString(value)
		case "os.version":
			attrs.OSVersion = anyString(value)
		case "host.arch":
			attrs.HostArch = anyString(value)
		// Additional identifiers
		case "user.id":
			attrs.UserID = anyString(value)
		case "user.email":
			attrs.UserEmail = anyString(value)
		// Custom resource attributes (from OTEL_RESOURCE_ATTRIBUTES)
		case "user.name":
			attrs.UserName = anyString(value)
		case "machine.name":
			attrs.MachineName = anyString(value)
		case "team.id":
			attrs.TeamID = anyString(value)
		case "pwd":
			attrs.Pwd = anyString(value)
		case "project", "project.path":
			// The request-level project, see detectProject.
		// Kept as fields and also forwarded in the attributes.
		case "service.name":
			attrs.ServiceName = anyString(value)
			attrs.Attributes = setAttribute(attrs.Attributes, key, anyValueToGo(value))
		case "service.version":
			attrs.ServiceVersion = anyString(value)
			attrs.Attributes = setAttribute(attrs.Attributes, key, anyValueToGo(value))
		case "wsl.version":
			attrs.WSLVersion = anyString(value)
			attrs.Attributes = setAttribute(attrs.Attributes, key, anyValueToGo(value))
		case "host.name":
			attrs.HostName = anyString(value)
			attrs.Attributes = setAttribute(attrs.Attributes, key, anyValueToGo(value))
		default:
			// env, vcs.*, custom OTEL_RESOURCE_ATTRIBUTES keys, ...
			attrs.Attributes = setAttribute(attrs.Attributes, key, anyValueToGo(value))
		}
	}
	// Claude sends both user.account_uuid and the tagged user.account_id; Codex only the latter.
	if attrs.UserAccountUUID == "" {
		attrs.UserAccountUUID = accountID
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
	metric.MachineName = firstNonEmpty(attrs.MachineName, attrs.HostName)
	metric.TeamID = attrs.TeamID
	metric.Pwd = attrs.Pwd
}

// applyResourceAttributesToEvent copies resource attributes into an event
func applyResourceAttributesToEvent(event *model.AICodeOtelEvent, attrs *model.AICodeOtelResourceAttributes) {
	// Standard resource attributes
	event.SessionID = attrs.SessionID
	event.EventKind = attrs.EventKind
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
	event.MachineName = firstNonEmpty(attrs.MachineName, attrs.HostName)
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
func (p *AICodeOtelProcessor) parseMetric(m *metricsv1.Metric, res *otelResource, scope string) []model.AICodeOtelMetric {
	name := m.GetName()
	metricType := mapMetricName(name)
	if metricType == "" {
		return nil // Unknown metric, skip
	}

	var dataPoints []*metricsv1.NumberDataPoint
	switch data := m.GetData().(type) {
	case *metricsv1.Metric_Sum:
		if data.Sum.GetAggregationTemporality() == metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
			p.warnCumulative(name)
		}
		dataPoints = data.Sum.GetDataPoints()
	case *metricsv1.Metric_Gauge:
		dataPoints = data.Gauge.GetDataPoints()
	}

	metrics := make([]model.AICodeOtelMetric, 0, len(dataPoints))
	for _, dp := range dataPoints {
		value := getDataPointValue(dp)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue // not representable in JSON
		}
		ts := dp.GetTimeUnixNano()
		metric := model.AICodeOtelMetric{
			MetricID:    stableMessageID(res, scope, name, dp),
			MetricType:  metricType,
			Timestamp:   int64(ts / 1e9),
			TimestampMs: int64(ts / 1e6),
			Value:       value,
			ClientType:  res.source,
		}
		// Apply resource attributes first
		applyResourceAttributesToMetric(&metric, res.attrs)
		// Then extract data point attributes (can override resource attrs)
		for _, attr := range dp.GetAttributes() {
			applyMetricAttribute(&metric, attr, metricType)
		}
		metrics = append(metrics, metric)
	}

	return metrics
}

// warnCumulative logs once that a metric arrived with cumulative temporality. The server sums
// data points, so cumulative values are counted again on every export.
func (p *AICodeOtelProcessor) warnCumulative(metricName string) {
	p.cumulativeWarning.Do(func() {
		slog.Warn("AICodeOtel: Received cumulative metrics; ShellTime expects delta temporality, so totals will be inflated. Set OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=delta (shelltime cc install does this).", "metric", metricName)
	})
}

// eventFallbacks collects attributes that only apply when a preferred one is absent, since OTLP
// attributes arrive in no particular order.
type eventFallbacks struct {
	effort               string
	modelReasoningEffort string
	reasoningEffort      string
	costMicros           *float64
	err                  string
	errMessage           string
	errType              string
	promptText           string
	callID               string
	toolUseID            string
	appEntrypoint        string
	originator           string
	toolInput            string
	argumentsInput       string
}

func (f *eventFallbacks) apply(event *model.AICodeOtelEvent, res *model.AICodeOtelResourceAttributes) {
	if effort := firstNonEmpty(f.effort, f.modelReasoningEffort, f.reasoningEffort); effort != "" {
		event.ReasoningEffort = effort
	}
	if event.CostUSD == nil && f.costMicros != nil {
		event.CostUSD = model.Float64Ref(*f.costMicros / 1e6)
	}
	event.Error = firstNonEmpty(f.err, f.errMessage, f.errType)
	if event.Prompt == "" {
		event.Prompt = f.promptText
	}
	event.CallID = firstNonEmpty(f.callID, f.toolUseID)
	if input := firstNonEmpty(f.toolInput, f.argumentsInput); input != "" {
		event.ToolInput = input
	}
	event.Entrypoint = firstNonEmpty(f.appEntrypoint, res.Entrypoint, f.originator, res.ServiceName)
}

// parseLogRecord parses an OTEL log record into a AICodeOtelEvent. It returns nil for records
// that aren't events and for dropped events.
func (p *AICodeOtelProcessor) parseLogRecord(lr *logsv1.LogRecord, res *otelResource, scope string) *model.AICodeOtelEvent {
	eventType := normalizeOtelEventName(resolveEventName(lr))
	if eventType == "" {
		return nil
	}

	ts := lr.GetTimeUnixNano()
	if ts == 0 {
		ts = lr.GetObservedTimeUnixNano()
	}
	event := &model.AICodeOtelEvent{
		EventID:     stableMessageID(res, scope, "", lr),
		EventType:   eventType,
		Timestamp:   int64(ts / 1e9),
		TimestampMs: int64(ts / 1e6),
		ClientType:  res.source,
	}

	// Apply resource attributes first
	applyResourceAttributesToEvent(event, res.attrs)

	// Resource attributes without a field (service.*, env, vcs.*, ...) are seeded first, so a
	// record with many attributes can't crowd them out; the record's own values override them.
	keys := make([]string, 0, len(res.attrs.Attributes))
	for key := range res.attrs.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		event.Attributes = setAttribute(event.Attributes, key, res.attrs.Attributes[key])
	}

	var fallbacks eventFallbacks
	for _, attr := range lr.GetAttributes() {
		applyEventAttribute(event, &fallbacks, attr.GetKey(), attr.GetValue())
	}
	fallbacks.apply(event, res.attrs)

	if event.SessionID == "" && event.ConversationID != "" {
		event.SessionID = event.ConversationID
	}

	return event
}

// applyEventAttribute maps one log record attribute onto the event. Attributes without a field
// go to the attributes catch-all.
func applyEventAttribute(event *model.AICodeOtelEvent, f *eventFallbacks, key string, value *commonv1.AnyValue) {
	switch key {
	case "event.name":
		// Resolved before the attributes, see resolveEventName.
	case "event.kind", "event_kind", "eventKind":
		event.EventKind = anyString(value)
	case "event.timestamp":
		event.EventTimestamp = anyString(value)
	case "event.sequence":
		event.Sequence = optInt64Ref(value)
	case "prompt.id":
		event.PromptID = anyString(value)
	case "request_id":
		event.RequestID = anyString(value)
	case "speed":
		event.Speed = anyString(value)
	case "query_source":
		event.QuerySource = anyString(value)
	case "effort":
		f.effort = anyString(value)
	case "model_reasoning_effort":
		f.modelReasoningEffort = anyString(value)
	case "reasoning_effort", "reasoningEffort":
		f.reasoningEffort = anyString(value)
	case "model":
		event.Model = anyString(value)

	// Cost, duration and tokens
	case "cost_usd":
		event.CostUSD = optFloat(value)
	case "cost_usd_micros":
		f.costMicros = optFloat(value)
	case "duration_ms":
		event.DurationMs = optInt(value)
	case "input_tokens", "input_token_count":
		event.InputTokens = optInt(value)
	case "output_tokens", "output_token_count":
		event.OutputTokens = optInt(value)
	case "cache_read_tokens", "cache_token_count", "cached_token_count", "cachedTokenCount":
		event.CacheReadTokens = optInt(value)
	case "cache_creation_tokens", "cache_write_token_count", "cacheWriteTokenCount":
		event.CacheCreationTokens = optInt(value)
	case "reasoning_tokens", "reasoning_token_count", "reasoningTokenCount":
		event.ReasoningTokens = optInt(value)
	case "tool_tokens", "toolTokens":
		event.ToolTokens = optInt(value)
	case "tool_token_count", "toolTokenCount":
		// Codex sends the response's total_tokens under this name; it is not a tool token count.
		if n, ok := optInt64(value); ok {
			event.Attributes = setAttribute(event.Attributes, "total_token_count", n)
		}

	// Tools
	case "tool_name":
		event.ToolName = anyString(value)
	case "success":
		event.Success = optBool(value)
	case "decision":
		event.Decision = anyString(value)
	case "source":
		event.Source = anyString(value)
	case "call_id", "callId":
		f.callID = anyString(value)
	case "tool_use_id":
		f.toolUseID = anyString(value)
	case "tool_parameters":
		// Claude sends a JSON object encoded as a string.
		raw := anyString(value)
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &params); err == nil && params != nil {
			event.ToolParameters = params
		} else {
			event.Attributes = setAttribute(event.Attributes, key, raw)
		}
	case "tool_input":
		f.toolInput = model.CapAICodeText(anyJSONString(value), model.AICodeOtelMaxTextBytes)
	case "arguments":
		// Codex: raw tool arguments, JSON or freeform (apply_patch).
		f.argumentsInput = model.CapAICodeText(anyJSONString(value), model.AICodeOtelMaxTextBytes)
	case "tool_arguments", "toolArguments":
		raw := anyJSONString(value)
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &args); err == nil && args != nil {
			event.ToolArguments = args
		} else if raw != "" {
			f.argumentsInput = model.CapAICodeText(raw, model.AICodeOtelMaxTextBytes)
		}
	case "output", "tool_output", "toolOutput":
		event.ToolOutput = model.CapAICodeText(anyString(value), model.AICodeOtelMaxTextBytes)

	// Errors: error wins over error.message, which wins over error_type.
	case "error":
		f.err = anyString(value)
	case "error.message":
		f.errMessage = anyString(value)
	case "error_type":
		f.errType = anyString(value)
		event.Attributes = setAttribute(event.Attributes, key, f.errType)

	// Prompts and responses
	case "prompt_length":
		event.PromptLength = optInt(value)
	case "prompt":
		if s := anyString(value); !isRedacted(s) {
			event.Prompt = s
		}
	case "prompt_text":
		if s := anyString(value); !isRedacted(s) {
			f.promptText = s
		}
	case "prompt_encrypted", "promptEncrypted":
		if b := optBool(value); b != nil {
			event.PromptEncrypted = *b
		}
	case "response":
		if s := anyString(value); !isRedacted(s) {
			event.Response = model.CapAICodeText(s, model.AICodeOtelMaxTextBytes)
		}
	case "response_length":
		event.ResponseLength = optInt(value)

	// API requests
	case "status_code", "http.response.status_code":
		event.StatusCode = optInt(value)
	case "attempt":
		event.Attempt = optInt(value)
	case "language":
		event.Language = anyString(value)
	case "provider", "provider_name", "providerName":
		event.Provider = anyString(value)

	// Codex conversation_starts
	case "auth_mode", "authMode":
		event.AuthMode = anyString(value)
	case "slug":
		event.Slug = anyString(value)
	case "context_window", "contextWindow":
		event.ContextWindow = optInt(value)
	case "approval_policy", "approvalPolicy":
		event.ApprovalPolicy = anyString(value)
	case "sandbox_policy", "sandboxPolicy":
		event.SandboxPolicy = anyString(value)
	case "mcp_servers", "mcpServers":
		event.MCPServers = stringList(value)
	case "profile", "active_profile", "activeProfile":
		event.Profile = anyString(value)
	case "reasoning_enabled", "reasoningEnabled":
		event.ReasoningEnabled = optBool(value)
	case "reasoning_summary", "reasoningSummary":
		event.ReasoningSummary = anyString(value)
	case "max_output_tokens", "maxOutputTokens":
		event.MaxOutputTokens = optInt(value)
	case "auto_compact_token_limit", "autoCompactTokenLimit":
		event.AutoCompactTokenLimit = optInt(value)

	// Entrypoint: app.entrypoint, then Codex originator, then service.name
	case "app.entrypoint":
		f.appEntrypoint = anyString(value)
	case "originator":
		f.originator = anyString(value)

	// Identity attributes on the record override the resource's.
	case "conversation.id", "conversationId":
		event.ConversationID = anyString(value)
	case "user.id":
		event.UserID = anyString(value)
	case "user.email":
		event.UserEmail = anyString(value)
	case "session.id":
		event.SessionID = anyString(value)
	case "app.version":
		event.AppVersion = anyString(value)
	case "organization.id":
		event.OrganizationID = anyString(value)
	case "user.account_uuid":
		event.UserAccountUUID = anyString(value)
	case "user.account_id":
		if event.UserAccountUUID == "" {
			event.UserAccountUUID = anyString(value)
		}
	case "terminal.type":
		event.TerminalType = anyString(value)
	case "os.type":
		event.OSType = anyString(value)
	case "os.version":
		event.OSVersion = anyString(value)
	case "host.arch":
		event.HostArch = anyString(value)
	case "user.name":
		event.UserName = anyString(value)
	case "machine.name":
		event.MachineName = anyString(value)
	case "team.id":
		event.TeamID = anyString(value)
	case "pwd":
		event.Pwd = anyString(value)
	case "project", "project.path":
		// The request-level project, see detectProject.

	default:
		event.Attributes = setAttribute(event.Attributes, key, anyValueToGo(value))
	}
}

// resolveEventName returns the event name of a log record: the event.name attribute, then the
// record's EventName field, then a string body that looks like an event name.
func resolveEventName(lr *logsv1.LogRecord) string {
	for _, attr := range lr.GetAttributes() {
		if attr.GetKey() == "event.name" {
			if name := strings.TrimSpace(anyString(attr.GetValue())); name != "" {
				return name
			}
		}
	}
	if name := strings.TrimSpace(lr.GetEventName()); name != "" {
		return name
	}
	if body := strings.TrimSpace(lr.GetBody().GetStringValue()); body != "" && len(body) <= 128 && !strings.ContainsAny(body, " \t\r\n") {
		return body
	}
	return ""
}

// normalizeOtelEventName strips the claude_code./codex. prefix and returns "" for dropped events.
func normalizeOtelEventName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "claude_code.")
	name = strings.TrimPrefix(name, "codex.")
	if model.IsDroppedAICodeOtelEvent(name) {
		return ""
	}
	return name
}

// mapMetricName maps OTEL metric names to our internal types. Only Claude Code exports OTLP
// metrics that ShellTime reads; Codex metrics carry no session id and stay off.
func mapMetricName(name string) string {
	switch name {
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
	default:
		return ""
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
func applyMetricAttribute(metric *model.AICodeOtelMetric, attr *commonv1.KeyValue, metricType string) {
	key := attr.GetKey()
	value := attr.GetValue()

	switch key {
	case "type":
		// token_usage: input/output/cacheRead/cacheCreation; lines_of_code: added/removed;
		// active_time: user/cli, which is no token type.
		switch metricType {
		case model.AICodeMetricTokenUsage:
			metric.TokenType = anyString(value)
		case model.AICodeMetricLinesOfCodeCount:
			metric.LinesType = anyString(value)
		default:
			metric.Attributes = setAttribute(metric.Attributes, key, anyValueToGo(value))
		}
	case "model":
		metric.Model = anyString(value)
	case "tool", "tool_name":
		metric.Tool = anyString(value)
	case "decision":
		metric.Decision = anyString(value)
	case "language":
		metric.Language = anyString(value)
	// Resource attributes at data point level - apply them (override if already set from resource)
	case "session.id":
		metric.SessionID = anyString(value)
	case "conversation.id":
		metric.ConversationID = anyString(value)
	case "user.account_uuid":
		metric.UserAccountUUID = anyString(value)
	case "user.account_id":
		if metric.UserAccountUUID == "" {
			metric.UserAccountUUID = anyString(value)
		}
	case "organization.id":
		metric.OrganizationID = anyString(value)
	case "terminal.type":
		metric.TerminalType = anyString(value)
	case "app.version":
		metric.AppVersion = anyString(value)
	case "os.type":
		metric.OSType = anyString(value)
	case "os.version":
		metric.OSVersion = anyString(value)
	case "host.arch":
		metric.HostArch = anyString(value)
	// Additional identifiers at data point level
	case "user.id":
		metric.UserID = anyString(value)
	case "user.email":
		metric.UserEmail = anyString(value)
	// OTEL_RESOURCE_ATTRIBUTES keys, which Claude Code copies onto data points
	case "user.name":
		metric.UserName = anyString(value)
	case "machine.name":
		metric.MachineName = anyString(value)
	case "team.id":
		metric.TeamID = anyString(value)
	case "pwd":
		metric.Pwd = anyString(value)
	default:
		metric.Attributes = setAttribute(metric.Attributes, key, anyValueToGo(value))
	}
}

// --- OTLP value helpers --------------------------------------------------------
//
// Claude Code and Codex encode the same logical type differently (Codex formats many numbers
// and flags as strings), so each helper accepts every encoding seen in practice and reports
// whether a value was present, so that 0 and false are kept.

// optInt64 reads an integer from an int, a double (rounded) or a numeric string.
func optInt64(v *commonv1.AnyValue) (int64, bool) {
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_IntValue:
		return x.IntValue, true
	case *commonv1.AnyValue_DoubleValue:
		return floatToInt64(x.DoubleValue)
	case *commonv1.AnyValue_StringValue:
		s := strings.TrimSpace(x.StringValue)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return floatToInt64(f)
		}
	}
	return 0, false
}

func floatToInt64(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) >= math.MaxInt64 {
		return 0, false
	}
	return int64(math.Round(f)), true
}

func optInt(v *commonv1.AnyValue) *int {
	if n, ok := optInt64(v); ok {
		return model.IntRef(int(n))
	}
	return nil
}

func optInt64Ref(v *commonv1.AnyValue) *int64 {
	if n, ok := optInt64(v); ok {
		return model.Int64Ref(n)
	}
	return nil
}

// optFloat reads a number from a double, an int or a numeric string.
func optFloat(v *commonv1.AnyValue) *float64 {
	var f float64
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_DoubleValue:
		f = x.DoubleValue
	case *commonv1.AnyValue_IntValue:
		f = float64(x.IntValue)
	case *commonv1.AnyValue_StringValue:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(x.StringValue), 64)
		if err != nil {
			return nil
		}
		f = parsed
	default:
		return nil
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return model.Float64Ref(f)
}

// optBool reads a flag from a bool or the strings "true"/"false".
func optBool(v *commonv1.AnyValue) *bool {
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_BoolValue:
		return model.BoolRef(x.BoolValue)
	case *commonv1.AnyValue_StringValue:
		switch strings.ToLower(strings.TrimSpace(x.StringValue)) {
		case "true":
			return model.BoolRef(true)
		case "false":
			return model.BoolRef(false)
		}
	}
	return nil
}

// stringList reads a list from an array or a comma-separated string (Codex joins mcp_servers
// with ", ").
func stringList(v *commonv1.AnyValue) []string {
	var items []string
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_ArrayValue:
		for _, item := range x.ArrayValue.GetValues() {
			items = append(items, anyString(item))
		}
	case *commonv1.AnyValue_StringValue:
		items = strings.Split(x.StringValue, ",")
	}
	var out []string
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// anyString reads a scalar as a string. Non-string scalars are formatted; arrays and maps yield "".
func anyString(v *commonv1.AnyValue) string {
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return x.StringValue
	case *commonv1.AnyValue_IntValue:
		return strconv.FormatInt(x.IntValue, 10)
	case *commonv1.AnyValue_DoubleValue:
		return strconv.FormatFloat(x.DoubleValue, 'f', -1, 64)
	case *commonv1.AnyValue_BoolValue:
		return strconv.FormatBool(x.BoolValue)
	}
	return ""
}

// anyJSONString reads a value meant to be text: strings as they are, arrays and maps as JSON.
func anyJSONString(v *commonv1.AnyValue) string {
	switch v.GetValue().(type) {
	case *commonv1.AnyValue_ArrayValue, *commonv1.AnyValue_KvlistValue:
		buf, err := json.Marshal(anyValueToGo(v))
		if err != nil {
			return ""
		}
		return string(buf)
	}
	return anyString(v)
}

// anyValueToGo converts any OTLP value to a JSON-encodable Go value, recursing into arrays and
// key/value lists. Non-finite doubles become strings, since JSON can't represent them.
func anyValueToGo(v *commonv1.AnyValue) any {
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return x.StringValue
	case *commonv1.AnyValue_BoolValue:
		return x.BoolValue
	case *commonv1.AnyValue_IntValue:
		return x.IntValue
	case *commonv1.AnyValue_DoubleValue:
		if math.IsNaN(x.DoubleValue) || math.IsInf(x.DoubleValue, 0) {
			return strconv.FormatFloat(x.DoubleValue, 'f', -1, 64)
		}
		return x.DoubleValue
	case *commonv1.AnyValue_ArrayValue:
		values := x.ArrayValue.GetValues()
		out := make([]any, 0, len(values))
		for _, item := range values {
			out = append(out, anyValueToGo(item))
		}
		return out
	case *commonv1.AnyValue_KvlistValue:
		values := x.KvlistValue.GetValues()
		out := make(map[string]any, len(values))
		for _, kv := range values {
			out[kv.GetKey()] = anyValueToGo(kv.GetValue())
		}
		return out
	case *commonv1.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(x.BytesValue)
	}
	return nil
}

// setAttribute adds key to the attributes catch-all, enforcing its limits: string values are
// capped at AICodeOtelMaxAttributeBytes and at most AICodeOtelMaxAttributes keys are kept.
// Empty values are skipped. It returns the (possibly new) map.
func setAttribute(attrs map[string]any, key string, value any) map[string]any {
	value = capAttributeValue(value, 0)
	if value == nil || value == "" || key == "" {
		return attrs
	}
	if _, exists := attrs[key]; !exists && len(attrs) >= model.AICodeOtelMaxAttributes {
		return attrs
	}
	if attrs == nil {
		attrs = make(map[string]any)
	}
	attrs[key] = value
	return attrs
}

// maxAttributeDepth and maxAttributeItems bound nested arrays and maps in attribute values.
const (
	maxAttributeDepth = 4
	maxAttributeItems = model.AICodeOtelMaxAttributes
)

func capAttributeValue(value any, depth int) any {
	switch x := value.(type) {
	case string:
		return model.CapAICodeText(x, model.AICodeOtelMaxAttributeBytes)
	case []any:
		if depth >= maxAttributeDepth {
			return nil
		}
		if len(x) > maxAttributeItems {
			x = x[:maxAttributeItems]
		}
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, capAttributeValue(item, depth+1))
		}
		return out
	case map[string]any:
		if depth >= maxAttributeDepth {
			return nil
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) > maxAttributeItems {
			keys = keys[:maxAttributeItems]
		}
		out := make(map[string]any, len(keys))
		for _, k := range keys {
			out[k] = capAttributeValue(x[k], depth+1)
		}
		return out
	}
	return value
}

// isRedacted reports whether a content attribute holds a redaction placeholder instead of text
// (Claude Code sends <REDACTED>, Codex [REDACTED]).
func isRedacted(s string) bool {
	switch strings.TrimSpace(s) {
	case "<REDACTED>", "[REDACTED]":
		return true
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

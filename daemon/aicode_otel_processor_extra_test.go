package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
)

// strVal is a helper for building an OTEL string AnyValue.
func strVal(s string) *commonv1.AnyValue {
	return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: s}}
}

func intVal(i int64) *commonv1.AnyValue {
	return &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: i}}
}

func dblVal(f float64) *commonv1.AnyValue {
	return &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: f}}
}

func boolVal(b bool) *commonv1.AnyValue {
	return &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: b}}
}

func kv(key string, v *commonv1.AnyValue) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: key, Value: v}
}

func serviceResource(serviceName string, extra ...*commonv1.KeyValue) *resourcev1.Resource {
	attrs := []*commonv1.KeyValue{kv("service.name", strVal(serviceName))}
	attrs = append(attrs, extra...)
	return &resourcev1.Resource{Attributes: attrs}
}

// captureProcessor wires a processor to a test HTTP server and records the
// AICodeOtelRequest bodies POSTed to /api/v1/cc/otel.
type captureProcessor struct {
	processor *AICodeOtelProcessor
	server    *httptest.Server
	mu        sync.Mutex
	requests  []model.AICodeOtelRequest
}

func newCaptureProcessor(t *testing.T, cfg model.ShellTimeConfig) *captureProcessor {
	t.Helper()
	cp := &captureProcessor{}
	cp.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/cc/otel", r.URL.Path)
		var req model.AICodeOtelRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		cp.mu.Lock()
		cp.requests = append(cp.requests, req)
		cp.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.AICodeOtelResponse{Success: true, MetricsProcessed: len(req.Metrics), EventsProcessed: len(req.Events)})
	}))
	t.Cleanup(cp.server.Close)

	cfg.APIEndpoint = cp.server.URL
	cp.processor = NewAICodeOtelProcessor(cfg)
	// Point the processor at the test server explicitly (NewAICodeOtelProcessor
	// copies APIEndpoint into the endpoint).
	cp.processor.endpoint.APIEndpoint = cp.server.URL
	return cp
}

func (cp *captureProcessor) captured() []model.AICodeOtelRequest {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	out := make([]model.AICodeOtelRequest, len(cp.requests))
	copy(out, cp.requests)
	return out
}

func TestProcessMetrics_SumAndGauge(t *testing.T) {
	t.Setenv("PWD", "/work/dir")
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	req := &collmetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: serviceResource("claude-code", kv("session.id", strVal("sess-1"))),
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{
						Metrics: []*metricsv1.Metric{
							{
								Name: "claude_code.token.usage",
								Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
									DataPoints: []*metricsv1.NumberDataPoint{
										{
											TimeUnixNano: 2_000_000_000,
											Value:        &metricsv1.NumberDataPoint_AsInt{AsInt: 123},
											Attributes: []*commonv1.KeyValue{
												kv("type", strVal("input")),
												kv("model", strVal("claude-3")),
											},
										},
									},
								}},
							},
							{
								Name: "claude_code.cost.usage",
								Data: &metricsv1.Metric_Gauge{Gauge: &metricsv1.Gauge{
									DataPoints: []*metricsv1.NumberDataPoint{
										{
											TimeUnixNano: 3_000_000_000,
											Value:        &metricsv1.NumberDataPoint_AsDouble{AsDouble: 0.42},
										},
									},
								}},
							},
							{
								// Unknown metric name -> skipped
								Name: "claude_code.unknown.thing",
								Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
									DataPoints: []*metricsv1.NumberDataPoint{{Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 9}}},
								}},
							},
						},
					},
				},
			},
		},
	}

	resp, err := cp.processor.ProcessMetrics(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	reqs := cp.captured()
	require.Len(t, reqs, 1)
	got := reqs[0]
	assert.Equal(t, model.AICodeOtelSourceClaudeCode, got.Source)
	assert.Equal(t, "/work/dir", got.Project)
	assert.NotEmpty(t, got.Host)
	require.Len(t, got.Metrics, 2) // unknown metric dropped

	// Sum metric (token usage)
	tokenMetric := got.Metrics[0]
	assert.Equal(t, model.AICodeMetricTokenUsage, tokenMetric.MetricType)
	assert.Equal(t, int64(2), tokenMetric.Timestamp) // nanos -> seconds
	assert.Equal(t, float64(123), tokenMetric.Value)
	assert.Equal(t, "input", tokenMetric.TokenType)
	assert.Equal(t, "claude-3", tokenMetric.Model)
	assert.Equal(t, "sess-1", tokenMetric.SessionID) // from resource attrs
	assert.Equal(t, model.AICodeOtelSourceClaudeCode, tokenMetric.ClientType)
	assert.NotEmpty(t, tokenMetric.MetricID)

	// Gauge metric (cost usage)
	costMetric := got.Metrics[1]
	assert.Equal(t, model.AICodeMetricCostUsage, costMetric.MetricType)
	assert.Equal(t, int64(3), costMetric.Timestamp)
	assert.Equal(t, 0.42, costMetric.Value)
}

func TestProcessMetrics_LinesOfCodeUsesLinesType(t *testing.T) {
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	req := &collmetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: serviceResource("claude-code", kv("project", strVal("my-proj"))),
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{
						Metrics: []*metricsv1.Metric{
							{
								Name: "claude_code.lines_of_code.count",
								Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
									DataPoints: []*metricsv1.NumberDataPoint{
										{
											Value:      &metricsv1.NumberDataPoint_AsInt{AsInt: 10},
											Attributes: []*commonv1.KeyValue{kv("type", strVal("added"))},
										},
									},
								}},
							},
						},
					},
				},
			},
		},
	}

	_, err := cp.processor.ProcessMetrics(context.Background(), req)
	require.NoError(t, err)

	reqs := cp.captured()
	require.Len(t, reqs, 1)
	assert.Equal(t, "my-proj", reqs[0].Project) // resource attr takes precedence
	require.Len(t, reqs[0].Metrics, 1)
	m := reqs[0].Metrics[0]
	assert.Equal(t, model.AICodeMetricLinesOfCodeCount, m.MetricType)
	assert.Equal(t, "added", m.LinesType)
	assert.Empty(t, m.TokenType)
}

func TestProcessMetrics_UnknownSourceSkipped(t *testing.T) {
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	req := &collmetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: serviceResource("vscode"),
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{Metrics: []*metricsv1.Metric{{Name: "claude_code.cost.usage"}}},
				},
			},
		},
	}

	_, err := cp.processor.ProcessMetrics(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, cp.captured(), "unknown source should produce no backend request")
}

func TestProcessMetrics_NoMetricsNoRequest(t *testing.T) {
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	// Known source but only an unknown metric -> no metrics -> no request sent.
	req := &collmetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: serviceResource("codex-cli"),
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{Metrics: []*metricsv1.Metric{{Name: "codex.totally.unknown"}}},
				},
			},
		},
	}

	_, err := cp.processor.ProcessMetrics(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, cp.captured())
}

func TestProcessMetrics_BackendErrorIsSwallowed(t *testing.T) {
	// Server returns 500; ProcessMetrics must still succeed (passthrough, no retry).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	processor := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "tok", APIEndpoint: server.URL})

	req := &collmetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: serviceResource("claude-code"),
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{Metrics: []*metricsv1.Metric{
						{
							Name: "claude_code.session.count",
							Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
								DataPoints: []*metricsv1.NumberDataPoint{{Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 1}}},
							}},
						},
					}},
				},
			},
		},
	}

	resp, err := processor.ProcessMetrics(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestProcessLogs_ClaudeApiRequestEvent(t *testing.T) {
	t.Setenv("PWD", "/logs/dir")
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	req := &collogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: serviceResource("claude-code", kv("user.email", strVal("e@x.com"))),
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							{
								TimeUnixNano: 5_000_000_000,
								Attributes: []*commonv1.KeyValue{
									kv("event.name", strVal("claude_code.api_request")),
									kv("model", strVal("claude-3-5")),
									kv("cost_usd", dblVal(0.01)),
									kv("duration_ms", intVal(250)),
									kv("input_tokens", intVal(100)),
									kv("output_tokens", strVal("50")), // string form
									kv("cache_read_tokens", intVal(10)),
									kv("success", boolVal(true)),
									kv("status_code", intVal(200)),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := cp.processor.ProcessLogs(context.Background(), req)
	require.NoError(t, err)

	reqs := cp.captured()
	require.Len(t, reqs, 1)
	assert.Equal(t, model.AICodeOtelSourceClaudeCode, reqs[0].Source)
	assert.Equal(t, "/logs/dir", reqs[0].Project)
	require.Len(t, reqs[0].Events, 1)
	ev := reqs[0].Events[0]
	assert.Equal(t, model.AICodeEventApiRequest, ev.EventType)
	assert.Equal(t, int64(5), ev.Timestamp)
	assert.Equal(t, "claude-3-5", ev.Model)
	assert.Equal(t, 0.01, ev.CostUSD)
	assert.Equal(t, 250, ev.DurationMs)
	assert.Equal(t, 100, ev.InputTokens)
	assert.Equal(t, 50, ev.OutputTokens)
	assert.Equal(t, 10, ev.CacheReadTokens)
	assert.True(t, ev.Success)
	assert.Equal(t, 200, ev.StatusCode)
	assert.Equal(t, "e@x.com", ev.UserEmail) // from resource attrs
	assert.NotEmpty(t, ev.EventID)
}

func TestProcessLogs_CodexConversationStartsMapsConvIDToSession(t *testing.T) {
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	req := &collogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: serviceResource("codex-cli", kv("project", strVal("codex-proj"))),
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							{
								// No TimeUnixNano -> falls back to ObservedTimeUnixNano
								ObservedTimeUnixNano: 7_000_000_000,
								Attributes: []*commonv1.KeyValue{
									kv("event.name", strVal("codex.conversation_starts")),
									kv("conversation.id", strVal("conv-9")),
									kv("auth_mode", strVal("apikey")),
									kv("approval_policy", strVal("auto")),
									kv("reasoning_enabled", boolVal(true)),
									kv("reasoning_effort", strVal("high")),
									kv("context_window", intVal(128000)),
									kv("mcp_servers", &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: &commonv1.ArrayValue{Values: []*commonv1.AnyValue{strVal("fs"), strVal("git")}}}}),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := cp.processor.ProcessLogs(context.Background(), req)
	require.NoError(t, err)

	reqs := cp.captured()
	require.Len(t, reqs, 1)
	assert.Equal(t, model.AICodeOtelSourceCodex, reqs[0].Source)
	assert.Equal(t, "codex-proj", reqs[0].Project)
	require.Len(t, reqs[0].Events, 1)
	ev := reqs[0].Events[0]
	assert.Equal(t, model.AICodeEventConversationStarts, ev.EventType)
	assert.Equal(t, int64(7), ev.Timestamp) // fell back to observed time
	assert.Equal(t, "conv-9", ev.ConversationID)
	assert.Equal(t, "conv-9", ev.SessionID) // sessionID derived from conversationID
	assert.Equal(t, "apikey", ev.AuthMode)
	assert.Equal(t, "auto", ev.ApprovalPolicy)
	assert.True(t, ev.ReasoningEnabled)
	assert.Equal(t, "high", ev.ReasoningEffort)
	assert.Equal(t, 128000, ev.ContextWindow)
	assert.Equal(t, []string{"fs", "git"}, ev.MCPServers)
}

func TestProcessLogs_ToolParametersJSONParsed(t *testing.T) {
	cp := newCaptureProcessor(t, model.ShellTimeConfig{Token: "tok"})

	req := &collogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: serviceResource("claude-code"),
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							{
								TimeUnixNano: 1_000_000_000,
								Attributes: []*commonv1.KeyValue{
									kv("event.name", strVal("claude_code.tool_result")),
									kv("tool_name", strVal("Bash")),
									kv("tool_parameters", strVal(`{"cmd":"ls","n":3}`)),
									kv("tool_arguments", strVal(`{"a":"b"}`)),
									kv("tool_parameters_bad_just_ignored", strVal("noop")),
								},
							},
							{
								// invalid JSON tool_parameters -> ignored, but event still valid
								TimeUnixNano: 1_000_000_000,
								Attributes: []*commonv1.KeyValue{
									kv("event.name", strVal("claude_code.tool_result")),
									kv("tool_parameters", strVal(`{not json`)),
								},
							},
							{
								// no event.name -> dropped (returns nil)
								TimeUnixNano: 1_000_000_000,
								Attributes: []*commonv1.KeyValue{
									kv("model", strVal("x")),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := cp.processor.ProcessLogs(context.Background(), req)
	require.NoError(t, err)

	reqs := cp.captured()
	require.Len(t, reqs, 1)
	require.Len(t, reqs[0].Events, 2) // third record (no event type) dropped

	first := reqs[0].Events[0]
	assert.Equal(t, model.AICodeEventToolResult, first.EventType)
	assert.Equal(t, "Bash", first.ToolName)
	require.NotNil(t, first.ToolParameters)
	assert.Equal(t, "ls", first.ToolParameters["cmd"])
	assert.InDelta(t, 3, first.ToolParameters["n"], 0.0001)
	require.NotNil(t, first.ToolArguments)
	assert.Equal(t, "b", first.ToolArguments["a"])

	second := reqs[0].Events[1]
	assert.Equal(t, model.AICodeEventToolResult, second.EventType)
	assert.Nil(t, second.ToolParameters) // bad JSON ignored
}

func TestParseLogRecord_AllAttributeBranches(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	// Resource attrs provide defaults; some are overridden by log-level attrs.
	resAttrs := &model.AICodeOtelResourceAttributes{
		SessionID:    "res-session",
		UserID:       "res-user",
		AppVersion:   "res-app",
		TerminalType: "res-term",
	}

	lr := &logsv1.LogRecord{
		TimeUnixNano: 10_000_000_000,
		Attributes: []*commonv1.KeyValue{
			kv("event.name", strVal("codex.api_error")),
			kv("event.kind", strVal("k1")),
			kv("event.timestamp", strVal("2025-01-01T00:00:00Z")),
			kv("cache_creation_tokens", intVal(7)),
			kv("decision", strVal("reject")),
			kv("source", strVal("user")),
			kv("error", strVal("boom")),
			kv("prompt_length", intVal(42)),
			kv("prompt", strVal("hello")),
			kv("attempt", intVal(2)),
			kv("error.message", strVal("overridden-error")),
			kv("language", strVal("python")),
			kv("reasoning_tokens", intVal(99)),
			kv("provider", strVal("openai")),
			kv("call_id", strVal("call-1")),
			kv("event_kind", strVal("ek-override")),
			kv("tool_tokens", intVal(3)),
			kv("slug", strVal("gpt-5")),
			kv("sandbox_policy", strVal("workspace")),
			kv("mcp_servers", &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: &commonv1.ArrayValue{Values: []*commonv1.AnyValue{strVal("a")}}}}),
			kv("profile", strVal("default")),
			kv("reasoning_summary", strVal("brief")),
			kv("max_output_tokens", intVal(1000)),
			kv("auto_compact_token_limit", intVal(2000)),
			kv("tool_output", strVal("done")),
			kv("prompt_encrypted", boolVal(true)),
			// override attributes (take precedence over resource attrs)
			kv("user.id", strVal("log-user")),
			kv("user.email", strVal("log@e")),
			kv("session.id", strVal("log-session")),
			kv("app.version", strVal("log-app")),
			kv("organization.id", strVal("log-org")),
			kv("user.account_uuid", strVal("log-acct")),
			kv("terminal.type", strVal("log-term")),
		},
	}

	ev := p.parseLogRecord(lr, resAttrs, model.AICodeOtelSourceCodex)
	require.NotNil(t, ev)
	assert.Equal(t, model.AICodeEventApiError, ev.EventType)
	assert.Equal(t, int64(10), ev.Timestamp)
	assert.Equal(t, "2025-01-01T00:00:00Z", ev.EventTimestamp)
	assert.Equal(t, 7, ev.CacheCreationTokens)
	assert.Equal(t, "reject", ev.Decision)
	assert.Equal(t, "user", ev.Source)
	// error.message overrides error
	assert.Equal(t, "overridden-error", ev.Error)
	assert.Equal(t, 42, ev.PromptLength)
	assert.Equal(t, "hello", ev.Prompt)
	assert.Equal(t, 2, ev.Attempt)
	assert.Equal(t, "python", ev.Language)
	assert.Equal(t, 99, ev.ReasoningTokens)
	assert.Equal(t, "openai", ev.Provider)
	assert.Equal(t, "call-1", ev.CallID)
	// event_kind is processed after event.kind, so it wins
	assert.Equal(t, "ek-override", ev.EventKind)
	assert.Equal(t, 3, ev.ToolTokens)
	assert.Equal(t, "gpt-5", ev.Slug)
	assert.Equal(t, "workspace", ev.SandboxPolicy)
	assert.Equal(t, []string{"a"}, ev.MCPServers)
	assert.Equal(t, "default", ev.Profile)
	assert.Equal(t, "brief", ev.ReasoningSummary)
	assert.Equal(t, 1000, ev.MaxOutputTokens)
	assert.Equal(t, 2000, ev.AutoCompactTokenLimit)
	assert.Equal(t, "done", ev.ToolOutput)
	assert.True(t, ev.PromptEncrypted)
	// overrides
	assert.Equal(t, "log-user", ev.UserID)
	assert.Equal(t, "log@e", ev.UserEmail)
	assert.Equal(t, "log-session", ev.SessionID)
	assert.Equal(t, "log-app", ev.AppVersion)
	assert.Equal(t, "log-org", ev.OrganizationID)
	assert.Equal(t, "log-acct", ev.UserAccountUUID)
	assert.Equal(t, "log-term", ev.TerminalType)
}

func TestParseLogRecord_CamelCaseCodexAliases(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	lr := &logsv1.LogRecord{
		TimeUnixNano: 1_000_000_000,
		Attributes: []*commonv1.KeyValue{
			kv("event.name", strVal("codex.tool_result")),
			kv("input_token_count", intVal(5)),
			kv("output_token_count", intVal(6)),
			kv("cachedTokenCount", intVal(7)),
			kv("reasoningTokenCount", intVal(8)),
			kv("providerName", strVal("openai")),
			kv("callId", strVal("c-2")),
			kv("toolTokens", intVal(9)),
			kv("authMode", strVal("oauth")),
			kv("contextWindow", intVal(64000)),
			kv("approvalPolicy", strVal("manual")),
			kv("sandboxPolicy", strVal("none")),
			kv("activeProfile", strVal("p")),
			kv("reasoningEnabled", boolVal(true)),
			kv("reasoningEffort", strVal("low")),
			kv("reasoningSummary", strVal("s")),
			kv("maxOutputTokens", intVal(100)),
			kv("autoCompactTokenLimit", intVal(200)),
			kv("toolOutput", strVal("ok")),
			kv("promptEncrypted", boolVal(true)),
			kv("conversationId", strVal("conv-camel")),
		},
	}

	ev := p.parseLogRecord(lr, &model.AICodeOtelResourceAttributes{}, model.AICodeOtelSourceCodex)
	require.NotNil(t, ev)
	assert.Equal(t, 5, ev.InputTokens)
	assert.Equal(t, 6, ev.OutputTokens)
	assert.Equal(t, 7, ev.CacheReadTokens)
	assert.Equal(t, 8, ev.ReasoningTokens)
	assert.Equal(t, "openai", ev.Provider)
	assert.Equal(t, "c-2", ev.CallID)
	assert.Equal(t, 9, ev.ToolTokens)
	assert.Equal(t, "oauth", ev.AuthMode)
	assert.Equal(t, 64000, ev.ContextWindow)
	assert.Equal(t, "manual", ev.ApprovalPolicy)
	assert.Equal(t, "none", ev.SandboxPolicy)
	assert.Equal(t, "p", ev.Profile)
	assert.True(t, ev.ReasoningEnabled)
	assert.Equal(t, "low", ev.ReasoningEffort)
	assert.Equal(t, "s", ev.ReasoningSummary)
	assert.Equal(t, 100, ev.MaxOutputTokens)
	assert.Equal(t, 200, ev.AutoCompactTokenLimit)
	assert.Equal(t, "ok", ev.ToolOutput)
	assert.True(t, ev.PromptEncrypted)
	// conversationId -> ConversationID and, since SessionID empty, -> SessionID
	assert.Equal(t, "conv-camel", ev.ConversationID)
	assert.Equal(t, "conv-camel", ev.SessionID)
}

func TestParseLogRecord_NilWhenNoEventType(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	attrs := &model.AICodeOtelResourceAttributes{}
	lr := &logsv1.LogRecord{Attributes: []*commonv1.KeyValue{kv("model", strVal("x"))}}
	assert.Nil(t, p.parseLogRecord(lr, attrs, model.AICodeOtelSourceClaudeCode))
}

func TestParseMetric_UnknownReturnsEmpty(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	m := &metricsv1.Metric{Name: "nope"}
	got := p.parseMetric(m, &model.AICodeOtelResourceAttributes{}, model.AICodeOtelSourceClaudeCode)
	assert.Empty(t, got)
}

func TestGetDataPointValue(t *testing.T) {
	assert.Equal(t, 1.5, getDataPointValue(&metricsv1.NumberDataPoint{Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 1.5}}))
	assert.Equal(t, float64(7), getDataPointValue(&metricsv1.NumberDataPoint{Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 7}}))
	assert.Equal(t, float64(0), getDataPointValue(&metricsv1.NumberDataPoint{}))
}

func TestApplyMetricAttribute(t *testing.T) {
	t.Run("decision/tool/language and identifiers", func(t *testing.T) {
		m := &model.AICodeOtelMetric{}
		applyMetricAttribute(m, kv("tool", strVal("Edit")), model.AICodeMetricCodeEditToolDecision)
		applyMetricAttribute(m, kv("decision", strVal("accept")), model.AICodeMetricCodeEditToolDecision)
		applyMetricAttribute(m, kv("language", strVal("go")), model.AICodeMetricCodeEditToolDecision)
		applyMetricAttribute(m, kv("user.id", strVal("u1")), model.AICodeMetricCodeEditToolDecision)
		applyMetricAttribute(m, kv("user.email", strVal("u@e")), model.AICodeMetricCodeEditToolDecision)
		applyMetricAttribute(m, kv("organization.id", strVal("o1")), model.AICodeMetricCodeEditToolDecision)
		applyMetricAttribute(m, kv("os.type", strVal("linux")), model.AICodeMetricCodeEditToolDecision)
		applyMetricAttribute(m, kv("host.arch", strVal("arm64")), model.AICodeMetricCodeEditToolDecision)
		assert.Equal(t, "Edit", m.Tool)
		assert.Equal(t, "accept", m.Decision)
		assert.Equal(t, "go", m.Language)
		assert.Equal(t, "u1", m.UserID)
		assert.Equal(t, "u@e", m.UserEmail)
		assert.Equal(t, "o1", m.OrganizationID)
		assert.Equal(t, "linux", m.OSType)
		assert.Equal(t, "arm64", m.HostArch)
	})

	t.Run("type maps to TokenType for token metric", func(t *testing.T) {
		m := &model.AICodeOtelMetric{}
		applyMetricAttribute(m, kv("type", strVal("output")), model.AICodeMetricTokenUsage)
		assert.Equal(t, "output", m.TokenType)
		assert.Empty(t, m.LinesType)
	})

	t.Run("type maps to LinesType for lines metric", func(t *testing.T) {
		m := &model.AICodeOtelMetric{}
		applyMetricAttribute(m, kv("type", strVal("removed")), model.AICodeMetricLinesOfCodeCount)
		assert.Equal(t, "removed", m.LinesType)
		assert.Empty(t, m.TokenType)
	})
}

func TestDetectProject(t *testing.T) {
	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})

	t.Run("from resource project attr", func(t *testing.T) {
		res := serviceResource("claude-code", kv("project", strVal("proj-a")))
		assert.Equal(t, "proj-a", p.detectProject(res, model.AICodeOtelSourceClaudeCode))
	})

	t.Run("from resource project.path attr", func(t *testing.T) {
		res := serviceResource("claude-code", kv("project.path", strVal("/p/b")))
		assert.Equal(t, "/p/b", p.detectProject(res, model.AICodeOtelSourceClaudeCode))
	})

	t.Run("claude env fallback", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_PROJECT", "claude-env")
		t.Setenv("PWD", "/should/not/use")
		res := serviceResource("claude-code")
		assert.Equal(t, "claude-env", p.detectProject(res, model.AICodeOtelSourceClaudeCode))
	})

	t.Run("codex env fallback", func(t *testing.T) {
		t.Setenv("CODEX_PROJECT", "codex-env")
		res := serviceResource("codex-cli")
		assert.Equal(t, "codex-env", p.detectProject(res, model.AICodeOtelSourceCodex))
	})

	t.Run("pwd fallback", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_PROJECT", "")
		t.Setenv("PWD", "/cwd/here")
		res := serviceResource("claude-code")
		assert.Equal(t, "/cwd/here", p.detectProject(res, model.AICodeOtelSourceClaudeCode))
	})

	t.Run("unknown fallback", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_PROJECT", "")
		t.Setenv("CODEX_PROJECT", "")
		t.Setenv("PWD", "")
		res := serviceResource("claude-code")
		assert.Equal(t, "unknown", p.detectProject(res, model.AICodeOtelSourceClaudeCode))
	})

	t.Run("nil resource pwd", func(t *testing.T) {
		t.Setenv("PWD", "/nilres")
		assert.Equal(t, "/nilres", p.detectProject(nil, model.AICodeOtelSourceClaudeCode))
	})
}

func TestWriteDebugFile(t *testing.T) {
	// Debug mode triggers writeDebugFile; verify the file is written.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // os.TempDir() honors TMPDIR on linux/darwin

	debugTrue := true
	cp := newCaptureProcessor(t, model.ShellTimeConfig{
		Token:      "tok",
		AICodeOtel: &model.AICodeOtel{Debug: &debugTrue},
	})
	require.True(t, cp.processor.debug)

	req := &collmetricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricsv1.ResourceMetrics{
			{
				Resource: serviceResource("claude-code"),
				ScopeMetrics: []*metricsv1.ScopeMetrics{
					{Metrics: []*metricsv1.Metric{{
						Name: "claude_code.session.count",
						Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{
							DataPoints: []*metricsv1.NumberDataPoint{{Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 1}}},
						}},
					}}},
				},
			},
		},
	}
	_, err := cp.processor.ProcessMetrics(context.Background(), req)
	require.NoError(t, err)

	debugPath := filepath.Join(tmp, "shelltime", "aicode-otel-debug-metrics.txt")
	data, err := os.ReadFile(debugPath)
	require.NoError(t, err, "debug file should be written when debug=true")
	assert.True(t, strings.Contains(string(data), "resourceMetrics") || len(data) > 0)
}

func TestWriteDebugFile_Direct(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	p := NewAICodeOtelProcessor(model.ShellTimeConfig{})
	p.writeDebugFile("direct-debug.txt", map[string]string{"hello": "world"})

	data, err := os.ReadFile(filepath.Join(tmp, "shelltime", "direct-debug.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "hello")
	assert.Contains(t, string(data), "world")
}

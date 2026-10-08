package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collmetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewAICodeOtelServer(t *testing.T) {
	proc := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "t"})
	server := NewAICodeOtelServer(54027, proc)
	require.NotNil(t, server)
	assert.Equal(t, 54027, server.port)
	assert.Same(t, proc, server.processor)
}

func TestAICodeOtelServer_StartStopLifecycle(t *testing.T) {
	proc := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "t"})
	// Port 0 -> OS assigns an ephemeral free port.
	server := NewAICodeOtelServer(0, proc)

	require.NoError(t, server.Start())
	require.NotNil(t, server.listener)

	// Stop must complete promptly without hanging.
	done := make(chan struct{})
	go func() {
		server.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not complete in time")
	}
}

func TestAICodeOtelServer_StopBeforeStart(t *testing.T) {
	server := NewAICodeOtelServer(0, NewAICodeOtelProcessor(model.ShellTimeConfig{}))
	// grpcServer is nil; Stop must be a safe no-op.
	assert.NotPanics(t, server.Stop)
}

func TestAICodeOtelServer_ExportRoundTrip(t *testing.T) {
	// Backend HTTP server the processor forwards to. We use empty requests so
	// no metrics/events are produced and the backend is never actually hit, but
	// the gRPC Export handlers are exercised end-to-end.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proc := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "t", APIEndpoint: backend.URL})
	server := NewAICodeOtelServer(0, proc)
	require.NoError(t, server.Start())
	defer server.Stop()

	addr := server.listener.Addr().String()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	metricsClient := collmetricsv1.NewMetricsServiceClient(conn)
	mResp, err := metricsClient.Export(ctx, &collmetricsv1.ExportMetricsServiceRequest{})
	require.NoError(t, err)
	require.NotNil(t, mResp)

	logsClient := collogsv1.NewLogsServiceClient(conn)
	lResp, err := logsClient.Export(ctx, &collogsv1.ExportLogsServiceRequest{})
	require.NoError(t, err)
	require.NotNil(t, lResp)
}

// rawBackend records the raw JSON bodies the processor POSTs.
type rawBackend struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies []string
}

func newRawBackend(t *testing.T) *rawBackend {
	t.Helper()
	b := &rawBackend{}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		b.mu.Lock()
		b.bodies = append(b.bodies, buf.String())
		b.mu.Unlock()
		_ = json.NewEncoder(w).Encode(model.AICodeOtelResponse{Success: true})
	}))
	t.Cleanup(b.server.Close)
	return b
}

func (b *rawBackend) captured() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.bodies...)
}

func startTestOtelServer(t *testing.T, backendURL string) *grpc.ClientConn {
	t.Helper()
	proc := NewAICodeOtelProcessor(model.ShellTimeConfig{Token: "t", APIEndpoint: backendURL})
	server := NewAICodeOtelServer(0, proc)
	require.NoError(t, server.Start())
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(server.listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestAICodeOtelServer_ExportRoundTripPayload sends spec-shaped Claude Code and Codex exports
// over gRPC and checks the JSON the backend receives.
func TestAICodeOtelServer_ExportRoundTripPayload(t *testing.T) {
	backend := newRawBackend(t)
	conn := startTestOtelServer(t, backend.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logs := &collogsv1.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{
		{
			Resource: claudeTestResource(),
			ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: []*logsv1.LogRecord{
				claudeRecord("tool_result",
					kv("tool_name", strVal("Bash")),
					kv("tool_use_id", strVal("toolu_01")),
					kv("success", strVal("false")),
					kv("duration_ms", intVal(0)),
					kv("tool_input", strVal(`{"command":"false"}`)),
				),
				claudeRecord("api_request", kv("cost_usd", intVal(0)), kv("input_tokens", intVal(0))),
			}}},
		},
		{
			Resource: codexTestResource(),
			ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: []*logsv1.LogRecord{
				codexRecord("conversation_starts", kv("mcp_servers", strVal("a, b")), kv("reasoning_enabled", boolVal(false))),
			}}},
		},
	}}
	_, err := collogsv1.NewLogsServiceClient(conn).Export(ctx, logs)
	require.NoError(t, err)

	metrics := &collmetricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricsv1.ResourceMetrics{{
		Resource: claudeTestResource(),
		ScopeMetrics: []*metricsv1.ScopeMetrics{{Metrics: []*metricsv1.Metric{
			sumMetric("claude_code.active_time.total", metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, 12, kv("type", strVal("user"))),
		}}},
	}}}
	_, err = collmetricsv1.NewMetricsServiceClient(conn).Export(ctx, metrics)
	require.NoError(t, err)

	bodies := backend.captured()
	require.Len(t, bodies, 3) // one per resource (claude logs, codex logs, claude metrics)
	claudeLogs, codexLogs, claudeMetrics := bodies[0], bodies[1], bodies[2]

	// False and zero values are sent, not omitted.
	assert.Contains(t, claudeLogs, `"success":false`)
	assert.Contains(t, claudeLogs, `"durationMs":0`)
	assert.Contains(t, claudeLogs, `"costUsd":0`)
	assert.Contains(t, claudeLogs, `"inputTokens":0`)
	assert.Contains(t, claudeLogs, `"timestampMs":1791446400123`)
	assert.Contains(t, claudeLogs, `"timestamp":1791446400,`, "seconds are kept for older servers")
	assert.Contains(t, claudeLogs, `"sequence":41`)
	assert.Contains(t, claudeLogs, `"promptId":"6f1c1d6e-prompt"`)
	assert.Contains(t, claudeLogs, `"callId":"toolu_01"`)
	assert.Contains(t, claudeLogs, `"toolInput":"{\"command\":\"false\"}"`)
	assert.Contains(t, claudeLogs, `"entrypoint":"cli"`)
	assert.Contains(t, claudeLogs, `"eventId":"ot1:`)

	var claudeReq struct {
		Events []map[string]any `json:"events"`
	}
	require.NoError(t, json.Unmarshal([]byte(claudeLogs), &claudeReq))
	require.Len(t, claudeReq.Events, 2)
	attrs, _ := claudeReq.Events[0]["attributes"].(map[string]any)
	assert.Equal(t, "https://github.com/example-org/example-repo", attrs["vcs.repository.url.full"])

	assert.Contains(t, codexLogs, `"mcpServers":["a","b"]`)
	assert.Contains(t, codexLogs, `"reasoningEnabled":false`)
	assert.Contains(t, codexLogs, `"sessionId":"conv-1"`)
	assert.Contains(t, codexLogs, `"machineName":"dev-box"`)

	assert.Contains(t, claudeMetrics, `"timestampMs":1791446400123`)
	assert.Contains(t, claudeMetrics, `"attributes":{"type":"user"}`)
	assert.Contains(t, claudeMetrics, `"metricId":"ot1:`)
	assert.NotContains(t, claudeMetrics, `"tokenType"`)
}

// TestAICodeOtelServer_AcceptsLargeExports checks the raised receive limit: gRPC's 4 MB default
// would reject this export.
func TestAICodeOtelServer_AcceptsLargeExports(t *testing.T) {
	backend := newRawBackend(t)
	conn := startTestOtelServer(t, backend.server.URL)

	var records []*logsv1.LogRecord
	for i := 0; i < 100; i++ {
		records = append(records, codexRecord("tool_result",
			kv("call_id", strVal(strings.Repeat("c", 8))),
			kv("output", strVal(strings.Repeat("o", 60<<10))),
		))
	}
	req := &collogsv1.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{{
		Resource:  codexTestResource(),
		ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: records}},
	}}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := collogsv1.NewLogsServiceClient(conn).Export(ctx, req)
	require.NoError(t, err)

	events := 0
	for _, body := range backend.captured() {
		var got model.AICodeOtelRequest
		require.NoError(t, json.Unmarshal([]byte(body), &got))
		events += len(got.Events)
	}
	assert.Equal(t, 100, events)
}

package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colltracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Fixtures shaped after the Claude Code tracing reference (monitoring-usage.md, "Traces"): every
// span carries the standard attributes plus span.type, and tool spans have blocked_on_user and
// execution children.

var (
	testTraceID = []byte{0x0a, 0xf7, 0x65, 0x19, 0x16, 0xcd, 0x43, 0xdd, 0x84, 0x48, 0xeb, 0x21, 0x1c, 0x80, 0x31, 0x9c}
	testRootID  = []byte{0xb7, 0xad, 0x6b, 0x71, 0x69, 0x20, 0x33, 0x31}
	testLLMID   = []byte{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}
)

func claudeSpan(name string, spanID, parentID []byte, startOffset, duration time.Duration, attrs ...*commonv1.KeyValue) *tracev1.Span {
	base := []*commonv1.KeyValue{
		kv("span.type", strVal(name)),
		kv("session.id", strVal("sess-claude")),
		kv("app.version", strVal("2.1.300")),
		kv("organization.id", strVal("org-1")),
		kv("user.account_uuid", strVal("acct-uuid")),
		kv("user.id", strVal("anon-1")),
		kv("user.email", strVal("dev@example.com")),
		kv("terminal.type", strVal("iTerm.app")),
	}
	start := uint64(v2TestTimeNano) + uint64(startOffset)
	return &tracev1.Span{
		TraceId:           testTraceID,
		SpanId:            spanID,
		ParentSpanId:      parentID,
		Name:              name,
		Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: start,
		EndTimeUnixNano:   start + uint64(duration),
		Attributes:        append(base, attrs...),
	}
}

func parseTestSpan(t *testing.T, resource *resourcev1.Resource, s *tracev1.Span) *model.AICodeOtelSpan {
	t.Helper()
	source := detectOtelSource(resource)
	require.NotEmpty(t, source)
	return parseSpan(s, source, extractResourceAttributes(resource))
}

func TestParseSpan_ClaudeInteraction(t *testing.T) {
	span := parseTestSpan(t, claudeTestResource(), claudeSpan("claude_code.interaction", testRootID, nil, 0, 4200*time.Millisecond,
		kv("user_prompt", strVal("fix the failing test")),
		kv("user_prompt_length", intVal(20)),
		kv("interaction.sequence", intVal(3)),
		kv("interaction.duration_ms", intVal(4200)),
	))
	require.NotNil(t, span)

	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", span.TraceID)
	assert.Equal(t, "b7ad6b7169203331", span.SpanID)
	assert.Empty(t, span.ParentSpanID, "a root span has no parent")
	assert.Equal(t, "claude_code.interaction", span.Name)
	assert.Equal(t, "internal", span.Kind)
	assert.Equal(t, int64(v2TestTimeNano), span.StartTimeUnixNano)
	assert.Equal(t, int64(v2TestTimeNano)+int64(4200*time.Millisecond), span.EndTimeUnixNano)
	assert.Empty(t, span.StatusCode, "unset status")
	assert.Equal(t, "sess-claude", span.SessionID)
	assert.Equal(t, model.AICodeOtelSourceClaudeCode, span.ClientType)

	assert.Equal(t, "fix the failing test", span.Attributes["user_prompt"])
	assert.Equal(t, int64(3), span.Attributes["interaction.sequence"])
	assert.Equal(t, "claude_code.interaction", span.Attributes["span.type"])
	assert.Equal(t, "2.1.300", span.Attributes["app.version"])
	assert.Equal(t, "iTerm.app", span.Attributes["terminal.type"])
	for _, key := range []string{"session.id", "user.email", "user.id", "user.account_uuid", "organization.id"} {
		assert.NotContains(t, span.Attributes, key, "the session identity isn't repeated per span")
	}
}

func TestParseSpan_ClaudeFailedLLMRequestWithEvents(t *testing.T) {
	s := claudeSpan("claude_code.llm_request", testLLMID, testRootID, 10*time.Millisecond, 900*time.Millisecond,
		kv("model", strVal("claude-opus-5-5")),
		kv("success", boolVal(false)),
		kv("status_code", intVal(529)),
		kv("error_class", strVal("server_overload")),
	)
	s.Status = &tracev1.Status{Code: tracev1.Status_STATUS_CODE_ERROR, Message: "Overloaded"}
	s.Kind = tracev1.Span_SPAN_KIND_CLIENT
	s.Events = []*tracev1.Span_Event{
		{Name: "gen_ai.request.attempt", TimeUnixNano: uint64(v2TestTimeNano) + uint64(20*time.Millisecond), Attributes: []*commonv1.KeyValue{
			kv("attempt", intVal(2)),
			kv("client_request_id", strVal("req-2")),
		}},
	}

	span := parseTestSpan(t, claudeTestResource(), s)
	require.NotNil(t, span)
	assert.Equal(t, "b7ad6b7169203331", span.ParentSpanID)
	assert.Equal(t, "client", span.Kind)
	assert.Equal(t, "error", span.StatusCode)
	assert.Equal(t, "Overloaded", span.StatusMessage)
	assert.Equal(t, false, span.Attributes["success"])
	assert.Equal(t, int64(529), span.Attributes["status_code"])

	require.Len(t, span.Events, 1)
	assert.Equal(t, "gen_ai.request.attempt", span.Events[0].Name)
	assert.Equal(t, int64(v2TestTimeNano)+int64(20*time.Millisecond), span.Events[0].TimeUnixNano)
	assert.Equal(t, int64(2), span.Events[0].Attributes["attempt"])
	assert.Equal(t, "req-2", span.Events[0].Attributes["client_request_id"])
}

func TestParseSpan_InvalidIDsAndTimes(t *testing.T) {
	res := claudeTestResource()

	noTrace := claudeSpan("claude_code.tool", testLLMID, nil, 0, time.Second)
	noTrace.TraceId = make([]byte, 16)
	assert.Nil(t, parseTestSpan(t, res, noTrace), "an all-zero trace id is invalid")

	shortSpan := claudeSpan("claude_code.tool", []byte{1, 2, 3}, nil, 0, time.Second)
	assert.Nil(t, parseTestSpan(t, res, shortSpan), "a span id must be 8 bytes")

	zeroParent := claudeSpan("claude_code.tool", testLLMID, make([]byte, 8), 0, time.Second)
	span := parseTestSpan(t, res, zeroParent)
	require.NotNil(t, span)
	assert.Empty(t, span.ParentSpanID, "an all-zero parent means no parent")

	backwards := claudeSpan("claude_code.tool", testLLMID, nil, time.Second, 0)
	backwards.EndTimeUnixNano = backwards.StartTimeUnixNano - 1
	span = parseTestSpan(t, res, backwards)
	require.NotNil(t, span)
	assert.Equal(t, span.StartTimeUnixNano, span.EndTimeUnixNano, "an end before the start is clamped")
}

func TestParseSpan_EventsAreCapped(t *testing.T) {
	s := claudeSpan("claude_code.llm_request", testLLMID, testRootID, 0, time.Second)
	for i := 0; i < model.AICodeOtelMaxSpanEvents+5; i++ {
		s.Events = append(s.Events, &tracev1.Span_Event{Name: "gen_ai.request.attempt", TimeUnixNano: uint64(v2TestTimeNano)})
	}
	span := parseTestSpan(t, claudeTestResource(), s)
	require.NotNil(t, span)
	assert.Len(t, span.Events, model.AICodeOtelMaxSpanEvents)
}

func TestParseSpan_KindAndStatus(t *testing.T) {
	testCases := []struct {
		kind       tracev1.Span_SpanKind
		status     tracev1.Status_StatusCode
		wantKind   string
		wantStatus string
	}{
		{tracev1.Span_SPAN_KIND_UNSPECIFIED, tracev1.Status_STATUS_CODE_UNSET, "", ""},
		{tracev1.Span_SPAN_KIND_INTERNAL, tracev1.Status_STATUS_CODE_OK, "internal", "ok"},
		{tracev1.Span_SPAN_KIND_SERVER, tracev1.Status_STATUS_CODE_ERROR, "server", "error"},
		{tracev1.Span_SPAN_KIND_CLIENT, tracev1.Status_STATUS_CODE_OK, "client", "ok"},
		{tracev1.Span_SPAN_KIND_PRODUCER, tracev1.Status_STATUS_CODE_UNSET, "producer", ""},
		{tracev1.Span_SPAN_KIND_CONSUMER, tracev1.Status_STATUS_CODE_UNSET, "consumer", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.kind.String()+"/"+tc.status.String(), func(t *testing.T) {
			s := claudeSpan("claude_code.tool", testLLMID, testRootID, 0, time.Second)
			s.Kind = tc.kind
			s.Status = &tracev1.Status{Code: tc.status}
			span := parseTestSpan(t, claudeTestResource(), s)
			require.NotNil(t, span)
			assert.Equal(t, tc.wantKind, span.Kind)
			assert.Equal(t, tc.wantStatus, span.StatusCode)
		})
	}
}

func TestParseSpan_NamesAreCapped(t *testing.T) {
	long := strings.Repeat("n", model.AICodeOtelMaxAttributeBytes*3)
	s := claudeSpan(long, testLLMID, testRootID, 0, time.Second)
	s.Events = []*tracev1.Span_Event{{Name: long, TimeUnixNano: uint64(v2TestTimeNano)}}

	span := parseTestSpan(t, claudeTestResource(), s)
	require.NotNil(t, span)
	assert.Len(t, span.Name, model.AICodeOtelMaxAttributeBytes)
	require.Len(t, span.Events, 1)
	assert.Len(t, span.Events[0].Name, model.AICodeOtelMaxAttributeBytes)
}

func TestParseSpan_CodexConversationID(t *testing.T) {
	s := &tracev1.Span{
		TraceId:           testTraceID,
		SpanId:            testRootID,
		Name:              "session_loop",
		StartTimeUnixNano: uint64(v2TestTimeNano),
		EndTimeUnixNano:   uint64(v2TestTimeNano) + uint64(time.Second),
		Attributes:        []*commonv1.KeyValue{kv("conversation.id", strVal("conv-1"))},
	}
	span := parseTestSpan(t, codexTestResource(), s)
	require.NotNil(t, span)
	assert.Equal(t, "conv-1", span.SessionID)
	assert.Equal(t, model.AICodeOtelSourceCodex, span.ClientType)
	assert.NotContains(t, span.Attributes, "conversation.id")
}

func TestFillSpanSessionIDs(t *testing.T) {
	spans := []model.AICodeOtelSpan{
		{TraceID: "t1", SpanID: "a"},
		{TraceID: "t1", SpanID: "b", SessionID: "sess-1"},
		{TraceID: "t2", SpanID: "c"},
		{TraceID: "t3", SpanID: "d", SessionID: "sess-3"},
	}
	fillSpanSessionIDs(spans)
	assert.Equal(t, "sess-1", spans[0].SessionID, "taken from a span of the same trace")
	assert.Equal(t, "sess-1", spans[1].SessionID)
	assert.Empty(t, spans[2].SessionID, "no span of the trace names a session")
	assert.Equal(t, "sess-3", spans[3].SessionID)
}

// TestAICodeOtelServer_TraceExportRoundTrip sends a Claude Code trace over gRPC and checks the
// JSON the backend receives.
func TestAICodeOtelServer_TraceExportRoundTrip(t *testing.T) {
	backend := newRawBackend(t)
	conn := startTestOtelServer(t, backend.server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	llm := claudeSpan("claude_code.llm_request", testLLMID, testRootID, 5*time.Millisecond, 800*time.Millisecond,
		kv("model", strVal("claude-opus-5-5")))
	// Without its own session.id, it takes the interaction's.
	attrs := llm.Attributes[:0]
	for _, attr := range llm.Attributes {
		if attr.GetKey() != "session.id" {
			attrs = append(attrs, attr)
		}
	}
	llm.Attributes = attrs
	req := &colltracev1.ExportTraceServiceRequest{ResourceSpans: []*tracev1.ResourceSpans{
		{
			Resource: claudeTestResource(),
			ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{
				claudeSpan("claude_code.interaction", testRootID, nil, 0, time.Second),
				llm,
			}}},
		},
		{
			Resource:   serviceResource("some-other-service"),
			ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{claudeSpan("other", testRootID, nil, 0, time.Second)}}},
		},
	}}
	resp, err := colltracev1.NewTraceServiceClient(conn).Export(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	bodies := backend.captured()
	require.Len(t, bodies, 1, "only the Claude Code resource is forwarded")
	var got model.AICodeOtelRequest
	require.NoError(t, json.Unmarshal([]byte(bodies[0]), &got))
	assert.Equal(t, model.AICodeOtelSourceClaudeCode, got.Source)
	assert.Empty(t, got.Events)
	assert.Empty(t, got.Metrics)
	require.Len(t, got.Spans, 2)
	assert.Equal(t, "claude_code.interaction", got.Spans[0].Name)
	assert.Equal(t, "claude_code.llm_request", got.Spans[1].Name)
	assert.Equal(t, "b7ad6b7169203331", got.Spans[1].ParentSpanID)
	assert.Equal(t, "sess-claude", got.Spans[1].SessionID)
	assert.Equal(t, "claude-opus-5-5", got.Spans[1].Attributes["model"])
	assert.Equal(t, int64(v2TestTimeNano)+int64(5*time.Millisecond), got.Spans[1].StartTimeUnixNano, "nanoseconds survive JSON")
}

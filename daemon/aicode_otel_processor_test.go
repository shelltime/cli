package daemon

import (
	"math"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestNewAICodeOtelProcessor(t *testing.T) {
	config := model.ShellTimeConfig{
		Token:       "test-token",
		APIEndpoint: "http://localhost:8080",
	}

	processor := NewAICodeOtelProcessor(config)
	if processor == nil {
		t.Fatal("NewAICodeOtelProcessor returned nil")
	}

	if processor.endpoint.Token != "test-token" {
		t.Errorf("Token mismatch")
	}
	if processor.endpoint.APIEndpoint != "http://localhost:8080" {
		t.Errorf("APIEndpoint mismatch")
	}
	if processor.hostname == "" {
		t.Error("hostname should not be empty")
	}
}

func TestNewAICodeOtelProcessor_Debug(t *testing.T) {
	debug := true
	config := model.ShellTimeConfig{
		Token: "token",
		AICodeOtel: &model.AICodeOtel{
			Debug: &debug,
		},
	}

	processor := NewAICodeOtelProcessor(config)
	if !processor.debug {
		t.Error("debug should be true when configured")
	}
}

func TestDetectOtelSource(t *testing.T) {
	testCases := []struct {
		name         string
		serviceName  string
		expectedType string
	}{
		{"claude code", "claude-code", model.AICodeOtelSourceClaudeCode},
		{"claude", "claude", model.AICodeOtelSourceClaudeCode},
		{"codex", "codex-cli", model.AICodeOtelSourceCodex},
		{"unknown", "vscode", ""},
		{"empty", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resource := &resourcev1.Resource{
				Attributes: []*commonv1.KeyValue{
					{
						Key: "service.name",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{StringValue: tc.serviceName},
						},
					},
				},
			}

			result := detectOtelSource(resource)
			if result != tc.expectedType {
				t.Errorf("Expected %s, got %s", tc.expectedType, result)
			}
		})
	}
}

func TestDetectOtelSource_NilResource(t *testing.T) {
	result := detectOtelSource(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil resource, got %s", result)
	}
}

func TestExtractResourceAttributes(t *testing.T) {
	resource := &resourcev1.Resource{
		Attributes: []*commonv1.KeyValue{
			{Key: "session.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "session-123"}}},
			{Key: "conversation.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "conv-456"}}},
			{Key: "app.version", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "1.0.0"}}},
			{Key: "organization.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "org-789"}}},
			{Key: "user.account_uuid", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "user-abc"}}},
			{Key: "terminal.type", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "terminal"}}},
			{Key: "os.type", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "linux"}}},
			{Key: "os.version", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "5.15"}}},
			{Key: "host.arch", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "amd64"}}},
			{Key: "user.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "user123"}}},
			{Key: "user.email", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "user@test.com"}}},
			{Key: "user.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "testuser"}}},
			{Key: "machine.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "workstation"}}},
			{Key: "team.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "team-xyz"}}},
			{Key: "pwd", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "/home/user/project"}}},
		},
	}

	attrs := extractResourceAttributes(resource)

	if attrs.SessionID != "session-123" {
		t.Errorf("SessionID mismatch")
	}
	if attrs.ConversationID != "conv-456" {
		t.Errorf("ConversationID mismatch")
	}
	if attrs.AppVersion != "1.0.0" {
		t.Errorf("AppVersion mismatch")
	}
	if attrs.OrganizationID != "org-789" {
		t.Errorf("OrganizationID mismatch")
	}
	if attrs.UserAccountUUID != "user-abc" {
		t.Errorf("UserAccountUUID mismatch")
	}
	if attrs.TerminalType != "terminal" {
		t.Errorf("TerminalType mismatch")
	}
	if attrs.OSType != "linux" {
		t.Errorf("OSType mismatch")
	}
	if attrs.OSVersion != "5.15" {
		t.Errorf("OSVersion mismatch")
	}
	if attrs.HostArch != "amd64" {
		t.Errorf("HostArch mismatch")
	}
	if attrs.UserID != "user123" {
		t.Errorf("UserID mismatch")
	}
	if attrs.UserEmail != "user@test.com" {
		t.Errorf("UserEmail mismatch")
	}
	if attrs.UserName != "testuser" {
		t.Errorf("UserName mismatch")
	}
	if attrs.MachineName != "workstation" {
		t.Errorf("MachineName mismatch")
	}
	if attrs.TeamID != "team-xyz" {
		t.Errorf("TeamID mismatch")
	}
	if attrs.Pwd != "/home/user/project" {
		t.Errorf("Pwd mismatch")
	}
}

func TestExtractResourceAttributes_NilResource(t *testing.T) {
	attrs := extractResourceAttributes(nil)
	if attrs == nil {
		t.Fatal("Should return empty struct, not nil")
	}
	if attrs.SessionID != "" {
		t.Error("SessionID should be empty")
	}
}

func TestMapMetricName_ClaudeCode(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"claude_code.session.count", model.AICodeMetricSessionCount},
		{"claude_code.token.usage", model.AICodeMetricTokenUsage},
		{"claude_code.cost.usage", model.AICodeMetricCostUsage},
		{"claude_code.lines_of_code.count", model.AICodeMetricLinesOfCodeCount},
		{"claude_code.commit.count", model.AICodeMetricCommitCount},
		{"claude_code.pull_request.count", model.AICodeMetricPullRequestCount},
		{"claude_code.active_time.total", model.AICodeMetricActiveTimeTotal},
		{"claude_code.code_edit_tool.decision", model.AICodeMetricCodeEditToolDecision},
		{"unknown.metric", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, mapMetricName(tc.input))
		})
	}
}

// Codex exports no such metrics (its OTLP metrics carry no session id), so the
// former codex.* aliases are gone.
func TestMapMetricName_CodexNamesAreNotMapped(t *testing.T) {
	for _, name := range []string{
		"codex.session.count", "codex.token.usage", "codex.cost.usage", "codex.lines_of_code.count",
		"codex.commit.count", "codex.pull_request.count", "codex.active_time.total", "codex.tool.call",
	} {
		assert.Empty(t, mapMetricName(name), name)
	}
}

func TestNormalizeOtelEventName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"claude_code.user_prompt", model.AICodeEventUserPrompt},
		{"user_prompt", model.AICodeEventUserPrompt}, // Claude's event.name attribute has no prefix
		{"claude_code.tool_result", model.AICodeEventToolResult},
		{"claude_code.api_request", model.AICodeEventApiRequest},
		{"claude_code.api_error", model.AICodeEventApiError},
		{"claude_code.tool_decision", model.AICodeEventToolDecision},
		{"claude_code.assistant_response", model.AICodeEventAssistantResponse},
		{"claude_code.hook_execution_complete", "hook_execution_complete"},
		{"codex.user_prompt", model.AICodeEventUserPrompt},
		{"codex.tool_result", model.AICodeEventToolResult},
		{"codex.exec_command", model.AICodeEventExecCommand},
		{"codex.conversation_starts", model.AICodeEventConversationStarts},
		{"codex.sse_event", model.AICodeEventSSEEvent},
		{"codex.agent_response", model.AICodeEventAgentResponse},
		{"codex.turn_cost", model.AICodeEventTurnCost},
		{" custom.event ", "custom.event"}, // unknown names pass through for the server's "other"
		// dropped
		{"claude_code.api_request_body", ""},
		{"claude_code.api_response_body", ""},
		{"claude_code.system_prompt", ""},
		{"api_request_body", ""},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizeOtelEventName(tc.input))
		})
	}
}

func arrVal(values ...*commonv1.AnyValue) *commonv1.AnyValue {
	return &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: &commonv1.ArrayValue{Values: values}}}
}

func TestOptInt(t *testing.T) {
	testCases := []struct {
		name     string
		value    *commonv1.AnyValue
		expected *int
	}{
		{"int", intVal(42), model.IntRef(42)},
		{"int zero is kept", intVal(0), model.IntRef(0)},
		{"integral double", dblVal(1200), model.IntRef(1200)},
		{"fractional double rounds", dblVal(12.6), model.IntRef(13)},
		{"numeric string", strVal("123"), model.IntRef(123)},
		{"string zero is kept", strVal("0"), model.IntRef(0)},
		{"padded string", strVal(" 7 "), model.IntRef(7)},
		{"float string", strVal("250.0"), model.IntRef(250)},
		{"invalid string", strVal("not-a-number"), nil},
		{"empty string", strVal(""), nil},
		{"bool", boolVal(true), nil},
		{"NaN", dblVal(math.NaN()), nil},
		{"nil", nil, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, optInt(tc.value))
		})
	}

	n := optInt64Ref(strVal("9007199254740993"))
	require.NotNil(t, n)
	assert.Equal(t, int64(9007199254740993), *n, "int64 strings keep full precision")
}

func TestOptBool(t *testing.T) {
	testCases := []struct {
		name     string
		value    *commonv1.AnyValue
		expected *bool
	}{
		{"bool true", boolVal(true), model.BoolRef(true)},
		{"bool false is kept", boolVal(false), model.BoolRef(false)},
		{"string true", strVal("true"), model.BoolRef(true)},
		{"string false is kept", strVal("false"), model.BoolRef(false)},
		{"string TRUE", strVal("TRUE"), model.BoolRef(true)},
		{"invalid string", strVal("maybe"), nil},
		{"int", intVal(1), nil},
		{"nil", nil, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, optBool(tc.value))
		})
	}
}

func TestOptFloat(t *testing.T) {
	testCases := []struct {
		name     string
		value    *commonv1.AnyValue
		expected *float64
	}{
		{"double", dblVal(3.14), model.Float64Ref(3.14)},
		{"double zero is kept", dblVal(0), model.Float64Ref(0)},
		{"int", intVal(1), model.Float64Ref(1)},
		{"int zero is kept", intVal(0), model.Float64Ref(0)},
		{"string", strVal("2.71"), model.Float64Ref(2.71)},
		{"invalid string", strVal("not-a-float"), nil},
		{"infinite", dblVal(math.Inf(1)), nil},
		{"bool", boolVal(true), nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, optFloat(tc.value))
		})
	}
}

func TestStringList(t *testing.T) {
	testCases := []struct {
		name     string
		value    *commonv1.AnyValue
		expected []string
	}{
		{"array", arrVal(strVal("a"), strVal("b"), strVal("c")), []string{"a", "b", "c"}},
		{"array skips empty", arrVal(strVal("a"), strVal(""), strVal(" b ")), []string{"a", "b"}},
		{"codex joined string", strVal("filesystem, github"), []string{"filesystem", "github"}},
		{"comma string", strVal("a,b"), []string{"a", "b"}},
		{"single", strVal("only"), []string{"only"}},
		{"empty string", strVal(""), nil},
		{"empty array", arrVal(), nil},
		{"int", intVal(3), nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, stringList(tc.value))
		})
	}
}

func TestAnyValueToGo(t *testing.T) {
	kvlist := &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: &commonv1.KeyValueList{Values: []*commonv1.KeyValue{
		kv("name", strVal("x")),
		kv("nested", arrVal(intVal(1), boolVal(false), dblVal(1.5))),
	}}}}

	assert.Equal(t, "s", anyValueToGo(strVal("s")))
	assert.Equal(t, int64(4), anyValueToGo(intVal(4)))
	assert.Equal(t, 2.5, anyValueToGo(dblVal(2.5)))
	assert.Equal(t, false, anyValueToGo(boolVal(false)))
	assert.Equal(t, "aGk=", anyValueToGo(&commonv1.AnyValue{Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte("hi")}}))
	assert.Equal(t, "NaN", anyValueToGo(dblVal(math.NaN())), "JSON can't encode NaN")
	assert.Nil(t, anyValueToGo(nil))
	assert.Equal(t, map[string]any{"name": "x", "nested": []any{int64(1), false, 1.5}}, anyValueToGo(kvlist))
}

func TestResolveEventName(t *testing.T) {
	testCases := []struct {
		name     string
		record   *logsv1.LogRecord
		expected string
	}{
		{
			"event.name attribute wins",
			&logsv1.LogRecord{
				EventName:  "claude_code.other",
				Body:       strVal("claude_code.body"),
				Attributes: []*commonv1.KeyValue{kv("event.name", strVal("user_prompt"))},
			},
			"user_prompt",
		},
		{"EventName field", &logsv1.LogRecord{EventName: "codex.skill_invocation", Body: strVal("x")}, "codex.skill_invocation"},
		{"string body", &logsv1.LogRecord{Body: strVal("claude_code.api_request")}, "claude_code.api_request"},
		{"free text body is no event name", &logsv1.LogRecord{Body: strVal("something went wrong")}, ""},
		{"empty event.name falls through", &logsv1.LogRecord{EventName: "x.y", Attributes: []*commonv1.KeyValue{kv("event.name", strVal(""))}}, "x.y"},
		{"nothing", &logsv1.LogRecord{}, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, resolveEventName(tc.record))
		})
	}
}

func TestApplyResourceAttributesToMetric(t *testing.T) {
	attrs := &model.AICodeOtelResourceAttributes{
		SessionID:       "session-1",
		ConversationID:  "conv-1",
		UserAccountUUID: "user-1",
		OrganizationID:  "org-1",
		TerminalType:    "terminal",
		AppVersion:      "1.0.0",
		OSType:          "linux",
		OSVersion:       "5.15",
		HostArch:        "amd64",
		UserID:          "user123",
		UserEmail:       "test@test.com",
		UserName:        "testuser",
		MachineName:     "workstation",
		TeamID:          "team-1",
		Pwd:             "/home/user",
	}

	metric := &model.AICodeOtelMetric{}
	applyResourceAttributesToMetric(metric, attrs)

	if metric.SessionID != "session-1" {
		t.Error("SessionID not applied")
	}
	if metric.ConversationID != "conv-1" {
		t.Error("ConversationID not applied")
	}
	if metric.UserAccountUUID != "user-1" {
		t.Error("UserAccountUUID not applied")
	}
	if metric.OrganizationID != "org-1" {
		t.Error("OrganizationID not applied")
	}
	if metric.TerminalType != "terminal" {
		t.Error("TerminalType not applied")
	}
	if metric.AppVersion != "1.0.0" {
		t.Error("AppVersion not applied")
	}
	if metric.OSType != "linux" {
		t.Error("OSType not applied")
	}
	if metric.OSVersion != "5.15" {
		t.Error("OSVersion not applied")
	}
	if metric.HostArch != "amd64" {
		t.Error("HostArch not applied")
	}
	if metric.UserID != "user123" {
		t.Error("UserID not applied")
	}
	if metric.UserEmail != "test@test.com" {
		t.Error("UserEmail not applied")
	}
	if metric.UserName != "testuser" {
		t.Error("UserName not applied")
	}
	if metric.MachineName != "workstation" {
		t.Error("MachineName not applied")
	}
	if metric.TeamID != "team-1" {
		t.Error("TeamID not applied")
	}
	if metric.Pwd != "/home/user" {
		t.Error("Pwd not applied")
	}
}

func TestApplyResourceAttributesToEvent(t *testing.T) {
	attrs := &model.AICodeOtelResourceAttributes{
		SessionID:       "session-1",
		ConversationID:  "conv-1",
		UserAccountUUID: "user-1",
	}

	event := &model.AICodeOtelEvent{}
	applyResourceAttributesToEvent(event, attrs)

	if event.SessionID != "session-1" {
		t.Error("SessionID not applied")
	}
	if event.ConversationID != "conv-1" {
		t.Error("ConversationID not applied")
	}
	if event.UserAccountUUID != "user-1" {
		t.Error("UserAccountUUID not applied")
	}
}

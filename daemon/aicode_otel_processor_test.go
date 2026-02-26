package daemon

import (
	"testing"

	"github.com/malamtime/cli/model"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
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
			result := mapMetricName(tc.input, model.AICodeOtelSourceClaudeCode)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestMapMetricName_Codex(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"codex.session.count", model.AICodeMetricSessionCount},
		{"codex.token.usage", model.AICodeMetricTokenUsage},
		{"codex.cost.usage", model.AICodeMetricCostUsage},
		{"codex.lines_of_code.count", model.AICodeMetricLinesOfCodeCount},
		{"codex.commit.count", model.AICodeMetricCommitCount},
		{"codex.pull_request.count", model.AICodeMetricPullRequestCount},
		{"codex.active_time.total", model.AICodeMetricActiveTimeTotal},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := mapMetricName(tc.input, model.AICodeOtelSourceCodex)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestMapEventName_ClaudeCode(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"claude_code.user_prompt", model.AICodeEventUserPrompt},
		{"claude_code.tool_result", model.AICodeEventToolResult},
		{"claude_code.api_request", model.AICodeEventApiRequest},
		{"claude_code.api_error", model.AICodeEventApiError},
		{"claude_code.tool_decision", model.AICodeEventToolDecision},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := mapEventName(tc.input, model.AICodeOtelSourceClaudeCode)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestMapEventName_Codex(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"codex.user_prompt", model.AICodeEventUserPrompt},
		{"codex.tool_result", model.AICodeEventToolResult},
		{"codex.api_request", model.AICodeEventApiRequest},
		{"codex.api_error", model.AICodeEventApiError},
		{"codex.tool_decision", model.AICodeEventToolDecision},
		{"codex.exec_command", model.AICodeEventExecCommand},
		{"codex.conversation_starts", model.AICodeEventConversationStarts},
		{"codex.sse_event", model.AICodeEventSSEEvent},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := mapEventName(tc.input, model.AICodeOtelSourceCodex)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestMapEventName_Unknown(t *testing.T) {
	// Unknown events should return as-is
	result := mapEventName("custom.event", "")
	if result != "custom.event" {
		t.Errorf("Unknown events should be returned as-is, got %s", result)
	}
}

func TestGetIntFromValue(t *testing.T) {
	testCases := []struct {
		name     string
		value    *commonv1.AnyValue
		expected int
	}{
		{
			"int value",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}},
			42,
		},
		{
			"string value",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "123"}},
			123,
		},
		{
			"invalid string",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "not-a-number"}},
			0,
		},
		{
			"empty string",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: ""}},
			0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getIntFromValue(tc.value)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestGetBoolFromValue(t *testing.T) {
	testCases := []struct {
		name     string
		value    *commonv1.AnyValue
		expected bool
	}{
		{
			"bool true",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}},
			true,
		},
		{
			"bool false",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: false}},
			false,
		},
		{
			"string true",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "true"}},
			true,
		},
		{
			"string false",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "false"}},
			false,
		},
		{
			"invalid string",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "maybe"}},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getBoolFromValue(tc.value)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGetFloatFromValue(t *testing.T) {
	testCases := []struct {
		name     string
		value    *commonv1.AnyValue
		expected float64
	}{
		{
			"double value",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 3.14}},
			3.14,
		},
		{
			"string value",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "2.71"}},
			2.71,
		},
		{
			"invalid string",
			&commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "not-a-float"}},
			0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getFloatFromValue(tc.value)
			if result != tc.expected {
				t.Errorf("Expected %f, got %f", tc.expected, result)
			}
		})
	}
}

func TestGetStringArrayFromValue(t *testing.T) {
	t.Run("valid array", func(t *testing.T) {
		value := &commonv1.AnyValue{
			Value: &commonv1.AnyValue_ArrayValue{
				ArrayValue: &commonv1.ArrayValue{
					Values: []*commonv1.AnyValue{
						{Value: &commonv1.AnyValue_StringValue{StringValue: "a"}},
						{Value: &commonv1.AnyValue_StringValue{StringValue: "b"}},
						{Value: &commonv1.AnyValue_StringValue{StringValue: "c"}},
					},
				},
			},
		}

		result := getStringArrayFromValue(value)
		if len(result) != 3 {
			t.Errorf("Expected 3 elements, got %d", len(result))
		}
		if result[0] != "a" || result[1] != "b" || result[2] != "c" {
			t.Errorf("Array content mismatch")
		}
	})

	t.Run("nil array", func(t *testing.T) {
		value := &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "not-array"}}
		result := getStringArrayFromValue(value)
		if result != nil {
			t.Error("Expected nil for non-array value")
		}
	})

	t.Run("empty array", func(t *testing.T) {
		value := &commonv1.AnyValue{
			Value: &commonv1.AnyValue_ArrayValue{
				ArrayValue: &commonv1.ArrayValue{
					Values: []*commonv1.AnyValue{},
				},
			},
		}
		result := getStringArrayFromValue(value)
		if len(result) != 0 {
			t.Errorf("Expected 0 elements, got %d", len(result))
		}
	})
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

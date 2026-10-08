package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitAICodeOtelRequest(t *testing.T) {
	req := &AICodeOtelRequest{Host: "host", Project: "/repo", Source: AICodeOtelSourceClaudeCode}
	for i := 0; i < 50; i++ {
		req.Events = append(req.Events, AICodeOtelEvent{
			EventID:   fmt.Sprintf("ot1:%02d", i),
			EventType: AICodeEventToolResult,
			ToolInput: strings.Repeat("x", 1000),
			Success:   BoolRef(false),
		})
	}
	for i := 0; i < 20; i++ {
		req.Metrics = append(req.Metrics, AICodeOtelMetric{MetricID: fmt.Sprintf("m%02d", i), MetricType: AICodeMetricTokenUsage, Value: float64(i)})
	}

	const maxBytes = 8 << 10
	chunks := SplitAICodeOtelRequest(req, maxBytes)
	require.Greater(t, len(chunks), 1)

	var eventIDs, metricIDs []string
	for _, chunk := range chunks {
		body, err := json.Marshal(chunk)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(body), maxBytes, "each body stays within the limit")
		assert.Equal(t, "host", chunk.Host)
		assert.Equal(t, "/repo", chunk.Project)
		assert.Equal(t, AICodeOtelSourceClaudeCode, chunk.Source)
		assert.True(t, len(chunk.Events) > 0 || len(chunk.Metrics) > 0, "no empty requests")
		for _, e := range chunk.Events {
			eventIDs = append(eventIDs, e.EventID)
		}
		for _, m := range chunk.Metrics {
			metricIDs = append(metricIDs, m.MetricID)
		}
	}
	require.Len(t, eventIDs, 50)
	require.Len(t, metricIDs, 20)
	for i, id := range eventIDs {
		assert.Equal(t, fmt.Sprintf("ot1:%02d", i), id, "order is kept")
	}
	for i, id := range metricIDs {
		assert.Equal(t, fmt.Sprintf("m%02d", i), id)
	}
}

func TestSplitAICodeOtelRequest_SmallRequestIsOneChunk(t *testing.T) {
	req := &AICodeOtelRequest{Host: "h", Project: "p", Events: []AICodeOtelEvent{{EventID: "a"}, {EventID: "b"}}}
	chunks := SplitAICodeOtelRequest(req, AICodeOtelMaxRequestBytes)
	require.Len(t, chunks, 1)
	assert.Equal(t, req.Events, chunks[0].Events)

	assert.Empty(t, SplitAICodeOtelRequest(&AICodeOtelRequest{Host: "h"}, AICodeOtelMaxRequestBytes))
	assert.Empty(t, SplitAICodeOtelRequest(nil, AICodeOtelMaxRequestBytes))
}

func TestSplitAICodeOtelRequest_OversizedItemGoesAlone(t *testing.T) {
	req := &AICodeOtelRequest{Events: []AICodeOtelEvent{
		{EventID: "small-1"},
		{EventID: "huge", ToolOutput: strings.Repeat("o", 4000)},
		{EventID: "small-2"},
	}}
	chunks := SplitAICodeOtelRequest(req, 1000)
	require.Len(t, chunks, 3)
	assert.Equal(t, "small-1", chunks[0].Events[0].EventID)
	assert.Equal(t, "huge", chunks[1].Events[0].EventID)
	assert.Equal(t, "small-2", chunks[2].Events[0].EventID)
}

func TestAICodeOtelEvent_PointersKeepFalseAndZero(t *testing.T) {
	body, err := json.Marshal(AICodeOtelEvent{
		EventID:      "ot1:x",
		EventType:    AICodeEventToolResult,
		Success:      BoolRef(false),
		DurationMs:   IntRef(0),
		CostUSD:      Float64Ref(0),
		Sequence:     Int64Ref(0),
		InputTokens:  IntRef(0),
		TimestampMs:  1791446400123,
		Attributes:   map[string]any{"error_type": "ShellError"},
		ToolInput:    `{"command":"ls"}`,
		PromptLength: IntRef(0),
	})
	require.NoError(t, err)
	s := string(body)
	for _, want := range []string{`"success":false`, `"durationMs":0`, `"costUsd":0`, `"sequence":0`, `"inputTokens":0`, `"promptLength":0`, `"timestampMs":1791446400123`, `"attributes":{"error_type":"ShellError"}`, `"toolInput":"{\"command\":\"ls\"}"`} {
		assert.Contains(t, s, want)
	}
	// Unset optional fields stay out.
	for _, absent := range []string{"outputTokens", "statusCode", "reasoningEnabled", "response", "promptId"} {
		assert.NotContains(t, s, `"`+absent+`"`)
	}
}

func TestIsDroppedAICodeOtelEvent(t *testing.T) {
	for _, name := range []string{"api_request_body", "api_response_body", "system_prompt"} {
		assert.True(t, IsDroppedAICodeOtelEvent(name), name)
	}
	for _, name := range []string{"api_request", "user_prompt", "claude_code.api_request_body", ""} {
		assert.False(t, IsDroppedAICodeOtelEvent(name), name)
	}
}

func TestCapAICodeText(t *testing.T) {
	assert.Equal(t, "abc", CapAICodeText("abc", 10))
	assert.Equal(t, "ab", CapAICodeText("abc", 2))
	// "é" is 2 bytes: cutting inside it backs off to the previous character boundary.
	assert.Equal(t, "a", CapAICodeText("aé", 2))
	assert.Equal(t, "", CapAICodeText("é", 1))
}

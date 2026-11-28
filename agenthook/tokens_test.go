package agenthook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursorStopTokenUsage(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

	t.Run("subtracts cache read and write from inclusive input_tokens", func(t *testing.T) {
		stdin := []byte(`{
			"hook_event_name": "stop",
			"conversation_id": "conv-1",
			"generation_id": "gen-1",
			"model": "cursor-grok-4.6-high-fast",
			"model_id": "grok-4.6",
			"status": "completed",
			"loop_count": 0,
			"input_tokens": 1180993,
			"output_tokens": 8146,
			"cache_read_tokens": 1007022,
			"cache_write_tokens": 173957
		}`)
		ev, err := Normalize("cursor", "", stdin, now, nil)
		require.NoError(t, err)
		assert.Equal(t, EventStop, ev.EventType)
		assert.Equal(t, "cursor-grok-4.6-high-fast", ev.Model)
		assert.Equal(t, "grok-4.6", ev.Metadata["model_id"])
		require.NotNil(t, ev.InputTokens)
		assert.Equal(t, 14, *ev.InputTokens)
		require.NotNil(t, ev.OutputTokens)
		assert.Equal(t, 8146, *ev.OutputTokens)
		require.NotNil(t, ev.CacheReadTokens)
		assert.Equal(t, 1007022, *ev.CacheReadTokens)
		require.NotNil(t, ev.CacheCreationTokens)
		assert.Equal(t, 173957, *ev.CacheCreationTokens)
		assert.Nil(t, ev.ReasoningTokens)
	})

	t.Run("clamps uncached input at zero when cache exceeds input", func(t *testing.T) {
		stdin := []byte(`{
			"hook_event_name": "stop",
			"conversation_id": "conv-2",
			"model_id": "claude-4.5-sonnet",
			"status": "completed",
			"input_tokens": 100,
			"output_tokens": 25,
			"cache_read_tokens": 120
		}`)
		ev, err := Normalize("cursor", "", stdin, now, nil)
		require.NoError(t, err)
		assert.Equal(t, "claude-4.5-sonnet", ev.Model)
		require.NotNil(t, ev.InputTokens)
		assert.Equal(t, 0, *ev.InputTokens)
		require.NotNil(t, ev.OutputTokens)
		assert.Equal(t, 25, *ev.OutputTokens)
		require.NotNil(t, ev.CacheReadTokens)
		assert.Equal(t, 120, *ev.CacheReadTokens)
		assert.Nil(t, ev.CacheCreationTokens)
	})

	t.Run("afterAgentResponse does not double count tokens", func(t *testing.T) {
		stdin := []byte(`{
			"hook_event_name": "afterAgentResponse",
			"conversation_id": "conv-1",
			"generation_id": "gen-1",
			"text": "done",
			"input_tokens": 5000,
			"output_tokens": 200
		}`)
		ev, err := Normalize("cursor", "", stdin, now, nil)
		require.NoError(t, err)
		assert.Equal(t, EventAgentResponse, ev.EventType)
		assert.Nil(t, ev.InputTokens)
		assert.Nil(t, ev.OutputTokens)
	})

	t.Run("omits token fields when absent or invalid", func(t *testing.T) {
		stdin := []byte(`{
			"hook_event_name": "stop",
			"conversation_id": "conv-3",
			"status": "completed",
			"input_tokens": -5,
			"output_tokens": true
		}`)
		ev, err := Normalize("cursor", "", stdin, now, nil)
		require.NoError(t, err)
		assert.Nil(t, ev.InputTokens)
		assert.Nil(t, ev.OutputTokens)
	})
}

func TestAntigravityPostInvocationTranscriptUsage(t *testing.T) {
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

	t.Run("reads exact step_index matching initialNumSteps", func(t *testing.T) {
		dir := t.TempDir()
		transcript := filepath.Join(dir, "transcript.jsonl")
		lines := "" +
			`{"step_index":0,"source":"USER","status":"DONE"}` + "\n" +
			`{"step_index":1,"source":"MODEL","status":"DONE","model_name":"gemini-3.6-flash-medium","input_tokens":1000,"output_tokens":50,"cache_read_tokens":200}` + "\n" +
			`{"step_index":4,"source":"MODEL","status":"IN_PROGRESS","input_tokens":50,"output_tokens":1}` + "\n" +
			`{"step_index":4,"source":"MODEL","status":"DONE","model_name":"gemini-3.1-pro-high","input_tokens":5225,"output_tokens":340,"cache_read_tokens":113928,"cache_creation_input_tokens":1200,"thinking_tokens":110}` + "\n"
		require.NoError(t, os.WriteFile(transcript, []byte(lines), 0o600))

		stdin := []byte(`{
			"conversationId": "agy-conv-1",
			"transcriptPath": "` + filepath.ToSlash(transcript) + `",
			"invocationNum": 2,
			"initialNumSteps": 4,
			"content": "Fixed it."
		}`)
		ev, err := Normalize("agy", AntigravityPostInvocation, stdin, now, nil)
		require.NoError(t, err)
		assert.Equal(t, EventAgentResponse, ev.EventType)
		assert.Equal(t, "gemini-3.1-pro-high", ev.Model)
		assert.Equal(t, int64(4), ev.Metadata["initial_num_steps"])
		require.NotNil(t, ev.InputTokens)
		assert.Equal(t, 5225, *ev.InputTokens)
		require.NotNil(t, ev.OutputTokens)
		assert.Equal(t, 340, *ev.OutputTokens)
		require.NotNil(t, ev.CacheReadTokens)
		assert.Equal(t, 113928, *ev.CacheReadTokens)
		require.NotNil(t, ev.CacheCreationTokens)
		assert.Equal(t, 1200, *ev.CacheCreationTokens)
		require.NotNil(t, ev.ReasoningTokens)
		assert.Equal(t, 110, *ev.ReasoningTokens)
	})

	t.Run("does not fall back to previous step when step_index not yet flushed", func(t *testing.T) {
		dir := t.TempDir()
		transcript := filepath.Join(dir, "transcript.jsonl")
		lines := `{"step_index":1,"source":"MODEL","status":"DONE","input_tokens":1000,"output_tokens":50}` + "\n"
		require.NoError(t, os.WriteFile(transcript, []byte(lines), 0o600))

		stdin := []byte(`{
			"conversationId": "agy-conv-1",
			"transcriptPath": "` + filepath.ToSlash(transcript) + `",
			"modelName": "gemini-3.6-flash-medium",
			"initialNumSteps": 7
		}`)
		ev, err := Normalize("agy", AntigravityPostInvocation, stdin, now, nil)
		require.NoError(t, err)
		assert.Nil(t, ev.InputTokens)
		assert.Nil(t, ev.OutputTokens)
	})

	t.Run("sums all completed model steps >= initialNumSteps and deduplicates per step_index", func(t *testing.T) {
		dir := t.TempDir()
		transcript := filepath.Join(dir, "transcript.jsonl")
		lines := "" +
			`{"step_index":2,"source":"MODEL","status":"DONE","model_name":"gemini-3.6-flash","input_tokens":900,"output_tokens":90}` + "\n" +
			`{"step_index":4,"source":"MODEL","status":"DONE","model_name":"gemini-3.1-pro","input_tokens":100,"output_tokens":10}` + "\n" +
			`{"step_index":4,"source":"MODEL","status":"DONE","model_name":"gemini-3.1-pro","input_tokens":500,"output_tokens":50,"cache_read_tokens":1000}` + "\n" +
			`{"step_index":5,"source":"TOOL","status":"DONE"}` + "\n" +
			`{"step_index":6,"step_type":"PLANNER_RESPONSE","status":"DONE","modelName":"gemini-3.1-pro-high","input_tokens":300,"output_tokens":40,"cache_read_tokens":2000,"cache_write_tokens":150,"reasoning_tokens":25}` + "\n"
		require.NoError(t, os.WriteFile(transcript, []byte(lines), 0o600))

		stdin := []byte(`{
			"conversationId": "agy-conv-multi",
			"transcriptPath": "` + filepath.ToSlash(transcript) + `",
			"initialNumSteps": 4
		}`)
		ev, err := Normalize("agy", AntigravityPostInvocation, stdin, now, nil)
		require.NoError(t, err)
		assert.Equal(t, "gemini-3.1-pro-high", ev.Model)
		require.NotNil(t, ev.InputTokens)
		assert.Equal(t, 800, *ev.InputTokens)
		require.NotNil(t, ev.OutputTokens)
		assert.Equal(t, 90, *ev.OutputTokens)
		require.NotNil(t, ev.CacheReadTokens)
		assert.Equal(t, 3000, *ev.CacheReadTokens)
		require.NotNil(t, ev.CacheCreationTokens)
		assert.Equal(t, 150, *ev.CacheCreationTokens)
		require.NotNil(t, ev.ReasoningTokens)
		assert.Equal(t, 25, *ev.ReasoningTokens)
	})

	t.Run("honors finalNumSteps upper bound and scans backwards across chunks", func(t *testing.T) {
		dir := t.TempDir()
		transcript := filepath.Join(dir, "transcript.jsonl")
		var buf []byte
		// Write > 64 KiB of earlier steps so backward scanning crosses chunk boundaries.
		pad := make([]byte, 2048)
		for i := range pad {
			pad[i] = 'a'
		}
		for step := 0; step < 45; step++ {
			buf = append(buf, []byte(`{"step_index":0,"source":"USER","status":"DONE","blob":"`)...)
			buf = append(buf, pad...)
			buf = append(buf, []byte("\"}\n")...)
		}
		buf = append(buf, []byte(`{"step_index":10,"source":"MODEL","status":"DONE","model_name":"gemini-3.1-pro","input_tokens":400,"output_tokens":60}`+"\n")...)
		buf = append(buf, []byte(`{"step_index":11,"source":"MODEL","status":"DONE","model_name":"gemini-3.1-pro","input_tokens":200,"output_tokens":30}`+"\n")...)
		buf = append(buf, []byte(`{"step_index":12,"source":"MODEL","status":"DONE","model_name":"gemini-3.1-pro","input_tokens":9999,"output_tokens":9999}`+"\n")...)
		require.NoError(t, os.WriteFile(transcript, buf, 0o600))

		stdin := []byte(`{
			"conversationId": "agy-conv-chunks",
			"transcriptPath": "` + filepath.ToSlash(transcript) + `",
			"initialNumSteps": 10,
			"finalNumSteps": 12
		}`)
		ev, err := Normalize("agy", AntigravityPostInvocation, stdin, now, nil)
		require.NoError(t, err)
		require.NotNil(t, ev.InputTokens)
		assert.Equal(t, 600, *ev.InputTokens)
		require.NotNil(t, ev.OutputTokens)
		assert.Equal(t, 90, *ev.OutputTokens)
	})

	t.Run("ignores non-jsonl paths, directories, and missing files", func(t *testing.T) {
		dir := t.TempDir()
		dirNamedJSONL := filepath.Join(dir, "fake.jsonl")
		require.NoError(t, os.Mkdir(dirNamedJSONL, 0o700))

		for _, path := range []string{"/nonexistent/transcript.db", "/nonexistent/transcript.jsonl", dirNamedJSONL} {
			stdin := []byte(`{
				"conversationId": "agy-conv-2",
				"transcriptPath": "` + filepath.ToSlash(path) + `",
				"initialNumSteps": 1
			}`)
			ev, err := Normalize("agy", AntigravityPostInvocation, stdin, now, nil)
			require.NoError(t, err)
			assert.Nil(t, ev.InputTokens)
			assert.Nil(t, ev.OutputTokens)
		}
		assert.Equal(t, "/abs/path.jsonl", expandHomePath("/abs/path.jsonl"))
		assert.Contains(t, expandHomePath("~/transcript.jsonl"), "transcript.jsonl")
	})
}

func TestPayloadNonNegInt(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
		ok   bool
	}{
		{"plain int", `{"n":42}`, 42, true},
		{"zero", `{"n":0}`, 0, true},
		{"string int", `{"n":"128"}`, 128, true},
		{"integral float", `{"n":1234.0}`, 1234, true},
		{"scientific notation", `{"n":1.2e3}`, 1200, true},
		{"string scientific", `{"n":"2.5e2"}`, 250, true},
		{"fractional float", `{"n":12.5}`, 0, false},
		{"negative", `{"n":-1}`, 0, false},
		{"boolean", `{"n":true}`, 0, false},
		{"null", `{"n":null}`, 0, false},
		{"empty string", `{"n":""}`, 0, false},
		{"non-numeric string", `{"n":"abc"}`, 0, false},
		{"object", `{"n":{}}`, 0, false},
		{"missing", `{}`, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p payload
			require.NoError(t, json.Unmarshal([]byte(tc.raw), &p))
			got, ok := p.nonNegInt("n")
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

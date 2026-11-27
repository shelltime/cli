package agenthook

import (
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

	t.Run("ignores non-jsonl paths and missing files", func(t *testing.T) {
		stdin := []byte(`{
			"conversationId": "agy-conv-2",
			"transcriptPath": "/nonexistent/transcript.db",
			"initialNumSteps": 1
		}`)
		ev, err := Normalize("agy", AntigravityPostInvocation, stdin, now, nil)
		require.NoError(t, err)
		assert.Nil(t, ev.InputTokens)
		assert.Nil(t, ev.OutputTokens)
	})
}

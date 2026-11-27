package agenthook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudEnvSignal(t *testing.T) {
	assert.Empty(t, CloudEnvSignal(nil))
	assert.Empty(t, CloudEnvSignal(envOf(nil)))

	for _, v := range []string{"true", "TRUE", "1", " true "} {
		assert.Equal(t, CloudSignalEnv, CloudEnvSignal(envOf(map[string]string{"SHELLTIME_CLOUD": v})), v)
	}
	for _, v := range []string{"", "false", "0", "yes"} {
		assert.Empty(t, CloudEnvSignal(envOf(map[string]string{"SHELLTIME_CLOUD": v})), v)
	}

	assert.Equal(t, CloudSignalClaudeCodeRemote, CloudEnvSignal(envOf(map[string]string{"CLAUDE_CODE_REMOTE": "true"})))
	assert.Empty(t, CloudEnvSignal(envOf(map[string]string{"CLAUDE_CODE_REMOTE": "false"})))
	assert.Empty(t, CloudEnvSignal(envOf(map[string]string{"CLAUDE_CODE_REMOTE_SESSION_ID": "cse_1"})), "the session id alone isn't the flag")

	both := envOf(map[string]string{"SHELLTIME_CLOUD": "true", "CLAUDE_CODE_REMOTE": "true"})
	assert.Equal(t, CloudSignalEnv, CloudEnvSignal(both), "the explicit variable is named first")
}

func TestNormalize_CloudCursor(t *testing.T) {
	t.Run("a background agent's session start", func(t *testing.T) {
		ev, err := Normalize("cursor", "sessionStart", []byte(`{"session_id":"c1","is_background_agent":true,"composer_mode":"agent"}`), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, CloudSignalCursorBackgroundAgent, ev.Cloud)
		assert.Equal(t, true, ev.Metadata["is_background_agent"], "Cursor's own field stays in metadata")
	})

	t.Run("a local session start", func(t *testing.T) {
		ev, err := Normalize("cursor", "sessionStart", []byte(`{"session_id":"c1","is_background_agent":false}`), testNow, nil)
		require.NoError(t, err)
		assert.Empty(t, ev.Cloud)
	})

	t.Run("the flag is read on any event that carries it", func(t *testing.T) {
		ev, err := Normalize("cursor", "stop", []byte(`{"conversation_id":"c1","is_background_agent":true}`), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, CloudSignalCursorBackgroundAgent, ev.Cloud)
	})

	t.Run("a flag of another type is passed over", func(t *testing.T) {
		ev, err := Normalize("cursor", "sessionStart", []byte(`{"session_id":"c1","is_background_agent":"true"}`), testNow, nil)
		require.NoError(t, err)
		assert.Empty(t, ev.Cloud)
	})

	t.Run("the environment marks every event", func(t *testing.T) {
		ev, err := Normalize("cursor", "postToolUse", []byte(`{"conversation_id":"c1","tool_name":"Shell","tool_input":{"command":"ls"}}`), testNow, envOf(map[string]string{"SHELLTIME_CLOUD": "true"}))
		require.NoError(t, err)
		assert.Equal(t, CloudSignalEnv, ev.Cloud)
	})

	t.Run("the agent's own word comes first", func(t *testing.T) {
		ev, err := Normalize("cursor", "sessionStart", []byte(`{"session_id":"c1","is_background_agent":true}`), testNow, envOf(map[string]string{"SHELLTIME_CLOUD": "true"}))
		require.NoError(t, err)
		assert.Equal(t, CloudSignalCursorBackgroundAgent, ev.Cloud)
	})

	t.Run("a remote workspace is metadata, not cloud", func(t *testing.T) {
		ev, err := Normalize("cursor", "stop", []byte(`{"conversation_id":"c1"}`), testNow, envOf(map[string]string{"CURSOR_CODE_REMOTE": "true"}))
		require.NoError(t, err)
		assert.Empty(t, ev.Cloud, "an SSH host of the user's is a remote workspace too")
		assert.Equal(t, true, ev.Metadata["remote_workspace"])

		ev, err = Normalize("cursor", "stop", []byte(`{"conversation_id":"c1"}`), testNow, envOf(map[string]string{"CURSOR_CODE_REMOTE": "false"}))
		require.NoError(t, err)
		assert.NotContains(t, ev.Metadata, "remote_workspace")
	})

	t.Run("a truncated payload is still marked", func(t *testing.T) {
		ev, err := NormalizeTruncated("cursor", "sessionStart", []byte(`{"session_id":"c1","is_background_agent":true,"composer_mode":"ag`), testNow, nil)
		require.NoError(t, err)
		assert.Equal(t, CloudSignalCursorBackgroundAgent, ev.Cloud)
	})
}

func TestNormalize_CloudAntigravity(t *testing.T) {
	payload := []byte(`{"conversationId":"a1","is_background_agent":true}`)

	ev, err := Normalize("agy", "Stop", payload, testNow, nil)
	require.NoError(t, err)
	assert.Empty(t, ev.Cloud, "Antigravity documents no cloud field; Cursor's isn't read for it")

	ev, err = Normalize("agy", "Stop", payload, testNow, envOf(map[string]string{"SHELLTIME_CLOUD": "true"}))
	require.NoError(t, err)
	assert.Equal(t, CloudSignalEnv, ev.Cloud)

	ev, err = Normalize("agy", "PostToolUse", []byte(`{"conversationId":"a1","toolCall":{"name":"run_command"}}`), testNow, envOf(map[string]string{"CLAUDE_CODE_REMOTE": "true"}))
	require.NoError(t, err)
	assert.Equal(t, CloudSignalClaudeCodeRemote, ev.Cloud)
}

func TestEvent_CloudJSON(t *testing.T) {
	ev := Event{ID: "id1", Agent: AgentCursor, EventType: EventStop}
	assert.NotContains(t, mustJSON(t, ev), "cloud", "left out when nothing showed a cloud run")

	ev.Cloud = CloudSignalEnv
	assert.JSONEq(t, `{"id":"id1","agent":"cursor","cloud":"shelltime_cloud","event_type":"stop"}`, mustJSON(t, ev))
}

package agenthook

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPassThrough(t *testing.T) {
	tests := []struct {
		agent, event, want string
	}{
		{"cursor", "beforeSubmitPrompt", `{"continue":true}`},
		{"cursor", "BeforeSubmitPrompt", `{"continue":true}`},
		{"cursor", "postToolUse", `{}`},
		{"cursor", "postToolUseFailure", `{}`},
		{"cursor", "afterAgentResponse", `{}`},
		{"cursor", "stop", `{}`},
		{"cursor", "sessionStart", `{}`},
		{"cursor", "sessionEnd", `{}`},
		{"cursor", "", `{}`},
		{"agy", "Stop", `{"decision":"allow"}`},
		{"antigravity", "Stop", `{"decision":"allow"}`},
		{"agy", "SessionStart", `{}`},
		{"agy", "PreInvocation", `{}`},
		{"agy", "PostToolUse", `{}`},
		{"agy", "PostInvocation", `{}`},
		{"agy", "", `{}`},
		{"", "beforeSubmitPrompt", `{}`},
		{"vim", "Stop", `{}`},
	}
	for _, tt := range tests {
		got := PassThrough(tt.agent, tt.event)
		assert.Equal(t, tt.want, string(got), "%s %s", tt.agent, tt.event)
		assert.True(t, json.Valid(got))
	}

	// Callers append to the reply; that never changes the next one
	first := append(PassThrough("cursor", "stop"), '\n')
	assert.Equal(t, "{}\n", string(first))
	assert.Equal(t, "{}", string(PassThrough("cursor", "stop")))
}

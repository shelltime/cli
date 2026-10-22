package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloudSessionIDFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "local machine", env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: ""},
		{name: "cloud session", env: map[string]string{"CLAUDE_CODE_REMOTE": "true", "CLAUDE_CODE_REMOTE_SESSION_ID": " cse_01AbC\n"}, want: "cse_01AbC"},
		{name: "remote flag off", env: map[string]string{"CLAUDE_CODE_REMOTE": "false", "CLAUDE_CODE_REMOTE_SESSION_ID": "cse_01AbC"}, want: ""},
		{name: "id without the flag", env: map[string]string{"CLAUDE_CODE_REMOTE_SESSION_ID": "cse_01AbC"}, want: ""},
		{name: "flag without an id", env: map[string]string{"CLAUDE_CODE_REMOTE": "true"}, want: ""},
		{name: "blank id", env: map[string]string{"CLAUDE_CODE_REMOTE": "true", "CLAUDE_CODE_REMOTE_SESSION_ID": "   "}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CloudSessionIDFromEnv(func(k string) string { return tt.env[k] }))
		})
	}
}

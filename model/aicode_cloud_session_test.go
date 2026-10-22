package model

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// With the network secret's placeholder, requests to the ShellTime API carry no Authorization
// header for the agent proxy to collide with; any other token is sent as before.
func TestSetCLIAuthorization(t *testing.T) {
	tests := []struct {
		token    string
		wantAuth []string
	}{
		{token: ProxyInjectedToken, wantAuth: nil},
		{token: "st_v1_abc", wantAuth: []string{"CLI st_v1_abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			var gotJSON, gotStream []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/ai/command-suggest" {
					gotStream = r.Header.Values("Authorization")
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data:[DONE]\n"))
					return
				}
				gotJSON = r.Header.Values("Authorization")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()
			endpoint := Endpoint{APIEndpoint: srv.URL, Token: tt.token}

			require.NoError(t, SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, any]{
				Context:  context.Background(),
				Endpoint: endpoint,
				Method:   http.MethodPost,
				Path:     "/api/v1/cc/otel",
				Payload:  map[string]string{},
			}))
			require.Equal(t, tt.wantAuth, gotJSON)

			require.NoError(t, NewAIService().QueryCommandStream(context.Background(), CommandSuggestVariables{}, endpoint, func(string) {}))
			require.Equal(t, tt.wantAuth, gotStream)
		})
	}
}

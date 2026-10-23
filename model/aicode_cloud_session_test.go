package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestIsAICloudOnly(t *testing.T) {
	on, off := true, false
	cloud := map[string]string{"CLAUDE_CODE_REMOTE": "true", "CLAUDE_CODE_REMOTE_SESSION_ID": "cse_01AbC"}
	tests := []struct {
		name string
		cfg  ShellTimeConfig
		env  map[string]string
		want bool
	}{
		{name: "local machine", env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: false},
		{name: "cloud environment", env: cloud, want: true},
		{name: "remote flag off", env: map[string]string{"CLAUDE_CODE_REMOTE": "false"}, want: false},
		{name: "on in the config", cfg: ShellTimeConfig{AICloudOnly: &on}, want: true},
		{name: "off in the config wins in the cloud", cfg: ShellTimeConfig{AICloudOnly: &off}, env: cloud, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsAICloudOnly(tt.cfg, func(k string) string { return tt.env[k] }))
		})
	}
}

// aiCloudOnly is read from either config file, and `shelltime auth` never writes it, so an
// unset value keeps following the environment.
func TestAICloudOnlyConfigFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("token: tok\n"), 0600))
	cfg, err := NewConfigService(dir).ReadConfigFile(context.Background())
	require.NoError(t, err)
	require.Nil(t, cfg.AICloudOnly)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.yaml"), []byte("aiCloudOnly: true\n"), 0600))
	cfg, err = NewConfigService(dir).ReadConfigFile(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cfg.AICloudOnly)
	require.True(t, *cfg.AICloudOnly)

	written, err := yaml.Marshal(DefaultConfig)
	require.NoError(t, err)
	require.NotContains(t, string(written), "aiCloudOnly")
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

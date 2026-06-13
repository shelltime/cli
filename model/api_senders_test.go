package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readJSONBody decodes the request body into v, failing the test on error.
func readJSONBody(t *testing.T, r *http.Request, v interface{}) {
	t.Helper()
	require.NoError(t, json.NewDecoder(r.Body).Decode(v))
}

func TestSendHeartbeatsToServer(t *testing.T) {
	t.Run("happy path posts to /api/v1/heartbeats with auth header", func(t *testing.T) {
		var gotPath, gotMethod, gotAuth string
		var gotPayload HeartbeatPayload
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotMethod = r.Method
			gotAuth = r.Header.Get("Authorization")
			readJSONBody(t, r, &gotPayload)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"processed":1}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "tok123", APIEndpoint: server.URL}
		payload := HeartbeatPayload{Heartbeats: []HeartbeatData{{HeartbeatID: "hb-1", Entity: "/a/b.go", Time: 100}}}

		err := SendHeartbeatsToServer(context.Background(), cfg, payload)
		require.NoError(t, err)
		assert.Equal(t, "/api/v1/heartbeats", gotPath)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Equal(t, "CLI tok123", gotAuth)
		require.Len(t, gotPayload.Heartbeats, 1)
		assert.Equal(t, "hb-1", gotPayload.Heartbeats[0].HeartbeatID)
	})

	t.Run("CodeTracking endpoint/token overrides global config", func(t *testing.T) {
		var gotAuth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		// global endpoint points nowhere usable; CodeTracking should win.
		cfg := ShellTimeConfig{
			Token:       "globalTok",
			APIEndpoint: "http://127.0.0.1:0",
			CodeTracking: &CodeTracking{
				APIEndpoint: server.URL,
				Token:       "ctTok",
			},
		}
		err := SendHeartbeatsToServer(context.Background(), cfg, HeartbeatPayload{})
		require.NoError(t, err)
		assert.Equal(t, "CLI ctTok", gotAuth)
	})

	t.Run("error response surfaces server error message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":400,"error":"bad heartbeat"}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
		err := SendHeartbeatsToServer(context.Background(), cfg, HeartbeatPayload{})
		require.Error(t, err)
		assert.Equal(t, "bad heartbeat", err.Error())
	})
}

func TestSendSessionProjectUpdate(t *testing.T) {
	t.Run("happy path posts session and project", func(t *testing.T) {
		var gotPath string
		var body sessionProjectRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			readJSONBody(t, r, &body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "t", APIEndpoint: server.URL}
		err := SendSessionProjectUpdate(context.Background(), cfg, "sess-1", "/home/me/proj")
		require.NoError(t, err)
		assert.Equal(t, "/api/v1/cc/session-project", gotPath)
		assert.Equal(t, "sess-1", body.SessionID)
		assert.Equal(t, "/home/me/proj", body.ProjectPath)
	})

	t.Run("error path returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "t", APIEndpoint: server.URL}
		err := SendSessionProjectUpdate(context.Background(), cfg, "s", "p")
		require.Error(t, err)
		assert.Equal(t, "boom", err.Error())
	})
}

func TestSendAICodeOtelData(t *testing.T) {
	t.Run("happy path posts to /api/v1/cc/otel and returns parsed response", func(t *testing.T) {
		var gotPath string
		var req AICodeOtelRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			readJSONBody(t, r, &req)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"eventsProcessed":2,"metricsProcessed":3}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "tok", APIEndpoint: server.URL}
		in := &AICodeOtelRequest{
			Host:    "host1",
			Project: "proj1",
			Source:  AICodeOtelSourceClaudeCode,
			Events:  []AICodeOtelEvent{{EventID: "e1", EventType: AICodeEventUserPrompt}},
		}
		resp, err := SendAICodeOtelData(context.Background(), in, endpoint)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.Equal(t, 2, resp.EventsProcessed)
		assert.Equal(t, 3, resp.MetricsProcessed)
		assert.Equal(t, "/api/v1/cc/otel", gotPath)
		assert.Equal(t, "host1", req.Host)
		require.Len(t, req.Events, 1)
		assert.Equal(t, "e1", req.Events[0].EventID)
	})

	t.Run("error path returns nil response and error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "bad", APIEndpoint: server.URL}
		resp, err := SendAICodeOtelData(context.Background(), &AICodeOtelRequest{}, endpoint)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, "unauthorized", err.Error())
	})
}

func TestSendAliasesToServer(t *testing.T) {
	t.Run("empty aliases is a no-op with no request", func(t *testing.T) {
		called := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "t", APIEndpoint: server.URL}
		err := SendAliasesToServer(context.Background(), endpoint, nil, false, "bash", "~/.bashrc")
		require.NoError(t, err)
		assert.False(t, called, "no HTTP request should be made for empty aliases")
	})

	t.Run("happy path posts aliases to /api/v1/import-alias", func(t *testing.T) {
		var gotPath string
		var body importShellAliasRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			readJSONBody(t, r, &body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"count":2}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "t", APIEndpoint: server.URL}
		aliases := []string{"alias ll='ls -la'", "alias gs='git status'"}
		err := SendAliasesToServer(context.Background(), endpoint, aliases, true, "zsh", "~/.zshrc")
		require.NoError(t, err)
		assert.Equal(t, "/api/v1/import-alias", gotPath)
		assert.Equal(t, aliases, body.Aliases)
		assert.True(t, body.IsFullRefresh)
		assert.Equal(t, "zsh", body.ShellType)
		assert.Equal(t, "~/.zshrc", body.FileLocation)
		// Hostname/username are populated from the environment; just assert non-empty.
		assert.NotEmpty(t, body.Hostname)
		assert.NotEmpty(t, body.Username)
	})

	t.Run("error path wraps server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"alias rejected"}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "t", APIEndpoint: server.URL}
		err := SendAliasesToServer(context.Background(), endpoint, []string{"alias x='y'"}, false, "bash", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "alias rejected")
		assert.Contains(t, err.Error(), "failed to send aliases to server")
	})
}

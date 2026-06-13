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

func TestQueryCommandStream_HappyPath(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotAccept string
	var gotVars CommandSuggestVariables
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotVars))

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Two data tokens then a [DONE] terminator.
		_, _ = w.Write([]byte("data:ls\ndata: -la\ndata:[DONE]\n"))
	}))
	defer server.Close()

	var tokens []string
	svc := NewAIService()
	err := svc.QueryCommandStream(
		context.Background(),
		CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "list files", Pwd: "/tmp", Hostname: "h"},
		Endpoint{APIEndpoint: server.URL, Token: "tok"},
		func(token string) { tokens = append(tokens, token) },
	)
	require.NoError(t, err)

	assert.Equal(t, "/api/v1/ai/command-suggest", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "CLI tok", gotAuth)
	assert.Equal(t, "text/event-stream", gotAccept)
	assert.Equal(t, "list files", gotVars.Query)

	// onToken receives the raw text after the "data:" prefix (note no trimming
	// of the leading space on the second line, matching the implementation).
	require.Equal(t, []string{"ls", " -la"}, tokens)
}

func TestQueryCommandStream_DoneWithoutTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data:[DONE]\n"))
	}))
	defer server.Close()

	called := false
	svc := NewAIService()
	err := svc.QueryCommandStream(
		context.Background(),
		CommandSuggestVariables{Shell: "zsh", Os: "darwin", Query: "q"},
		Endpoint{APIEndpoint: server.URL, Token: "t"},
		func(string) { called = true },
	)
	require.NoError(t, err)
	assert.False(t, called, "no token callback before [DONE]")
}

func TestQueryCommandStream_EventErrorBranch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// An SSE error event followed by its data line.
		_, _ = w.Write([]byte("event: error\ndata:something blew up\n"))
	}))
	defer server.Close()

	svc := NewAIService()
	err := svc.QueryCommandStream(
		context.Background(),
		CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "q"},
		Endpoint{APIEndpoint: server.URL, Token: "t"},
		func(string) {},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server error: something blew up")
}

func TestQueryCommandStream_TrailingSlashEndpointNormalized(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data:[DONE]\n"))
	}))
	defer server.Close()

	svc := NewAIService()
	// Trailing slash on the endpoint must not produce a double slash in the path.
	err := svc.QueryCommandStream(
		context.Background(),
		CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "q"},
		Endpoint{APIEndpoint: server.URL + "/", Token: "t"},
		func(string) {},
	)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/ai/command-suggest", gotPath)
}

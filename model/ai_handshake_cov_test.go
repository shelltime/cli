package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQueryCommandStream_Non200WithErrorMessage covers the non-200 branch where
// the body parses into an errorResponse with a message.
func TestQueryCommandStream_Non200WithErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"quota exceeded"}`))
	}))
	defer server.Close()

	svc := NewAIService()
	err := svc.QueryCommandStream(context.Background(),
		CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "q"},
		Endpoint{APIEndpoint: server.URL, Token: "t"},
		func(string) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota exceeded")
}

// TestQueryCommandStream_Non200Unparseable covers the non-200 branch where the
// body is not a parseable errorResponse: falls back to "server returned status".
func TestQueryCommandStream_Non200Unparseable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("plain text error"))
	}))
	defer server.Close()

	svc := NewAIService()
	err := svc.QueryCommandStream(context.Background(),
		CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "q"},
		Endpoint{APIEndpoint: server.URL, Token: "t"},
		func(string) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server returned status 500")
}

// TestQueryCommandStream_RequestCreationError covers the http.NewRequest error
// branch by supplying an endpoint with a control character in the URL.
func TestQueryCommandStream_RequestCreationError(t *testing.T) {
	svc := NewAIService()
	err := svc.QueryCommandStream(context.Background(),
		CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "q"},
		Endpoint{APIEndpoint: "http://exa\x7fmple", Token: "t"},
		func(string) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

// TestQueryCommandStream_SendError covers the client.Do error branch via an
// unroutable endpoint.
func TestQueryCommandStream_SendError(t *testing.T) {
	svc := NewAIService()
	err := svc.QueryCommandStream(context.Background(),
		CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "q"},
		Endpoint{APIEndpoint: "http://127.0.0.1:1", Token: "t"},
		func(string) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send request")
}

// TestHandshakeSend_RequestCreationError covers handshakeService.send's
// NewRequestWithContext error path (bad URL).
func TestHandshakeSend_RequestCreationError(t *testing.T) {
	hs := NewHandshakeService(ShellTimeConfig{APIEndpoint: "http://exa\x7fmple"})
	_, err := hs.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handshake init error")
}

// TestHandshakeCheck_SendError covers Check's send error branch via an
// unroutable endpoint (client.Do fails).
func TestHandshakeCheck_SendError(t *testing.T) {
	hs := NewHandshakeService(ShellTimeConfig{APIEndpoint: "http://127.0.0.1:1"})
	_, err := hs.Check(context.Background(), "hid")
	require.Error(t, err)
}

// TestHandshakeInit_MalformedSuccessBody covers the json.Unmarshal-on-result
// branch of send when the server returns 200 with invalid JSON.
func TestHandshakeInit_MalformedSuccessBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	hs := NewHandshakeService(ShellTimeConfig{APIEndpoint: server.URL})
	_, err := hs.Init(context.Background())
	require.Error(t, err, "unmarshal failure surfaces as an init error")
}

package model

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchAICodeBackfillStatus(t *testing.T) {
	var gotPath, gotAuth string
	var req AICodeBackfillSessionsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		readJSONBody(t, r, &req)
		_, _ = w.Write([]byte(`{"sessions":[{"sessionId":"s1","status":"backfilled","eventCount":7}]}`))
	}))
	defer server.Close()

	statuses, err := FetchAICodeBackfillStatus(context.Background(), Endpoint{Token: "tok", APIEndpoint: server.URL}, AICodeClientCodex, []string{"s1", "s2"})

	require.NoError(t, err)
	assert.Equal(t, "/api/v1/cc/backfill/sessions", gotPath)
	assert.Equal(t, "CLI tok", gotAuth)
	assert.Equal(t, AICodeClientCodex, req.ClientType)
	assert.Equal(t, []string{"s1", "s2"}, req.SessionIDs)
	require.Len(t, statuses, 1)
	assert.Equal(t, AICodeBackfillSessionStatus{SessionID: "s1", Status: AICodeBackfillStatusBackfilled, EventCount: 7}, statuses[0])
}

func TestSendAICodeBackfill(t *testing.T) {
	var gotPath string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		readJSONBody(t, r, &body)
		_, _ = w.Write([]byte(`{"success":true,"accepted":1,"summarized":1,"skippedSessions":[{"sessionId":"s2","status":"live"}]}`))
	}))
	defer server.Close()

	failed := false
	resp, err := SendAICodeBackfill(context.Background(), Endpoint{APIEndpoint: server.URL}, AICodeBackfillRequest{
		ClientType: AICodeClientClaudeCode,
		Events: []AICodeBackfillEvent{{
			EventID:   "bf1:abc",
			EventType: AICodeEventToolResult,
			SessionID: "s1",
			Success:   &failed,
		}},
		CompletedSessionIDs: []string{"s1"},
	})

	require.NoError(t, err)
	assert.Equal(t, "/api/v1/cc/backfill", gotPath)
	assert.Equal(t, 1, resp.Accepted)
	assert.Equal(t, []AICodeBackfillSessionStatus{{SessionID: "s2", Status: AICodeBackfillStatusLive}}, resp.SkippedSessions)

	events := body["events"].([]any)
	event := events[0].(map[string]any)
	// A failed tool call must reach the server as false, not be dropped.
	assert.Equal(t, false, event["success"])
	assert.NotContains(t, event, "inputTokens")
	assert.Equal(t, []any{"s1"}, body["completedSessionIds"])
	assert.NotContains(t, body, "aiSummary")
}

func TestCompleteAICodeBackfill(t *testing.T) {
	var gotPath string
	var req AICodeBackfillCompleteRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		readJSONBody(t, r, &req)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	err := CompleteAICodeBackfill(context.Background(), Endpoint{APIEndpoint: server.URL}, AICodeBackfillCompleteRequest{ClientType: AICodeClientClaudeCode, From: from, To: to})

	require.NoError(t, err)
	assert.Equal(t, "/api/v1/cc/backfill/complete", gotPath)
	assert.True(t, from.Equal(req.From))
	assert.True(t, to.Equal(req.To))
}

func TestSendHTTPRequestJSON_ReturnsHTTPStatusError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{name: "server error message", status: http.StatusServiceUnavailable, body: `{"code":503,"error":"pricing unavailable"}`, wantMsg: "pricing unavailable"},
		{name: "unparseable body", status: http.StatusNotFound, body: `404 page not found`, wantMsg: "HTTP error: 404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			err := CompleteAICodeBackfill(context.Background(), Endpoint{APIEndpoint: server.URL}, AICodeBackfillCompleteRequest{})

			var statusErr *HTTPStatusError
			require.True(t, errors.As(err, &statusErr))
			assert.Equal(t, tt.status, statusErr.StatusCode)
			assert.Equal(t, tt.wantMsg, err.Error())
		})
	}
}

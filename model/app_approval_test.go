package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppApprovalRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/auth/app-approval/request", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("User-Agent"), "shelltimeCLI@")
		assert.Empty(t, r.Header.Get("Authorization"))

		var payload appApprovalRequestBody
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "me@example.com", payload.Email)
		assert.Equal(t, "cli", payload.Client)
		assert.NotEmpty(t, payload.Hostname)
		assert.NotEmpty(t, payload.OS)

		w.Write([]byte(`{"requestId":"rid","pollToken":"ptok","matchCode":"42","expiresIn":300,"interval":2}`))
	}))
	defer server.Close()

	ticket, err := NewAppApprovalService(ShellTimeConfig{APIEndpoint: server.URL}).Request(context.Background(), "me@example.com")
	require.NoError(t, err)
	assert.Equal(t, AppApprovalTicket{RequestID: "rid", PollToken: "ptok", MatchCode: "42", ExpiresIn: 300, Interval: 2}, ticket)
}

func TestAppApprovalRequestRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":404,"error":"This account can't approve sign-ins in the ShellTime iOS app."}`))
	}))
	defer server.Close()

	_, err := NewAppApprovalService(ShellTimeConfig{APIEndpoint: server.URL}).Request(context.Background(), "me@example.com")
	var apiErr *AppApprovalError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.Equal(t, "This account can't approve sign-ins in the ShellTime iOS app.", apiErr.Message)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestAppApprovalErrorWithoutEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>bad gateway</html>`))
	}))
	defer server.Close()

	_, err := NewAppApprovalService(ShellTimeConfig{APIEndpoint: server.URL}).Poll(context.Background(), "rid", "ptok")
	var apiErr *AppApprovalError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
	assert.Equal(t, "Bad Gateway", apiErr.Message)
}

func TestAppApprovalPoll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/auth/app-approval/poll", r.URL.Path)
		var payload appApprovalPollBody
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, appApprovalPollBody{RequestID: "rid", PollToken: "ptok"}, payload)

		w.Write([]byte(`{"status":"approved","openToken":{"id":9,"token":"st_v1_abc"}}`))
	}))
	defer server.Close()

	result, err := NewAppApprovalService(ShellTimeConfig{APIEndpoint: server.URL}).Poll(context.Background(), "rid", "ptok")
	require.NoError(t, err)
	assert.Equal(t, AppApprovalApproved, result.Status)
	require.NotNil(t, result.OpenToken)
	assert.Equal(t, "st_v1_abc", result.OpenToken.Token)
}

func TestAppApprovalUnreachable(t *testing.T) {
	_, err := NewAppApprovalService(ShellTimeConfig{APIEndpoint: "http://127.0.0.1:1"}).Request(context.Background(), "me@example.com")
	assert.Error(t, err)
	var apiErr *AppApprovalError
	assert.False(t, errors.As(err, &apiErr))
}

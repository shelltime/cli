package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/malamtime/cli/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// withStConfig swaps the package-level config service for the duration of a test.
func withStConfig(t *testing.T, cs model.ConfigService) {
	t.Helper()
	prev := stConfig
	stConfig = cs
	t.Cleanup(func() { stConfig = prev })
}

func TestHandlePubSubHeartbeat_EmptyPayloadSkips(t *testing.T) {
	mockCS := model.NewMockConfigService(t)
	// ReadConfigFile must NOT be called for an empty payload.
	withStConfig(t, mockCS)

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{}}
	err := handlePubSubHeartbeat(context.Background(), payload)
	assert.NoError(t, err)
}

func TestHandlePubSubHeartbeat_SuccessfulSend(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/heartbeats", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(model.HeartbeatResponse{})
	}))
	defer server.Close()

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)
	withStConfig(t, mockCS)

	// Use a HOME we control so a failure (if any) wouldn't pollute the real home.
	home := t.TempDir()
	t.Setenv("HOME", home)

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{
		{HeartbeatID: "hb-1", Entity: "f.go", Time: 1, Project: "p"},
	}}
	err := handlePubSubHeartbeat(context.Background(), payload)
	require.NoError(t, err)
	assert.Equal(t, int32(1), hits.Load())

	// On success nothing is written to the local heartbeat log.
	_, statErr := os.Stat(filepath.Join(home, ".shelltime", "coding-heartbeat.data.log"))
	assert.True(t, os.IsNotExist(statErr), "no local file should be written on success")
}

func TestHandlePubSubHeartbeat_FailureSavesToFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "error": "boom"})
	}))
	defer server.Close()

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)
	withStConfig(t, mockCS)

	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".shelltime"), 0755))

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{
		{HeartbeatID: "hb-saved", Entity: "f.go", Time: 2, Project: "p"},
	}}

	// Send failure -> data persisted locally -> returns nil (no nack).
	err := handlePubSubHeartbeat(context.Background(), payload)
	require.NoError(t, err)

	content, readErr := os.ReadFile(filepath.Join(home, ".shelltime", "coding-heartbeat.data.log"))
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "hb-saved")
}

func TestHandlePubSubHeartbeat_FailureAndSaveFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 500, "error": "boom"})
	}))
	defer server.Close()

	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{
		Token:       "tok",
		APIEndpoint: server.URL,
	}, nil)
	withStConfig(t, mockCS)

	// HOME points at a path whose .shelltime dir does NOT exist, so the
	// fallback save also fails and the error is propagated (saveErr returned).
	home := t.TempDir()
	t.Setenv("HOME", home)

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{
		{HeartbeatID: "hb-x", Entity: "f.go", Time: 3},
	}}
	err := handlePubSubHeartbeat(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "heartbeat log file")
}

func TestHandlePubSubHeartbeat_ConfigReadError(t *testing.T) {
	mockCS := model.NewMockConfigService(t)
	mockCS.On("ReadConfigFile", mock.Anything).Return(model.ShellTimeConfig{}, errors.New("cfg fail"))
	withStConfig(t, mockCS)

	payload := model.HeartbeatPayload{Heartbeats: []model.HeartbeatData{
		{HeartbeatID: "hb", Entity: "f.go", Time: 4},
	}}
	err := handlePubSubHeartbeat(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cfg fail")
}

func TestHandlePubSubHeartbeat_UnmarshalError(t *testing.T) {
	mockCS := model.NewMockConfigService(t)
	withStConfig(t, mockCS)

	// A payload that marshals fine but cannot unmarshal into HeartbeatPayload:
	// Heartbeats is []HeartbeatData, supply a string for it.
	bad := map[string]interface{}{"heartbeats": "not-an-array"}
	err := handlePubSubHeartbeat(context.Background(), bad)
	assert.Error(t, err)
}

package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type handshakeTestSuite struct {
	suite.Suite
}

func (s *handshakeTestSuite) TestHandshakeInitSuccess() {
	t := s.T()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/handshake/init", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Contains(t, r.Header.Get("User-Agent"), "shelltimeCLI@")

		// Decode request body
		var payload handshakeInitRequest
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)

		// Verify payload
		assert.NotEmpty(t, payload.Hostname)
		assert.NotEmpty(t, payload.OS)
		assert.NotEmpty(t, payload.OSVersion)

		// Send response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"encodedId": "test-handshake-id"}`))
	}))
	defer server.Close()

	config := ShellTimeConfig{
		APIEndpoint: server.URL,
	}

	hs := NewHandshakeService(config)
	id, err := hs.Init(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "test-handshake-id", id)
}

func (s *handshakeTestSuite) TestHandshakeInitError() {
	t := s.T()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code": 400, "error": "test error"}`))
	}))
	defer server.Close()

	config := ShellTimeConfig{
		APIEndpoint: server.URL,
	}

	hs := NewHandshakeService(config)
	_, err := hs.Init(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test error")
}

func (s *handshakeTestSuite) TestHandshakeCheckWithToken() {
	t := s.T()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/handshake/check", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Decode request body
		var payload handshakeCheckRequest
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)

		// Verify payload
		assert.Equal(t, "test-handshake-id", payload.EncodedID)

		// Send response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"encodedId": "test-handshake-id", "openToken": {"id": 1, "token": "test-token"}}`))
	}))
	defer server.Close()

	config := ShellTimeConfig{
		APIEndpoint: server.URL,
	}

	hs := NewHandshakeService(config)
	token, err := hs.Check(context.Background(), "test-handshake-id")
	assert.NoError(t, err)
	assert.Equal(t, "test-token", token)
}

func (s *handshakeTestSuite) TestHandshakeCheckWithoutToken() {
	t := s.T()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"encodedId": "test-handshake-id"}`))
	}))
	defer server.Close()

	config := ShellTimeConfig{
		APIEndpoint: server.URL,
	}

	hs := NewHandshakeService(config)
	token, err := hs.Check(context.Background(), "test-handshake-id")
	assert.NoError(t, err)
	assert.Empty(t, token)
}

func (s *handshakeTestSuite) TestHandshakeCheckError() {
	t := s.T()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code": 400, "error": "test error"}`))
	}))
	defer server.Close()

	config := ShellTimeConfig{
		APIEndpoint: server.URL,
	}

	hs := NewHandshakeService(config)
	_, err := hs.Check(context.Background(), "test-handshake-id")
	assert.Error(t, err)
	assert.Equal(t, "test error", err.Error())
}

func TestHandshakeTestSuite(t *testing.T) {
	suite.Run(t, new(handshakeTestSuite))
}

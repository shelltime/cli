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

type openTokenTestSuite struct {
	suite.Suite
}

func (s *openTokenTestSuite) TestGetOpenTokenPublicKey() {
	// Test successful request
	s.T().Run("successful request", func(t *testing.T) {
		expectedPublicKey := "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA..."
		expectedTokenID := 42

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request method and path
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/v1/opentoken/publickey", r.URL.Path)
			assert.Equal(t, "42", r.URL.Query().Get("tid"))

			// Verify headers
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Contains(t, r.Header.Get("User-Agent"), "shelltimeCLI@")
			assert.Equal(t, "CLI testToken", r.Header.Get("Authorization"))

			// Send successful response
			response := OpenTokenPublicKeyResponse{
				Data: OpenTokenPublicKey{
					ID:        expectedTokenID,
					PublicKey: expectedPublicKey,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		endpoint := Endpoint{
			Token:       "testToken",
			APIEndpoint: server.URL,
		}

		result, err := GetOpenTokenPublicKey(context.Background(), endpoint, expectedTokenID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, expectedTokenID, result.ID)
		assert.Equal(t, expectedPublicKey, result.PublicKey)
	})

	// Test error response
	s.T().Run("error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "/api/v1/opentoken/publickey", r.URL.Path)
			assert.Equal(t, "999", r.URL.Query().Get("tid"))

			// Send error response
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code": 404, "error": "token not found"}`))
		}))
		defer server.Close()

		endpoint := Endpoint{
			Token:       "testToken",
			APIEndpoint: server.URL,
		}

		result, err := GetOpenTokenPublicKey(context.Background(), endpoint, 999)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, "token not found", err.Error())
	})

	// Test server error
	s.T().Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Send internal server error
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code": 500, "error": "internal server error"}`))
		}))
		defer server.Close()

		endpoint := Endpoint{
			Token:       "testToken",
			APIEndpoint: server.URL,
		}

		result, err := GetOpenTokenPublicKey(context.Background(), endpoint, 1)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, "internal server error", err.Error())
	})

	// Test invalid response format
	s.T().Run("invalid response format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Send invalid JSON
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`invalid json`))
		}))
		defer server.Close()

		endpoint := Endpoint{
			Token:       "testToken",
			APIEndpoint: server.URL,
		}

		result, err := GetOpenTokenPublicKey(context.Background(), endpoint, 1)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid character")
	})

	// Test network error
	s.T().Run("network error", func(t *testing.T) {
		// Use an invalid URL to simulate network error
		endpoint := Endpoint{
			Token:       "testToken",
			APIEndpoint: "http://localhost:99999", // Invalid port
		}

		result, err := GetOpenTokenPublicKey(context.Background(), endpoint, 1)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestOpenTokenTestSuite(t *testing.T) {
	suite.Run(t, new(openTokenTestSuite))
}
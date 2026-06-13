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

// graphQLRequestBody mirrors the JSON shape sent by SendGraphQLRequest.
type graphQLRequestBody struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func TestFetchCurrentUserProfile(t *testing.T) {
	t.Run("happy path parses user data and posts to graphql endpoint", func(t *testing.T) {
		var gotPath string
		var reqBody graphQLRequestBody
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":42,"login":"alice"}}}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
		profile, err := FetchCurrentUserProfile(context.Background(), cfg)
		require.NoError(t, err)
		assert.Equal(t, 42, profile.FetchUser.ID)
		assert.Equal(t, "alice", profile.FetchUser.Login)
		assert.Equal(t, "/api/v2/graphql", gotPath)
		assert.Contains(t, reqBody.Query, "fetchCurrentUserProfile")
	})

	t.Run("graphql errors array surfaces as error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errors":[{"message":"not authenticated"}]}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
		_, err := FetchCurrentUserProfile(context.Background(), cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})

	t.Run("http error surfaces error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"upstream down"}`))
		}))
		defer server.Close()

		cfg := ShellTimeConfig{Token: "tok", APIEndpoint: server.URL}
		_, err := FetchCurrentUserProfile(context.Background(), cfg)
		require.Error(t, err)
		assert.Equal(t, "upstream down", err.Error())
	})
}

func TestFetchCommandsFromServer(t *testing.T) {
	t.Run("happy path parses edges", func(t *testing.T) {
		var reqBody graphQLRequestBody
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchCommands":{"count":1,"edges":[{"id":7,"shell":"zsh","command":"ls -la","mainCommand":"ls"}]}}}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "tok", APIEndpoint: server.URL}
		filter := &SearchCommandsFilter{Command: "ls"}
		pagination := &SearchCommandsPagination{Limit: 10}
		result, err := FetchCommandsFromServer(context.Background(), endpoint, filter, pagination)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 1, result.Count)
		require.Len(t, result.Edges, 1)
		assert.Equal(t, 7, result.Edges[0].ID)
		assert.Equal(t, "ls -la", result.Edges[0].Command)
		assert.Equal(t, "ls", result.Edges[0].MainCommand)
		// Variables should carry filter + pagination.
		assert.Contains(t, reqBody.Variables, "filter")
		assert.Contains(t, reqBody.Variables, "pagination")
	})

	t.Run("graphql error returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errors":[{"message":"invalid filter"}]}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "tok", APIEndpoint: server.URL}
		_, err := FetchCommandsFromServer(context.Background(), endpoint, &SearchCommandsFilter{}, &SearchCommandsPagination{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid filter")
	})
}

func TestFetchDotfilesFromServer(t *testing.T) {
	t.Run("happy path parses apps and records", func(t *testing.T) {
		var reqBody graphQLRequestBody
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"dotfiles":{"totalCount":1,"apps":[{"app":"bash","files":[{"path":"~/.bashrc","records":[{"id":3,"content":"echo hi","contentHash":"abc"}]}]}]}}}}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "tok", APIEndpoint: server.URL}
		filter := &DotfileFilter{Apps: []string{"bash"}}
		resp, err := FetchDotfilesFromServer(context.Background(), endpoint, filter)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 1, resp.Data.FetchUser.Dotfiles.TotalCount)
		require.Len(t, resp.Data.FetchUser.Dotfiles.Apps, 1)
		app := resp.Data.FetchUser.Dotfiles.Apps[0]
		assert.Equal(t, "bash", app.App)
		require.Len(t, app.Files, 1)
		assert.Equal(t, "~/.bashrc", app.Files[0].Path)
		require.Len(t, app.Files[0].Records, 1)
		assert.Equal(t, "echo hi", app.Files[0].Records[0].Content)
		assert.Contains(t, reqBody.Variables, "filter")
	})

	t.Run("nil filter omits filter variable", func(t *testing.T) {
		var reqBody graphQLRequestBody
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"fetchUser":{"id":1,"dotfiles":{"totalCount":0,"apps":[]}}}}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "tok", APIEndpoint: server.URL}
		resp, err := FetchDotfilesFromServer(context.Background(), endpoint, nil)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.NotContains(t, reqBody.Variables, "filter")
	})

	t.Run("error path returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "tok", APIEndpoint: server.URL}
		_, err := FetchDotfilesFromServer(context.Background(), endpoint, nil)
		require.Error(t, err)
		assert.Equal(t, "forbidden", err.Error())
	})
}

func TestSendDotfilesToServer(t *testing.T) {
	t.Run("empty slice short-circuits", func(t *testing.T) {
		called := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "t", APIEndpoint: server.URL}
		userID, err := SendDotfilesToServer(context.Background(), endpoint, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, userID)
		assert.False(t, called)
	})

	t.Run("happy path posts to /api/v1/dotfiles/push and returns userId", func(t *testing.T) {
		var gotPath string
		var body dotfilePushRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":1,"failed":0,"userId":99,"results":[]}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "t", APIEndpoint: server.URL}
		dotfiles := []DotfileItem{{App: "bash", Path: "~/.bashrc", Content: "echo"}}
		userID, err := SendDotfilesToServer(context.Background(), endpoint, dotfiles)
		require.NoError(t, err)
		assert.Equal(t, 99, userID)
		assert.Equal(t, "/api/v1/dotfiles/push", gotPath)
		require.Len(t, body.Dotfiles, 1)
		// Hostname auto-filled when empty.
		assert.NotEmpty(t, body.Dotfiles[0].Hostname)
	})

	t.Run("error path wraps error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"rejected"}`))
		}))
		defer server.Close()

		endpoint := Endpoint{Token: "t", APIEndpoint: server.URL}
		_, err := SendDotfilesToServer(context.Background(), endpoint, []DotfileItem{{App: "bash", Path: "p", Content: "c"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rejected")
		assert.Contains(t, err.Error(), "failed to send dotfiles to server")
	})
}

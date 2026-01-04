package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPRequestOptions_Defaults(t *testing.T) {
	opts := HTTPRequestOptions[interface{}, interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: "http://localhost", Token: "test"},
		Method:   http.MethodGet,
		Path:     "/test",
	}

	if opts.ContentType != "" {
		t.Error("ContentType should default to empty")
	}

	if opts.Timeout != 0 {
		t.Error("Timeout should default to 0")
	}
}

func TestSendHTTPRequestJSON_Success(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "CLI test-token" {
			t.Errorf("Unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	type request struct {
		Data string `json:"data"`
	}
	type response struct {
		Status string `json:"status"`
	}

	var resp response
	err := SendHTTPRequestJSON(HTTPRequestOptions[request, response]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "test-token"},
		Method:   http.MethodPost,
		Path:     "/api/test",
		Payload:  request{Data: "test"},
		Response: &resp,
	})

	if err != nil {
		t.Fatalf("SendHTTPRequestJSON failed: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", resp.Status)
	}
}

func TestSendHTTPRequestJSON_NoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Method:   http.MethodPost,
		Path:     "/api/test",
		Payload:  map[string]string{"key": "value"},
	})

	if err != nil {
		t.Fatalf("Should not error on NoContent: %v", err)
	}
}

func TestSendHTTPRequestJSON_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errorResponse{ErrorMessage: "bad request"})
	}))
	defer server.Close()

	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Method:   http.MethodPost,
		Path:     "/api/test",
		Payload:  map[string]string{},
	})

	if err == nil {
		t.Fatal("Expected error for bad request")
	}

	if err.Error() != "bad request" {
		t.Errorf("Expected 'bad request', got '%s'", err.Error())
	}
}

func TestSendHTTPRequestJSON_CustomContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-custom" {
			t.Errorf("Expected custom content type, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, interface{}]{
		Context:     context.Background(),
		Endpoint:    Endpoint{APIEndpoint: server.URL, Token: "token"},
		Method:      http.MethodPost,
		Path:        "/api/test",
		Payload:     map[string]string{},
		ContentType: "application/x-custom",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestSendHTTPRequestJSON_CustomTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, map[string]string]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Method:   http.MethodPost,
		Path:     "/api/test",
		Payload:  map[string]string{},
		Timeout:  50 * time.Millisecond,
	})

	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestSendHTTPRequestJSON_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorResponse{ErrorMessage: "internal error"})
	}))
	defer server.Close()

	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Method:   http.MethodPost,
		Path:     "/api/test",
		Payload:  map[string]string{},
	})

	if err == nil {
		t.Fatal("Expected error for server error")
	}
}

func TestSendHTTPRequestJSON_InvalidURL(t *testing.T) {
	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: "http://invalid-url-that-doesnt-exist.local:99999", Token: "token"},
		Method:   http.MethodPost,
		Path:     "/api/test",
		Payload:  map[string]string{},
		Timeout:  1 * time.Second,
	})

	if err == nil {
		t.Error("Expected error for invalid URL")
	}
}

func TestSendHTTPRequestJSON_NilResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	// Response is nil - should still work
	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]string, interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Method:   http.MethodPost,
		Path:     "/api/test",
		Payload:  map[string]string{},
		Response: nil,
	})

	if err != nil {
		t.Fatalf("Should not error with nil response: %v", err)
	}
}

func TestGraphQLResponse_Structure(t *testing.T) {
	type TestData struct {
		Field string `json:"field"`
	}

	resp := GraphQLResponse[TestData]{
		Data: TestData{Field: "value"},
		Errors: []GraphQLError{
			{
				Message: "test error",
				Extensions: map[string]interface{}{
					"code": "TEST_ERROR",
				},
				Path: []interface{}{"query", "field"},
			},
		},
	}

	// Marshal and unmarshal
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded GraphQLResponse[TestData]
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Data.Field != "value" {
		t.Errorf("Expected field 'value', got '%s'", decoded.Data.Field)
	}

	if len(decoded.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(decoded.Errors))
	}

	if decoded.Errors[0].Message != "test error" {
		t.Errorf("Expected error message 'test error', got '%s'", decoded.Errors[0].Message)
	}
}

func TestGraphQLError_Structure(t *testing.T) {
	err := GraphQLError{
		Message: "Field not found",
		Extensions: map[string]interface{}{
			"code":      "NOT_FOUND",
			"timestamp": "2024-01-15",
		},
		Path: []interface{}{"query", "user", 0, "name"},
	}

	data, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("Failed to marshal: %v", marshalErr)
	}

	var decoded GraphQLError
	if unmarshalErr := json.Unmarshal(data, &decoded); unmarshalErr != nil {
		t.Fatalf("Failed to unmarshal: %v", unmarshalErr)
	}

	if decoded.Message != "Field not found" {
		t.Errorf("Message mismatch")
	}

	if decoded.Extensions["code"] != "NOT_FOUND" {
		t.Errorf("Extensions mismatch")
	}

	if len(decoded.Path) != 4 {
		t.Errorf("Path should have 4 elements")
	}
}

func TestSendGraphQLRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a POST to GraphQL endpoint
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v2/graphql" {
			t.Errorf("Expected /api/v2/graphql, got %s", r.URL.Path)
		}

		// Parse request body
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		if body["query"] == nil {
			t.Error("Request should contain query")
		}

		// Send response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{"result": "success"},
		})
	}))
	defer server.Close()

	type response struct {
		Data struct {
			Result string `json:"result"`
		} `json:"data"`
	}

	var resp response
	err := SendGraphQLRequest(GraphQLRequestOptions[response]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Query:    "query { test }",
		Response: &resp,
	})

	if err != nil {
		t.Fatalf("SendGraphQLRequest failed: %v", err)
	}

	if resp.Data.Result != "success" {
		t.Errorf("Expected result 'success', got '%s'", resp.Data.Result)
	}
}

func TestSendGraphQLRequest_WithVariables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)

		// Verify variables are present
		if body["variables"] == nil {
			t.Error("Request should contain variables")
		}

		vars := body["variables"].(map[string]interface{})
		if vars["id"] != "123" {
			t.Errorf("Expected id '123', got '%v'", vars["id"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{"id": "123"},
		})
	}))
	defer server.Close()

	type response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	var resp response
	err := SendGraphQLRequest(GraphQLRequestOptions[response]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Query:    "query GetItem($id: ID!) { item(id: $id) { id } }",
		Variables: map[string]interface{}{
			"id": "123",
		},
		Response: &resp,
	})

	if err != nil {
		t.Fatalf("SendGraphQLRequest failed: %v", err)
	}
}

func TestSendGraphQLRequest_CustomTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer server.Close()

	err := SendGraphQLRequest(GraphQLRequestOptions[interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{APIEndpoint: server.URL, Token: "token"},
		Query:    "query { test }",
		Timeout:  50 * time.Millisecond,
	})

	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestGraphQLRequestOptions_DefaultTimeout(t *testing.T) {
	opts := GraphQLRequestOptions[interface{}]{
		Context:  context.Background(),
		Endpoint: Endpoint{},
		Query:    "query { test }",
	}

	if opts.Timeout != 0 {
		t.Error("Timeout should default to 0 (will use 30s internally)")
	}
}

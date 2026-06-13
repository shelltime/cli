package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type m3payload struct {
	Name string `json:"name"`
}
type m3resp struct {
	OK bool `json:"ok"`
}

// TestSendHTTPRequestJSON_ErrorBodyNotJSON covers the branch where a non-200
// response body cannot be parsed as an errorResponse: a generic "HTTP error: N"
// is returned instead of the (absent) server message.
func TestSendHTTPRequestJSON_ErrorBodyNotJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>gateway down</html>"))
	}))
	defer server.Close()

	err := SendHTTPRequestJSON(HTTPRequestOptions[m3payload, m3resp]{
		Context:  context.Background(),
		Endpoint: Endpoint{Token: "t", APIEndpoint: server.URL},
		Method:   http.MethodPost,
		Path:     "/x",
		Payload:  m3payload{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP error: 502")
}

// TestSendGraphQLRequest_SurfacesGraphQLErrors covers the branch in
// SendGraphQLRequest where the HTTP layer succeeds (200) but the GraphQL body
// carries an errors array; the first error message is surfaced.
func TestSendGraphQLRequest_SurfacesGraphQLErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{},"errors":[{"message":"field missing"}]}`))
	}))
	defer server.Close()

	var resp GraphQLResponse[map[string]interface{}]
	err := SendGraphQLRequest(GraphQLRequestOptions[GraphQLResponse[map[string]interface{}]]{
		Context:  context.Background(),
		Endpoint: Endpoint{Token: "t", APIEndpoint: server.URL},
		Query:    "query { x }",
		Response: &resp,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field missing")
}

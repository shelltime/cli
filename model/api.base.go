package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPRequestOptions contains all options for sending an HTTP request
type HTTPRequestOptions[T any, R any] struct {
	Context     context.Context
	Endpoint    Endpoint
	Method      string
	Path        string
	Payload     T
	Response    *R
	ContentType string        // Optional, defaults to "application/json"
	Timeout     time.Duration // Optional, defaults to 10 seconds
}

// SendHTTPRequestJSON is a generic HTTP request function that sends JSON data and unmarshals the response
func SendHTTPRequestJSON[T any, R any](opts HTTPRequestOptions[T, R]) error {
	ctx, span := modelTracer.Start(opts.Context, "http.send.json")
	defer span.End()

	jsonData, err := json.Marshal(opts.Payload)
	if err != nil {
		slog.Error("failed to marshal payload", slog.Any("err", err))
		return err
	}

	timeout := time.Second * 10
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, opts.Endpoint.APIEndpoint+opts.Path, bytes.NewBuffer(jsonData))
	if err != nil {
		slog.Error("failed to create request", slog.Any("err", err))
		return err
	}

	contentType := "application/json"
	if opts.ContentType != "" {
		contentType = opts.ContentType
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", fmt.Sprintf("shelltimeCLI@%s", commitID))
	req.Header.Set("Authorization", "CLI "+opts.Endpoint.Token)

	slog.Debug("http request", slog.String("url", req.URL.String()))

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("failed to send request", slog.Any("err", err))
		return err
	}
	defer resp.Body.Close()

	slog.Debug("http response", slog.String("status", resp.Status))

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to read response body", slog.Any("err", err))
		return err
	}

	if resp.StatusCode != http.StatusOK {
		var msg errorResponse
		err = json.Unmarshal(buf, &msg)
		if err != nil {
			slog.Error("Failed to parse error response", slog.Any("err", err))
			return fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}
		slog.Error("Error response", slog.String("message", msg.ErrorMessage))
		return errors.New(msg.ErrorMessage)
	}

	// Only try to unmarshal if we have a response struct
	if opts.Response != nil {
		err = json.Unmarshal(buf, opts.Response)
		if err != nil {
			slog.Error("Failed to unmarshal JSON response", slog.Any("err", err))
			return err
		}
	}

	return nil
}

// GraphQLResponse is a generic wrapper for GraphQL responses
type GraphQLResponse[T any] struct {
	Data   T              `json:"data"`
	Errors []GraphQLError `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error
type GraphQLError struct {
	Message    string                 `json:"message"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
	Path       []interface{}          `json:"path,omitempty"`
}

// GraphQLRequestOptions contains options for GraphQL requests
type GraphQLRequestOptions[R any] struct {
	Context   context.Context
	Endpoint  Endpoint
	Query     string
	Variables map[string]interface{}
	Response  *R
	Timeout   time.Duration // Optional, defaults to 30 seconds
}

// SendGraphQLRequest sends a GraphQL request and unmarshals the response
func SendGraphQLRequest[R any](opts GraphQLRequestOptions[R]) error {
	ctx, span := modelTracer.Start(opts.Context, "graphql.send")
	defer span.End()

	// Build GraphQL payload
	payload := map[string]interface{}{
		"query": opts.Query,
	}
	if opts.Variables != nil {
		payload["variables"] = opts.Variables
	}

	// Build GraphQL endpoint path
	graphQLPath := "/api/v2/graphql"

	// Set default timeout
	timeout := time.Second * 30
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	// Use the new JSON HTTP request function
	err := SendHTTPRequestJSON(HTTPRequestOptions[map[string]interface{}, R]{
		Context:  ctx,
		Endpoint: opts.Endpoint,
		Method:   http.MethodPost,
		Path:     graphQLPath,
		Payload:  payload,
		Response: opts.Response,
		Timeout:  timeout,
	})

	if err != nil {
		// The error is already formatted by SendHTTPRequestJSON
		return err
	}

	// Check for GraphQL errors in the response if we have a response
	if opts.Response != nil {
		// Marshal response back to check for errors
		respBytes, err := json.Marshal(opts.Response)
		if err == nil {
			var errorCheck struct {
				Errors []GraphQLError `json:"errors,omitempty"`
			}
			if err := json.Unmarshal(respBytes, &errorCheck); err == nil && len(errorCheck.Errors) > 0 {
				// Return the first error message if there are GraphQL errors
				return fmt.Errorf("GraphQL error: %s", errorCheck.Errors[0].Message)
			}
		}
	}

	return nil
}

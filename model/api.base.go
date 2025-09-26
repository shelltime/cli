package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
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
	ContentType string        // Optional, defaults to "application/msgpack"
	Timeout     time.Duration // Optional, defaults to 10 seconds
}

// SendHTTPRequest is a generic HTTP request function that sends data and unmarshals the response
func SendHTTPRequest[T any, R any](opts HTTPRequestOptions[T, R]) error {
	ctx, span := modelTracer.Start(opts.Context, "http.send")
	defer span.End()

	jsonData, err := msgpack.Marshal(opts.Payload)
	if err != nil {
		logrus.Errorln(err)
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
		logrus.Errorln(err)
		return err
	}

	contentType := "application/msgpack"
	if opts.ContentType != "" {
		contentType = opts.ContentType
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", fmt.Sprintf("shelltimeCLI@%s", commitID))
	req.Header.Set("Authorization", "CLI "+opts.Endpoint.Token)

	logrus.Traceln("http: ", req.URL.String())

	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorln(err)
		return err
	}
	defer resp.Body.Close()

	logrus.Traceln("http: ", resp.Status)

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorln(err)
		return err
	}

	if resp.StatusCode != http.StatusOK {
		var msg errorResponse
		err = json.Unmarshal(buf, &msg)
		if err != nil {
			logrus.Errorln("Failed to parse error response:", err)
			return fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}
		logrus.Errorln("Error response:", msg.ErrorMessage)
		return errors.New(msg.ErrorMessage)
	}

	// Only try to unmarshal if we have a response struct
	if opts.Response != nil {
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "json") {
			err = json.Unmarshal(buf, opts.Response)
			if err != nil {
				logrus.Errorln("Failed to unmarshal JSON response:", err)
				return err
			}
			return nil
		}
		if strings.Contains(contentType, "msgpack") {
			err = msgpack.Unmarshal(buf, opts.Response)
			if err != nil {
				logrus.Errorln("Failed to unmarshal response:", err)
				return err
			}
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

	payload := map[string]interface{}{
		"query": opts.Query,
	}
	if opts.Variables != nil {
		payload["variables"] = opts.Variables
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal GraphQL query: %w", err)
	}

	timeout := time.Second * 30
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	// Build GraphQL endpoint URL
	graphQLEndpoint := opts.Endpoint.APIEndpoint
	graphQLEndpoint = strings.TrimSuffix(graphQLEndpoint, "/")
	if !strings.HasSuffix(graphQLEndpoint, "/api/v2/graphql") {
		graphQLEndpoint += "/api/v2/graphql"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphQLEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "CLI "+opts.Endpoint.Token)
	req.Header.Set("User-Agent", fmt.Sprintf("shelltimeCLI@%s", commitID))

	logrus.Traceln("graphql: ", req.URL.String())

	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorln(err)
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	logrus.Traceln("graphql: ", resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Unmarshal the response
	if opts.Response != nil {
		if err := json.Unmarshal(body, opts.Response); err != nil {
			return fmt.Errorf("failed to parse GraphQL response: %w", err)
		}

		// Check for GraphQL errors in the response
		// Try to extract errors if the response type is GraphQLResponse
		var errorCheck struct {
			Errors []GraphQLError `json:"errors,omitempty"`
		}
		if err := json.Unmarshal(body, &errorCheck); err == nil && len(errorCheck.Errors) > 0 {
			// Return the first error message if there are GraphQL errors
			return fmt.Errorf("GraphQL error: %s", errorCheck.Errors[0].Message)
		}
	}

	return nil
}

package model

import (
	"context"
	"net/http"
	"time"
)

// SendAICodeHooksData sends hook event data to the backend
// POST /api/v1/cc/aicode-hooks
func SendAICodeHooksData(ctx context.Context, req *AICodeHooksRequest, endpoint Endpoint) (*AICodeHooksResponse, error) {
	ctx, span := modelTracer.Start(ctx, "aicode_hooks.send")
	defer span.End()

	var resp AICodeHooksResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*AICodeHooksRequest, AICodeHooksResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/aicode-hooks",
		Payload:  req,
		Response: &resp,
		Timeout:  30 * time.Second,
	})

	if err != nil {
		return nil, err
	}

	return &resp, nil
}

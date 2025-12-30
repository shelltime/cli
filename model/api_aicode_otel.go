package model

import (
	"context"
	"net/http"
	"time"
)

// SendAICodeOtelData sends OTEL data to the backend immediately
// POST /api/v1/cc/otel
func SendAICodeOtelData(ctx context.Context, req *AICodeOtelRequest, endpoint Endpoint) (*AICodeOtelResponse, error) {
	ctx, span := modelTracer.Start(ctx, "aicode_otel.send")
	defer span.End()

	var resp AICodeOtelResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*AICodeOtelRequest, AICodeOtelResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/otel",
		Payload:  req,
		Response: &resp,
		Timeout:  30 * time.Second,
	})

	if err != nil {
		return nil, err
	}

	return &resp, nil
}

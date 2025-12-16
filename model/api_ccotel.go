package model

import (
	"context"
	"net/http"
	"time"
)

// SendCCOtelData sends OTEL data to the backend immediately
// POST /api/v1/cc/otel
func SendCCOtelData(ctx context.Context, req *CCOtelRequest, endpoint Endpoint) (*CCOtelResponse, error) {
	ctx, span := modelTracer.Start(ctx, "ccotel.send")
	defer span.End()

	var resp CCOtelResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*CCOtelRequest, CCOtelResponse]{
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

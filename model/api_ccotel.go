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

// SendCCSessionEnd notifies the backend that a session has ended
// POST /api/v1/cc/session/end
func SendCCSessionEnd(ctx context.Context, req *CCSessionEndRequest, endpoint Endpoint) error {
	ctx, span := modelTracer.Start(ctx, "ccotel.session.end")
	defer span.End()

	var resp CCSessionEndResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*CCSessionEndRequest, CCSessionEndResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/session/end",
		Payload:  req,
		Response: &resp,
		Timeout:  30 * time.Second,
	})

	return err
}

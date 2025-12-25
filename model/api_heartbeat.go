package model

import (
	"context"
	"net/http"
	"time"
)

// SendHeartbeatsToServer sends heartbeat data to the server
func SendHeartbeatsToServer(ctx context.Context, cfg ShellTimeConfig, payload HeartbeatPayload) error {
	ctx, span := modelTracer.Start(ctx, "api.sendHeartbeats")
	defer span.End()

	endpoint := Endpoint{
		Token:       cfg.Token,
		APIEndpoint: cfg.APIEndpoint,
	}

	var response HeartbeatResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[HeartbeatPayload, HeartbeatResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/heartbeats",
		Payload:  payload,
		Response: &response,
		Timeout:  10 * time.Second,
	})

	if err != nil {
		return err
	}

	return nil
}

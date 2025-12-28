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

	// Use custom endpoint/token from CodeTracking if specified, otherwise fall back to global
	apiEndpoint := cfg.APIEndpoint
	token := cfg.Token

	if cfg.CodeTracking != nil {
		if cfg.CodeTracking.APIEndpoint != "" {
			apiEndpoint = cfg.CodeTracking.APIEndpoint
		}
		if cfg.CodeTracking.Token != "" {
			token = cfg.CodeTracking.Token
		}
	}

	endpoint := Endpoint{
		Token:       token,
		APIEndpoint: apiEndpoint,
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

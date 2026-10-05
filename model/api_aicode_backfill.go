package model

import (
	"context"
	"net/http"
	"time"
)

const aiCodeBackfillTimeout = 60 * time.Second

// FetchAICodeBackfillStatus asks which of the given sessions the server
// already has. Sessions it knows nothing about are not in the result.
// POST /api/v1/cc/backfill/sessions
func FetchAICodeBackfillStatus(ctx context.Context, endpoint Endpoint, clientType string, sessionIDs []string) ([]AICodeBackfillSessionStatus, error) {
	ctx, span := modelTracer.Start(ctx, "aicode_backfill.status")
	defer span.End()

	var resp AICodeBackfillSessionsResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[AICodeBackfillSessionsRequest, AICodeBackfillSessionsResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/backfill/sessions",
		Payload:  AICodeBackfillSessionsRequest{ClientType: clientType, SessionIDs: sessionIDs},
		Response: &resp,
		Timeout:  aiCodeBackfillTimeout,
	})
	if err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

// SendAICodeBackfill uploads one batch of historical events.
// POST /api/v1/cc/backfill
func SendAICodeBackfill(ctx context.Context, endpoint Endpoint, req AICodeBackfillRequest) (*AICodeBackfillResponse, error) {
	ctx, span := modelTracer.Start(ctx, "aicode_backfill.send")
	defer span.End()

	var resp AICodeBackfillResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[AICodeBackfillRequest, AICodeBackfillResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/backfill",
		Payload:  req,
		Response: &resp,
		Timeout:  aiCodeBackfillTimeout,
	})
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// CompleteAICodeBackfill tells the server a backfill finished so it can
// refresh activity data and caches for the backfilled range.
// POST /api/v1/cc/backfill/complete
func CompleteAICodeBackfill(ctx context.Context, endpoint Endpoint, req AICodeBackfillCompleteRequest) error {
	ctx, span := modelTracer.Start(ctx, "aicode_backfill.complete")
	defer span.End()

	return SendHTTPRequestJSON(HTTPRequestOptions[AICodeBackfillCompleteRequest, struct{}]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/backfill/complete",
		Payload:  req,
		Timeout:  aiCodeBackfillTimeout,
	})
}

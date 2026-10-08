package model

import (
	"context"
	"net/http"
	"time"
)

type sessionPullRequestsRequest struct {
	SessionID string   `json:"sessionId"`
	URLs      []string `json:"urls"`
}

type sessionPullRequestsResponse struct{}

// SendSessionPullRequests links pull requests opened in a Claude Code session
// (`gh pr create`) to that session on the server
func SendSessionPullRequests(ctx context.Context, config ShellTimeConfig, sessionID string, urls []string) error {
	ctx, span := modelTracer.Start(ctx, "session_pull_requests.send")
	defer span.End()

	var resp sessionPullRequestsResponse
	return SendHTTPRequestJSON(HTTPRequestOptions[*sessionPullRequestsRequest, sessionPullRequestsResponse]{
		Context: ctx,
		Endpoint: Endpoint{
			APIEndpoint: config.APIEndpoint,
			Token:       config.Token,
		},
		Method: http.MethodPost,
		Path:   "/api/v1/cc/session-pull-requests",
		Payload: &sessionPullRequestsRequest{
			SessionID: sessionID,
			URLs:      urls,
		},
		Response: &resp,
		Timeout:  5 * time.Second,
	})
}

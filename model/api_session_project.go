package model

import (
	"context"
	"net/http"
	"time"
)

type sessionProjectRequest struct {
	SessionID   string `json:"sessionId"`
	ProjectPath string `json:"projectPath"`
	// Repository is the canonical URL of the git repository ProjectPath belongs to (see
	// CanonicalRepositoryURL); empty outside a repository or without a remote.
	Repository string `json:"repository,omitempty"`
}

type sessionProjectResponse struct{}

// SendSessionProjectUpdate sends a session's working directory, and the repository it belongs
// to when there is one, to the server, which fills them in on the session's events.
func SendSessionProjectUpdate(ctx context.Context, config ShellTimeConfig, sessionID, projectPath, repository string) error {
	ctx, span := modelTracer.Start(ctx, "session_project.send")
	defer span.End()

	var resp sessionProjectResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*sessionProjectRequest, sessionProjectResponse]{
		Context: ctx,
		Endpoint: Endpoint{
			APIEndpoint: config.APIEndpoint,
			Token:       config.Token,
		},
		Method: http.MethodPost,
		Path:   "/api/v1/cc/session-project",
		Payload: &sessionProjectRequest{
			SessionID:   sessionID,
			ProjectPath: projectPath,
			Repository:  repository,
		},
		Response: &resp,
		Timeout:  5 * time.Second,
	})

	return err
}

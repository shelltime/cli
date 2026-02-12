package model

import (
	"context"
	"net/http"
	"time"
)

type sessionProjectRequest struct {
	SessionID   string `json:"session_id"`
	ProjectPath string `json:"project_path"`
}

type sessionProjectResponse struct{}

// SendSessionProjectUpdate sends a session-to-project path mapping to the server
func SendSessionProjectUpdate(ctx context.Context, config ShellTimeConfig, sessionID, projectPath string) error {
	ctx, span := modelTracer.Start(ctx, "session_project.send")
	defer span.End()

	var resp sessionProjectResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*sessionProjectRequest, sessionProjectResponse]{
		Context: ctx,
		Endpoint: Endpoint{
			APIEndpoint: config.APIEndpoint,
			Token:       config.Token,
		},
		Method:  http.MethodPost,
		Path:    "/api/v1/cc/session-project",
		Payload: &sessionProjectRequest{
			SessionID:   sessionID,
			ProjectPath: projectPath,
		},
		Response: &resp,
		Timeout:  5 * time.Second,
	})

	return err
}

package model

import (
	"context"
	"net/http"
	"time"
)

// Server limits of the project repository endpoints.
const (
	ProjectRepositoriesMaxHosts    = 20
	ProjectRepositoriesMaxProjects = 500
)

// UnresolvedProject is a project the server still groups by folder: sessions on Host that ran in
// Pwd and have no git repository.
type UnresolvedProject struct {
	Host     string `json:"host"`
	Pwd      string `json:"pwd"`
	Sessions int    `json:"sessions"`
}

// ProjectRepository moves the sessions of a folder project into the project of the git
// repository the folder is a clone of.
type ProjectRepository struct {
	Host       string `json:"host"`
	Pwd        string `json:"pwd"`
	Repository string `json:"repository"` // canonical URL, see CanonicalRepositoryURL
}

type unresolvedProjectsRequest struct {
	Hosts []string `json:"hosts"`
}

type unresolvedProjectsResponse struct {
	Projects []UnresolvedProject `json:"projects"`
	// Truncated is true when the server matched more folders than it listed.
	Truncated bool `json:"truncated"`
}

type projectRepositoriesRequest struct {
	Projects []ProjectRepository `json:"projects"`
}

type projectRepositoriesResponse struct {
	Updated int `json:"updated"`
}

// FetchUnresolvedProjects lists the folder projects of the given hosts, the names this machine
// has reported its sessions under. truncated is true when the server matched more than it
// listed.
func FetchUnresolvedProjects(ctx context.Context, endpoint Endpoint, hosts []string) (projects []UnresolvedProject, truncated bool, err error) {
	ctx, span := modelTracer.Start(ctx, "project_repositories.unresolved")
	defer span.End()

	var resp unresolvedProjectsResponse
	err = SendHTTPRequestJSON(HTTPRequestOptions[*unresolvedProjectsRequest, unresolvedProjectsResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/projects/unresolved",
		Payload:  &unresolvedProjectsRequest{Hosts: hosts},
		Response: &resp,
		Timeout:  30 * time.Second,
	})
	return resp.Projects, resp.Truncated, err
}

// SendProjectRepositories moves folder projects into their repositories' projects and returns
// how many sessions moved.
func SendProjectRepositories(ctx context.Context, endpoint Endpoint, projects []ProjectRepository) (int, error) {
	ctx, span := modelTracer.Start(ctx, "project_repositories.send")
	defer span.End()

	var resp projectRepositoriesResponse
	err := SendHTTPRequestJSON(HTTPRequestOptions[*projectRepositoriesRequest, projectRepositoriesResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/cc/projects/repositories",
		Payload:  &projectRepositoriesRequest{Projects: projects},
		Response: &resp,
		Timeout:  30 * time.Second,
	})
	return resp.Updated, err
}

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// DotfileItem represents a single dotfile to be sent to the server
type DotfileItem struct {
	App            string                 `json:"app" msgpack:"app"`
	Path           string                 `json:"path" msgpack:"path"`
	Content        string                 `json:"content" msgpack:"content"`
	FileModifiedAt *time.Time             `json:"fileModifiedAt" msgpack:"fileModifiedAt"`
	FileType       string                 `json:"fileType" msgpack:"fileType"`
	Metadata       map[string]interface{} `json:"metadata" msgpack:"metadata"`
	Hostname       string                 `json:"hostname" msgpack:"hostname"`
}

type dotfilePushRequest struct {
	Dotfiles []DotfileItem `json:"dotfiles" msgpack:"dotfiles"`
}

type dotfileResponseItem struct {
	ID          int                    `json:"id"`
	App         string                 `json:"app"`
	Path        string                 `json:"path"`
	ContentHash string                 `json:"contentHash"`
	Size        int64                  `json:"size"`
	FileType    string                 `json:"fileType"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	Status      string                 `json:"status"` // "created", "existing", "error"
	Error       string                 `json:"error,omitempty"`
}

type dotfilePushResponse struct {
	Success int                   `json:"success"`
	Failed  int                   `json:"failed"`
	Results []dotfileResponseItem `json:"results"`
	UserID  int                   `json:"userId"`
}

// SendDotfilesToServer sends the collected dotfiles to the server
func SendDotfilesToServer(ctx context.Context, endpoint Endpoint, dotfiles []DotfileItem) (int, error) {
	if len(dotfiles) == 0 {
		logrus.Infoln("No dotfiles to send")
		return 0, nil
	}

	// Get system info for hostname
	hostname, err := os.Hostname()
	if err != nil {
		logrus.Warnln("Failed to get hostname:", err)
		hostname = "unknown"
	}

	// Set hostname for all dotfiles if not already set
	for i := range dotfiles {
		if dotfiles[i].Hostname == "" {
			dotfiles[i].Hostname = hostname
		}
	}

	payload := dotfilePushRequest{
		Dotfiles: dotfiles,
	}

	var resp dotfilePushResponse

	err = SendHTTPRequest(HTTPRequestOptions[dotfilePushRequest, dotfilePushResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/dotfiles/push",
		Payload:  payload,
		Response: &resp,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to send dotfiles to server: %w", err)
	}

	logrus.Infof("Pushed dotfiles successfully - Success: %d, Failed: %d", resp.Success, resp.Failed)

	// Log any errors
	for _, result := range resp.Results {
		if result.Status == "error" {
			logrus.Warnf("Error pushing %s (%s): %s", result.App, result.Path, result.Error)
		}
	}

	return resp.UserID, nil
}

// GraphQL types for fetching dotfiles
type DotfileFilter struct {
	Apps []string `json:"apps,omitempty"`
}

type DotfileRecord struct {
	ID             int                    `json:"id"`
	Content        string                 `json:"content"`
	ContentHash    string                 `json:"contentHash"`
	Size           int64                  `json:"size"`
	FileModifiedAt *time.Time             `json:"fileModifiedAt"`
	FileType       string                 `json:"fileType"`
	Metadata       map[string]interface{} `json:"metadata"`
	Host           struct {
		ID       int    `json:"id"`
		Hostname string `json:"hostname"`
		Alias    string `json:"alias"`
	} `json:"host"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type DotfileFile struct {
	Path    string          `json:"path"`
	Records []DotfileRecord `json:"records"`
}

type DotfileAppResponse struct {
	App   string        `json:"app"`
	Files []DotfileFile `json:"files"`
}

type FetchDotfilesResponse struct {
	Data struct {
		FetchUser struct {
			ID       int `json:"id"`
			Dotfiles struct {
				TotalCount int                  `json:"totalCount"`
				Apps       []DotfileAppResponse `json:"apps"`
			} `json:"dotfiles"`
		} `json:"fetchUser"`
	} `json:"data"`
}

// FetchDotfilesFromServer fetches dotfiles from the server using GraphQL
func FetchDotfilesFromServer(ctx context.Context, endpoint Endpoint, filter *DotfileFilter) (*FetchDotfilesResponse, error) {
	query := `query fetchUserDotfiles($filter: DotfileFilter) {
		fetchUser {
			id
			dotfiles(filter: $filter) {
				totalCount
				apps {
					app
					files {
						path
						records {
							id
							content
							contentHash
							size
							fileModifiedAt
							fileType
							metadata
							host {
								id
								hostname
								alias
							}
							createdAt
							updatedAt
						}
					}
				}
			}
		}
	}`

	variables := map[string]interface{}{}
	if filter != nil {
		variables["filter"] = filter
	}

	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: time.Second * 30,
	}

	// Use web endpoint for GraphQL queries
	graphQLEndpoint := endpoint.APIEndpoint
	graphQLEndpoint = strings.TrimSuffix(graphQLEndpoint, "/")
	if !strings.HasSuffix(graphQLEndpoint, "/api/v2/graphql") {
		graphQLEndpoint += "/api/v2/graphql"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphQLEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "CLI "+endpoint.Token)
	req.Header.Set("User-Agent", fmt.Sprintf("shelltimeCLI@%s", commitID))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result FetchDotfilesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	return &result, nil
}

package model

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
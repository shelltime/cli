package model

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/user"

	"github.com/sirupsen/logrus"
)

// Alias represents a shell alias
type Alias struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Shell string `json:"shell"`
}

type importShellAliasRequest struct {
	Aliases       []string `json:"aliases" msgpack:"aliases"`
	IsFullRefresh bool     `json:"isFullRefresh" msgpack:"isFullRefresh"`
	ShellType     string   `json:"shellType" msgpack:"shellType"`
	FileLocation  string   `json:"fileLocation" msgpack:"fileLocation"`

	Hostname  string `json:"hostname" msgpack:"hostname"`
	Username  string `json:"username" msgpack:"username"`
	OS        string `json:"os" msgpack:"os"`
	OSVersion string `json:"osVersion" msgpack:"osVersion"`
}

type importShellAliasResponse struct {
	Success bool `json:"success"`
	Count   int  `json:"count"`
}

// SendAliasesToServer sends the collected aliases to the server
func SendAliasesToServer(ctx context.Context, endpoint Endpoint, aliases []string, isFullyRefresh bool, shellType, fileLocation string) error {
	if len(aliases) == 0 {
		logrus.Infoln("No aliases to send")
		return nil
	}

	sysInfo, err := GetOSAndVersion()
	if err != nil {
		logrus.Warnln(err)
		sysInfo = &SysInfo{
			Os:      "unknown",
			Version: "unknown",
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		logrus.Warnln("Failed to get hostname:", err)
		hostname = "unknown"
	}

	username := os.Getenv("USER")
	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			logrus.Warnln("Failed to get username:", err)
			username = "unknown"
		} else {
			username = currentUser.Username
		}
	}

	payload := importShellAliasRequest{
		Aliases:       aliases,
		IsFullRefresh: isFullyRefresh,
		ShellType:     shellType,
		FileLocation:  fileLocation,
		Hostname:      hostname,
		Username:      username,
		OS:            sysInfo.Os,
		OSVersion:     sysInfo.Version,
	}

	var resp importShellAliasResponse

	err = SendHTTPRequest(HTTPRequestOptions[importShellAliasRequest, importShellAliasResponse]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/import-alias",
		Payload:  payload,
		Response: &resp,
	})
	if err != nil {
		return fmt.Errorf("failed to send aliases to server: %w", err)
	}

	logrus.Infoln("save aliases successfully", resp.Count)
	return nil
}

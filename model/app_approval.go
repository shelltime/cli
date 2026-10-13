package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// AppApprovalTicket is a sign-in request waiting for approval in the ShellTime
// iOS app.
type AppApprovalTicket struct {
	RequestID string `json:"requestId"`
	// PollToken authenticates our polls; only this CLI has it
	PollToken string `json:"pollToken"`
	// MatchCode is also shown in the iOS app, so the user can check the
	// request they approve is this one
	MatchCode string `json:"matchCode"`
	ExpiresIn int    `json:"expiresIn"`
	// Interval is the number of seconds between polls
	Interval int `json:"interval"`
}

type AppApprovalStatus string

const (
	AppApprovalPending  AppApprovalStatus = "pending"
	AppApprovalApproved AppApprovalStatus = "approved"
	AppApprovalDenied   AppApprovalStatus = "denied"
	AppApprovalExpired  AppApprovalStatus = "expired"
)

type AppApprovalPollResult struct {
	Status    AppApprovalStatus `json:"status"`
	ExpiresIn int               `json:"expiresIn,omitempty"`
	// OpenToken is set on the one poll that collects an approved request
	OpenToken *struct {
		ID    int    `json:"id"`
		Token string `json:"token"`
	} `json:"openToken,omitempty"`
}

// AppApprovalError is the API refusing a request, with its message for the user.
type AppApprovalError struct {
	StatusCode int
	Message    string
}

func (e *AppApprovalError) Error() string {
	return fmt.Sprintf("app approval: HTTP %d: %s", e.StatusCode, e.Message)
}

// AppApprovalService signs the CLI in by having the user approve a push in the
// ShellTime iOS app.
type AppApprovalService interface {
	// Request pushes a sign-in request to the iOS app of the account with this email.
	Request(ctx context.Context, email string) (AppApprovalTicket, error)
	// Poll reports the request's status; the first poll after approval carries the token.
	Poll(ctx context.Context, requestID, pollToken string) (AppApprovalPollResult, error)
}

type appApprovalService struct {
	config ShellTimeConfig
}

func NewAppApprovalService(config ShellTimeConfig) AppApprovalService {
	return appApprovalService{config: config}
}

type appApprovalRequestBody struct {
	Email     string `json:"email"`
	Client    string `json:"client"`
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	OSVersion string `json:"osVersion"`
}

type appApprovalPollBody struct {
	RequestID string `json:"requestId"`
	PollToken string `json:"pollToken"`
}

func (s appApprovalService) Request(ctx context.Context, email string) (AppApprovalTicket, error) {
	sysInfo, err := GetOSAndVersion()
	if err != nil {
		slog.Error("failed to get OS version", slog.Any("err", err))
		sysInfo = &SysInfo{Os: "unknown", Version: "unknown"}
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	var ticket AppApprovalTicket
	err = s.post(ctx, "/request", appApprovalRequestBody{
		Email:     email,
		Client:    "cli",
		Hostname:  hostname,
		OS:        sysInfo.Os,
		OSVersion: sysInfo.Version,
	}, &ticket)
	return ticket, err
}

func (s appApprovalService) Poll(ctx context.Context, requestID, pollToken string) (AppApprovalPollResult, error) {
	var result AppApprovalPollResult
	err := s.post(ctx, "/poll", appApprovalPollBody{RequestID: requestID, PollToken: pollToken}, &result)
	return result, err
}

// post sends an unauthenticated request: the CLI has no token yet.
func (s appApprovalService) post(ctx context.Context, path string, payload, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.APIEndpoint+"/api/v1/auth/app-approval"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("shelltimeCLI@%s", commitID))

	resp, err := NewHTTPClient(30 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		message := http.StatusText(resp.StatusCode)
		var errResp errorResponse
		if json.Unmarshal(buf, &errResp) == nil && errResp.ErrorMessage != "" {
			message = errResp.ErrorMessage
		}
		return &AppApprovalError{StatusCode: resp.StatusCode, Message: message}
	}
	return json.Unmarshal(buf, result)
}

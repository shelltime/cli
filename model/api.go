package model

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
)

type errorResponse struct {
	ErrorCode    int    `json:"code"`
	ErrorMessage string `json:"error"`
}

type TrackingData struct {
	SessionID     int64  `json:"sessionId"`
	Command       string `json:"command"`
	StartTime     int64  `json:"startTime"`
	EndTime       int64  `json:"endTime"`
	StartTimeNano int64  `json:"startTimeNano"`
	EndTimeNano   int64  `json:"endTimeNano"`
	Result        int    `json:"result"`
	PPID          int    `json:"ppid,omitempty"`
	ViaSSH        *bool  `json:"viaSsh,omitempty"`
}

type TrackingMetaData struct {
	Hostname    string `json:"hostname"`
	Username    string `json:"username"`
	OS          string `json:"os"`
	OSVersion   string `json:"osVersion"`
	Shell       string `json:"shell"`
	Terminal    string `json:"terminal,omitempty"`
	Multiplexer string `json:"multiplexer,omitempty"`

	// CliEngine is the storage engine that buffered these commands:
	// StorageEngineFile or StorageEngineBolt.
	CliEngine string `json:"cliEngine,omitempty"`

	// 0: cli, 1: daemon
	Source int `json:"source"`
}

type PostTrackArgs struct {
	// nano timestamp
	CursorID int64            `json:"cursorId"`
	Data     []TrackingData   `json:"data"`
	Meta     TrackingMetaData `json:"meta"`

	Encrypted string `json:"encrypted"`
	// a base64 encoded AES-GCM key that encrypted by PublicKey from open token
	AesKey string `json:"aesKey"`
	// the AES-GCM nonce. not encrypted
	Nonce string `json:"nonce"`
}

func doSendData(ctx context.Context, endpoint Endpoint, data PostTrackArgs) error {
	err := SendHTTPRequestJSON(HTTPRequestOptions[PostTrackArgs, any]{
		Context:  ctx,
		Endpoint: endpoint,
		Method:   http.MethodPost,
		Path:     "/api/v1/track",
		Payload:  data,
		Response: nil,
	})
	slog.Debug("http track request", slog.String("path", "/api/v1/track"), slog.Int("dataLen", len(data.Data)))

	if err != nil {
		slog.Error("failed to send data", slog.Any("err", err))
		return err
	}
	return nil
}

// func SendLocalDataToServer(ctx context.Context, config ShellTimeConfig, cursor time.Time, trackingData []TrackingData, meta TrackingMetaData) error {
func SendLocalDataToServer(ctx context.Context, config ShellTimeConfig, data PostTrackArgs) error {
	ctx, span := modelTracer.Start(ctx, "sync.local")
	defer span.End()
	authPair := make([]Endpoint, 0, len(config.Endpoints)+1)
	if config.Token != "" {
		authPair = append(authPair, Endpoint{
			Token:       config.Token,
			APIEndpoint: config.APIEndpoint,
		})
	}
	authPair = append(authPair, config.Endpoints...)
	if len(authPair) == 0 {
		slog.Debug("no token available. do not sync to server")
		return nil
	}

	var wg sync.WaitGroup

	// One slot per endpoint, so what is returned doesn't depend on which request
	// happened to finish first.
	errs := make([]error, len(authPair))

	for i, pair := range authPair {
		wg.Go(func() {
			errs[i] = doSendData(ctx, pair, data)
		})
	}

	wg.Wait()

	failed := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			failed = append(failed, err)
		}
	}
	switch len(failed) {
	case 0:
		return nil
	case 1:
		return failed[0]
	default:
		// Every failure, in endpoint order (errors.Is and errors.As look at all of them)
		return errors.Join(failed...)
	}
}

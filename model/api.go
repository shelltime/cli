package model

import (
	"context"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

type errorResponse struct {
	ErrorCode    int    `json:"code"`
	ErrorMessage string `json:"error"`
}

type TrackingData struct {
	SessionID     int64  `json:"sessionId" msgpack:"sessionId"`
	Command       string `json:"command" msgpack:"command"`
	StartTime     int64  `json:"startTime" msgpack:"startTime"`
	EndTime       int64  `json:"endTime" msgpack:"endTime"`
	StartTimeNano int64  `json:"startTimeNano" msgpack:"startTimeNano"`
	EndTimeNano   int64  `json:"endTimeNano" msgpack:"endTimeNano"`
	Result        int    `json:"result" msgpack:"result"`
}

type TrackingMetaData struct {
	Hostname  string `json:"hostname" msgpack:"hostname"`
	Username  string `json:"username" msgpack:"username"`
	OS        string `json:"os" msgpack:"os"`
	OSVersion string `json:"osVersion" msgpack:"osVersion"`
	Shell     string `json:"shell" msgpack:"shell"`

	// 0: cli, 1: daemon
	Source int `json:"source" msgpack:"source"`
}

type PostTrackArgs struct {
	// nano timestamp
	CursorID int64            `json:"cursorId" msgpack:"cursorId"`
	Data     []TrackingData   `json:"data" msgpack:"data"`
	Meta     TrackingMetaData `json:"meta" msgpack:"meta"`

	Encrypted string `json:"encrypted" msgpack:"encrypted"`
	// a base64 encoded AES-GCM key that encrypted by PublicKey from open token
	AesKey string `json:"aesKey" msgpack:"aesKey"`
	// the AES-GCM nonce. not encrypted
	Nonce string `json:"nonce" msgpack:"nonce"`
}

func doSendData(ctx context.Context, endpoint Endpoint, data PostTrackArgs) error {

	err := SendHTTPRequest(HTTPRequestOptions[PostTrackArgs, any]{
		Context: ctx,
		Endpoint: endpoint,
		Method: http.MethodPost,
		Path: "/api/v1/track",
		Payload: data,
		Response: nil,
	})
	logrus.Traceln("http: ", "/api/v1/track", len(data.Data), data.Meta)

	if err != nil {
		logrus.Errorln(err)
		return err
	}
	return nil
}

// func SendLocalDataToServer(ctx context.Context, config ShellTimeConfig, cursor time.Time, trackingData []TrackingData, meta TrackingMetaData) error {
func SendLocalDataToServer(ctx context.Context, config ShellTimeConfig, data PostTrackArgs) error {
	ctx, span := modelTracer.Start(ctx, "sync.local")
	defer span.End()
	if config.Token == "" {
		logrus.Traceln("no token available. do not sync to server")
		return nil
	}

	var wg sync.WaitGroup

	wg.Add(len(config.Endpoints) + 1)

	authPair := make([]Endpoint, len(config.Endpoints)+1)

	authPair[0] = Endpoint{
		Token:       config.Token,
		APIEndpoint: config.APIEndpoint,
	}

	copy(authPair[1:], config.Endpoints)

	errs := make(chan error, len(authPair))

	for _, pair := range authPair {
		go func(pair Endpoint) {
			defer wg.Done()
			err := doSendData(ctx, pair, data)
			errs <- err
		}(pair)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

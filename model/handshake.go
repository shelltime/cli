package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type handshakeResponse struct {
	EncodedID string `json:"encodedId"`
	OpenToken *struct {
		ID    int    `json:"id"`
		Token string `json:"token"`
	} `json:"openToken,omitempty"`
}

type HandshakeService interface {
	Init(ctx context.Context) (string, error)
	Check(ctx context.Context, handshakeId string) (string, error)
}

type handshakeService struct {
	config ShellTimeConfig
}

func NewHandshakeService(config ShellTimeConfig) HandshakeService {
	return handshakeService{
		config: config,
	}
}

func (hs handshakeService) send(ctx context.Context, path string, jsonData []byte) (result handshakeResponse, errResp errorResponse, err error) {
	hc := http.Client{
		Timeout:   time.Second * 30,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, "POST", hs.config.APIEndpoint+"/api/v1/handshake"+path, bytes.NewBuffer(jsonData))
	if err != nil {
		logrus.Errorln(err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("shelltimeCLI@%s", commitID))
	resp, err := hc.Do(req)
	if err != nil {
		logrus.Errorln(err)
		return
	}
	defer resp.Body.Close()

	logrus.Traceln("http: ", resp.Status)
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorln(err)
		return
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		err = json.Unmarshal(buf, &result)
		return
	} else {
		err = json.Unmarshal(buf, &errResp)
		return
	}
}

type handshakeInitRequest struct {
	Hostname  string `json:"hostname" msgpack:"hostname"`
	OS        string `json:"os" msgpack:"os"`
	OSVersion string `json:"osVersion" msgpack:"osVersion"`
}

func (hs handshakeService) Init(ctx context.Context) (string, error) {
	sysInfo, err := GetOSAndVersion()
	if err != nil {
		logrus.Errorln(err)
		sysInfo = &SysInfo{
			Os:      "unknown",
			Version: "unknown",
		}
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	data := handshakeInitRequest{
		Hostname:  hostname,
		OS:        sysInfo.Os,
		OSVersion: sysInfo.Version,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorln(err)
		return "", err
	}

	result, errResp, err := hs.send(ctx, "/init", jsonData)
	if err != nil {
		return "", fmt.Errorf("handshake init error: %v", err)
	}

	if errResp.ErrorCode >= 300 {
		return "", errors.New(errResp.ErrorMessage)
	}

	return result.EncodedID, nil
}

type handshakeCheckRequest struct {
	EncodedID string `json:"hid" msgpack:"hid"`
}

func (hs handshakeService) Check(ctx context.Context, handshakeId string) (token string, err error) {
	data := handshakeCheckRequest{
		EncodedID: handshakeId,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		logrus.Errorln(err)
		return "", err
	}

	result, errResp, err := hs.send(ctx, "/check", jsonData)

	if err != nil {
		return
	}
	if errResp.ErrorCode >= 300 {
		err = errors.New(errResp.ErrorMessage)
		return
	}

	if result.OpenToken == nil {
		return
	}

	token = result.OpenToken.Token
	return
}

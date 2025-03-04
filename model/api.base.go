package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// HTTPRequestOptions contains all options for sending an HTTP request
type HTTPRequestOptions[T any, R any] struct {
	Context     context.Context
	Endpoint    Endpoint
	Method      string
	Path        string
	Payload     T
	Response    *R
	ContentType string        // Optional, defaults to "application/msgpack"
	Timeout     time.Duration // Optional, defaults to 10 seconds
}

// SendHTTPRequest is a generic HTTP request function that sends data and unmarshals the response
func SendHTTPRequest[T any, R any](opts HTTPRequestOptions[T, R]) error {
	ctx, span := modelTracer.Start(opts.Context, "http.send")
	defer span.End()

	jsonData, err := msgpack.Marshal(opts.Payload)
	if err != nil {
		logrus.Errorln(err)
		return err
	}

	timeout := time.Second * 10
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, opts.Endpoint.APIEndpoint+opts.Path, bytes.NewBuffer(jsonData))
	if err != nil {
		logrus.Errorln(err)
		return err
	}

	contentType := "application/msgpack"
	if opts.ContentType != "" {
		contentType = opts.ContentType
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", fmt.Sprintf("shelltimeCLI@%s", commitID))
	req.Header.Set("Authorization", "CLI "+opts.Endpoint.Token)

	logrus.Traceln("http: ", req.URL.String())

	resp, err := client.Do(req)
	if err != nil {
		logrus.Errorln(err)
		return err
	}
	defer resp.Body.Close()

	logrus.Traceln("http: ", resp.Status)

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		logrus.Errorln(err)
		return err
	}

	if resp.StatusCode != http.StatusOK {
		var msg errorResponse
		err = json.Unmarshal(buf, &msg)
		if err != nil {
			logrus.Errorln("Failed to parse error response:", err)
			return fmt.Errorf("HTTP error: %d", resp.StatusCode)
		}
		logrus.Errorln("Error response:", msg.ErrorMessage)
		return errors.New(msg.ErrorMessage)
	}

	// Only try to unmarshal if we have a response struct
	if opts.Response != nil {
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, "json") {
			err = json.Unmarshal(buf, opts.Response)
			if err != nil {
				logrus.Errorln("Failed to unmarshal JSON response:", err)
				return err
			}
			return nil
		}
		if strings.Contains(contentType, "msgpack") {
			err = msgpack.Unmarshal(buf, opts.Response)
			if err != nil {
				logrus.Errorln("Failed to unmarshal response:", err)
				return err
			}
		}
	}

	return nil
}

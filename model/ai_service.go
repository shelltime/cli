package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AIService interface {
	QueryCommandStream(ctx context.Context, vars CommandSuggestVariables, endpoint Endpoint, onToken func(token string)) error
}

type CommandSuggestVariables struct {
	Shell string `json:"shell"`
	Os    string `json:"os"`
	Query string `json:"query"`
}

type sseAIService struct{}

func NewAIService() AIService {
	return &sseAIService{}
}

func (s *sseAIService) QueryCommandStream(
	ctx context.Context,
	vars CommandSuggestVariables,
	endpoint Endpoint,
	onToken func(token string),
) error {
	body, err := json.Marshal(vars)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := strings.TrimRight(endpoint.APIEndpoint, "/") + "/api/v1/ai/command-suggest"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "CLI "+endpoint.Token)

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			var errResp errorResponse
			if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.ErrorMessage != "" {
				return fmt.Errorf("%s", errResp.ErrorMessage)
			}
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	var isError bool
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			isError = false
			continue
		}

		if v, ok := stripSSEField(line, "event:"); ok {
			if v == "error" {
				isError = true
			}
			continue
		}

		if v, ok := stripSSEField(line, "data:"); ok {
			if isError {
				return fmt.Errorf("server error: %s", v)
			}
			if v == "[DONE]" {
				return nil
			}
			onToken(v)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}

// stripSSEField returns the value after prefix, stripping one optional leading
// space per the SSE specification (§9.2 "If value starts with a U+0020 SPACE
// character, remove it from value").
func stripSSEField(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	v := line[len(prefix):]
	if strings.HasPrefix(v, " ") {
		v = v[1:]
	}
	return v, true
}

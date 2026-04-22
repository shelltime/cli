package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryCommandStream_ErrorResponseBody(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   interface{}
		expectedErrMsg string
	}{
		{
			name:       "quota exceeded returns error message from body",
			statusCode: http.StatusTooManyRequests,
			responseBody: errorResponse{
				ErrorCode:    http.StatusTooManyRequests,
				ErrorMessage: "monthly AI credit quota exceeded",
			},
			expectedErrMsg: "monthly AI credit quota exceeded",
		},
		{
			name:       "unauthorized returns error message from body",
			statusCode: http.StatusUnauthorized,
			responseBody: errorResponse{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "unauthorized",
			},
			expectedErrMsg: "unauthorized",
		},
		{
			name:       "service unavailable returns error message from body",
			statusCode: http.StatusServiceUnavailable,
			responseBody: errorResponse{
				ErrorCode:    http.StatusServiceUnavailable,
				ErrorMessage: "AI service is not available",
			},
			expectedErrMsg: "AI service is not available",
		},
		{
			name:           "non-JSON response falls back to status code",
			statusCode:     http.StatusInternalServerError,
			responseBody:   "not json",
			expectedErrMsg: fmt.Sprintf("server returned status %d", http.StatusInternalServerError),
		},
		{
			name:       "empty error message falls back to status code",
			statusCode: http.StatusBadRequest,
			responseBody: errorResponse{
				ErrorCode:    http.StatusBadRequest,
				ErrorMessage: "",
			},
			expectedErrMsg: fmt.Sprintf("server returned status %d", http.StatusBadRequest),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if s, ok := tt.responseBody.(string); ok {
					w.Write([]byte(s))
				} else {
					json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			svc := NewAIService()
			err := svc.QueryCommandStream(
				context.Background(),
				CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "test"},
				Endpoint{APIEndpoint: server.URL, Token: "test-token"},
				func(token string) {},
			)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.expectedErrMsg {
				t.Errorf("expected error %q, got %q", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

func TestQueryCommandStream_SSEParsing(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantErr       bool
		wantErrSubstr string
		wantTokens    []string
	}{
		{
			name:       "data with space and [DONE] terminates cleanly",
			body:       "data: [DONE]\n\n",
			wantTokens: nil,
		},
		{
			name:       "data without space and [DONE] terminates cleanly",
			body:       "data:[DONE]\n\n",
			wantTokens: nil,
		},
		{
			name:       "single data token with leading space is stripped",
			body:       "data: hello\n\ndata: [DONE]\n\n",
			wantTokens: []string{"hello"},
		},
		{
			name:       "single data token without leading space passes through",
			body:       "data:hello\n\ndata:[DONE]\n\n",
			wantTokens: []string{"hello"},
		},
		{
			name:       "multi-token stream concatenates without spurious spaces",
			body:       "data: ls\n\ndata:  -la\n\ndata: [DONE]\n\n",
			wantTokens: []string{"ls", " -la"},
		},
		{
			name:          "event error with space",
			body:          "event: error\ndata: boom\n\n",
			wantErr:       true,
			wantErrSubstr: "boom",
		},
		{
			name:          "event error without space",
			body:          "event:error\ndata:boom\n\n",
			wantErr:       true,
			wantErrSubstr: "boom",
		},
		{
			name:       "blank line resets error state between events",
			body:       "event: error\n\ndata: hello\n\ndata: [DONE]\n\n",
			wantTokens: []string{"hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			var got []string
			svc := NewAIService()
			err := svc.QueryCommandStream(
				context.Background(),
				CommandSuggestVariables{Shell: "bash", Os: "linux", Query: "test"},
				Endpoint{APIEndpoint: server.URL, Token: "test-token"},
				func(token string) { got = append(got, token) },
			)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (tokens=%v)", got)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("expected error to contain %q, got %q", tt.wantErrSubstr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.wantTokens) {
				t.Fatalf("token count mismatch: want %d %v, got %d %v", len(tt.wantTokens), tt.wantTokens, len(got), got)
			}
			for i, tok := range tt.wantTokens {
				if got[i] != tok {
					t.Errorf("token[%d] = %q, want %q", i, got[i], tok)
				}
			}
		})
	}
}

func TestStripSSEField(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		prefix  string
		wantVal string
		wantOk  bool
	}{
		{"no match", "foo:bar", "data:", "", false},
		{"match no space", "data:hello", "data:", "hello", true},
		{"match one space stripped", "data: hello", "data:", "hello", true},
		{"match two spaces preserves second", "data:  hello", "data:", " hello", true},
		{"empty value no space", "data:", "data:", "", true},
		{"empty value one space", "data: ", "data:", "", true},
		{"event error with space", "event: error", "event:", "error", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := stripSSEField(tt.line, tt.prefix)
			if ok != tt.wantOk || v != tt.wantVal {
				t.Errorf("stripSSEField(%q, %q) = (%q, %v), want (%q, %v)", tt.line, tt.prefix, v, ok, tt.wantVal, tt.wantOk)
			}
		})
	}
}

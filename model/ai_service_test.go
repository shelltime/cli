package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

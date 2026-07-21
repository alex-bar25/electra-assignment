package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"electra-assignment/internal/service"
)

func TestRouterReturnsJSONErrors(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		wantStatus    int
		wantErrorCode string
		wantAllow     string
	}{
		{name: "unknown route", method: http.MethodGet, path: "/missing", wantStatus: http.StatusNotFound, wantErrorCode: "not_found"},
		{name: "unsupported method", method: http.MethodPost, path: "/health", wantStatus: http.StatusMethodNotAllowed, wantErrorCode: "method_not_allowed", wantAllow: http.MethodGet},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			newTestHandler().ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("content type = %q, want application/json", response.Header().Get("Content-Type"))
			}
			if response.Header().Get("Allow") != test.wantAllow {
				t.Fatalf("Allow = %q, want %q", response.Header().Get("Allow"), test.wantAllow)
			}
			var body errorResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != test.wantErrorCode {
				t.Fatalf("error code = %q, want %q", body.Code, test.wantErrorCode)
			}
		})
	}
}

func newTestHandler() http.Handler {
	return New(service.New(), slog.New(slog.DiscardHandler))
}

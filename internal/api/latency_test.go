package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"electra-assignment/internal/service"
)

func BenchmarkSessionLifecycle(b *testing.B) {
	configBody, err := json.Marshal(testStationConfig())
	if err != nil {
		b.Fatalf("marshal station config: %v", err)
	}

	requests := []struct {
		method     string
		path       string
		body       []byte
		wantStatus int
	}{
		{method: http.MethodPut, path: "/api/v1/station/config", body: configBody, wantStatus: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/sessions", body: []byte(`{"id":"session-1","connectorId":"connector-1","requestedPowerKw":100,"vehicleMaxPowerKw":100}`), wantStatus: http.StatusCreated},
		{method: http.MethodPatch, path: "/api/v1/sessions/session-1", body: []byte(`{"requestedPowerKw":60}`), wantStatus: http.StatusOK},
		{method: http.MethodDelete, path: "/api/v1/sessions/session-1", wantStatus: http.StatusOK},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		handler := New(service.New(), slog.New(slog.DiscardHandler))
		for _, request := range requests {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(request.method, request.path, bytes.NewReader(request.body)))
			if response.Code != request.wantStatus {
				b.Fatalf("%s %s status = %d, want %d; body = %s", request.method, request.path, response.Code, request.wantStatus, response.Body.String())
			}
		}
	}
}

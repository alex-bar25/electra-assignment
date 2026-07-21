package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"electra-assignment/internal/domain"
	"electra-assignment/internal/service"
)

func TestAdvanceSimulation(t *testing.T) {
	station := service.New()
	if _, err := station.Configure(testBESSStationConfig()); err != nil {
		t.Fatalf("configure station: %v", err)
	}
	response := httptest.NewRecorder()

	New(station, slog.New(slog.DiscardHandler)).ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/simulation/tick",
		bytes.NewBufferString(`{"elapsedSeconds":3600}`),
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	state := decodeStationState(t, response)
	if state.BESS == nil || state.BESS.SocPercent != 100 || state.BESS.Mode != domain.BESSModeIdle {
		t.Fatalf("BESS state = %#v, want 100%% and idle", state.BESS)
	}
}

func TestAdvanceSimulationErrors(t *testing.T) {
	tests := []struct {
		name       string
		configure  bool
		withBESS   bool
		method     string
		body       string
		wantStatus int
		wantCode   string
		wantAllow  string
	}{
		{name: "malformed request", configure: true, withBESS: true, method: http.MethodPost, body: `{`, wantStatus: http.StatusBadRequest, wantCode: "invalid_tick"},
		{name: "invalid duration", configure: true, withBESS: true, method: http.MethodPost, body: `{"elapsedSeconds":0}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_tick"},
		{name: "station not configured", method: http.MethodPost, body: `{"elapsedSeconds":60}`, wantStatus: http.StatusNotFound, wantCode: "station_not_configured"},
		{name: "BESS not configured", configure: true, method: http.MethodPost, body: `{"elapsedSeconds":60}`, wantStatus: http.StatusNotFound, wantCode: "bess_not_configured"},
		{name: "unsupported method", configure: true, withBESS: true, method: http.MethodGet, wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", wantAllow: http.MethodPost},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			station := service.New()
			if test.configure {
				config := testStationConfig()
				if test.withBESS {
					config = testBESSStationConfig()
				}
				if _, err := station.Configure(config); err != nil {
					t.Fatalf("configure station: %v", err)
				}
			}
			response := httptest.NewRecorder()

			New(station, slog.New(slog.DiscardHandler)).ServeHTTP(response, httptest.NewRequest(
				test.method,
				"/api/v1/simulation/tick",
				bytes.NewBufferString(test.body),
			))

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if response.Header().Get("Allow") != test.wantAllow {
				t.Fatalf("Allow = %q, want %q", response.Header().Get("Allow"), test.wantAllow)
			}
			var body errorResponse
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.Code != test.wantCode {
				t.Fatalf("error code = %q, want %q", body.Code, test.wantCode)
			}
		})
	}
}

func testBESSStationConfig() domain.StationConfig {
	config := testStationConfig()
	config.BESS = &domain.BESSConfig{
		EnergyCapacityKwh: 100, SocPercent: 50,
		MaxChargePowerKw: 50, MaxDischargePowerKw: 50, MinSocPercent: 10,
	}
	return config
}

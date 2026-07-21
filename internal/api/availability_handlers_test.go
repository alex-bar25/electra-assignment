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

func TestUpdateConnectorAvailability(t *testing.T) {
	station := configuredAvailabilityService(t)
	startAvailabilitySession(t, station, "session-1", "connector-1")
	startAvailabilitySession(t, station, "session-2", "connector-2")
	response := httptest.NewRecorder()

	New(station, slog.New(slog.DiscardHandler)).ServeHTTP(response, httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/connectors/connector-1",
		bytes.NewBufferString(`{"status":"unavailable"}`),
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	state := decodeStationState(t, response)
	if len(state.Sessions) != 1 || state.Sessions[0].ID != "session-2" || state.Sessions[0].AssignedPowerKw != 100 {
		t.Fatalf("sessions = %#v, want session-2 at 100 kW", state.Sessions)
	}
	connector := state.Chargers[0].Connectors[0]
	if connector.Status != domain.OperationalStatusUnavailable || connector.Occupied {
		t.Fatalf("connector state = %#v, want unavailable and unoccupied", connector)
	}
}

func TestUpdateChargerAvailability(t *testing.T) {
	station := configuredAvailabilityService(t)
	startAvailabilitySession(t, station, "session-1", "connector-1")
	startAvailabilitySession(t, station, "session-2", "connector-3")
	response := httptest.NewRecorder()

	New(station, slog.New(slog.DiscardHandler)).ServeHTTP(response, httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/chargers/charger-1",
		bytes.NewBufferString(`{"status":"unavailable"}`),
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	state := decodeStationState(t, response)
	if len(state.Sessions) != 1 || state.Sessions[0].ID != "session-2" || state.Sessions[0].AssignedPowerKw != 100 {
		t.Fatalf("sessions = %#v, want session-2 at 100 kW", state.Sessions)
	}
	if state.Chargers[0].Status != domain.OperationalStatusUnavailable || state.Chargers[0].CurrentPowerKw != 0 {
		t.Fatalf("charger state = %#v, want unavailable at 0 kW", state.Chargers[0])
	}
}

func TestUpdateHardwareAvailabilityErrors(t *testing.T) {
	tests := []struct {
		name       string
		configured bool
		method     string
		path       string
		body       string
		wantStatus int
		wantCode   string
		wantAllow  string
	}{
		{name: "malformed request", configured: true, method: http.MethodPatch, path: "/api/v1/connectors/connector-1", body: `{`, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "invalid status", configured: true, method: http.MethodPatch, path: "/api/v1/connectors/connector-1", body: `{"status":"faulted"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_hardware_status"},
		{name: "unknown charger", configured: true, method: http.MethodPatch, path: "/api/v1/chargers/missing", body: `{"status":"unavailable"}`, wantStatus: http.StatusNotFound, wantCode: "charger_not_found"},
		{name: "unknown connector", configured: true, method: http.MethodPatch, path: "/api/v1/connectors/missing", body: `{"status":"unavailable"}`, wantStatus: http.StatusNotFound, wantCode: "connector_not_found"},
		{name: "station not configured", method: http.MethodPatch, path: "/api/v1/connectors/connector-1", body: `{"status":"unavailable"}`, wantStatus: http.StatusNotFound, wantCode: "station_not_configured"},
		{name: "unsupported method", configured: true, method: http.MethodGet, path: "/api/v1/connectors/connector-1", wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", wantAllow: http.MethodPatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			station := service.New()
			if test.configured {
				if _, err := station.Configure(availabilityStationConfig()); err != nil {
					t.Fatalf("configure station: %v", err)
				}
			}
			response := httptest.NewRecorder()

			New(station, slog.New(slog.DiscardHandler)).ServeHTTP(response, httptest.NewRequest(
				test.method,
				test.path,
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

func configuredAvailabilityService(t *testing.T) *service.Service {
	t.Helper()
	station := service.New()
	if _, err := station.Configure(availabilityStationConfig()); err != nil {
		t.Fatalf("configure station: %v", err)
	}
	return station
}

func startAvailabilitySession(t *testing.T, station *service.Service, sessionID, connectorID string) {
	t.Helper()
	if _, err := station.StartSession(domain.Session{
		ID: sessionID, ConnectorID: connectorID, RequestedPowerKw: 100, VehicleMaxPowerKw: 100,
	}); err != nil {
		t.Fatalf("start %s: %v", sessionID, err)
	}
}

func decodeStationState(t *testing.T, response *httptest.ResponseRecorder) service.StationState {
	t.Helper()
	var state service.StationState
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return state
}

func availabilityStationConfig() domain.StationConfig {
	return domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 100,
		Chargers: []domain.ChargerConfig{
			{
				ID: "charger-1", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable,
				Connectors: []domain.ConnectorConfig{
					{ID: "connector-1", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable},
					{ID: "connector-2", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable},
				},
			},
			{
				ID: "charger-2", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable,
				Connectors: []domain.ConnectorConfig{
					{ID: "connector-3", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable},
				},
			},
		},
	}
}

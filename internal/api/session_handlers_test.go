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

func TestStartSession(t *testing.T) {
	station := service.New()
	if err := station.Configure(testStationConfig()); err != nil {
		t.Fatalf("configure station: %v", err)
	}
	handler := New(station, slog.New(slog.DiscardHandler))
	requestBody := `{
		"id":"session-1",
		"connectorId":"connector-1",
		"requestedPowerKw":60,
		"vehicleMaxPowerKw":80
	}`
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(requestBody)))

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var session domain.Session
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if session.ID != "session-1" || session.AssignedPowerKw != 60 || session.Status != domain.SessionStatusCharging {
		t.Fatalf("started session = %#v", session)
	}
}

func TestStartSessionMapsLifecycleErrors(t *testing.T) {
	tests := []struct {
		name          string
		connectorID   string
		existing      *domain.Session
		wantStatus    int
		wantErrorCode string
	}{
		{
			name:          "unknown connector",
			connectorID:   "unknown",
			wantStatus:    http.StatusNotFound,
			wantErrorCode: "connector_not_found",
		},
		{
			name:          "duplicate session",
			connectorID:   "connector-1",
			existing:      &domain.Session{ID: "session-1", ConnectorID: "connector-1", RequestedPowerKw: 20, VehicleMaxPowerKw: 20},
			wantStatus:    http.StatusConflict,
			wantErrorCode: "duplicate_session",
		},
		{
			name:          "occupied connector",
			connectorID:   "connector-1",
			existing:      &domain.Session{ID: "existing", ConnectorID: "connector-1", RequestedPowerKw: 20, VehicleMaxPowerKw: 20},
			wantStatus:    http.StatusConflict,
			wantErrorCode: "connector_occupied",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			station := service.New()
			if err := station.Configure(testStationConfig()); err != nil {
				t.Fatalf("configure station: %v", err)
			}
			if test.existing != nil {
				if _, err := station.StartSession(*test.existing); err != nil {
					t.Fatalf("seed session: %v", err)
				}
			}
			handler := New(station, slog.New(slog.DiscardHandler))
			requestBody := `{"id":"session-1","connectorId":"` + test.connectorID + `","requestedPowerKw":20,"vehicleMaxPowerKw":20}`
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(requestBody)))

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
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

func TestUpdateSessionRecomputesAllocations(t *testing.T) {
	station := service.New()
	if err := station.Configure(testStationConfig()); err != nil {
		t.Fatalf("configure station: %v", err)
	}
	if _, err := station.StartSession(domain.Session{
		ID: "session-1", ConnectorID: "connector-1", RequestedPowerKw: 100, VehicleMaxPowerKw: 100,
	}); err != nil {
		t.Fatalf("start first session: %v", err)
	}
	if _, err := station.StartSession(domain.Session{
		ID: "session-2", ConnectorID: "connector-2", RequestedPowerKw: 100, VehicleMaxPowerKw: 100,
	}); err != nil {
		t.Fatalf("start second session: %v", err)
	}
	handler := New(station, slog.New(slog.DiscardHandler))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/sessions/session-2",
		bytes.NewBufferString(`{"requestedPowerKw":20}`),
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var updated domain.Session
	if err := json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if updated.ID != "session-2" || updated.AssignedPowerKw != 20 || updated.EffectiveDemandKw != 20 {
		t.Fatalf("updated session = %#v", updated)
	}

	state, err := station.Snapshot()
	if err != nil {
		t.Fatalf("get station state: %v", err)
	}
	if state.Sessions[0].AssignedPowerKw != 80 || state.Sessions[1].AssignedPowerKw != 20 {
		t.Fatalf("allocations = %#v, want 80/20 kW", state.Sessions)
	}
}

func TestUpdateSessionRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name          string
		sessionID     string
		body          string
		wantStatus    int
		wantErrorCode string
	}{
		{name: "empty update", sessionID: "session-1", body: `{}`, wantStatus: http.StatusBadRequest, wantErrorCode: "invalid_request"},
		{name: "invalid power", sessionID: "session-1", body: `{"requestedPowerKw":-1}`, wantStatus: http.StatusBadRequest, wantErrorCode: "invalid_session"},
		{name: "unknown session", sessionID: "missing", body: `{"requestedPowerKw":20}`, wantStatus: http.StatusNotFound, wantErrorCode: "session_not_found"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			station := service.New()
			if err := station.Configure(testStationConfig()); err != nil {
				t.Fatalf("configure station: %v", err)
			}
			if _, err := station.StartSession(domain.Session{
				ID: "session-1", ConnectorID: "connector-1", RequestedPowerKw: 100, VehicleMaxPowerKw: 100,
			}); err != nil {
				t.Fatalf("start session: %v", err)
			}
			handler := New(station, slog.New(slog.DiscardHandler))
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, httptest.NewRequest(
				http.MethodPatch,
				"/api/v1/sessions/"+test.sessionID,
				bytes.NewBufferString(test.body),
			))

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
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

func TestStopSessionRedistributesAllocations(t *testing.T) {
	station := service.New()
	if err := station.Configure(testStationConfig()); err != nil {
		t.Fatalf("configure station: %v", err)
	}
	for _, session := range []domain.Session{
		{ID: "session-1", ConnectorID: "connector-1", RequestedPowerKw: 100, VehicleMaxPowerKw: 100},
		{ID: "session-2", ConnectorID: "connector-2", RequestedPowerKw: 100, VehicleMaxPowerKw: 100},
	} {
		if _, err := station.StartSession(session); err != nil {
			t.Fatalf("start %s: %v", session.ID, err)
		}
	}
	handler := New(station, slog.New(slog.DiscardHandler))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/session-1", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var state service.StationState
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(state.Sessions) != 1 || state.Sessions[0].ID != "session-2" || state.Sessions[0].AssignedPowerKw != 100 {
		t.Fatalf("state sessions = %#v, want session-2 at 100 kW", state.Sessions)
	}
}

func TestStopSessionReturnsNotFound(t *testing.T) {
	station := service.New()
	if err := station.Configure(testStationConfig()); err != nil {
		t.Fatalf("configure station: %v", err)
	}
	handler := New(station, slog.New(slog.DiscardHandler))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/missing", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNotFound, response.Body.String())
	}
	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "session_not_found" {
		t.Fatalf("error code = %q, want session_not_found", body.Code)
	}
}

func testStationConfig() domain.StationConfig {
	return domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 100,
		Chargers: []domain.ChargerConfig{{
			ID: "charger-1", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable,
			Connectors: []domain.ConnectorConfig{
				{ID: "connector-1", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable},
				{ID: "connector-2", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable},
			},
		}},
	}
}

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

func testStationConfig() domain.StationConfig {
	return domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 100,
		Chargers: []domain.ChargerConfig{{
			ID: "charger-1", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable,
			Connectors: []domain.ConnectorConfig{{
				ID: "connector-1", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable,
			}},
		}},
	}
}

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"electra-assignment/internal/domain"
	"electra-assignment/internal/service"
)

func TestGetStationBeforeConfiguration(t *testing.T) {
	handler := newTestHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/station", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	var body errorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != "station_not_configured" {
		t.Fatalf("error code = %q, want station_not_configured", body.Code)
	}
}

func TestConfigureAndGetStation(t *testing.T) {
	handler := newTestHandler()
	config := domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 100,
		Chargers: []domain.ChargerConfig{{
			ID: "charger-1", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable,
			Connectors: []domain.ConnectorConfig{{
				ID: "connector-1", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable,
			}},
		}},
	}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	configured := httptest.NewRecorder()
	handler.ServeHTTP(configured, httptest.NewRequest(http.MethodPut, "/api/v1/station/config", bytes.NewReader(body)))
	if configured.Code != http.StatusOK {
		t.Fatalf("configure status = %d, want %d; body = %s", configured.Code, http.StatusOK, configured.Body.String())
	}
	var configuredState service.StationState
	if err := json.NewDecoder(configured.Body).Decode(&configuredState); err != nil {
		t.Fatalf("decode configure response: %v", err)
	}
	if configuredState.StationID != "station-1" || configuredState.AvailableStationPowerKw != 100 {
		t.Fatalf("configured state = %#v", configuredState)
	}

	queried := httptest.NewRecorder()
	handler.ServeHTTP(queried, httptest.NewRequest(http.MethodGet, "/api/v1/station", nil))
	if queried.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", queried.Code, http.StatusOK)
	}
	var queriedState service.StationState
	if err := json.NewDecoder(queried.Body).Decode(&queriedState); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if queriedState.StationID != configuredState.StationID || len(queriedState.Chargers) != 1 {
		t.Fatalf("queried state = %#v", queriedState)
	}
}

func TestConcurrentConfigureReturnsOwnState(t *testing.T) {
	handler := newTestHandler()
	const requestCount = 1000

	start := make(chan struct{})
	errors := make(chan error, requestCount)
	var requests sync.WaitGroup
	for index := range requestCount {
		stationID := fmt.Sprintf("station-%d", index)
		config := testStationConfig()
		config.ID = stationID
		body, err := json.Marshal(config)
		if err != nil {
			t.Fatalf("marshal station %q: %v", stationID, err)
		}

		requests.Add(1)
		go func() {
			defer requests.Done()
			<-start

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(
				http.MethodPut,
				"/api/v1/station/config",
				bytes.NewReader(body),
			))
			if response.Code != http.StatusOK {
				errors <- fmt.Errorf("station %q status = %d; body = %s", stationID, response.Code, response.Body.String())
				return
			}

			var state service.StationState
			if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
				errors <- fmt.Errorf("decode station %q response: %w", stationID, err)
				return
			}
			if state.StationID != stationID {
				errors <- fmt.Errorf("station %q received state for %q", stationID, state.StationID)
			}
		}()
	}

	close(start)
	requests.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func TestConfigureStationRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{name: "malformed JSON", body: "{", wantCode: "invalid_request"},
		{name: "invalid configuration", body: `{"id":"","gridCapacityKw":0,"chargers":[]}`, wantCode: "invalid_station_config"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newTestHandler()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/api/v1/station/config", bytes.NewBufferString(test.body)))

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
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

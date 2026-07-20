package service

import (
	"errors"
	"testing"

	"electra-assignment/internal/domain"
)

func TestServiceConfigureAndSnapshot(t *testing.T) {
	service := New()
	if _, err := service.Snapshot(); !errors.Is(err, ErrStationNotConfigured) {
		t.Fatalf("Snapshot() error = %v, want ErrStationNotConfigured", err)
	}

	if err := service.Configure(testStationConfig()); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Config.ID != "station-1" || len(state.Sessions) != 0 {
		t.Fatalf("Snapshot() = %#v, want configured station without sessions", state)
	}
	if state.GridImportKw != 0 || state.AvailableGridPowerKw != 100 {
		t.Fatalf("power summary = (%v, %v), want (0, 100)", state.GridImportKw, state.AvailableGridPowerKw)
	}
	if state.LastUpdatedAt.IsZero() {
		t.Fatal("LastUpdatedAt is zero")
	}
}

func TestServiceSnapshotReturnsCopies(t *testing.T) {
	service := New()
	if err := service.Configure(testStationConfig()); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	first, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	first.Config.Chargers[0].Connectors[0].ID = "changed"

	second, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if second.Config.Chargers[0].Connectors[0].ID != "connector-1" {
		t.Fatalf("stored connector ID = %q, want connector-1", second.Config.Chargers[0].Connectors[0].ID)
	}
}

func TestSessionsForSnapshotUsesStableSummationOrder(t *testing.T) {
	sessions, gridImport := sessionsForSnapshot(map[string]domain.Session{
		"session-c": {ID: "session-c", AssignedPowerKw: 1},
		"session-a": {ID: "session-a", AssignedPowerKw: 1e16},
		"session-b": {ID: "session-b", AssignedPowerKw: 1},
	})

	if sessions[0].ID != "session-a" || sessions[1].ID != "session-b" || sessions[2].ID != "session-c" {
		t.Fatalf("session order = %q, %q, %q", sessions[0].ID, sessions[1].ID, sessions[2].ID)
	}
	if gridImport != 1e16 {
		t.Fatalf("grid import = %v, want stable sorted sum %v", gridImport, 1e16)
	}
}

func testStationConfig() domain.StationConfig {
	return domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 100,
		Chargers: []domain.ChargerConfig{{
			ID:         "charger-1",
			MaxPowerKw: 200,
			Status:     domain.OperationalStatusAvailable,
			Connectors: []domain.ConnectorConfig{
				{ID: "connector-1", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
				{ID: "connector-2", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
			},
		}},
	}
}

func TestServiceStartSessionRecomputesBeforeReturning(t *testing.T) {
	service := configuredService(t, testStationConfig())
	first, err := service.StartSession(testSession("session-1", "connector-1", 100))
	if err != nil || first.AssignedPowerKw != 100 {
		t.Fatalf("first StartSession() = (%#v, %v), want 100 kW", first, err)
	}

	second, err := service.StartSession(testSession("session-2", "connector-2", 100))
	if err != nil || second.AssignedPowerKw != 50 {
		t.Fatalf("second StartSession() = (%#v, %v), want 50 kW", second, err)
	}
	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(state.Sessions) != 2 || state.Sessions[0].AssignedPowerKw != 50 || state.Sessions[1].AssignedPowerKw != 50 {
		t.Fatalf("sessions = %#v, want two 50 kW assignments", state.Sessions)
	}
	if state.GridImportKw != 100 || state.AvailableGridPowerKw != 0 {
		t.Fatalf("power summary = (%v, %v), want (100, 0)", state.GridImportKw, state.AvailableGridPowerKw)
	}
}

func TestServiceStartSessionRejectsInvalidOperations(t *testing.T) {
	t.Run("station not configured", func(t *testing.T) {
		_, err := New().StartSession(testSession("session-1", "connector-1", 100))
		if !errors.Is(err, ErrStationNotConfigured) {
			t.Fatalf("error = %v, want ErrStationNotConfigured", err)
		}
	})

	t.Run("invalid power", func(t *testing.T) {
		service := configuredService(t, testStationConfig())
		_, err := service.StartSession(testSession("session-1", "connector-1", 0))
		if err == nil {
			t.Fatal("error = nil, want validation error")
		}
	})

	t.Run("unknown connector", func(t *testing.T) {
		service := configuredService(t, testStationConfig())
		_, err := service.StartSession(testSession("session-1", "missing", 100))
		if !errors.Is(err, ErrConnectorNotFound) {
			t.Fatalf("error = %v, want ErrConnectorNotFound", err)
		}
	})

	for _, resource := range []string{"charger", "connector"} {
		t.Run("unavailable "+resource, func(t *testing.T) {
			config := testStationConfig()
			if resource == "charger" {
				config.Chargers[0].Status = domain.OperationalStatusUnavailable
			} else {
				config.Chargers[0].Connectors[0].Status = domain.OperationalStatusUnavailable
			}
			service := configuredService(t, config)
			_, err := service.StartSession(testSession("session-1", "connector-1", 100))
			if !errors.Is(err, ErrHardwareUnavailable) {
				t.Fatalf("error = %v, want ErrHardwareUnavailable", err)
			}
		})
	}

	t.Run("duplicate session", func(t *testing.T) {
		service := configuredService(t, testStationConfig())
		if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
			t.Fatalf("first StartSession() error = %v", err)
		}
		_, err := service.StartSession(testSession("session-1", "connector-2", 100))
		if !errors.Is(err, ErrDuplicateSession) {
			t.Fatalf("error = %v, want ErrDuplicateSession", err)
		}
	})

	t.Run("occupied connector", func(t *testing.T) {
		service := configuredService(t, testStationConfig())
		if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
			t.Fatalf("first StartSession() error = %v", err)
		}
		_, err := service.StartSession(testSession("session-2", "connector-1", 100))
		if !errors.Is(err, ErrConnectorOccupied) {
			t.Fatalf("error = %v, want ErrConnectorOccupied", err)
		}
	})
}

func configuredService(t *testing.T, config domain.StationConfig) *Service {
	t.Helper()
	service := New()
	if err := service.Configure(config); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	return service
}

func testSession(id, connectorID string, demand float64) domain.Session {
	return domain.Session{
		ID:                id,
		ConnectorID:       connectorID,
		RequestedPowerKw:  demand,
		VehicleMaxPowerKw: demand,
	}
}

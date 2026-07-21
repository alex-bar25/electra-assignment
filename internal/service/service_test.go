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
	if state.StationID != "station-1" || state.GridCapacityKw != 100 || len(state.Sessions) != 0 {
		t.Fatalf("Snapshot() = %#v, want configured station without sessions", state)
	}
	if state.GridImportKw != 0 || state.AvailableGridPowerKw != 100 || state.AvailableStationPowerKw != 100 {
		t.Fatalf("power summary = (%v, %v, %v), want (0, 100, 100)", state.GridImportKw, state.AvailableGridPowerKw, state.AvailableStationPowerKw)
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
	first.Chargers[0].Connectors[0].ID = "changed"

	second, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if second.Chargers[0].Connectors[0].ID != "connector-1" {
		t.Fatalf("stored connector ID = %q, want connector-1", second.Chargers[0].Connectors[0].ID)
	}
}

func TestCloneConfigCopiesBESS(t *testing.T) {
	config := testStationConfig()
	config.BESS = &domain.BESSConfig{
		EnergyCapacityKwh: 200, SocPercent: 50,
		MaxChargePowerKw: 200, MaxDischargePowerKw: 200, MinSocPercent: 10,
	}

	cloned := cloneConfig(config)
	cloned.BESS.SocPercent = 80

	if config.BESS.SocPercent != 50 {
		t.Fatalf("original BESS SoC = %v, want 50", config.BESS.SocPercent)
	}
}

func TestServiceSnapshotIncludesHardwarePowerState(t *testing.T) {
	service := configuredService(t, testStationConfig())
	if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	if _, err := service.StartSession(testSession("session-2", "connector-2", 100)); err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	charger := state.Chargers[0]
	if charger.CurrentPowerKw != 100 || len(charger.Connectors) != 2 {
		t.Fatalf("charger = %#v, want 100 kW across two connectors", charger)
	}
	wantSessionIDs := []string{"session-1", "session-2"}
	for index, connector := range charger.Connectors {
		wantSessionID := wantSessionIDs[index]
		if !connector.Occupied || connector.ActiveSessionID != wantSessionID || connector.AssignedPowerKw != 50 {
			t.Fatalf("connector = %#v, want %s at 50 kW", connector, wantSessionID)
		}
	}
}

func TestSessionsForSnapshotUsesStableSummationOrder(t *testing.T) {
	sessions, assignedPower := sessionsForSnapshot(map[string]domain.Session{
		"session-c": {ID: "session-c", AssignedPowerKw: 1},
		"session-a": {ID: "session-a", AssignedPowerKw: 1e16},
		"session-b": {ID: "session-b", AssignedPowerKw: 1},
	})

	if sessions[0].ID != "session-a" || sessions[1].ID != "session-b" || sessions[2].ID != "session-c" {
		t.Fatalf("session order = %q, %q, %q", sessions[0].ID, sessions[1].ID, sessions[2].ID)
	}
	if assignedPower != 1e16 {
		t.Fatalf("assigned power = %v, want stable sorted sum %v", assignedPower, 1e16)
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
	if first.MinimumPowerKw != 5 || first.EffectiveDemandKw != 100 || first.Status != domain.SessionStatusCharging {
		t.Fatalf("first session = %#v, want normalized minimum, effective demand, and charging status", first)
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

func TestServiceStartSessionCanWaitForMinimumPower(t *testing.T) {
	config := testStationConfig()
	config.GridCapacityKw = 5
	service := configuredService(t, config)
	if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	waiting, err := service.StartSession(testSession("session-2", "connector-2", 100))
	if err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}
	if waiting.AssignedPowerKw != 0 || waiting.Status != domain.SessionStatusWaitingForPower {
		t.Fatalf("waiting session = %#v, want waiting at 0 kW", waiting)
	}
}

func TestServiceUpdateSessionRecomputesBeforeReturning(t *testing.T) {
	service := configuredService(t, testStationConfig())
	if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	second, err := service.StartSession(testSession("session-2", "connector-2", 100))
	if err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}

	requested := 20.0
	updated, err := service.UpdateSession("session-2", SessionUpdate{RequestedPowerKw: &requested})
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if updated.AssignedPowerKw != 20 || updated.EffectiveDemandKw != 20 || !updated.StartedAt.Equal(second.StartedAt) {
		t.Fatalf("updated session = %#v, want 20 kW with preserved start time", updated)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Sessions[0].AssignedPowerKw != 80 || state.Sessions[1].AssignedPowerKw != 20 {
		t.Fatalf("sessions = %#v, want 80/20 kW", state.Sessions)
	}
}

func TestServiceChargingCurveUpdateRecomputesBeforeReturning(t *testing.T) {
	service := configuredService(t, testStationConfig())
	if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	if _, err := service.StartSession(testSession("session-2", "connector-2", 100)); err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}

	curveLimit := 20.0
	updated, err := service.UpdateSession("session-2", SessionUpdate{ChargingCurveLimitKw: &curveLimit})
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if updated.RequestedPowerKw != 100 || updated.ChargingCurveLimitKw == nil ||
		*updated.ChargingCurveLimitKw != 20 || updated.EffectiveDemandKw != 20 || updated.AssignedPowerKw != 20 {
		t.Fatalf("updated session = %#v, want curve-limited allocation of 20 kW", updated)
	}

	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Sessions[0].AssignedPowerKw != 80 || state.Sessions[1].AssignedPowerKw != 20 {
		t.Fatalf("sessions = %#v, want 80/20 kW", state.Sessions)
	}
}

func TestServiceUpdateSessionReconsidersWaitingSessions(t *testing.T) {
	config := testStationConfig()
	config.GridCapacityKw = 10
	service := configuredService(t, config)
	first := testSession("session-1", "connector-1", 100)
	first.MinimumPowerKw = 10
	if _, err := service.StartSession(first); err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	if _, err := service.StartSession(testSession("session-2", "connector-2", 100)); err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}

	five := 5.0
	if _, err := service.UpdateSession("session-1", SessionUpdate{
		RequestedPowerKw:  &five,
		VehicleMaxPowerKw: &five,
		MinimumPowerKw:    &five,
	}); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	for _, session := range state.Sessions {
		if session.AssignedPowerKw != 5 || session.Status != domain.SessionStatusCharging {
			t.Fatalf("session = %#v, want charging at 5 kW", session)
		}
	}
}

func TestServiceUpdateSessionRejectsInvalidOperationsAtomically(t *testing.T) {
	service := configuredService(t, testStationConfig())
	if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	four := 4.0
	if _, err := service.UpdateSession("session-1", SessionUpdate{RequestedPowerKw: &four}); !errors.Is(err, ErrMinimumExceedsDemand) {
		t.Fatalf("invalid update error = %v, want ErrMinimumExceedsDemand", err)
	}
	state, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if state.Sessions[0].RequestedPowerKw != 100 || state.Sessions[0].AssignedPowerKw != 100 {
		t.Fatalf("session changed after invalid update: %#v", state.Sessions[0])
	}
	if _, err := service.UpdateSession("missing", SessionUpdate{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown update error = %v, want ErrSessionNotFound", err)
	}
}

func TestServiceStopSessionRecomputesBeforeReturning(t *testing.T) {
	config := testStationConfig()
	config.GridCapacityKw = 5
	service := configuredService(t, config)
	if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	if _, err := service.StartSession(testSession("session-2", "connector-2", 100)); err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}

	state, err := service.StopSession("session-1")
	if err != nil {
		t.Fatalf("StopSession() error = %v", err)
	}
	if len(state.Sessions) != 1 || state.Sessions[0].ID != "session-2" ||
		state.Sessions[0].AssignedPowerKw != 5 || state.Sessions[0].Status != domain.SessionStatusCharging {
		t.Fatalf("state = %#v, want session-2 charging at 5 kW", state)
	}
}

func TestServiceStopSessionRejectsUnknownSession(t *testing.T) {
	service := configuredService(t, testStationConfig())
	if _, err := service.StopSession("missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("StopSession() error = %v, want ErrSessionNotFound", err)
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

	t.Run("minimum exceeds effective demand", func(t *testing.T) {
		service := configuredService(t, testStationConfig())
		_, err := service.StartSession(testSession("session-1", "connector-1", 4))
		if !errors.Is(err, ErrMinimumExceedsDemand) {
			t.Fatalf("error = %v, want ErrMinimumExceedsDemand", err)
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

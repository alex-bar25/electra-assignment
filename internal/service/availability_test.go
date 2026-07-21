package service

import (
	"errors"
	"reflect"
	"testing"

	"electra-assignment/internal/domain"
)

func TestUpdateConnectorStatusEndsSessionAndRedistributesPower(t *testing.T) {
	service := configuredService(t, testStationConfig())
	if _, err := service.StartSession(testSession("session-1", "connector-1", 100)); err != nil {
		t.Fatalf("start first session: %v", err)
	}
	if _, err := service.StartSession(testSession("session-2", "connector-2", 100)); err != nil {
		t.Fatalf("start second session: %v", err)
	}

	state, err := service.UpdateConnectorStatus("connector-1", domain.OperationalStatusUnavailable)
	if err != nil {
		t.Fatalf("UpdateConnectorStatus() error = %v", err)
	}
	if len(state.Sessions) != 1 || state.Sessions[0].ID != "session-2" || state.Sessions[0].AssignedPowerKw != 100 {
		t.Fatalf("sessions = %#v, want session-2 at 100 kW", state.Sessions)
	}
	unavailable := state.Chargers[0].Connectors[0]
	if unavailable.Status != domain.OperationalStatusUnavailable || unavailable.Occupied || unavailable.AssignedPowerKw != 0 {
		t.Fatalf("unavailable connector = %#v, want unavailable and unoccupied", unavailable)
	}
}

func TestUpdateChargerStatusEndsAttachedSessionsAndRedistributesPower(t *testing.T) {
	service := configuredService(t, availabilityStationConfig())
	for _, session := range []domain.Session{
		testSession("session-1", "connector-1", 100),
		testSession("session-2", "connector-2", 100),
		testSession("session-3", "connector-3", 100),
	} {
		if _, err := service.StartSession(session); err != nil {
			t.Fatalf("start %s: %v", session.ID, err)
		}
	}

	state, err := service.UpdateChargerStatus("charger-1", domain.OperationalStatusUnavailable)
	if err != nil {
		t.Fatalf("UpdateChargerStatus() error = %v", err)
	}
	if len(state.Sessions) != 1 || state.Sessions[0].ID != "session-3" || state.Sessions[0].AssignedPowerKw != 100 {
		t.Fatalf("sessions = %#v, want session-3 at 100 kW", state.Sessions)
	}
	if state.Chargers[0].Status != domain.OperationalStatusUnavailable || state.Chargers[0].CurrentPowerKw != 0 {
		t.Fatalf("unavailable charger = %#v, want unavailable at 0 kW", state.Chargers[0])
	}
	for _, connector := range state.Chargers[0].Connectors {
		if connector.Occupied || connector.AssignedPowerKw != 0 {
			t.Fatalf("connector = %#v, want unoccupied at 0 kW", connector)
		}
	}
}

func TestUpdateConnectorStatusRestoresHardwareForNewSessions(t *testing.T) {
	service := configuredService(t, testStationConfig())
	if _, err := service.UpdateConnectorStatus("connector-1", domain.OperationalStatusUnavailable); err != nil {
		t.Fatalf("make connector unavailable: %v", err)
	}
	state, err := service.UpdateConnectorStatus("connector-1", domain.OperationalStatusAvailable)
	if err != nil {
		t.Fatalf("restore connector: %v", err)
	}
	if state.Chargers[0].Connectors[0].Status != domain.OperationalStatusAvailable {
		t.Fatalf("connector = %#v, want available", state.Chargers[0].Connectors[0])
	}

	session, err := service.StartSession(testSession("session-1", "connector-1", 100))
	if err != nil {
		t.Fatalf("start session on restored connector: %v", err)
	}
	if session.AssignedPowerKw != 100 {
		t.Fatalf("session assigned power = %v, want 100", session.AssignedPowerKw)
	}
}

func TestUpdateHardwareStatusRejectsInvalidOperationsAtomically(t *testing.T) {
	service := configuredService(t, testStationConfig())
	before, err := service.Snapshot()
	if err != nil {
		t.Fatalf("snapshot before updates: %v", err)
	}

	if _, err := service.UpdateConnectorStatus("missing", domain.OperationalStatusAvailable); !errors.Is(err, ErrConnectorNotFound) {
		t.Fatalf("unknown connector error = %v, want ErrConnectorNotFound", err)
	}
	assertStationStateEqual(t, service, before)

	if _, err := service.UpdateChargerStatus("missing", domain.OperationalStatusAvailable); !errors.Is(err, ErrChargerNotFound) {
		t.Fatalf("unknown charger error = %v, want ErrChargerNotFound", err)
	}
	assertStationStateEqual(t, service, before)

	if _, err := service.UpdateConnectorStatus("connector-1", "broken"); err == nil {
		t.Fatal("invalid status error = nil, want an error")
	}
	assertStationStateEqual(t, service, before)
}

func TestUpdateHardwareStatusRequiresConfiguredStation(t *testing.T) {
	service := New()
	if _, err := service.UpdateChargerStatus("charger-1", domain.OperationalStatusUnavailable); !errors.Is(err, ErrStationNotConfigured) {
		t.Fatalf("charger update error = %v, want ErrStationNotConfigured", err)
	}
	if _, err := service.UpdateConnectorStatus("connector-1", domain.OperationalStatusUnavailable); !errors.Is(err, ErrStationNotConfigured) {
		t.Fatalf("connector update error = %v, want ErrStationNotConfigured", err)
	}
}

func assertStationStateEqual(t *testing.T, service *Service, want StationState) {
	t.Helper()
	got, err := service.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("station state changed after rejected update:\ngot:  %#v\nwant: %#v", got, want)
	}
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

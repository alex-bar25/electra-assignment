package service

import (
	"testing"

	"electra-assignment/internal/domain"
)

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

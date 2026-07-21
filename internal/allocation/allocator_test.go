package allocation

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"electra-assignment/internal/domain"
)

const testEpsilon = 1e-6

func TestAllocateRespectsEffectiveDemandLimits(t *testing.T) {
	curveLimit := 60.0
	tests := []struct {
		name      string
		grid      float64
		charger   float64
		connector float64
		requested float64
		vehicle   float64
		curve     *float64
		wantPower float64
	}{
		{name: "request", grid: 400, charger: 300, connector: 200, requested: 50, vehicle: 180, wantPower: 50},
		{name: "vehicle", grid: 400, charger: 300, connector: 200, requested: 180, vehicle: 70, wantPower: 70},
		{name: "charging curve", grid: 400, charger: 300, connector: 200, requested: 180, vehicle: 170, curve: &curveLimit, wantPower: 60},
		{name: "connector", grid: 400, charger: 300, connector: 40, requested: 180, vehicle: 170, wantPower: 40},
		{name: "charger", grid: 400, charger: 30, connector: 200, requested: 180, vehicle: 170, wantPower: 30},
		{name: "grid", grid: 20, charger: 300, connector: 200, requested: 180, vehicle: 170, wantPower: 20},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := stationWithOneCharger(test.grid, test.charger, test.connector)
			sessions := []domain.Session{{
				ID:                   "session-1",
				ConnectorID:          "connector-1",
				RequestedPowerKw:     test.requested,
				VehicleMaxPowerKw:    test.vehicle,
				ChargingCurveLimitKw: test.curve,
			}}

			assertPower(t, Allocate(config, sessions), "session-1", test.wantPower)
		})
	}
}

func TestAllocateSharesAndRedistributesGridPower(t *testing.T) {
	t.Run("equal demand receives equal power", func(t *testing.T) {
		config := stationWithOneCharger(100, 200, 200, 200)
		sessions := []domain.Session{
			testSession("session-1", "connector-1", 100),
			testSession("session-2", "connector-2", 100),
		}

		assignments := Allocate(config, sessions)
		assertPower(t, assignments, "session-1", 50)
		assertPower(t, assignments, "session-2", 50)
	})

	t.Run("unused low demand share is redistributed", func(t *testing.T) {
		config := stationWithOneCharger(100, 200, 200, 200)
		sessions := []domain.Session{
			testSession("session-1", "connector-1", 20),
			testSession("session-2", "connector-2", 100),
		}

		assignments := Allocate(config, sessions)
		assertPower(t, assignments, "session-1", 20)
		assertPower(t, assignments, "session-2", 80)
	})
}

func TestAllocateRespectsSharedChargerLimit(t *testing.T) {
	config := domain.StationConfig{ID: "station-1", GridCapacityKw: 200, Chargers: []domain.ChargerConfig{
		{ID: "charger-1", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable, Connectors: []domain.ConnectorConfig{
			{ID: "connector-1", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
			{ID: "connector-2", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
		}},
		{ID: "charger-2", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable, Connectors: []domain.ConnectorConfig{
			{ID: "connector-3", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
		}},
	}}
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 200),
		testSession("session-2", "connector-2", 200),
		testSession("session-3", "connector-3", 200),
	}

	assignments := Allocate(config, sessions)
	assertPower(t, assignments, "session-1", 50)
	assertPower(t, assignments, "session-2", 50)
	assertPower(t, assignments, "session-3", 100)
}

func TestAllocateRedistributesPastFullCharger(t *testing.T) {
	config := domain.StationConfig{ID: "station-1", GridCapacityKw: 100, Chargers: []domain.ChargerConfig{
		{ID: "charger-1", MaxPowerKw: 5, Status: domain.OperationalStatusAvailable, Connectors: []domain.ConnectorConfig{
			{ID: "connector-1", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable},
		}},
		{ID: "charger-2", MaxPowerKw: 95, Status: domain.OperationalStatusAvailable, Connectors: []domain.ConnectorConfig{
			{ID: "connector-2", Type: "CCS", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable},
		}},
	}}
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 100),
		testSession("session-2", "connector-2", 100),
	}

	assignments := Allocate(config, sessions)
	assertPower(t, assignments, "session-1", 5)
	assertPower(t, assignments, "session-2", 95)
}

func TestAllocateReturnsZeroForUnavailableHardware(t *testing.T) {
	config := stationWithOneCharger(100, 100, 100, 100)
	config.Chargers[0].Connectors[1].Status = domain.OperationalStatusUnavailable
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 100),
		testSession("session-2", "connector-2", 100),
	}

	assignments := Allocate(config, sessions)
	assertPower(t, assignments, "session-1", 100)
	assertPower(t, assignments, "session-2", 0)
}

func TestAllocateProducesStableOutput(t *testing.T) {
	config := stationWithOneCharger(100, 200, 200, 200)
	forward := []domain.Session{
		testSession("session-1", "connector-1", 100),
		testSession("session-2", "connector-2", 100),
	}
	reverse := []domain.Session{forward[1], forward[0]}

	first := Allocate(config, forward)
	second := Allocate(config, reverse)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Allocate() differs by input order:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestAllocateWaitsWhenMinimumCannotBeReserved(t *testing.T) {
	config := stationWithOneCharger(5, 100, 100, 100)
	start := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	first := testSession("session-1", "connector-1", 100)
	first.StartedAt = start
	second := testSession("session-2", "connector-2", 100)
	second.StartedAt = start.Add(time.Second)

	assignments := Allocate(config, []domain.Session{second, first})
	assertAssignment(t, assignments, "session-1", 5, domain.SessionStatusCharging)
	assertAssignment(t, assignments, "session-2", 0, domain.SessionStatusWaitingForPower)
}

func TestAllocateUsesSessionIDToBreakAdmissionTies(t *testing.T) {
	config := stationWithOneCharger(5, 100, 100, 100)
	sessionB := testSession("session-b", "connector-1", 100)
	sessionA := testSession("session-a", "connector-2", 100)

	assignments := Allocate(config, []domain.Session{sessionB, sessionA})
	assertAssignment(t, assignments, "session-a", 5, domain.SessionStatusCharging)
	assertAssignment(t, assignments, "session-b", 0, domain.SessionStatusWaitingForPower)
}

func TestAllocateKeepsFinalAllocationsFairAfterMinimums(t *testing.T) {
	config := stationWithOneCharger(30, 100, 100, 100)
	first := testSession("session-1", "connector-1", 100)
	first.MinimumPowerKw = 10
	second := testSession("session-2", "connector-2", 100)
	second.MinimumPowerKw = 5

	assignments := Allocate(config, []domain.Session{first, second})
	assertAssignment(t, assignments, "session-1", 15, domain.SessionStatusCharging)
	assertAssignment(t, assignments, "session-2", 15, domain.SessionStatusCharging)
}

func stationWithOneCharger(grid, charger float64, connectors ...float64) domain.StationConfig {
	connectorConfigs := make([]domain.ConnectorConfig, 0, len(connectors))
	for index, maximum := range connectors {
		connectorConfigs = append(connectorConfigs, domain.ConnectorConfig{
			ID:         fmt.Sprintf("connector-%d", index+1),
			Type:       "CCS",
			MaxPowerKw: maximum,
			Status:     domain.OperationalStatusAvailable,
		})
	}
	return domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: grid,
		Chargers: []domain.ChargerConfig{{
			ID:         "charger-1",
			MaxPowerKw: charger,
			Status:     domain.OperationalStatusAvailable,
			Connectors: connectorConfigs,
		}},
	}
}

func testSession(id, connectorID string, demand float64) domain.Session {
	return domain.Session{
		ID:                id,
		ConnectorID:       connectorID,
		RequestedPowerKw:  demand,
		VehicleMaxPowerKw: demand,
	}
}

func assertPower(t *testing.T, assignments []Assignment, sessionID string, want float64) {
	t.Helper()
	for _, assignment := range assignments {
		if assignment.SessionID == sessionID {
			if difference := assignment.AssignedPowerKw - want; difference < -testEpsilon || difference > testEpsilon {
				t.Fatalf("session %q assigned power = %v, want %v", sessionID, assignment.AssignedPowerKw, want)
			}
			return
		}
	}
	t.Fatalf("assignment for session %q not found", sessionID)
}

func assertAssignment(t *testing.T, assignments []Assignment, sessionID string, wantPower float64, wantStatus domain.SessionStatus) {
	t.Helper()
	for _, assignment := range assignments {
		if assignment.SessionID == sessionID {
			if difference := assignment.AssignedPowerKw - wantPower; difference < -testEpsilon || difference > testEpsilon {
				t.Fatalf("session %q assigned power = %v, want %v", sessionID, assignment.AssignedPowerKw, wantPower)
			}
			if assignment.Status != wantStatus {
				t.Fatalf("session %q status = %q, want %q", sessionID, assignment.Status, wantStatus)
			}
			return
		}
	}
	t.Fatalf("assignment for session %q not found", sessionID)
}

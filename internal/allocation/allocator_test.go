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
	curveLimit := 180.0
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
		{name: "charging curve", grid: 400, charger: 300, connector: 250, requested: 220, vehicle: 200, curve: &curveLimit, wantPower: 180},
		{name: "connector", grid: 400, charger: 300, connector: 40, requested: 180, vehicle: 170, wantPower: 40},
		{name: "charger", grid: 400, charger: 30, connector: 200, requested: 180, vehicle: 170, wantPower: 30},
		{name: "grid", grid: 150, charger: 300, connector: 300, requested: 250, vehicle: 250, wantPower: 150},
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

			assertPower(t, Allocate(config, sessions, config.GridCapacityKw), "session-1", test.wantPower)
		})
	}
}

func TestAllocateUsesExplicitStationSupply(t *testing.T) {
	config := stationWithOneCharger(100, 250, 180)
	sessions := []domain.Session{testSession("session-1", "connector-1", 300)}

	assignments := Allocate(config, sessions, 300)

	assertPower(t, assignments, "session-1", 180)
}

func TestAllocateSharesAndRedistributesGridPower(t *testing.T) {
	t.Run("equal demand receives equal power", func(t *testing.T) {
		config := stationWithOneCharger(300, 600, 300, 300)
		sessions := []domain.Session{
			testSession("session-1", "connector-1", 250),
			testSession("session-2", "connector-2", 250),
		}

		assignments := Allocate(config, sessions, config.GridCapacityKw)
		assertPower(t, assignments, "session-1", 150)
		assertPower(t, assignments, "session-2", 150)
	})

	t.Run("unused low demand share is redistributed", func(t *testing.T) {
		config := stationWithOneCharger(300, 600, 300, 300)
		sessions := []domain.Session{
			testSession("session-1", "connector-1", 50),
			testSession("session-2", "connector-2", 300),
		}

		assignments := Allocate(config, sessions, config.GridCapacityKw)
		assertPower(t, assignments, "session-1", 50)
		assertPower(t, assignments, "session-2", 250)
	})
}

func TestAllocateRedistributesAcrossThreeDemandLevels(t *testing.T) {
	config := stationWithOneCharger(300, 400, 400, 400, 400)
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 50),
		testSession("session-2", "connector-2", 120),
		testSession("session-3", "connector-3", 300),
	}

	assignments := Allocate(config, sessions, config.GridCapacityKw)
	assertPower(t, assignments, "session-1", 50)
	assertPower(t, assignments, "session-2", 120)
	assertPower(t, assignments, "session-3", 130)
}

func TestAllocateRespectsSharedChargerLimit(t *testing.T) {
	config := stationWithOneCharger(500, 300, 300, 300)
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 250),
		testSession("session-2", "connector-2", 250),
	}

	assignments := Allocate(config, sessions, config.GridCapacityKw)
	assertPower(t, assignments, "session-1", 150)
	assertPower(t, assignments, "session-2", 150)
}

func TestAllocateRedistributesPastFullCharger(t *testing.T) {
	config := domain.StationConfig{ID: "station-1", GridCapacityKw: 400, Chargers: []domain.ChargerConfig{
		{ID: "charger-1", MaxPowerKw: 100, Status: domain.OperationalStatusAvailable, Connectors: []domain.ConnectorConfig{
			{ID: "connector-1", Type: "CCS", MaxPowerKw: 300, Status: domain.OperationalStatusAvailable},
		}},
		{ID: "charger-2", MaxPowerKw: 300, Status: domain.OperationalStatusAvailable, Connectors: []domain.ConnectorConfig{
			{ID: "connector-2", Type: "CCS", MaxPowerKw: 300, Status: domain.OperationalStatusAvailable},
		}},
	}}
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 300),
		testSession("session-2", "connector-2", 300),
	}

	assignments := Allocate(config, sessions, config.GridCapacityKw)
	assertPower(t, assignments, "session-1", 100)
	assertPower(t, assignments, "session-2", 300)
}

func TestAllocateReturnsZeroForUnavailableHardware(t *testing.T) {
	config := stationWithOneCharger(100, 100, 100, 100)
	config.Chargers[0].Connectors[1].Status = domain.OperationalStatusUnavailable
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 100),
		testSession("session-2", "connector-2", 100),
	}

	assignments := Allocate(config, sessions, config.GridCapacityKw)
	assertPower(t, assignments, "session-1", 100)
	assertPower(t, assignments, "session-2", 0)
}

func TestAllocateProducesStableOutput(t *testing.T) {
	forwardConfig := domain.StationConfig{
		ID:             "station-1",
		GridCapacityKw: 300,
		Chargers: []domain.ChargerConfig{
			{
				ID:         "charger-1",
				MaxPowerKw: 140,
				Status:     domain.OperationalStatusAvailable,
				Connectors: []domain.ConnectorConfig{
					{ID: "connector-1", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
					{ID: "connector-2", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
				},
			},
			{
				ID:         "charger-2",
				MaxPowerKw: 200,
				Status:     domain.OperationalStatusAvailable,
				Connectors: []domain.ConnectorConfig{
					{ID: "connector-3", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
					{ID: "connector-4", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
				},
			},
		},
	}
	reverseConfig := forwardConfig
	reverseConfig.Chargers = []domain.ChargerConfig{
		forwardConfig.Chargers[1],
		forwardConfig.Chargers[0],
	}
	for i := range reverseConfig.Chargers {
		connectors := reverseConfig.Chargers[i].Connectors
		reverseConfig.Chargers[i].Connectors = []domain.ConnectorConfig{connectors[1], connectors[0]}
	}

	forwardSessions := []domain.Session{
		testSession("session-1", "connector-1", 200),
		testSession("session-2", "connector-2", 80),
		testSession("session-3", "connector-3", 200),
		testSession("session-4", "connector-4", 40),
	}
	reverseSessions := []domain.Session{
		forwardSessions[3],
		forwardSessions[2],
		forwardSessions[1],
		forwardSessions[0],
	}

	first := Allocate(forwardConfig, forwardSessions, forwardConfig.GridCapacityKw)
	second := Allocate(reverseConfig, reverseSessions, reverseConfig.GridCapacityKw)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Allocate() differs by configuration or session order:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestAllocateWaitsWhenMinimumCannotBeReserved(t *testing.T) {
	config := stationWithOneCharger(100, 300, 100, 100, 100)
	start := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	first := testSession("session-1", "connector-1", 100)
	first.MinimumPowerKw = 40
	first.StartedAt = start
	second := testSession("session-2", "connector-2", 100)
	second.MinimumPowerKw = 40
	second.StartedAt = start.Add(time.Second)
	third := testSession("session-3", "connector-3", 100)
	third.MinimumPowerKw = 40
	third.StartedAt = start.Add(2 * time.Second)

	assignments := Allocate(config, []domain.Session{third, second, first}, config.GridCapacityKw)
	assertAssignment(t, assignments, "session-1", 50, domain.SessionStatusCharging)
	assertAssignment(t, assignments, "session-2", 50, domain.SessionStatusCharging)
	assertAssignment(t, assignments, "session-3", 0, domain.SessionStatusWaitingForPower)
}

func TestAllocateUsesSessionIDToBreakAdmissionTies(t *testing.T) {
	config := stationWithOneCharger(5, 100, 100, 100)
	sessionB := testSession("session-b", "connector-1", 100)
	sessionA := testSession("session-a", "connector-2", 100)

	assignments := Allocate(config, []domain.Session{sessionB, sessionA}, config.GridCapacityKw)
	assertAssignment(t, assignments, "session-a", 5, domain.SessionStatusCharging)
	assertAssignment(t, assignments, "session-b", 0, domain.SessionStatusWaitingForPower)
}

func TestAllocateKeepsFinalAllocationsFairAfterMinimums(t *testing.T) {
	config := stationWithOneCharger(30, 100, 100, 100)
	first := testSession("session-1", "connector-1", 100)
	first.MinimumPowerKw = 10
	second := testSession("session-2", "connector-2", 100)
	second.MinimumPowerKw = 5

	assignments := Allocate(config, []domain.Session{first, second}, config.GridCapacityKw)
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

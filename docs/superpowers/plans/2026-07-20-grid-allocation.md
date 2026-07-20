# Grid Allocation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a pure deterministic allocator that max-min shares grid power while respecting session, connector, charger, and availability constraints.

**Architecture:** `allocation.Allocate` accepts validated station configuration and active sessions, then returns stable session assignments. A progressive-filling loop raises every active session equally until a session demand, charger capacity, or grid capacity is exhausted.

**Tech Stack:** Go 1.26, standard library, built-in `testing` package.

## Global Constraints

- Use only the Go standard library.
- Keep allocation pure and independent from HTTP and mutable service state.
- Never depend on Go map iteration order for allocation decisions or output order.
- Use kW consistently and a small epsilon for floating-point comparisons.
- Do not add BESS, minimum-power admission, lifecycle handling, or concurrency in this slice.

---

### Task 1: Deterministic max-min grid allocation

**Files:**

- Create: `internal/allocation/allocator.go`
- Test: `internal/allocation/allocator_test.go`

**Interfaces:**

- Consumes: `domain.StationConfig` and `[]domain.Session`.
- Produces: `Allocate(domain.StationConfig, []domain.Session) []Assignment` with assignments sorted by session ID.

- [ ] **Step 1: Write the failing tests**

Create `internal/allocation/allocator_test.go` with separate tests for:

```go
package allocation

import (
	"fmt"
	"reflect"
	"testing"

	"electra-assignment/internal/domain"
)

const testEpsilon = 1e-6

func TestAllocateRespectsEffectiveDemandLimits(t *testing.T) {
	curveLimit := 60.0
	tests := []struct {
		name       string
		grid       float64
		charger    float64
		connector  float64
		requested  float64
		vehicle    float64
		curve      *float64
		wantPower  float64
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
				ID: "session-1", ConnectorID: "connector-1",
				RequestedPowerKw: test.requested, VehicleMaxPowerKw: test.vehicle,
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

func stationWithOneCharger(grid, charger float64, connectors ...float64) domain.StationConfig {
	connectorConfigs := make([]domain.ConnectorConfig, 0, len(connectors))
	for index, maximum := range connectors {
		connectorConfigs = append(connectorConfigs, domain.ConnectorConfig{
			ID: fmt.Sprintf("connector-%d", index+1), Type: "CCS",
			MaxPowerKw: maximum, Status: domain.OperationalStatusAvailable,
		})
	}
	return domain.StationConfig{
		ID: "station-1", GridCapacityKw: grid,
		Chargers: []domain.ChargerConfig{{
			ID: "charger-1", MaxPowerKw: charger,
			Status: domain.OperationalStatusAvailable, Connectors: connectorConfigs,
		}},
	}
}

func testSession(id, connectorID string, demand float64) domain.Session {
	return domain.Session{
		ID: id, ConnectorID: connectorID,
		RequestedPowerKw: demand, VehicleMaxPowerKw: demand,
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
```

- [ ] **Step 2: Run tests to verify RED**

Run: `go test ./internal/allocation -v`

Expected: FAIL because `Assignment` and `Allocate` do not exist.

- [ ] **Step 3: Implement the pure progressive-filling allocator**

Create `internal/allocation/allocator.go` with:

```go
package allocation

import (
	"sort"

	"electra-assignment/internal/domain"
)

const epsilon = 1e-9

type Assignment struct {
	SessionID       string  `json:"sessionId"`
	AssignedPowerKw float64 `json:"assignedPowerKw"`
}

type connectorLocation struct {
	charger   domain.ChargerConfig
	connector domain.ConnectorConfig
}

type allocationState struct {
	sessionID string
	chargerID string
	demandKw  float64
	assigned  float64
	active    bool
}

func Allocate(config domain.StationConfig, sessions []domain.Session) []Assignment {
	locations := connectorLocations(config)
	states := make([]allocationState, 0, len(sessions))
	remainingByCharger := make(map[string]float64, len(config.Chargers))
	for _, charger := range config.Chargers {
		remainingByCharger[charger.ID] = charger.MaxPowerKw
	}

	for _, session := range sessions {
		state := allocationState{sessionID: session.ID}
		location, exists := locations[session.ConnectorID]
		if exists && location.charger.Status == domain.OperationalStatusAvailable && location.connector.Status == domain.OperationalStatusAvailable {
			state.chargerID = location.charger.ID
			state.demandKw = effectiveDemand(session, location.connector.MaxPowerKw)
			state.active = state.demandKw > epsilon
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].sessionID < states[j].sessionID })

	remainingGrid := config.GridCapacityKw
	for {
		activeCount, activeByCharger := countActive(states)
		if activeCount == 0 || remainingGrid <= epsilon {
			break
		}

		step := remainingGrid / float64(activeCount)
		for _, state := range states {
			if !state.active {
				continue
			}
			chargerShare := remainingByCharger[state.chargerID] / float64(activeByCharger[state.chargerID])
			step = minimum(step, chargerShare, state.demandKw-state.assigned)
		}
		if step <= epsilon {
			break
		}

		for index := range states {
			if !states[index].active {
				continue
			}
			states[index].assigned += step
			remainingGrid -= step
			remainingByCharger[states[index].chargerID] -= step
		}

		for index := range states {
			if !states[index].active {
				continue
			}
			if states[index].demandKw-states[index].assigned <= epsilon || remainingByCharger[states[index].chargerID] <= epsilon {
				states[index].active = false
			}
		}
	}

	assignments := make([]Assignment, 0, len(states))
	for _, state := range states {
		assignments = append(assignments, Assignment{SessionID: state.sessionID, AssignedPowerKw: state.assigned})
	}
	return assignments
}

func connectorLocations(config domain.StationConfig) map[string]connectorLocation {
	locations := make(map[string]connectorLocation)
	for _, charger := range config.Chargers {
		for _, connector := range charger.Connectors {
			locations[connector.ID] = connectorLocation{charger: charger, connector: connector}
		}
	}
	return locations
}

func effectiveDemand(session domain.Session, connectorMax float64) float64 {
	demand := minimum(session.RequestedPowerKw, session.VehicleMaxPowerKw, connectorMax)
	if session.ChargingCurveLimitKw != nil {
		demand = minimum(demand, *session.ChargingCurveLimitKw)
	}
	return demand
}

func countActive(states []allocationState) (int, map[string]int) {
	activeByCharger := make(map[string]int)
	activeCount := 0
	for _, state := range states {
		if state.active {
			activeCount++
			activeByCharger[state.chargerID]++
		}
	}
	return activeCount, activeByCharger
}

func minimum(values ...float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
```

- [ ] **Step 4: Format and verify GREEN**

Run: `gofmt -w internal/allocation`

Run: `go test ./internal/allocation -v`

Expected: PASS.

Run: `go test ./...`

Expected: PASS for domain and allocation packages.

- [ ] **Step 5: Run static checks and commit**

Run: `go vet ./...`

Expected: exit 0 with no diagnostics.

```bash
git add internal/allocation/allocator.go internal/allocation/allocator_test.go
git commit -m "feat: add deterministic power allocation"
```

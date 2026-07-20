# Minimum Power Admission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure every charging session either receives at least its useful minimum power or remains active at `0 kW` with `waiting_for_power` status.

**Architecture:** Normalize omitted session minimums to `5 kW`. The allocator admits sessions by start time and session ID, reserves admitted minimums against station and charger capacity, then applies the existing progressive-filling algorithm to surplus power.

**Tech Stack:** Go 1.26, standard library, existing domain/allocation/service packages.

## Global Constraints

- Default omitted or zero minimum power to exactly `5 kW`.
- Allow a positive per-session override.
- Reject a normalized minimum above the session or charger maximum.
- Waiting sessions receive exactly `0 kW`.
- Charging sessions receive at least their normalized minimum.
- Admission priority is start time and then session ID only when minimums cannot all be satisfied.
- Keep BESS, HTTP, update, and stop behavior outside this slice.

---

### Task 1: Session minimum and status domain behavior

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/validation.go`
- Modify: `internal/domain/validation_test.go`

**Interfaces:**
- Produces: `DefaultMinimumPowerKw`, `SessionStatusCharging`, `SessionStatusWaitingForPower`, `NormalizeMinimumPowerKw(float64) float64`, and `EffectiveDemandKw(Session, float64) float64`.

- [ ] **Step 1: Add failing domain behavior tests**

Append these focused tests to `internal/domain/validation_test.go`:

```go
func TestSessionValidateRejectsNegativeMinimumPower(t *testing.T) {
	session := Session{
		ID: "session-1", ConnectorID: "connector-1",
		RequestedPowerKw: 100, VehicleMaxPowerKw: 100,
		MinimumPowerKw: -1,
	}
	if err := session.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want an error")
	}
}

func TestNormalizeMinimumPowerKw(t *testing.T) {
	if got := NormalizeMinimumPowerKw(0); got != 5 {
		t.Fatalf("NormalizeMinimumPowerKw(0) = %v, want 5", got)
	}
	if got := NormalizeMinimumPowerKw(12); got != 12 {
		t.Fatalf("NormalizeMinimumPowerKw(12) = %v, want 12", got)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/domain -run 'TestSessionValidateRejectsNegativeMinimumPower|TestNormalizeMinimumPowerKw' -v`

Expected: FAIL because the minimum field and normalization function do not exist.

- [ ] **Step 3: Add domain fields, constants, and helpers**

Add to `internal/domain/types.go`:

```go
const DefaultMinimumPowerKw = 5.0

type SessionStatus string

const (
	SessionStatusCharging       SessionStatus = "charging"
	SessionStatusWaitingForPower SessionStatus = "waiting_for_power"
)
```

Add these fields to `Session`:

```go
MinimumPowerKw   float64       `json:"minimumPowerKw"`
EffectiveDemandKw float64      `json:"effectiveDemandKw"`
Status           SessionStatus `json:"status"`
```

Add to `internal/domain/validation.go`:

```go
if session.MinimumPowerKw < 0 || math.IsInf(session.MinimumPowerKw, 0) || math.IsNaN(session.MinimumPowerKw) {
	return fmt.Errorf("minimum power cannot be negative or non-finite")
}
```

Add these helpers:

```go
func NormalizeMinimumPowerKw(value float64) float64 {
	if value == 0 {
		return DefaultMinimumPowerKw
	}
	return value
}

func EffectiveDemandKw(session Session, connectorMaxPowerKw float64) float64 {
	demand := minimumPower(session.RequestedPowerKw, session.VehicleMaxPowerKw, connectorMaxPowerKw)
	if session.ChargingCurveLimitKw != nil {
		demand = minimumPower(demand, *session.ChargingCurveLimitKw)
	}
	return demand
}

func minimumPower(values ...float64) float64 {
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

Run: `gofmt -w internal/domain`

Run: `go test ./internal/domain -v`

Expected: PASS.

---

### Task 2: Deterministic minimum admission and fair surplus

**Files:**
- Modify: `internal/allocation/allocator.go`
- Modify: `internal/allocation/allocator_test.go`

**Interfaces:**
- `Assignment` additionally produces `EffectiveDemandKw` and `Status`.
- `Allocate` preserves its existing signature and stable session-ID output ordering.

- [ ] **Step 1: Add failing minimum admission tests**

Add three tests to `internal/allocation/allocator_test.go`:

```go
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

func TestAllocateSharesSurplusAfterMinimums(t *testing.T) {
	config := stationWithOneCharger(30, 100, 100, 100)
	first := testSession("session-1", "connector-1", 100)
	first.MinimumPowerKw = 10
	second := testSession("session-2", "connector-2", 100)
	second.MinimumPowerKw = 5

	assignments := Allocate(config, []domain.Session{first, second})
	assertAssignment(t, assignments, "session-1", 17.5, domain.SessionStatusCharging)
	assertAssignment(t, assignments, "session-2", 12.5, domain.SessionStatusCharging)
}
```

Import `time` and add this helper:

```go
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
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/allocation -run 'TestAllocateWaits|TestAllocateUsesSessionID|TestAllocateSharesSurplus' -v`

Expected: FAIL because assignment status and minimum admission are not implemented.

- [ ] **Step 3: Extend allocation state and results**

Add these fields:

```go
// Assignment
EffectiveDemandKw float64              `json:"effectiveDemandKw"`
Status            domain.SessionStatus `json:"status"`

// allocationState
minimumKw float64
startedAt time.Time
eligible  bool
status    domain.SessionStatus
```

In `prepareAllocation`, set effective demand using `domain.EffectiveDemandKw`, normalize minimum power, preserve start time, and mark available sessions eligible. Initialize eligible sessions as waiting.

- [ ] **Step 4: Add deterministic admission before surplus allocation**

Change `Allocate` to:

```go
func Allocate(config domain.StationConfig, sessions []domain.Session) []Assignment {
	states, remainingByCharger := prepareAllocation(config, sessions)
	remainingGrid := admitSessions(states, config.GridCapacityKw, remainingByCharger)
	distributePower(states, remainingGrid, remainingByCharger)
	return assignmentsFromStates(states)
}
```

Add:

```go
func admitSessions(states []allocationState, remainingGrid float64, remainingByCharger map[string]float64) float64 {
	priority := make([]int, 0, len(states))
	for index := range states {
		if states[index].eligible {
			priority = append(priority, index)
		}
	}
	sort.Slice(priority, func(i, j int) bool {
		left, right := states[priority[i]], states[priority[j]]
		if left.startedAt.Equal(right.startedAt) {
			return left.sessionID < right.sessionID
		}
		return left.startedAt.Before(right.startedAt)
	})

	for _, index := range priority {
		state := &states[index]
		if state.minimumKw > state.demandKw+epsilon ||
			state.minimumKw > remainingGrid+epsilon ||
			state.minimumKw > remainingByCharger[state.chargerID]+epsilon {
			continue
		}
		state.assigned = state.minimumKw
		state.status = domain.SessionStatusCharging
		state.active = state.demandKw-state.assigned > epsilon
		remainingGrid -= state.minimumKw
		remainingByCharger[state.chargerID] -= state.minimumKw
	}
	return remainingGrid
}
```

Update `assignmentsFromStates` to return effective demand and status. Remove the allocator-local `effectiveDemand` function after all callers use `domain.EffectiveDemandKw`.

- [ ] **Step 5: Format and verify allocation**

Run: `gofmt -w internal/allocation`

Run: `go test ./internal/allocation -v`

Expected: PASS for existing and minimum-admission scenarios.

---

### Task 3: Normalize and validate minimums in the service

**Files:**
- Modify: `internal/service/service.go`
- Modify: `internal/service/service_test.go`

**Interfaces:**
- `StartSession` defaults omitted minimums, rejects impossible minimums, and returns effective demand and status after synchronous recomputation.

- [ ] **Step 1: Add failing service tests**

Extend the successful start assertion to require `MinimumPowerKw == 5`, `EffectiveDemandKw == 100`, and `Status == charging`.

Add:

```go
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
```

Add an invalid-operation subtest with demand `4 kW` and omitted minimum, expecting `ErrMinimumExceedsDemand`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/service -run TestServiceStartSession -v`

Expected: FAIL because the service does not normalize, validate, or store minimum admission results.

- [ ] **Step 3: Add service normalization and result storage**

Add:

```go
var ErrMinimumExceedsDemand = errors.New("minimum power exceeds effective demand")
```

After connector lookup and availability validation in `StartSession`:

```go
session.MinimumPowerKw = domain.NormalizeMinimumPowerKw(session.MinimumPowerKw)
session.EffectiveDemandKw = domain.EffectiveDemandKw(session, connector.MaxPowerKw)
if session.MinimumPowerKw > session.EffectiveDemandKw || session.MinimumPowerKw > charger.MaxPowerKw {
	return domain.Session{}, ErrMinimumExceedsDemand
}
```

In `recomputeLocked`, copy all allocation result fields:

```go
session.EffectiveDemandKw = assignment.EffectiveDemandKw
session.AssignedPowerKw = assignment.AssignedPowerKw
session.Status = assignment.Status
```

- [ ] **Step 4: Run complete verification**

Run: `gofmt -w internal/domain internal/allocation internal/service`

Run: `go test ./...`

Run: `go test -race ./...`

Run: `go vet ./...`

Expected: all commands exit successfully with no failures or diagnostics.

- [ ] **Step 5: Commit the implementation**

```bash
git add CLARIFICATIONS.md internal/domain/types.go internal/domain/validation.go internal/domain/validation_test.go internal/allocation/allocator.go internal/allocation/allocator_test.go internal/service/service.go internal/service/service_test.go
git commit -m "feat: enforce minimum charging power"
```

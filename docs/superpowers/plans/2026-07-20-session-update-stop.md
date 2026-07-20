# Session Update and Stop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add atomic session update and stop operations that synchronously recompute all allocations before returning.

**Architecture:** Keep lifecycle orchestration inside the existing mutex-protected service. Updates apply optional demand-limit fields to a copy, validate the complete candidate, then replace stored state; stops remove the session and return the recomputed station snapshot.

**Tech Stack:** Go 1.26, standard library, existing domain/allocation/service packages.

## Global Constraints

- Modify only `internal/service/service.go` and `internal/service/service_test.go` for production behavior.
- Preserve session ID, connector ID, and start timestamp during updates.
- Recompute allocations before successful update or stop calls return.
- Failed updates must not change stored state.
- Remove stopped sessions from active state; do not add history storage.
- Add no dependencies or new abstraction layers.

---

### Task 1: Atomic session updates

**Files:**
- Modify: `internal/service/service.go`
- Modify: `internal/service/service_test.go`

**Interfaces:**
- Produces: `SessionUpdate`, `ErrSessionNotFound`, and `(*Service).UpdateSession(string, SessionUpdate) (domain.Session, error)`.

- [ ] **Step 1: Write failing update tests**

Use this update shape:

```go
type SessionUpdate struct {
	RequestedPowerKw     *float64
	VehicleMaxPowerKw    *float64
	ChargingCurveLimitKw *float64
	MinimumPowerKw       *float64
}
```

Add these tests:

```go
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
		RequestedPowerKw: &five,
		VehicleMaxPowerKw: &five,
		MinimumPowerKw: &five,
	}); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	state, _ := service.Snapshot()
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
	state, _ := service.Snapshot()
	if state.Sessions[0].RequestedPowerKw != 100 || state.Sessions[0].AssignedPowerKw != 100 {
		t.Fatalf("session changed after invalid update: %#v", state.Sessions[0])
	}
	if _, err := service.UpdateSession("missing", SessionUpdate{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown update error = %v, want ErrSessionNotFound", err)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/service -run TestServiceUpdateSession -v`

Expected: FAIL because `SessionUpdate`, `UpdateSession`, and `ErrSessionNotFound` do not exist.

- [ ] **Step 3: Implement the minimal update operation**

Add:

```go
var ErrSessionNotFound = errors.New("session not found")

type SessionUpdate struct {
	RequestedPowerKw     *float64
	VehicleMaxPowerKw    *float64
	ChargingCurveLimitKw *float64
	MinimumPowerKw       *float64
}
```

Implement `UpdateSession` by locking, checking configuration and existence, copying the stored session, applying only non-nil fields, validating the candidate, normalizing its minimum, recalculating effective demand using the existing connector, rejecting an impossible minimum, preserving identity/start time, setting `UpdatedAt`, storing it, and calling `recomputeLocked` before returning the stored result.

Keep the field application explicit:

```go
if update.RequestedPowerKw != nil {
	candidate.RequestedPowerKw = *update.RequestedPowerKw
}
if update.VehicleMaxPowerKw != nil {
	candidate.VehicleMaxPowerKw = *update.VehicleMaxPowerKw
}
if update.ChargingCurveLimitKw != nil {
	curveLimit := *update.ChargingCurveLimitKw
	candidate.ChargingCurveLimitKw = &curveLimit
}
if update.MinimumPowerKw != nil {
	candidate.MinimumPowerKw = domain.NormalizeMinimumPowerKw(*update.MinimumPowerKw)
}
```

- [ ] **Step 4: Verify GREEN**

Run: `gofmt -w internal/service`

Run: `go test ./internal/service -run TestServiceUpdateSession -v`

Expected: PASS.

---

### Task 2: Session stop and immediate waiting-session admission

**Files:**
- Modify: `internal/service/service.go`
- Modify: `internal/service/service_test.go`

**Interfaces:**
- Consumes: `ErrSessionNotFound` from Task 1.
- Produces: `(*Service).StopSession(string) (StationState, error)`.

- [ ] **Step 1: Write failing stop tests**

Add:

```go
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
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/service -run TestServiceStopSession -v`

Expected: FAIL because `StopSession` does not exist.

- [ ] **Step 3: Implement the minimal stop operation**

Add `StopSession` using the existing lock:

```go
func (service *Service) StopSession(sessionID string) (StationState, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return StationState{}, ErrStationNotConfigured
	}
	if _, exists := service.sessions[sessionID]; !exists {
		return StationState{}, ErrSessionNotFound
	}

	delete(service.sessions, sessionID)
	service.recomputeLocked(time.Now().UTC())
	return service.snapshotLocked(), nil
}
```

- [ ] **Step 4: Verify GREEN and the complete slice**

Run: `gofmt -w internal/service`

Run: `go test ./internal/service -v`

Run: `go test ./...`

Run: `go test -race ./...`

Run: `go vet ./...`

Expected: all commands exit successfully with no failures or diagnostics.

- [ ] **Step 5: Commit**

```bash
git add internal/service/service.go internal/service/service_test.go
git commit -m "feat: update and stop charging sessions"
```

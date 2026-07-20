# Service Configuration and Session Start Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a mutex-protected single-station service that accepts configuration, returns safe snapshots, starts sessions, and recomputes allocation before returning.

**Architecture:** The service owns one copied station configuration and a session map behind one `sync.Mutex`. Public reads return deep copies; accepted starts validate references and occupancy, add the session, run the pure allocator under the same lock, and only then return.

**Tech Stack:** Go 1.26, standard library, existing domain and allocation packages.

## Global Constraints

- Use one in-memory station and one coarse mutex.
- Keep mutations and allocation recomputation atomic.
- Use only the Go standard library and existing internal packages.
- Reconfiguration replaces the station and clears active sessions.
- Do not add update, stop, HTTP, BESS, or minimum-power behavior in this slice.
- Test observable service behavior without mocks or sleeps.

---

### Task 1: Configure and snapshot one station

**Files:**
- Create: `internal/service/service.go`
- Test: `internal/service/service_test.go`

**Interfaces:**
- Produces: `New() *Service`, `Configure(domain.StationConfig) error`, and `Snapshot() (StationState, error)`.
- `StationState` exposes copied configuration, sorted sessions, grid import, available grid power, and last update time.

- [ ] **Step 1: Write failing configuration and snapshot tests**

Create `internal/service/service_test.go`:

```go
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

func testStationConfig() domain.StationConfig {
	return domain.StationConfig{
		ID: "station-1", GridCapacityKw: 100,
		Chargers: []domain.ChargerConfig{{
			ID: "charger-1", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable,
			Connectors: []domain.ConnectorConfig{
				{ID: "connector-1", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
				{ID: "connector-2", Type: "CCS", MaxPowerKw: 200, Status: domain.OperationalStatusAvailable},
			},
		}},
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/service -run 'TestServiceConfigureAndSnapshot|TestServiceSnapshotReturnsCopies' -v`

Expected: FAIL because `New`, `ErrStationNotConfigured`, and the service methods do not exist.

- [ ] **Step 3: Implement configuration and safe snapshots**

Create `internal/service/service.go`:

```go
package service

import (
	"errors"
	"sort"
	"sync"
	"time"

	"electra-assignment/internal/domain"
)

var ErrStationNotConfigured = errors.New("station is not configured")

type StationState struct {
	Config               domain.StationConfig `json:"config"`
	Sessions             []domain.Session     `json:"sessions"`
	GridImportKw         float64              `json:"gridImportKw"`
	AvailableGridPowerKw float64              `json:"availableGridPowerKw"`
	LastUpdatedAt        time.Time            `json:"lastUpdatedAt"`
}

type Service struct {
	mu            sync.Mutex
	config        *domain.StationConfig
	sessions      map[string]domain.Session
	lastUpdatedAt time.Time
}

func New() *Service {
	return &Service{sessions: make(map[string]domain.Session)}
}

func (service *Service) Configure(config domain.StationConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	config = cloneConfig(config)

	service.mu.Lock()
	defer service.mu.Unlock()
	service.config = &config
	service.sessions = make(map[string]domain.Session)
	service.lastUpdatedAt = time.Now().UTC()
	return nil
}

func (service *Service) Snapshot() (StationState, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return StationState{}, ErrStationNotConfigured
	}
	return service.snapshotLocked(), nil
}

func (service *Service) snapshotLocked() StationState {
	sessions := make([]domain.Session, 0, len(service.sessions))
	gridImport := 0.0
	for _, session := range service.sessions {
		sessions = append(sessions, cloneSession(session))
		gridImport += session.AssignedPowerKw
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID < sessions[j].ID })
	available := service.config.GridCapacityKw - gridImport
	if available < 0 {
		available = 0
	}
	return StationState{
		Config: cloneConfig(*service.config), Sessions: sessions,
		GridImportKw: gridImport, AvailableGridPowerKw: available,
		LastUpdatedAt: service.lastUpdatedAt,
	}
}

func cloneConfig(config domain.StationConfig) domain.StationConfig {
	clone := config
	clone.Chargers = make([]domain.ChargerConfig, len(config.Chargers))
	for index, charger := range config.Chargers {
		clone.Chargers[index] = charger
		clone.Chargers[index].Connectors = append([]domain.ConnectorConfig(nil), charger.Connectors...)
	}
	return clone
}

func cloneSession(session domain.Session) domain.Session {
	clone := session
	if session.ChargingCurveLimitKw != nil {
		curveLimit := *session.ChargingCurveLimitKw
		clone.ChargingCurveLimitKw = &curveLimit
	}
	return clone
}
```

- [ ] **Step 4: Format and verify GREEN**

Run: `gofmt -w internal/service`

Run: `go test ./internal/service -run 'TestServiceConfigureAndSnapshot|TestServiceSnapshotReturnsCopies' -v`

Expected: PASS.

---

### Task 2: Start sessions and recompute synchronously

**Files:**
- Modify: `internal/service/service.go`
- Modify: `internal/service/service_test.go`

**Interfaces:**
- Consumes: `allocation.Allocate`, configured station state, and validated `domain.Session`.
- Produces: `StartSession(domain.Session) (domain.Session, error)` plus sentinel errors for duplicate, unknown, occupied, and unavailable resources.

- [ ] **Step 1: Add failing successful-start and rejection tests**

Append to `internal/service/service_test.go`:

```go
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
		ID: id, ConnectorID: connectorID,
		RequestedPowerKw: demand, VehicleMaxPowerKw: demand,
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/service -run TestServiceStartSession -v`

Expected: FAIL because `StartSession` and its resource errors do not exist.

- [ ] **Step 3: Add session start and atomic recomputation**

Add `electra-assignment/internal/allocation` to `service.go` imports and add:

```go
var (
	ErrDuplicateSession    = errors.New("session already exists")
	ErrConnectorNotFound   = errors.New("connector not found")
	ErrConnectorOccupied   = errors.New("connector is occupied")
	ErrHardwareUnavailable = errors.New("charger or connector is unavailable")
)

func (service *Service) StartSession(session domain.Session) (domain.Session, error) {
	session = cloneSession(session)
	if err := session.Validate(); err != nil {
		return domain.Session{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if service.config == nil {
		return domain.Session{}, ErrStationNotConfigured
	}
	if _, exists := service.sessions[session.ID]; exists {
		return domain.Session{}, ErrDuplicateSession
	}
	charger, connector, exists := findConnector(*service.config, session.ConnectorID)
	if !exists {
		return domain.Session{}, ErrConnectorNotFound
	}
	if charger.Status != domain.OperationalStatusAvailable || connector.Status != domain.OperationalStatusAvailable {
		return domain.Session{}, ErrHardwareUnavailable
	}
	for _, active := range service.sessions {
		if active.ConnectorID == session.ConnectorID {
			return domain.Session{}, ErrConnectorOccupied
		}
	}

	now := time.Now().UTC()
	session.AssignedPowerKw = 0
	session.StartedAt = now
	session.UpdatedAt = now
	service.sessions[session.ID] = session
	service.recomputeLocked(now)
	return cloneSession(service.sessions[session.ID]), nil
}

func (service *Service) recomputeLocked(now time.Time) {
	sessions := make([]domain.Session, 0, len(service.sessions))
	for _, session := range service.sessions {
		sessions = append(sessions, session)
	}
	for _, assignment := range allocation.Allocate(*service.config, sessions) {
		session := service.sessions[assignment.SessionID]
		session.AssignedPowerKw = assignment.AssignedPowerKw
		service.sessions[assignment.SessionID] = session
	}
	service.lastUpdatedAt = now
}

func findConnector(config domain.StationConfig, connectorID string) (domain.ChargerConfig, domain.ConnectorConfig, bool) {
	for _, charger := range config.Chargers {
		for _, connector := range charger.Connectors {
			if connector.ID == connectorID {
				return charger, connector, true
			}
		}
	}
	return domain.ChargerConfig{}, domain.ConnectorConfig{}, false
}
```

- [ ] **Step 4: Format and verify the service-start slice**

Run: `gofmt -w internal/service`

Run: `go test ./internal/service -v`

Expected: PASS.

Run: `go test -race ./...`

Expected: PASS for domain, allocation, and service packages.

Run: `go vet ./...`

Expected: exit 0 with no diagnostics.

- [ ] **Step 5: Commit the slice**

```bash
git add internal/service/service.go internal/service/service_test.go
git commit -m "feat: configure station and start sessions"
```

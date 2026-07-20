# Domain Model and Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the Go module and the smallest domain model with focused validation for station configuration and session power inputs.

**Architecture:** Keep plain data structures in `internal/domain/types.go` and validation rules in `internal/domain/validation.go`. Validation returns ordinary descriptive errors; HTTP-specific error mapping belongs to a later slice.

**Tech Stack:** Go 1.26, standard library, built-in `testing` package.

## Global Constraints

- Use only the Go standard library.
- Use explicit power names ending in `Kw`.
- Do not test plain struct declarations, JSON tags, getters, or constructors without behavior.
- Keep optional BESS and minimum-power admission out of this slice.
- Write each behavioral test before its production implementation and verify the expected failure.

## File map

- `go.mod`: module declaration and Go version.
- `internal/domain/types.go`: operational status, station, charger, connector, and session data structures.
- `internal/domain/validation.go`: configuration and session validation behavior.
- `internal/domain/validation_test.go`: compact table-driven tests for meaningful accepted and rejected inputs.

---

### Task 1: Station configuration validation

**Files:**

- Create: `go.mod`
- Create: `internal/domain/types.go`
- Create: `internal/domain/validation.go`
- Test: `internal/domain/validation_test.go`

**Interfaces:**

- Consumes: no earlier implementation.
- Produces: `StationConfig.Validate() error` and the configuration types used by allocation and service slices.

- [ ] **Step 1: Add the module and failing station validation tests**

Create `go.mod`:

```go
module electra-assignment

go 1.26.0
```

Create `internal/domain/validation_test.go`:

```go
package domain

import (
	"math"
	"testing"
)

func TestStationConfigValidate(t *testing.T) {
	t.Run("accepts a valid configuration", func(t *testing.T) {
		if err := validStationConfig().Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*StationConfig)
	}{
		{name: "blank station ID", mutate: func(config *StationConfig) { config.ID = "" }},
		{name: "non-positive grid capacity", mutate: func(config *StationConfig) { config.GridCapacityKw = 0 }},
		{name: "non-finite grid capacity", mutate: func(config *StationConfig) { config.GridCapacityKw = math.NaN() }},
		{name: "duplicate charger ID", mutate: func(config *StationConfig) {
			config.Chargers = append(config.Chargers, ChargerConfig{
				ID: "charger-1", MaxPowerKw: 100, Status: OperationalStatusAvailable,
				Connectors: []ConnectorConfig{{ID: "connector-2", Type: "CCS", MaxPowerKw: 100, Status: OperationalStatusAvailable}},
			})
		}},
		{name: "non-positive charger capacity", mutate: func(config *StationConfig) { config.Chargers[0].MaxPowerKw = -1 }},
		{name: "charger without connectors", mutate: func(config *StationConfig) { config.Chargers[0].Connectors = nil }},
		{name: "invalid charger status", mutate: func(config *StationConfig) { config.Chargers[0].Status = "broken" }},
		{name: "duplicate connector ID", mutate: func(config *StationConfig) {
			config.Chargers = append(config.Chargers, ChargerConfig{
				ID: "charger-2", MaxPowerKw: 100, Status: OperationalStatusAvailable,
				Connectors: []ConnectorConfig{{ID: "connector-1", Type: "CCS", MaxPowerKw: 100, Status: OperationalStatusAvailable}},
			})
		}},
		{name: "blank connector type", mutate: func(config *StationConfig) { config.Chargers[0].Connectors[0].Type = "" }},
		{name: "non-positive connector capacity", mutate: func(config *StationConfig) { config.Chargers[0].Connectors[0].MaxPowerKw = 0 }},
		{name: "invalid connector status", mutate: func(config *StationConfig) { config.Chargers[0].Connectors[0].Status = "broken" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validStationConfig()
			test.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}

func validStationConfig() StationConfig {
	return StationConfig{
		ID:             "station-1",
		GridCapacityKw: 400,
		Chargers: []ChargerConfig{{
			ID:         "charger-1",
			MaxPowerKw: 300,
			Status:     OperationalStatusAvailable,
			Connectors: []ConnectorConfig{{
				ID: "connector-1", Type: "CCS", MaxPowerKw: 200, Status: OperationalStatusAvailable,
			}},
		}},
	}
}
```

- [ ] **Step 2: Run the tests and verify the expected compile failure**

Run: `go test ./internal/domain -run TestStationConfigValidate -v`

Expected: FAIL because `StationConfig`, `ChargerConfig`, and `ConnectorConfig` do not exist.

- [ ] **Step 3: Add the minimal configuration types**

Create `internal/domain/types.go`:

```go
package domain

import "time"

type OperationalStatus string

const (
	OperationalStatusAvailable   OperationalStatus = "available"
	OperationalStatusUnavailable OperationalStatus = "unavailable"
)

type StationConfig struct {
	ID             string          `json:"id"`
	GridCapacityKw float64         `json:"gridCapacityKw"`
	Chargers       []ChargerConfig `json:"chargers"`
}

type ChargerConfig struct {
	ID         string            `json:"id"`
	MaxPowerKw float64           `json:"maxPowerKw"`
	Status     OperationalStatus `json:"status"`
	Connectors []ConnectorConfig `json:"connectors"`
}

type ConnectorConfig struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	MaxPowerKw float64           `json:"maxPowerKw"`
	Status     OperationalStatus `json:"status"`
}

type Session struct {
	ID                       string    `json:"id"`
	ConnectorID              string    `json:"connectorId"`
	RequestedPowerKw         float64   `json:"requestedPowerKw"`
	VehicleMaxPowerKw        float64   `json:"vehicleMaxPowerKw"`
	ChargingCurveLimitKw     *float64  `json:"chargingCurveLimitKw,omitempty"`
	AssignedPowerKw          float64   `json:"assignedPowerKw"`
	StartedAt                time.Time `json:"startedAt"`
	UpdatedAt                time.Time `json:"updatedAt"`
}
```

- [ ] **Step 4: Add minimal configuration validation**

Create `internal/domain/validation.go` with:

```go
package domain

import (
	"fmt"
	"math"
	"strings"
)

func (config StationConfig) Validate() error {
	if strings.TrimSpace(config.ID) == "" {
		return fmt.Errorf("station ID is required")
	}
	if !isPositiveFinite(config.GridCapacityKw) {
		return fmt.Errorf("grid capacity must be positive")
	}

	chargerIDs := make(map[string]struct{}, len(config.Chargers))
	connectorIDs := make(map[string]struct{})
	for _, charger := range config.Chargers {
		if err := validateCharger(charger, chargerIDs, connectorIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateCharger(charger ChargerConfig, chargerIDs, connectorIDs map[string]struct{}) error {
	if strings.TrimSpace(charger.ID) == "" {
		return fmt.Errorf("charger ID is required")
	}
	if _, exists := chargerIDs[charger.ID]; exists {
		return fmt.Errorf("duplicate charger ID %q", charger.ID)
	}
	chargerIDs[charger.ID] = struct{}{}
	if !isPositiveFinite(charger.MaxPowerKw) {
		return fmt.Errorf("charger %q maximum power must be positive", charger.ID)
	}
	if !charger.Status.valid() {
		return fmt.Errorf("charger %q has invalid status %q", charger.ID, charger.Status)
	}
	if len(charger.Connectors) == 0 {
		return fmt.Errorf("charger %q must have at least one connector", charger.ID)
	}
	for _, connector := range charger.Connectors {
		if err := validateConnector(connector, connectorIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateConnector(connector ConnectorConfig, connectorIDs map[string]struct{}) error {
	if strings.TrimSpace(connector.ID) == "" {
		return fmt.Errorf("connector ID is required")
	}
	if _, exists := connectorIDs[connector.ID]; exists {
		return fmt.Errorf("duplicate connector ID %q", connector.ID)
	}
	connectorIDs[connector.ID] = struct{}{}
	if strings.TrimSpace(connector.Type) == "" {
		return fmt.Errorf("connector %q type is required", connector.ID)
	}
	if !isPositiveFinite(connector.MaxPowerKw) {
		return fmt.Errorf("connector %q maximum power must be positive", connector.ID)
	}
	if !connector.Status.valid() {
		return fmt.Errorf("connector %q has invalid status %q", connector.ID, connector.Status)
	}
	return nil
}

func (status OperationalStatus) valid() bool {
	return status == OperationalStatusAvailable || status == OperationalStatusUnavailable
}

func isPositiveFinite(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}
```

- [ ] **Step 5: Format and verify station validation**

Run: `gofmt -w internal/domain`

Run: `go test ./internal/domain -run TestStationConfigValidate -v`

Expected: PASS with the valid case and each invalid case behaving as specified.

---

### Task 2: Session input validation

**Files:**

- Modify: `internal/domain/validation.go`
- Modify: `internal/domain/validation_test.go`

**Interfaces:**

- Consumes: `Session` and `isPositiveFinite` from Task 1.
- Produces: `Session.Validate() error`, used later by the service before reference and occupancy checks.

- [ ] **Step 1: Add failing session validation tests**

Append to `internal/domain/validation_test.go`:

```go
func TestSessionValidate(t *testing.T) {
	curveLimit := 120.0
	valid := Session{
		ID: "session-1", ConnectorID: "connector-1",
		RequestedPowerKw: 150, VehicleMaxPowerKw: 140,
		ChargingCurveLimitKw: &curveLimit,
	}

	t.Run("accepts valid power limits", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	tests := []struct {
		name   string
		mutate func(*Session)
	}{
		{name: "blank session ID", mutate: func(session *Session) { session.ID = "" }},
		{name: "blank connector ID", mutate: func(session *Session) { session.ConnectorID = "" }},
		{name: "non-positive requested power", mutate: func(session *Session) { session.RequestedPowerKw = 0 }},
		{name: "non-positive vehicle maximum", mutate: func(session *Session) { session.VehicleMaxPowerKw = -1 }},
		{name: "non-positive curve limit", mutate: func(session *Session) {
			invalid := 0.0
			session.ChargingCurveLimitKw = &invalid
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := valid
			test.mutate(&session)
			if err := session.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}
```

Do not test assigned power or timestamps because lifecycle code owns those values.

- [ ] **Step 2: Run the focused test and verify the expected failure**

Run: `go test ./internal/domain -run TestSessionValidate -v`

Expected: FAIL because `Session.Validate` does not exist.

- [ ] **Step 3: Add minimal session validation**

Append to `internal/domain/validation.go`:

```go
func (session Session) Validate() error {
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session ID is required")
	}
	if strings.TrimSpace(session.ConnectorID) == "" {
		return fmt.Errorf("connector ID is required")
	}
	if !isPositiveFinite(session.RequestedPowerKw) {
		return fmt.Errorf("requested power must be positive")
	}
	if !isPositiveFinite(session.VehicleMaxPowerKw) {
		return fmt.Errorf("vehicle maximum power must be positive")
	}
	if session.ChargingCurveLimitKw != nil && !isPositiveFinite(*session.ChargingCurveLimitKw) {
		return fmt.Errorf("charging curve limit must be positive when provided")
	}
	return nil
}
```

- [ ] **Step 4: Format and verify the complete domain slice**

Run: `gofmt -w internal/domain`

Run: `go test ./internal/domain -v`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 5: Commit the domain slice**

```bash
git add go.mod internal/domain/types.go internal/domain/validation.go internal/domain/validation_test.go
git commit -m "feat: add station domain validation"
```

## Mission

Build a small, polished **Station Energy Management System** for one EV charging station.

The service accepts station configuration and charging-session events, computes safe power allocations, reacts synchronously to state changes, and exposes the current station state through an HTTP REST API.

This is a four-hour take-home assignment. Prefer a complete, explainable solution over architectural breadth.

The submission should be:

- Correct and deterministic
- Easy to run and review
- Well tested around core behaviour
- Clearly documented
- Free of unnecessary infrastructure
- Explainable in a technical debrief

AI-assisted development is allowed, but every line and design decision must remain understandable by the candidate.

---

## Scope

### Required core

Implement:

- A Go backend
- An HTTP REST API
- One station instance
- In-memory state
- Safe concurrent access to mutable state
- Session start, update, and stop handling
- Deterministic fair power allocation
- Grid, charger, connector, vehicle, and charging-curve limits
- Synchronous allocation recomputation after accepted state changes
- Automated tests for the core allocation and lifecycle behaviour
- Docker support
- A concise README with assumptions, trade-offs, and run instructions
- Runnable example requests or a small demonstration script

### Optional extensions

Only implement these after the core behaviour is complete and tested:

- Minimum useful charging-power admission and `waiting_for_power`
- BESS boost and charging
- Lightweight BESS State-of-Charge tracking
- Charger and connector availability updates
- Additional concurrency and edge-case tests

### Explicitly out of scope

Do not add:

- A frontend
- Authentication or authorization
- External databases
- Kafka or another event broker
- Redis
- Kubernetes
- Datadog, Sentry, or another hosted observability product
- OCPP integration
- Multi-station orchestration
- Distributed services or distributed locking
- Production high-availability infrastructure
- Battery chemistry, temperature, degradation, or tariff modelling

These may be mentioned briefly as production follow-ups, but they should not be implemented.

---

## Implementation Order

Work in this order:

1. Domain model and validation
2. Grid-only allocation engine
3. Session lifecycle
4. REST API
5. Core automated tests
6. Docker and runnable examples
7. README and cleanup
8. Optional minimum-power admission
9. Optional BESS support
10. Additional hardening only if time remains

The allocation engine and its invariants are the critical path.

Do not sacrifice a working core, tests, Docker usability, or documentation for optional features.

---

## Product Decisions and Assumptions

### Fair allocation

When total demand exceeds available station power, active sessions are treated equally by default.

Use deterministic max-min fair allocation, such as iterative water filling:

- Give eligible sessions an equal share of constrained power.
- Cap each session by its effective maximum demand.
- Redistribute unused capacity from low-demand sessions.
- Respect station and charger-level shared limits.
- Do not use first-come-first-served allocation during ordinary fair sharing.
- Do not add booking, fleet, customer, or State-of-Charge priority tiers.

Equivalent inputs must always produce equivalent allocations. Never rely on Go map iteration order as a tie-breaker.

### Immediate redistribution

Every accepted state-changing request must recompute allocations synchronously before returning.

At minimum this includes:

- Session start
- Session update
- Session stop
- Optional availability changes
- Optional BESS state changes

The in-memory computation should comfortably satisfy the sub-second reaction requirement.

### Minimum useful charging power — optional

The virtual PM confirmed that minimum-power admission is a reasonable extension, but not a core requirement.

The preferred core behaviour is simple:

- Sessions are admitted.
- They receive a fair allocation up to their effective maximum demand.
- If a minimum threshold is represented but cannot be met, the documented fallback may pause the session at `0 kW` or allow a smaller allocation. Choose one simple policy and document it.

Optional advanced policy:

1. Each session may declare a minimum useful charging power.
2. If available supply cannot satisfy every eligible session's minimum, admit sessions deterministically by start time and then session ID.
3. Non-admitted sessions remain active with status `waiting_for_power` and receive `0 kW`.
4. Apply max-min fair sharing only across admitted sessions.
5. Re-evaluate waiting sessions after each accepted state change.

Only implement this if it is a natural, small extension of the finished core allocator.

### BESS — optional

BESS is an advanced feature from the assignment brief.

If implemented, keep it deliberately small:

- Discharge to supplement the grid when EV demand exceeds grid capacity.
- Never exceed maximum BESS discharge power.
- Never discharge below the configured minimum SoC, such as 10%.
- Charge using spare grid capacity only after EV demand is served.
- Never reduce EV allocations merely to charge the BESS.
- Never exceed maximum BESS charge power or 100% SoC.

Track SoC with a simple energy tally:

```text
energy delta in kWh = power in kW × elapsed time in hours
```

Assume 100% efficiency and document that simplification.

Do not model chemistry, degradation, temperature, nonlinear curves, or real-time hardware behaviour.

A deterministic simulation tick is acceptable if time progression is needed for testing. It should remain a small helper or endpoint, not a separate simulation subsystem.

---

## Technical Direction

### Language and HTTP

Use Go.

Prefer Go's standard `net/http` package or a minimal router with a clear reason. Avoid large frameworks.

### State and concurrency

Use in-memory state.

Protect mutations with a mutex so that:

- A mutation and allocation recomputation are atomic.
- Readers do not observe partially applied changes.
- Allocation runs against one consistent state snapshot.
- The allocation algorithm itself remains pure where practical.

A mutex-protected service layer is sufficient. Do not add an internal event bus.

Document that process restarts reset state and that persistence or event replay would be production follow-ups.

### Numeric representation

Use kilowatts for power and kilowatt-hours for energy.

Prefer explicit names such as:

- `GridCapacityKw`
- `RequestedPowerKw`
- `AssignedPowerKw`
- `EnergyCapacityKwh`
- `SocPercent`

Floating-point values are acceptable. Use a small documented epsilon in comparisons and tests.

---

## Suggested Repository Structure

```text
.
├── AGENTS.md
├── README.md
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── allocation/
│   ├── api/
│   ├── domain/
│   └── service/
└── examples/
    ├── station.json
    └── scenarios.sh
```

Keep boundaries simple:

- `domain`: station, charger, connector, session, optional BESS, validation
- `allocation`: pure allocation logic and result types
- `service`: in-memory state, locking, lifecycle orchestration, recomputation
- `api`: handlers, DTOs, JSON, routing, status-code mapping
- `examples`: sample configuration and reviewer-friendly commands

Do not restructure working code merely to match this exact tree.

---

## Core Domain Model

### Station

A station contains:

- Stable station ID
- Grid import capacity in kW
- Chargers
- Optional BESS
- Current EV demand and allocation
- Current grid import
- Last update timestamp

The service handles one station only.

### Charger

A charger contains:

- Stable charger ID
- Maximum charger power in kW
- One or more connectors
- Operational availability status

The sum of allocations across its connectors must never exceed the charger maximum.

### Connector

A connector contains:

- Stable connector ID
- Charger ID
- Connector type
- Maximum connector power in kW
- Operational availability status
- Optional active session

Only one active session may exist on a connector.

Dynamic charger or connector availability-update endpoints are optional. However, availability must always be represented in station configuration and exposed through station status responses.

### Charging session

A session contains:

- Stable session ID
- Connector ID
- Requested power in kW
- Vehicle maximum power in kW
- Optional charging-curve limit in kW
- Assigned power in kW
- Start and update timestamps
- Optional minimum useful power and status

Its effective maximum demand is the minimum of:

```text
requested power
vehicle maximum power
charging-curve limit, when provided
connector maximum power
```

Reject negative values, duplicate active session IDs, occupied connectors, and invalid references.

### Optional BESS

A BESS may contain:

- Energy capacity in kWh
- Current SoC percentage
- Maximum charge power in kW
- Maximum discharge power in kW
- Minimum allowed SoC percentage
- Current power and mode

Use one documented sign convention consistently. A simple option is:

- Positive power: discharging into the station
- Negative power: charging from the grid
- Zero: idle

---

## Hard Invariants

After every accepted state-changing request:

1. Grid import never exceeds configured grid capacity.
2. Grid import is never negative.
3. Connector allocation never exceeds connector capacity.
4. Session allocation never exceeds effective maximum demand.
5. Charger allocations never exceed charger capacity.
6. A connector has at most one active session.
7. Unavailable hardware, if supported, receives zero allocation.
8. Allocations are recomputed before the response is returned.
9. Equivalent inputs produce equivalent outputs.
10. Power uses kW and energy uses kWh.

If minimum-power admission is implemented:

11. A charging session receives at least its minimum useful power.
12. A waiting session receives exactly zero power.

If BESS is implemented:

13. Charge and discharge limits are respected.
14. SoC remains between the configured minimum and 100%.
15. EV allocations are never reduced merely to charge the BESS.
16. BESS discharge never exceeds the power required by EV demand.

Tests should directly cover the implemented invariants.

---

## Allocation Policy

Use the clearest correct deterministic max-min fair algorithm.

The allocator should:

1. Determine eligible active sessions.
2. Calculate each session's effective maximum demand.
3. Determine available station supply.
4. Share constrained supply fairly.
5. Respect connector and charger-level limits.
6. Redistribute unused capacity from low-demand sessions.
7. Return zero for ineligible, unavailable, or optionally waiting sessions.
8. Produce stable output ordering.

Before allocation, sort stable identifiers where ordering matters. Do not depend on map order.

A hierarchical implementation is acceptable:

1. Allocate station supply across chargers based on demand and charger limits.
2. Allocate each charger's share across its sessions.
3. Redistribute unused capacity where needed.

A global iterative algorithm is also acceptable. Choose the approach that is easiest to prove and explain.

---

## Session Lifecycle

### Start

- Validate the request and referenced connector/charger.
- Reject duplicate active session IDs.
- Reject an occupied or unavailable connector.
- Add the session.
- Recompute allocations synchronously.
- Return the created session or updated station state.

### Update

Allow updates to relevant demand limits, such as:

- Requested power
- Vehicle maximum power
- Charging-curve limit
- Optional minimum useful power

Validate, apply atomically, recompute, and return the updated state.

### Stop

- Validate that the session exists.
- Remove or mark it stopped.
- Set its allocation to zero.
- Recompute remaining allocations synchronously.
- Return the updated station state.

When capacity becomes available, remaining sessions should receive it immediately.

---

## Minimal HTTP API

Keep the API compact. A suitable core surface is:

```http
GET    /health
PUT    /api/v1/station/config
GET    /api/v1/station
POST   /api/v1/sessions
PATCH  /api/v1/sessions/{sessionId}
DELETE /api/v1/sessions/{sessionId}
```

Optional endpoints:

```http
PATCH /api/v1/chargers/{chargerId}
PATCH /api/v1/connectors/{connectorId}
POST  /api/v1/simulation/tick
```

Use conventional status codes:

- `200 OK` for reads and successful updates
- `201 Created` for a new session
- `400 Bad Request` for malformed or invalid input
- `404 Not Found` for unknown resources
- `409 Conflict` for duplicates or occupied connectors
- `500 Internal Server Error` only for unexpected failures

Use one consistent, simple JSON error shape. Stable machine-readable codes are helpful but do not build a large error taxonomy unless it comes naturally.

The station response should clearly distinguish:

- Requested demand
- Effective demand
- Assigned power
- Session status, if used
- Grid capacity and current grid import
- Available grid power
- Available station power
- Charger and connector availability
- Optional BESS contribution and SoC

For grid-only operation:

```text
available grid power = max(0, grid capacity - current grid import)
```

When BESS is implemented, available station power may additionally include the BESS discharge power currently permitted by its power and SoC limits.

---

## Test Scenarios

Implement a focused set of tests and briefly explain what each validates.

### Core scenarios

1. **Single session** — receives all power allowed by station and physical limits.
2. **Two constrained sessions** — share available power fairly.
3. **Low-demand redistribution** — unused share is reassigned to another session.
4. **Session stop** — remaining sessions immediately receive freed capacity.
5. **Effective-demand limits** — vehicle, charging curve, connector, and charger limits are respected.
6. **Shared charger bottleneck** — sessions on one charger do not exceed its total limit.
7. **Grid invariant** — grid capacity is never exceeded.
8. **Invalid lifecycle operations** — unknown connector, duplicate session, occupied connector, and invalid power are rejected.
9. **Determinism** — equivalent inputs in different insertion order produce the same result.

### Optional scenarios

10. **Minimum-power admission** — a session waits when its useful minimum cannot be met.
11. **Waiting session resumes** — capacity changes cause immediate reconsideration.
12. **BESS boost** — discharge supplements the grid without crossing power or SoC limits.
13. **BESS charging** — spare grid capacity charges the battery without reducing EV allocations.
14. **SoC tally** — an explicit elapsed-time update changes SoC deterministically.
15. **Concurrent mutations** — race detector passes and final invariants hold.

Do not create dozens of tests merely to satisfy a checklist. Prefer a compact suite that demonstrates the important invariants clearly.

Useful commands:

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
```

Use tolerances for floating-point assertions. Avoid tests based on sleeps or wall-clock timing.

---

## Docker and Runnable Examples

Provide a small multi-stage Docker build and a `docker-compose.yml` that runs only the API.

Reviewers should be able to use:

```bash
docker compose up --build
```

A sample station may use values from the brief, such as:

- Grid capacity: `400 kW`
- Two chargers with `300 kW` maximum each
- Two connectors per charger
- Optional BESS: `200 kWh`, `200 kW` charge/discharge, `10%` minimum SoC

Provide either clear `curl` examples or one small script such as:

```bash
./examples/scenarios.sh
```

The example should demonstrate the main flow: configure, start sessions, observe sharing, update or stop a session, and observe redistribution. Optional features can be shown only if implemented.

---

## README Requirements

Keep the README concise and reviewer-friendly. It should explain:

1. What the system does
2. Implemented scope and omitted optional features
3. How to run locally and with Docker
4. How to run tests and examples
5. Repository structure and package boundaries
6. Allocation algorithm and deterministic behaviour
7. Important assumptions and trade-offs
8. How the sub-second reaction requirement is satisfied
9. Brief production follow-ups

Document candidate-owned decisions clearly, especially:

- Fairness policy
- Charger-level sharing behaviour
- State storage and concurrency model
- Minimum-power behaviour, whether implemented or not
- BESS simplifications, if implemented

Do not turn the README into a full production design document.

---

## Coding Standards

- Use small, focused functions.
- Keep HTTP handlers thin.
- Keep allocation logic independent from HTTP concerns.
- Avoid global mutable state.
- Keep dependencies minimal.
- Use explicit names with units.
- Return actionable errors.
- Do not duplicate business rules.
- Comment non-obvious decisions, not obvious syntax.
- Format all Go code.
- Do not weaken invariants merely to make tests pass.

---

## AI Agent Rules

When using Codex or another coding agent:

1. Read the assignment and this file before changing code.
2. Inspect the existing repository first.
3. Implement the smallest coherent slice.
4. Complete and verify the core before optional features.
5. Add or update tests with behaviour changes.
6. Run relevant verification commands after edits.
7. Do not add dependencies without a clear reason.
8. Do not invent new product requirements.
9. Record meaningful assumptions in the README.
10. Preserve deterministic behaviour.
11. Never rely on map iteration order.
12. Keep allocation logic out of HTTP handlers.
13. Do not add a frontend or unnecessary infrastructure.
14. Avoid large rewrites late in the assignment.
15. Prefer reviewer usability and explainability over novelty.

---

## Definition of Done

### Core completion

The submission is complete when:

- A station can be configured and queried.
- Sessions can start, update, and stop.
- Every accepted event triggers synchronous recomputation.
- Power is shared deterministically and fairly.
- Low-demand sessions return unused capacity.
- Grid, charger, connector, vehicle, and charging-curve limits are respected.
- Core automated tests pass.
- Docker startup works from a clean checkout.
- Runnable examples demonstrate the main behaviour.
- README instructions and assumptions are accurate.
- No unnecessary infrastructure has been added.
- The candidate can explain every implemented decision.

### Optional completion

Optional work is successful when it is small, tested, and does not compromise the core:

- Minimum useful power and waiting behaviour, if implemented
- BESS boost and spare-capacity charging, if implemented
- Lightweight deterministic SoC tracking, if implemented

---

## Final Guardrail

This is a four-hour assignment, not a production rewrite of Electra's platform.

Ship the smallest polished backend that demonstrates correct allocation, immediate redistribution, clear interfaces, strong core tests, and sound engineering judgement.

A functioning and explainable system is stronger than an unfinished advanced one.

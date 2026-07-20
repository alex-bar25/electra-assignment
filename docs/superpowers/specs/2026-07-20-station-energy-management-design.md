# Station Energy Management System Design

## Goal and scope

Build one small Go HTTP service that manages one EV charging station in memory. The core supports station configuration, session start/update/stop, synchronous allocation recomputation, and station status reads.

The core includes minimum-power admission so sessions never receive an unusable trickle. It deliberately excludes BESS behavior, dynamic hardware availability updates, persistence, authentication, and external infrastructure until the required core is complete.

## Architecture

Use only the Go standard library and four focused internal packages:

- `domain`: station configuration, chargers, connectors, sessions, and validation.
- `allocation`: a pure deterministic power-allocation function.
- `service`: mutex-protected in-memory state and session lifecycle orchestration.
- `api`: thin `net/http` handlers, JSON DTO handling, and HTTP error mapping.

`cmd/server` constructs the service and HTTP server. No repository interfaces, dependency injection framework, event bus, or speculative abstractions are needed.

## Domain model

A station configuration has an ID, grid capacity, and a list of chargers. A charger has an ID, maximum power, availability, and connectors. A connector has an ID, type, maximum power, and availability. IDs are unique within their resource type.

An active session has an ID, connector ID, requested power, vehicle maximum power, an optional charging-curve limit, minimum useful power, effective demand, assigned power, status, and timestamps. Its effective demand is the minimum of its positive applicable limits: requested, vehicle, connector, optional curve, and charger capacity.

`MinimumPowerKw` defaults to `5 kW` when omitted or zero. A positive session-specific value overrides the default. A session minimum must not exceed its effective demand. Session status is either `charging` or `waiting_for_power`.

Configuration validation rejects blank IDs, non-positive capacities, duplicate charger or connector IDs, and chargers without connectors. Session boundary validation rejects blank IDs, non-positive required power values, non-positive provided curve limits, negative or non-finite minimum power, a normalized minimum above effective demand, unknown or unavailable connectors, duplicate active session IDs, and occupied connectors.

## Allocation

The allocator receives an immutable view of configuration and active sessions and returns assignments keyed by session ID. It does not mutate shared state.

Eligible sessions are considered for admission by start time and then session ID. Admission reserves each session's minimum against remaining station and charger capacity. A session that cannot reserve its minimum remains active with `waiting_for_power` status and receives `0 kW`.

Admitted sessions begin at their reserved minimum. Allocation then uses deterministic progressive filling for surplus power: raise the sessions with the lowest current allocation until they reach the next allocation level, station capacity, a session effective-demand cap, or a shared charger cap. Freeze constrained sessions and continue redistributing remaining capacity. This keeps final allocations max-min fair even when sessions declare different minimums.

Unavailable hardware and waiting sessions receive zero allocation. Output ordering is stable and no decision relies on Go map iteration order. Floating-point comparisons use one small package-level epsilon.

## State and request flow

The service owns the single configured station and active sessions behind one mutex. A configuration or session mutation validates, applies the change, recomputes all assignments synchronously, stores the result, and only then releases the lock. Reads return copies so callers cannot mutate shared state.

This coarse lock keeps each mutation and recomputation atomic. It is appropriate for one small in-memory station and makes partially updated state impossible to observe.

Station snapshots derive an OPS-oriented read model from configuration and active sessions. It includes station capacity, current import and headroom, per-charger current power, and per-connector availability, occupancy, active session, and assigned power. The read model is rebuilt for each snapshot rather than stored as a second mutable representation.

## HTTP API and errors

The API surface is:

- `GET /health`
- `PUT /api/v1/station/config`
- `GET /api/v1/station`
- `POST /api/v1/sessions`
- `PATCH /api/v1/sessions/{sessionId}`
- `DELETE /api/v1/sessions/{sessionId}`

Handlers decode JSON, call the service, and encode responses. Domain/service errors remain small typed or sentinel errors that handlers map to `400`, `404`, or `409`. Unexpected errors map to `500`. Error responses use one consistent JSON object containing a short machine-readable code and message.

## Testing

Tests cover behavior, not declarations or implementation details:

- A small table of meaningful configuration and session validation failures.
- Allocation scenarios for one session, fair sharing, redistribution, effective limits, shared charger limits, grid limits, minimum admission, deterministic priority, and deterministic insertion order.
- Service lifecycle scenarios proving start/update/stop recompute immediately, reject invalid operations, and reconsider waiting sessions when capacity changes.
- A compact handler test set for routing, status codes, and the main successful flow.

No tests are added for plain structs, JSON tags in isolation, getters, constructors without behavior, or exhaustive permutations of equivalent cases. Tests avoid mocks where direct calls are simpler.

## Delivery slices

Implementation proceeds in reviewable chunks:

1. Go module, domain types, validation, and focused validation tests.
2. Pure allocator and its core invariant tests.
3. Minimum-power admission and focused allocator/service tests.
4. Mutex-protected service update/stop lifecycle tests.
5. HTTP API and focused handler tests.
6. Server wiring, Docker files, example script, README, and final verification.

After each chunk, format and run the relevant focused tests. Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and a clean build before completion.

## Assumptions and trade-offs

- Zero is not a useful configured capacity or active-session demand, so configured maximums and required session limits must be positive.
- Omitting the charging-curve limit means it does not constrain demand.
- Stopped sessions are removed from active state rather than retained as history.
- Process restart loses state by design.
- A single coarse lock favors clarity and correctness over unnecessary concurrency within one station.
- The `5 kW` default is an assignment-level useful-power assumption; callers may supply a vehicle-specific override.
- Admission priority is start time and then session ID. It is used only when every session minimum cannot be satisfied; ordinary surplus sharing has no first-come-first-served priority.
- BESS is reconsidered only after the complete core is working, tested, runnable, and documented.

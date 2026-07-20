# Station Energy Management System Design

## Goal and scope

Build one small Go HTTP service that manages one EV charging station in memory. The core supports station configuration, session start/update/stop, synchronous allocation recomputation, and station status reads.

The first version deliberately excludes minimum-power admission, BESS behavior, dynamic hardware availability updates, persistence, authentication, and external infrastructure.

## Architecture

Use only the Go standard library and four focused internal packages:

- `domain`: station configuration, chargers, connectors, sessions, and validation.
- `allocation`: a pure deterministic power-allocation function.
- `service`: mutex-protected in-memory state and session lifecycle orchestration.
- `api`: thin `net/http` handlers, JSON DTO handling, and HTTP error mapping.

`cmd/server` constructs the service and HTTP server. No repository interfaces, dependency injection framework, event bus, or speculative abstractions are needed.

## Domain model

A station configuration has an ID, grid capacity, and a list of chargers. A charger has an ID, maximum power, availability, and connectors. A connector has an ID, type, maximum power, and availability. IDs are unique within their resource type.

An active session has an ID, connector ID, requested power, vehicle maximum power, an optional charging-curve limit, assigned power, and timestamps. Its effective demand is the minimum of its positive applicable limits: requested, vehicle, connector, optional curve, and charger capacity.

Configuration validation rejects blank IDs, non-positive capacities, duplicate charger or connector IDs, and chargers without connectors. Session boundary validation rejects blank IDs, non-positive required power values, non-positive provided curve limits, unknown or unavailable connectors, duplicate active session IDs, and occupied connectors.

## Allocation

The allocator receives an immutable view of configuration and active sessions and returns assignments keyed by session ID. It does not mutate shared state.

Eligible sessions are sorted by stable IDs. Allocation uses deterministic progressive filling: raise all unconstrained sessions equally until station capacity, a session effective-demand cap, or a shared charger cap is reached. Freeze constrained sessions and continue redistributing remaining capacity among eligible sessions. This produces max-min fairness while respecting station, charger, connector, vehicle, request, and charging-curve limits.

Unavailable hardware receives zero allocation. Output ordering is stable and no decision relies on Go map iteration order. Floating-point comparisons use one small package-level epsilon.

## State and request flow

The service owns the single configured station and active sessions behind one mutex. A configuration or session mutation validates, applies the change, recomputes all assignments synchronously, stores the result, and only then releases the lock. Reads return copies so callers cannot mutate shared state.

This coarse lock keeps each mutation and recomputation atomic. It is appropriate for one small in-memory station and makes partially updated state impossible to observe.

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
- Allocation scenarios for one session, fair sharing, redistribution, effective limits, shared charger limits, grid limits, and deterministic insertion order.
- Service lifecycle scenarios proving start/update/stop recompute immediately and reject invalid operations.
- A compact handler test set for routing, status codes, and the main successful flow.

No tests are added for plain structs, JSON tags in isolation, getters, constructors without behavior, or exhaustive permutations of equivalent cases. Tests avoid mocks where direct calls are simpler.

## Delivery slices

Implementation proceeds in reviewable chunks:

1. Go module, domain types, validation, and focused validation tests.
2. Pure allocator and its core invariant tests.
3. Mutex-protected service and lifecycle tests.
4. HTTP API and focused handler tests.
5. Server wiring, Docker files, example script, README, and final verification.

After each chunk, format and run the relevant focused tests. Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and a clean build before completion.

## Assumptions and trade-offs

- Zero is not a useful configured capacity or active-session demand, so configured maximums and required session limits must be positive.
- Omitting the charging-curve limit means it does not constrain demand.
- Stopped sessions are removed from active state rather than retained as history.
- Process restart loses state by design.
- A single coarse lock favors clarity and correctness over unnecessary concurrency within one station.
- Optional features are reconsidered only after the complete core is working, tested, runnable, and documented.

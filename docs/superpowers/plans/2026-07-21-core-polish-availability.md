# Core Polish and Dynamic Availability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the remaining non-README core polish and add runtime charger/connector availability with atomic session termination and power redistribution.

**Architecture:** Keep the existing standard-library HTTP API and single mutex-protected service. Routing fallbacks remain transport-only, availability mutation remains service-owned, and Docker/examples only package the already-runnable server.

**Tech Stack:** Go 1.26, `net/http`, `log/slog`, Go tests/benchmarks, Docker Compose, and Python 3 standard library for runnable acceptance scenarios.

## Global Constraints

- Do not add production dependencies.
- Keep one in-memory station and one service mutex.
- Every accepted state change recomputes allocations synchronously before returning.
- Preserve deterministic allocation and stable output ordering.
- Use the existing `{code,message}` JSON error shape.
- README and BESS are outside this plan.
- Preserve the user-authored `TEST_SCENARIOS.md`; only add the latency scenario needed by `AGENTS.md`.

---

### Task 1: JSON routing errors

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/http.go`
- Test: `internal/api/station_handlers_test.go`

**Interfaces:**
- Produces: JSON `404 not_found` for unknown routes.
- Produces: JSON `405 method_not_allowed` plus `Allow` for known paths with unsupported methods.

- [ ] **Step 1: Write failing routing tests**

Add a table test that sends `GET /missing` and `POST /health`, decodes `errorResponse`, and asserts `404/not_found` and `405/method_not_allowed`. Assert `Allow: GET` for `/health`.

- [ ] **Step 2: Verify the tests fail**

Run:

```bash
go test ./internal/api -run TestRouterReturnsJSONErrors -count=1
```

Expected: failure because the standard mux writes plain-text responses.

- [ ] **Step 3: Add minimal fallback handlers**

Register path-only fallbacks after the method-specific routes:

```go
mux.HandleFunc("/health", api.methodNotAllowed(http.MethodGet))
mux.HandleFunc("/api/v1/station/config", api.methodNotAllowed(http.MethodPut))
mux.HandleFunc("/api/v1/station", api.methodNotAllowed(http.MethodGet))
mux.HandleFunc("/api/v1/sessions", api.methodNotAllowed(http.MethodPost))
mux.HandleFunc("/api/v1/sessions/{sessionId}", api.methodNotAllowed("PATCH, DELETE"))
mux.HandleFunc("/", api.notFound)
```

Implement the closures in `http.go` using `writeError` and the existing logger.

- [ ] **Step 4: Verify API tests**

```bash
go test ./internal/api -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/router.go internal/api/http.go internal/api/station_handlers_test.go
git commit -m "fix: return JSON routing errors"
```

---

### Task 2: Representative lifecycle latency benchmark

**Files:**
- Create: `internal/api/latency_test.go`
- Modify: `TEST_SCENARIOS.md`

**Interfaces:**
- Produces: `BenchmarkSessionLifecycle`, exercising configure, start, update, and stop through the real HTTP handler.

- [ ] **Step 1: Add the benchmark**

For each benchmark iteration, create a service and handler, configure the two-connector test station, then send one start, one update, and one stop request through `ServeHTTP`. Fail immediately on any unexpected status.

- [ ] **Step 2: Add the documented latency scenario**

Add a core scenario to `TEST_SCENARIOS.md` explaining that the full synchronous HTTP lifecycle must remain below one second and provide this exact command:

```bash
go test ./internal/api -run '^$' -bench BenchmarkSessionLifecycle -benchtime=100x
```

- [ ] **Step 3: Run the benchmark**

Expected: `ns/op` comfortably below `1,000,000,000 ns` for a representative lifecycle.

- [ ] **Step 4: Verify API tests**

```bash
go test ./internal/api -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/latency_test.go TEST_SCENARIOS.md
git commit -m "test: demonstrate event latency"
```

---

### Task 3: Docker packaging

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.dockerignore`

**Interfaces:**
- Produces: one API container listening on port `8080`.

- [ ] **Step 1: Add a multi-stage Dockerfile**

Use `golang:1.26-alpine` to build `./cmd/server` with `CGO_ENABLED=0`, then copy the binary into a `scratch` runtime image. Expose `8080` and run `/station-server`.

- [ ] **Step 2: Add Docker Compose configuration**

Define one `api` service built from the repository root and map host port `8080` to container port `8080`. Do not add databases, queues, or sidecars.

- [ ] **Step 3: Add a minimal `.dockerignore`**

Exclude `.git`, local editor files, and locally built binaries only.

- [ ] **Step 4: Verify packaging**

Run:

```bash
docker compose config
docker compose build
```

If Docker is unavailable, report that limitation without claiming the image was built.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile docker-compose.yml .dockerignore
git commit -m "build: add Docker packaging"
```

---

### Task 4: Runnable HTTP acceptance scenarios

**Files:**
- Create: `examples/scenarios.json`
- Create: `examples/run_scenarios.py`

**Interfaces:**
- Consumes: API at `BASE_URL`, default `http://localhost:8080`.
- Produces: reviewer-friendly configure/start/share/update/stop/query flow with explicit assertions.

- [ ] **Step 1: Add scenario data**

Create `scenarios.json` containing the brief's representative station (`400 kW` grid, two `300 kW` available chargers, and two `CCS` connectors per charger) plus the session start and update payloads used by the acceptance flow.

- [ ] **Step 2: Add the Python runner**

Use only Python 3's standard `json`, `os`, `sys`, and `urllib` modules. Keep the request sequence and assertions explicit rather than creating a generic JSON assertion language. Demonstrate and assert:

1. Station configuration.
2. First session start.
3. Second session start and fair sharing.
4. Station query.
5. Session demand update and redistribution.
6. Session stop and redistribution.
7. Final station query.

Support `BASE_URL`, defaulting to `http://localhost:8080`. Print one concise PASS line per step and exit non-zero with an actionable failure message.

- [ ] **Step 3: Exercise the runner**

Start `go run ./cmd/server`, run `python3 examples/run_scenarios.py`, and confirm all HTTP status and allocation assertions pass. Stop the temporary process afterward.

- [ ] **Step 4: Commit**

```bash
git add examples/scenarios.json examples/run_scenarios.py
git commit -m "test: add runnable API scenarios"
```

---

### Task 5: Service-layer hardware availability

**Files:**
- Modify: `internal/service/service.go`
- Create: `internal/service/availability.go`
- Create: `internal/service/availability_test.go`

**Interfaces:**
- Produces: `UpdateChargerStatus(chargerID string, status domain.OperationalStatus) (StationState, error)`.
- Produces: `UpdateConnectorStatus(connectorID string, status domain.OperationalStatus) (StationState, error)`.
- Produces: `ErrChargerNotFound`.

- [ ] **Step 1: Write failing connector tests**

Verify that making an occupied connector unavailable removes its session, reports the connector as unavailable/unoccupied, and reallocates its power to the remaining session before returning.

- [ ] **Step 2: Write failing charger tests**

Use two chargers. Verify that making one charger unavailable removes every session on its connectors and reallocates power to a session on the other charger.

- [ ] **Step 3: Write focused recovery and rejection tests**

Verify that restored hardware accepts a new session. Verify unknown IDs and invalid status values return errors without changing the prior station snapshot.

- [ ] **Step 4: Verify tests fail to compile**

```bash
go test ./internal/service -run 'TestUpdate(Charger|Connector)Status' -count=1
```

- [ ] **Step 5: Implement atomic service methods**

Under the existing mutex:

1. Require configured station state.
2. Clone the configuration.
3. Locate and update the requested hardware status.
4. Validate the cloned configuration before applying it.
5. Remove sessions attached to newly unavailable hardware.
6. Replace the stored configuration.
7. Recompute allocations with one timestamp.
8. Return `snapshotLocked()`.

Do not create session history or a second service.

- [ ] **Step 6: Verify service tests**

```bash
go test ./internal/service -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/service/service.go internal/service/availability.go internal/service/availability_test.go
git commit -m "feat: update hardware availability"
```

---

### Task 6: Hardware availability HTTP API

**Files:**
- Modify: `internal/api/router.go`
- Create: `internal/api/availability_handlers.go`
- Create: `internal/api/availability_handlers_test.go`

**Interfaces:**
- Consumes: service availability methods from Task 5.
- Produces: `PATCH /api/v1/chargers/{chargerId}`.
- Produces: `PATCH /api/v1/connectors/{connectorId}`.

- [ ] **Step 1: Write failing endpoint tests**

For both routes, send `{"status":"unavailable"}` and assert `200 OK`, updated OPS status, affected session removal, and redistributed assigned power.

- [ ] **Step 2: Write a compact error-mapping table**

Cover malformed/invalid status (`400`), unknown charger/connector (`404`), and station not configured (`404`) using the existing JSON error shape.

- [ ] **Step 3: Verify tests fail**

```bash
go test ./internal/api -run 'TestUpdate(Charger|Connector)Availability' -count=1
```

- [ ] **Step 4: Implement thin handlers**

Decode this strict DTO:

```go
type availabilityUpdateRequest struct {
    Status domain.OperationalStatus `json:"status"`
}
```

Call the service, log accepted and rejected changes, map known errors, and return the service's recomputed station state. Register path-only `405` fallbacks with `Allow: PATCH`.

- [ ] **Step 5: Verify API tests**

```bash
go test ./internal/api -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/router.go internal/api/availability_handlers.go internal/api/availability_handlers_test.go
git commit -m "feat: expose hardware availability API"
```

---

### Task 7: Acceptance coverage and full verification

**Files:**
- Modify: `internal/api/latency_test.go`
- Modify: `examples/scenarios.json`
- Modify: `examples/run_scenarios.py`
- Modify: `TEST_SCENARIOS.md`
- Modify: `docs/superpowers/plans/2026-07-21-core-polish-availability.md` (mark Task 7 complete)

**Interfaces:**
- Keeps: `BenchmarkSessionLifecycle` as the documented full synchronous HTTP-flow benchmark.
- Extends: the runnable Docker scenario through connector and charger availability events.
- Produces: explicit evidence of session termination, immediate redistribution, OPS visibility, and restored-hardware reuse.

- [x] **Step 1: Expand the lifecycle benchmark across the complete successful API surface**

Keep one explicit request table. In a fresh handler per iteration, exercise health, station configuration/query, session start/update/stop, connector unavailable/available, and charger unavailable/available. Every request must assert its expected HTTP status. Do not add error cases to the benchmark; focused handler tests already cover them.

The benchmark reports the duration of the entire request sequence, not an individual request. Keep the documented threshold comparison against `1,000,000,000 ns/op`, which is therefore stricter than the brief's per-event requirement.

- [x] **Step 2: Extend the structured example data**

Add two explicit replacement sessions on `connector-1` to `examples/scenarios.json`. They allow the runner to demonstrate that a restored connector accepts a new session, preserve stop-redistribution coverage, and seed another active session before the charger outage without reusing a stopped session ID.

- [x] **Step 3: Extend the Python runner through both availability endpoints**

After the existing demand-update flow:

1. `PATCH /api/v1/connectors/connector-1` to `unavailable`.
2. Assert the affected session is absent, `session-2` immediately receives `300 kW`, and OPS reports the connector as unavailable and unoccupied.
3. Restore the connector to `available`, assert its OPS status, and start a replacement session on it.
4. Stop that replacement session and assert `session-2` immediately returns to `300 kW`.
5. Start the second replacement session on `connector-1`.
6. `PATCH /api/v1/chargers/charger-1` to `unavailable`.
7. Assert the affected session is absent, `session-2` immediately receives `300 kW`, and OPS reports the charger as unavailable at `0 kW`.
8. Restore the charger to `available` and assert the final OPS state.

Keep the runner explicit and standard-library-only. Do not turn it into a generic scenario language or duplicate invalid-request coverage already present in Go tests.

- [x] **Step 4: Update the scenario documentation**

Update scenario 10 and its coverage-matrix row to name the allocator, service, HTTP, and Docker coverage now present. Update scenario 13 to state exactly which successful routes the benchmark exercises and that `ns/op` represents the complete flow.

- [x] **Step 5: Exercise the benchmark and packaged example**

Run:

```bash
go test ./internal/api -run '^$' -bench BenchmarkSessionLifecycle -benchtime=100x
docker compose up --build -d
python3 examples/run_scenarios.py
```

Expected: the benchmark remains comfortably below `1,000,000,000 ns/op`, and the runner prints PASS lines for lifecycle, connector outage/restoration, charger outage/restoration, redistribution, and final OPS state.

- [x] **Step 6: Run all static and automated verification**

```bash
gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
```

- [x] **Step 7: Review repository state and commit the checkpoint**

Run `git status --short` and `git diff --check`. Preserve unrelated user changes and report any untracked files accurately.

```bash
git add internal/api/latency_test.go examples/scenarios.json examples/run_scenarios.py TEST_SCENARIOS.md docs/superpowers/plans/2026-07-21-core-polish-availability.md
git commit -m "test: exercise availability scenarios"
```

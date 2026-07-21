# Testing and Scenarios

The test strategy prioritizes the load-management behavior that the assignment evaluates: physical limits, fair allocation, redistribution, deterministic lifecycle changes, and optional BESS behavior. Packaging and malformed/concurrent-operation rigor are covered proportionately rather than expanded into an infrastructure test system.

Each scenario below states why it was selected and the behavior it validates.

Direct numeric scenarios use the same values in their primary Go fixtures. Compound scenarios such as invalid operations, hardware availability, and BESS behavior intentionally map to several focused tests because no single test should prove unrelated branches.

## Test layers

- `internal/allocation` tests the pure algorithm and physical invariants without HTTP or mutable state.
- `internal/service` tests lifecycle orchestration, atomic validation, synchronous recomputation, availability, BESS dispatch, and SoC accounting.
- `internal/api` tests JSON contracts, routing, status-code mapping, and the real HTTP lifecycle benchmark.
- `examples/run_scenarios.py` and the Postman collection demonstrate a successful reviewer flow against the packaged API.

Focused Go tests are the source of correctness. The Python and Postman flows are intentionally readable demonstrations, not replacements for unit and service tests.

## Scenario catalogue

### 1. Single session receives its full effective demand

**Setup**

- Grid capacity: `400 kW`
- Charger capacity: `300 kW`
- Connector capacity: `250 kW`
- Session requested power: `220 kW`
- Vehicle maximum power: `200 kW`
- Charging-curve limit: `180 kW`

**Expected result**

The session receives `180 kW`.

**Why this scenario matters**

This validates the basic allocation path and confirms that the assigned power is capped by the most restrictive session or hardware limit.

---

### 2. Single session is capped by station grid capacity

**Setup**

- Grid capacity: `150 kW`
- Charger capacity: `300 kW`
- Connector capacity: `300 kW`
- Session effective demand: `250 kW`

**Expected result**

The session receives `150 kW`.

The grid import must never exceed `150 kW`.

**Why this scenario matters**

This validates the system’s most important safety invariant: charging demand must never exceed the station’s grid connection.

---

### 3. Two equal sessions share constrained power fairly

**Setup**

- Grid capacity: `300 kW`
- Two sessions each request `250 kW`
- Both sessions have identical physical limits

**Expected result**

Each session receives `150 kW`.

**Why this scenario matters**

This validates the selected fairness policy when total demand exceeds available station power.

---

### 4. Unused capacity is redistributed

**Setup**

- Grid capacity: `300 kW`
- Session A effective demand: `50 kW`
- Session B effective demand: `300 kW`

**Expected result**

- Session A receives `50 kW`
- Session B receives `250 kW`

**Why this scenario matters**

A naïve equal split would allocate `150 kW` to each session and waste `100 kW` because Session A cannot use its full share.

This scenario validates iterative redistribution, or water-filling behavior, so available power is not unnecessarily left unused.

---

### 5. Three sessions with different demand limits

**Setup**

- Grid capacity: `300 kW`
- Session A effective demand: `50 kW`
- Session B effective demand: `120 kW`
- Session C effective demand: `300 kW`

**Expected result**

- Session A receives `50 kW`
- Session B receives `120 kW`
- Session C receives `130 kW`

**Why this scenario matters**

This is a stronger fairness test than a simple two-session split. It validates repeated redistribution after multiple sessions reach their individual demand limits.

---

### 6. Shared charger capacity is respected

**Setup**

- Grid capacity: `500 kW`
- One charger with a maximum capacity of `300 kW`
- Two connectors on that charger
- Two sessions each request `250 kW`

**Expected result**

The combined allocation across both connectors must not exceed `300 kW`.

Under equal sharing, each session receives `150 kW`.

**Why this scenario matters**

The station may have enough grid power while an individual charger remains the bottleneck.

This validates that charger capacity is treated as a shared constraint across its connectors.

---

### 7. Spare power is redistributed across chargers

**Setup**

- Grid capacity: `400 kW`
- Charger A maximum: `100 kW`
- Charger B maximum: `300 kW`
- One high-demand session on each charger

**Expected result**

- Session on Charger A receives `100 kW`
- Session on Charger B receives `300 kW`

**Why this scenario matters**

A naïve station-level equal split would assign `200 kW` to each charger, leaving Charger A unable to consume `100 kW` of its share.

This validates that unused power caused by one charger’s bottleneck is redistributed to sessions on other chargers.

---

### 8. Session stop causes immediate redistribution

**Setup**

- Grid capacity: `300 kW`
- Two sessions each request `250 kW`
- Initial allocation: `150 kW` each

Stop one of the sessions.

**Expected result**

The remaining session immediately receives `250 kW`, assuming its charger and connector permit it.

The updated allocation must be visible in the response to the stop event.

**Why this scenario matters**

This validates synchronous recomputation and the requirement to react immediately when station conditions change.

---

### 9. Charging-curve update reduces demand and redistributes power

**Setup**

- Grid capacity: `300 kW`
- Session A and Session B initially receive `150 kW` each
- Session A’s charging-curve limit changes to `80 kW`

In a focused API case, start a `100 kW` session capped by a `20 kW` charging-curve limit, then clear that limit with JSON `null`.

**Expected result**

- Session A receives `80 kW`
- Session B receives up to `220 kW`, subject to its own limits
- The focused session's curve limit becomes absent and its effective demand and allocation return to `100 kW`

**Why this scenario matters**

Vehicle power demand changes throughout a charging session, and an optional curve limit must be removable when it no longer applies.

This validates dynamic session updates, immediate redistribution, and that explicit `null` clears an optional limit rather than being rejected as an empty update.

---

### 10. Connector or charger unavailable

**Setup**

- Two active sessions are receiving power
- The connector or charger serving Session A becomes unavailable

**Expected result**

- Session A is ended and removed from the active-session state
- Session B immediately receives any newly available station power
- OPS reports the affected connector or charger as unavailable and drawing `0 kW`
- Restored hardware can accept a new session
- All grid, charger, and connector constraints remain valid

**Why this scenario matters**

Real charging infrastructure can become unavailable because of faults, maintenance, or communication problems.

This validates operational-state handling and safe redistribution after hardware availability changes.

---

### 11. Deterministic allocation regardless of input order

**Setup**

Create equivalent station states multiple times while varying:

- Charger order in configuration
- Connector order
- Session insertion order
- Go map iteration order

**Expected result**

Every equivalent state produces the same allocation result.

**Why this scenario matters**

Deterministic behavior is important for debugging, testing, and operating critical infrastructure.

This catches accidental dependence on unordered map iteration or request ordering.

---

### 12. Invalid event does not corrupt existing state

**Setup**

Start one valid session and record the current station state.

Then submit invalid operations such as:

- Starting a session on an unknown connector
- Starting a second session on an occupied connector
- Reusing an active session ID
- Sending negative requested power
- Updating an unknown session
- Stopping an unknown session

**Expected result**

- The request is rejected with an appropriate HTTP status
- The previously valid session and allocation remain unchanged
- No partially applied mutation is visible

**Why this scenario matters**

Validation failures must be atomic.

This scenario verifies that malformed or conflicting events cannot leave the station in an inconsistent state.

---

### 13. Concurrent configuration responses remain request-consistent

**Setup**

Send `1,000` valid station-configuration requests concurrently, each with a distinct station ID.

**Expected result**

- Every response contains the station ID submitted by that request
- Configuration replacement and response construction remain race-free
- A later request may replace stored state, but it cannot change an earlier request's response

**Why this scenario matters**

A successful state-changing response should describe the mutation performed by that request, even when another mutation arrives immediately afterward.

This validates that configuration and its returned snapshot form one atomic service operation rather than two separately locked calls.

---

### 14. Synchronous HTTP lifecycle remains responsive

**Setup**

Run the representative lifecycle benchmark:

```bash
go test ./internal/api -run '^$' -bench BenchmarkSessionLifecycle -benchtime=100x
```

Each benchmark operation creates a fresh station service and exercises the complete successful HTTP flow through the real router: health, BESS station configuration/query, simulation tick, session start/update/stop, connector outage/restoration, and charger outage/restoration.

**Expected result**

The benchmark completes without an unexpectedly slow or blocking operation. Its timing is informational and will vary by machine; there is no cross-device pass/fail threshold.

**Why this scenario matters**

This is a lightweight regression check for the deliberately short in-memory reaction path. The functional guarantee comes from the design: accepted events validate, mutate, recompute, and return inline without external I/O, queues, sleeps, or background processing.

---

## Implemented Extension Scenarios

These scenarios cover the minimum-power behavior implemented after the core grid-only allocator.

### 15. Minimum-power admission and waiting behavior

**Setup**

- Grid capacity: `100 kW`
- Three sessions each declare a minimum useful power of `40 kW`

**Expected result**

Only two sessions can be admitted while maintaining their minimum useful allocation.

- The first two admitted sessions receive `50 kW` each.
- The remaining session stays active at `0 kW` with `waiting_for_power`.

The admission order should follow the documented deterministic policy.

**Why this scenario matters**

Allocating tiny amounts of unusable power to every session may be worse than temporarily pausing one session.

This validates the minimum-threshold policy chosen for this submission.

---

### 16. Waiting session is reconsidered when capacity becomes available

**Setup**

Continue from the previous minimum-power scenario.

Reduce the first admitted session's requested, vehicle-maximum, and minimum power to `20 kW`.

**Expected result**

The updated session receives `20 kW`, the second session keeps its `40 kW` minimum, and the waiting session is immediately reconsidered and begins charging at `40 kW`.

**Why this scenario matters**

This validates that admission is not a one-time decision and that waiting sessions respond correctly to station-state changes.

---

## Optional Advanced Scenarios

### 17. BESS boost, spare-grid charging, and SoC floor

**Setup**

- Grid capacity: `400 kW`
- Two EV sessions request `300 kW` each on separate capable chargers
- BESS energy capacity: `200 kWh`
- BESS maximum charge and discharge power: `200 kW`
- BESS minimum SoC: `10%`
- BESS current SoC: `50%`

First observe the BESS with zero and then one EV session. Start the second session and advance the simulation twice by `15 minutes`.

**Expected result**

- With no sessions, the BESS charges at `200 kW` and grid import is `200 kW`.
- With one `300 kW` session, EV demand is served first and BESS charging falls to `100 kW`.
- With both sessions, each receives `300 kW`, grid import remains `400 kW`, and the BESS discharges at `200 kW`.
- The first tick changes SoC from `50%` to `25%` using `power × elapsed time`.
- The second tick reaches the `10%` floor, stops discharge, and immediately reallocates EV power to `200/200 kW`.
- BESS charge, discharge, and SoC remain within their configured limits throughout.

**Why this scenario matters**

This validates the primary value of the optional BESS feature, EV-first charging priority, deterministic energy accounting, and the battery's most important safety boundary.

---

## Execution Strategy

### Go verification

Run the complete test suite:

```bash
go test ./...
```

Run the race detector across the same packages:

```bash
go test -race ./...
```

Run static analysis and confirm every package builds:

```bash
go vet ./...
go build ./...
```

The core business packages have focused coverage for their important branches. Coverage percentage is treated as supporting evidence rather than a target; scenario quality and direct invariant assertions matter more than maximizing a number.

### Lifecycle benchmark

Run the complete successful HTTP sequence 100 times:

```bash
go test ./internal/api -run '^$' -bench BenchmarkSessionLifecycle -benchtime=100x
```

One benchmark operation creates a fresh service and exercises health, BESS configuration and tick, station query, session start/update/stop, connector outage/restoration, and charger outage/restoration through the real router.

Treat the reported timing as a local regression and sanity signal, not a portable SLA. The brief's approximately one-second target communicates that events should feel real-time; this implementation addresses that by keeping recomputation synchronous, in memory, and free of blocking external dependencies.

### Docker and Python scenario

Build and start the same container a reviewer will run:

```bash
docker compose up --build -d
```

Run the packaged scenario:

```bash
python3 examples/run_scenarios.py
```

The runner uses only Python's standard library and defaults to `http://localhost:8080`. Set `BASE_URL` to target another local port:

```bash
BASE_URL=http://localhost:9090 python3 examples/run_scenarios.py
```

It configures a grid-only station first, then demonstrates session sharing, updates, outages, recovery, and stop redistribution. It reconfigures the same station with a BESS and demonstrates spare-grid charging, EV priority, battery boost, deterministic SoC change, and minimum-SoC fallback.

### Postman collection

Import [`../examples/electra-station.postman_collection.json`](../examples/electra-station.postman_collection.json) into Postman after starting the Docker API.

The collection variable `baseUrl` defaults to `http://localhost:8080`. Run the complete collection in order because later requests intentionally build on state created by earlier requests. The 23 requests cover the same successful grid-only and BESS phases as the Python runner and contain response assertions in Postman test scripts.

### Automated coverage map

| Scenarios | Primary Go coverage                                                                                                                                                                                                                                        | Runnable Docker coverage                                                                           |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| 1–2       | `TestAllocateRespectsEffectiveDemandLimits`                                                                                                                                                                                                                | Basic first-session allocation only                                                                |
| 3–4       | `TestAllocateSharesAndRedistributesGridPower`                                                                                                                                                                                                              | Fair sharing and update redistribution                                                             |
| 5         | `TestAllocateRedistributesAcrossThreeDemandLevels`                                                                                                                                                                                                         | Covered by focused Go test                                                                         |
| 6         | `TestAllocateRespectsSharedChargerLimit`                                                                                                                                                                                                                   | Covered by focused Go test                                                                         |
| 7         | `TestAllocateRedistributesPastFullCharger`                                                                                                                                                                                                                 | Covered by focused Go test                                                                         |
| 8–9       | `TestServiceStopSessionRecomputesBeforeReturning`, `TestServiceChargingCurveUpdateRecomputesBeforeReturning`, `TestUpdateSessionClearsChargingCurveLimit`                                                                                                   | Update and stop redistribution                                                                     |
| 10        | `TestAllocateReturnsZeroForUnavailableHardware`, `TestUpdateConnectorStatusEndsSessionAndRedistributesPower`, `TestUpdateChargerStatusEndsAttachedSessionsAndRedistributesPower`, `TestUpdateConnectorAvailability`, `TestUpdateChargerAvailability`       | Connector and charger outage/restoration, session termination, redistribution, and OPS visibility  |
| 11        | `TestAllocateProducesStableOutput`, `TestSessionsForSnapshotUsesStableSummationOrder`                                                                                                                                                                      | Covered by focused Go/service tests                                                                |
| 12        | `TestServiceStartSessionRejectsInvalidOperations`, `TestServiceUpdateSessionRejectsInvalidOperationsAtomically`, `TestStartSessionMapsLifecycleErrors`                                                                                                     | Docker runner is intentionally limited to the successful lifecycle                                 |
| 13        | `TestConcurrentConfigureReturnsOwnState`                                                                                                                                                                                                                   | Covered by focused HTTP test                                                                        |
| 14        | `BenchmarkSessionLifecycle`                                                                                                                                                                                                                                | The packaged runner confirms the same successful mutation paths through real HTTP                  |
| 15–16     | `TestAllocateWaitsWhenMinimumCannotBeReserved`, `TestServiceUpdateSessionReconsidersWaitingSessions`                                                                                                                                                       | Covered by focused Go/service tests                                                                |
| 17        | `TestBESSDischargeBoostsStationSupply`, `TestBESSAtMinimumSocDoesNotDischarge`, `TestBESSChargesOnlyFromPowerLeftAfterEVs`, `TestAdvanceSimulationUpdatesBESSSocFromPowerFlow`, `TestAdvanceSimulationClampsBESSSocAndRecomputes`, `TestAdvanceSimulation` | Spare-grid charging, EV priority, battery boost, deterministic SoC ticks, and minimum-SoC fallback |

The core allocation scenarios are implemented as fast unit tests around the pure allocation engine.

Lifecycle, validation, availability, and response semantics are covered through service-layer or HTTP tests.

The runnable Docker scenario demonstrates:

1. Configuring the station
2. Starting one session
3. Starting a second session
4. Observing constrained fair sharing
5. Updating one session’s demand
6. Observing redistribution
7. Stopping one session
8. Querying the final station state

The runner continues through availability recovery and the complete BESS flow after this core sequence. Invalid operations remain focused Go/API tests so the reviewer demonstration stays concise and readable.

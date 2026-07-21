## Test Scenarios

The following scenarios validate the core allocation behavior, physical constraints, event handling, determinism, and failure cases of the Station Energy Management System.

Each scenario includes why it was selected and the behavior it is expected to validate.

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

**Expected result**

- Session A receives `80 kW`
- Session B receives up to `220 kW`, subject to its own limits

**Why this scenario matters**

Vehicle power demand changes throughout a charging session.

This validates dynamic session updates and confirms that newly available power is immediately redistributed rather than remaining unused.

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

### 13. Synchronous HTTP lifecycle remains below one second

**Setup**

Run the representative lifecycle benchmark:

```bash
go test ./internal/api -run '^$' -bench BenchmarkSessionLifecycle -benchtime=100x
```

Each benchmark operation creates a fresh station service and exercises the complete successful HTTP flow through the real router: health, station configuration/query, session start/update/stop, connector outage/restoration, and charger outage/restoration.

**Expected result**

The reported `ns/op` for the entire request sequence remains below `1,000,000,000 ns` (one second). This is a stricter demonstration than measuring the brief's limit independently for each accepted event.

**Why this scenario matters**

This demonstrates that accepted HTTP state changes and their synchronous allocation recomputations comfortably satisfy the reaction-time requirement without relying on sleeps or wall-clock-sensitive tests.

---

## Implemented Extension Scenarios

These scenarios cover the minimum-power behavior implemented after the core grid-only allocator.

### 14. Minimum-power admission and waiting behavior

**Setup**

- Grid capacity: `100 kW`
- Three sessions each declare a minimum useful power of `40 kW`

**Expected result**

Only two sessions can be admitted while maintaining their minimum useful allocation.

The remaining session stays active but receives `0 kW` and enters `waiting_for_power`.

The admission order should follow the documented deterministic policy.

**Why this scenario matters**

Allocating tiny amounts of unusable power to every session may be worse than temporarily pausing one session.

This validates the minimum-threshold policy chosen for this submission.

---

### 15. Waiting session is reconsidered when capacity becomes available

**Setup**

Continue from the previous minimum-power scenario.

Stop one admitted session or reduce its requested power.

**Expected result**

The waiting session is immediately reconsidered and begins charging when enough capacity becomes available.

**Why this scenario matters**

This validates that admission is not a one-time decision and that waiting sessions respond correctly to station-state changes.

---

## Optional Advanced Scenarios

### 16. BESS boost and SoC floor

**Setup**

- Grid capacity: `200 kW`
- EV demand: `300 kW`
- BESS maximum discharge power: `150 kW`
- BESS minimum SoC: `10%`
- BESS current SoC is above the minimum

**Expected result**

- The BESS contributes up to `100 kW`
- EV allocation reaches `300 kW`
- Grid import remains at or below `200 kW`
- BESS discharge does not exceed its configured maximum
- BESS does not discharge below the minimum SoC

Repeat the scenario when the BESS is already at its minimum SoC.

In that case, the BESS must not discharge and EV allocation remains constrained by grid capacity.

**Why this scenario matters**

This validates the primary value of the optional BESS feature while also enforcing its most important safety boundary.

---

## Execution Strategy

### Automated coverage map

| Scenarios | Primary Go coverage                                                                                                                                                                                                                                  | Runnable Docker coverage                                                                          |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| 1–2       | `TestAllocateRespectsEffectiveDemandLimits`                                                                                                                                                                                                          | Basic first-session allocation only                                                               |
| 3–4       | `TestAllocateSharesAndRedistributesGridPower`                                                                                                                                                                                                        | Fair sharing and update redistribution                                                            |
| 5         | `TestAllocateRedistributesAcrossThreeDemandLevels`                                                                                                                                                                                                   | Covered by focused Go test                                                                        |
| 6         | `TestAllocateRespectsSharedChargerLimit`                                                                                                                                                                                                             | Covered by focused Go test                                                                        |
| 7         | `TestAllocateRedistributesPastFullCharger`                                                                                                                                                                                                           | Covered by focused Go test                                                                        |
| 8–9       | `TestServiceStopSessionRecomputesBeforeReturning`, `TestServiceChargingCurveUpdateRecomputesBeforeReturning`                                                                                                                                         | Update and stop redistribution                                                                    |
| 10        | `TestAllocateReturnsZeroForUnavailableHardware`, `TestUpdateConnectorStatusEndsSessionAndRedistributesPower`, `TestUpdateChargerStatusEndsAttachedSessionsAndRedistributesPower`, `TestUpdateConnectorAvailability`, `TestUpdateChargerAvailability` | Connector and charger outage/restoration, session termination, redistribution, and OPS visibility |
| 11        | `TestAllocateProducesStableOutput`                                                                                                                                                                                                                   | Covered by focused Go test                                                                        |
| 12        | `TestServiceStartSessionRejectsInvalidOperations`, `TestServiceUpdateSessionRejectsInvalidOperationsAtomically`, `TestStartSessionMapsLifecycleErrors`                                                                                               | Docker runner is intentionally limited to the successful lifecycle                                |
| 13        | `BenchmarkSessionLifecycle`                                                                                                                                                                                                                          | The packaged runner confirms the same successful mutation paths through real HTTP                 |
| 14–15     | `TestAllocateWaitsWhenMinimumCannotBeReserved`, `TestServiceUpdateSessionReconsidersWaitingSessions`                                                                                                                                                 | Covered by focused Go/service tests                                                               |
| 16        | Added only if BESS is implemented                                                                                                                                                                                                                    | Added only if BESS is implemented                                                                 |

The core allocation scenarios should primarily be implemented as fast, table-driven unit tests around the pure allocation engine.

Lifecycle, validation, availability, and response semantics should be covered through service-layer or HTTP integration tests.

At least one runnable end-to-end scenario should demonstrate:

1. Configuring the station
2. Starting one session
3. Starting a second session
4. Observing constrained fair sharing
5. Updating one session’s demand
6. Observing redistribution
7. Stopping one session
8. Querying the final station state

The runnable scenario can use `curl`, a shell script, or structured sample inputs.

Every test description should state the behavior, edge case, or invariant it validates.

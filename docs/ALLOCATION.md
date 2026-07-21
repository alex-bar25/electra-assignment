# Allocation and Power Policy

## Goals

The allocator answers one station-level question: given the current sessions, hardware configuration, and available station supply, how much power should each session receive now?

The policy is designed to be:

- Safe under every configured physical limit
- Fair when supply is constrained
- Work-conserving when a session cannot use its share
- Deterministic for equivalent inputs
- Compatible with a useful minimum charging threshold
- Small enough to explain and test directly

The allocator is a pure function. It does not mutate station state, acquire locks, read the clock, or dispatch the BESS.

## Units and numeric tolerance

- Power is represented in kilowatts (`kW`).
- BESS energy is represented in kilowatt-hours (`kWh`).
- BESS State of Charge is represented as a percentage.
- Floating-point values are accepted for this simulation.

Allocation comparisons use a small internal epsilon of `1e-9` to avoid treating insignificant floating-point differences as remaining demand or capacity. Tests use a looser `1e-6` tolerance where arithmetic may be fractional.

## Effective demand

A session cannot consume more power than its tightest individual limit:

```text
effective demand = min(
    requested power,
    vehicle maximum power,
    charging-curve limit when present,
    connector maximum power
)
```

The charger maximum is handled separately because it is shared across every connector on that charger. Station supply is also shared across all eligible sessions.

Requested power, vehicle maximum, optional charging-curve limit, connector maximum, charger maximum, and grid/BESS supply must all be positive finite values where applicable. Invalid lifecycle input is rejected before stored state changes.

## Available station supply

In grid-only operation:

```text
available station supply = grid capacity
```

When a BESS is configured above its minimum SoC:

```text
potential station supply = grid capacity + maximum BESS discharge power
```

The allocator receives that explicit supply limit; it does not need to know its source. After EV allocation, the service limits actual BESS discharge to the EV load above grid capacity. The battery therefore never discharges merely because it could.

At the BESS minimum SoC, permitted discharge is zero and allocation falls back to grid-only supply.

BESS charging is not reserved before allocation. EV sessions are allocated first, then genuinely spare grid capacity may charge the battery.

## Eligibility

A session is eligible when:

- Its connector exists.
- Its charger and connector are available.
- Its effective demand is greater than the allocation epsilon.

Ineligible or unavailable sessions receive zero from the pure allocator. In the live service, an availability event also ends and removes sessions attached to the unavailable hardware before recomputation.

Only one active or waiting session may occupy a connector; the service enforces that lifecycle invariant before allocation.

## Deterministic minimum-power admission

An omitted or zero session minimum is normalized to `5 kW`. This is a candidate-owned assumption: allocations below a few kilowatts may be useless or rejected by an EV, so pausing one session can be more useful than giving every session a trickle.

A session whose declared minimum exceeds its effective demand or charger maximum is rejected as invalid.

When the station cannot meet every useful minimum, admission is deterministic:

1. Sort eligible sessions by start time.
2. Break equal-start-time ties by session ID.
3. Admit a session to charging only if its minimum fits both remaining station supply and remaining charger capacity.
4. Reserve the admitted session's minimum.
5. Keep a session that is not admitted to charging active with `waiting_for_power` and exactly `0 kW`.

Session creation itself is still accepted: admission here means participation in the current power allocation, not whether the session exists. This keeps the waiting session visible and lets every accepted state change reconsider it automatically.

This ordering is used only for minimum admission. It is not first-come-first-served allocation after sessions are admitted.

Every accepted state change reruns admission, so a waiting session is reconsidered after a start, update, stop, availability event, configuration replacement, or BESS tick.

## Max-min fair distribution

After useful minimums are reserved, the allocator distributes remaining supply through iterative level raising:

```text
1. Build eligible session state and effective demand.
2. Sort stable identifiers.
3. Reserve useful minimums by start time then session ID.
4. Mark sessions that cannot receive their minimum as waiting at 0 kW.
5. Find the lowest currently assigned active sessions.
6. Raise that lowest group together.
7. Stop a session or charger group at its demand or physical cap.
8. Redistribute remaining power until no supply or eligible demand remains.
```

For each iteration, the common increase is the minimum of:

- Remaining station supply divided across the lowest active group
- The increase needed to reach the next existing allocation level
- Remaining capacity on each affected charger divided across its lowest active sessions
- Each affected session's remaining effective demand

Sessions at the same lowest level receive the same increase. A session becomes inactive for further raising when it reaches effective demand or its charger has no remaining capacity.

Starting with different useful minimums does not permanently preserve unequal allocations. Lower admitted sessions are raised first until they catch higher ones, after which they rise together where constraints allow.

## Charger-level constraints and redistribution

Each charger starts with its configured remaining capacity. Every reservation and fair-share increase reduces both station supply and that charger's remaining power.

If one charger reaches its maximum, sessions on other chargers can continue receiving station supply. Capacity is not split into fixed per-charger buckets, so a low-demand or low-capacity charger cannot strand power that another charger can use.

This matters when the station grid is not the only bottleneck. For example, a `400 kW` grid can still contain a `100 kW` charger and a `300 kW` charger. One high-demand session on each should receive `100/300 kW`, not a fixed `200/200 kW` charger split that wastes `100 kW`.

## BESS interaction

The allocator treats BESS discharge as additional potential station supply only while SoC is above the configured minimum.

The service then derives actual dispatch:

```text
EV load > grid capacity:
    BESS discharge = min(EV load - grid capacity, maximum discharge power)

EV load <= grid capacity and SoC < 100%:
    BESS charge = min(grid capacity - EV load, maximum charge power)
```

The second branch uses negative signed BESS power. Charging is calculated after EV allocation, so it cannot reduce an EV assignment.

When an explicit simulation tick reaches minimum SoC, the service removes BESS discharge from available supply and reruns allocation before returning. Sessions may receive a smaller fair share or enter `waiting_for_power`, depending on their minimums.

## Hard invariants

After every accepted state-changing request:

1. Grid import is between zero and configured grid capacity.
2. A connector's allocation never exceeds connector capacity.
3. A session never exceeds its effective demand.
4. Allocations on one charger never exceed charger capacity.
5. A connector has at most one active or waiting session.
6. Unavailable hardware receives zero allocation.
7. Allocation is recomputed before the response returns.
8. Equivalent inputs produce equivalent output ordering and assignments.
9. An admitted charging session receives at least its useful minimum.
10. A waiting session receives exactly `0 kW`.
11. BESS charge and discharge remain within configured power limits.
12. BESS SoC remains between its configured minimum and `100%`.
13. EV allocations are never reduced merely to charge the BESS.
14. BESS discharge never exceeds EV load above grid capacity.

The service, rather than the pure allocator alone, enforces the grid-import and BESS invariants because it owns signed battery dispatch.

## Worked examples

### Equal constrained demand

- Grid: `300 kW`
- Two sessions: `250 kW` effective demand each
- No lower hardware bottleneck

Both sessions are at the same level, so the result is `150/150 kW`.

### Low-demand redistribution

- Grid: `300 kW`
- Session A effective demand: `50 kW`
- Session B effective demand: `300 kW`

An initial equal level would reach Session A's cap. Its unused share is redistributed, producing `50/250 kW` rather than wasting power.

### Useful minimum admission

- Grid: `100 kW`
- Three sessions with a `40 kW` useful minimum
- Equal demand and no charger bottleneck

Only two minimum reservations fit. The first two sessions by start time/session ID are admitted and rise to `50/50 kW`; the third remains `waiting_for_power` at `0 kW`. It is reconsidered on the next accepted state change.

### Redistribution across chargers

- Grid: `400 kW`
- Charger A maximum: `100 kW`
- Charger B maximum: `300 kW`
- One high-demand session on each

Charger A stops at its physical cap and the remaining supply continues to Charger B, producing `100/300 kW`.

### BESS boost

- Grid: `400 kW`
- BESS maximum discharge: `200 kW`
- BESS SoC above its minimum
- Two sessions with `300 kW` effective demand on separate capable chargers

Potential station supply is `600 kW`, producing `300/300 kW`. Grid import remains `400 kW`, and BESS discharge is `200 kW`.

At minimum SoC, supply returns to `400 kW` and equal sharing becomes `200/200 kW`.

## Determinism

The implementation never uses Go map iteration as a tie-breaker:

- Assignment output is sorted by session ID.
- Minimum admission is sorted by start time then session ID.
- OPS sessions, chargers, and connectors are sorted by ID.
- Aggregate power is summed in stable session order.

Equivalent configuration and session state therefore produce equivalent assignments regardless of insertion order.

## Complexity and trade-offs

Preparing lookup state and sorting sessions costs `O(n log n)` for `n` sessions. Fair distribution repeatedly scans the active-session set as sessions reach demand levels or charger limits. For the small number of connectors at one station, this direct approach is comfortably within the reaction target and is easier to audit than a more optimized data structure.

The optional lifecycle benchmark provides a local regression signal, but no cross-device timing claim is necessary. The allocator operates entirely in memory over a station-sized input, so a more optimized data structure would be premature.

The policy deliberately does not include customer tiers, reservations, fleet ownership, tariff optimization, or vehicle-SoC priority. Those policies could be layered onto admission in a different product, but adding them here would make fairness less transparent and invent requirements not present in the assignment.

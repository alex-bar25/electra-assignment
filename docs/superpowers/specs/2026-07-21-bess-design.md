# Minimal BESS Design

## Goal

Add an optional on-site battery that supplements grid power for EV charging, charges only from grid power left after EV demand is served, tracks State of Charge through an explicit deterministic tick, and appears in OPS state.

The implementation remains one in-memory service with one mutex. It does not model battery chemistry, efficiency loss, autonomous time, tariffs, temperature, degradation, or hardware communication.

## Approaches considered

### Recommended: service-owned dispatch with a supply-aware allocator

The service derives the power currently available from the grid and BESS, passes that total supply to the pure allocator, then calculates actual BESS and grid power from the resulting EV load.

This keeps SoC and runtime battery state in the service while leaving fair sharing in the allocator.

### Rejected: put BESS state inside the allocator

This would couple the pure allocation algorithm to elapsed time and mutable SoC. It would make the critical allocation code harder to explain and test.

### Rejected: background battery simulation

A timer or goroutine would introduce nondeterministic tests, shutdown concerns, and unnecessary concurrency. An explicit tick is sufficient for the assignment.

## Domain model

`StationConfig` accepts an optional `bess` object:

```json
{
  "energyCapacityKwh": 200,
  "socPercent": 50,
  "maxChargePowerKw": 200,
  "maxDischargePowerKw": 200,
  "minSocPercent": 10
}
```

When BESS is present:

- Energy capacity, maximum charge power, and maximum discharge power must be positive and finite.
- Minimum SoC must be greater than `0%` and less than `100%`.
- Initial SoC must be between the configured minimum and `100%`, inclusive.

Runtime OPS state exposes the same limits plus:

- `currentPowerKw`
- `mode`: `idle`, `charging`, or `discharging`

Power sign convention:

- Positive BESS power means discharging into the station.
- Negative BESS power means charging from the grid.
- Zero means idle.

## Power dispatch

Each synchronous recomputation follows this order:

1. Determine permitted discharge capacity. It is zero at the minimum SoC and otherwise equals the configured maximum discharge power.
2. Allocate EV power fairly using grid capacity plus permitted BESS discharge as station supply.
3. Sum assigned EV power.
4. If EV power exceeds grid capacity, discharge only the shortfall, capped by maximum discharge power.
5. Otherwise, if SoC is below `100%`, charge from spare grid capacity, capped by maximum charge power.
6. Never reduce an EV allocation merely to charge the BESS.

The resulting grid import is:

```text
grid import = EV assigned power - BESS current power
```

Because charging power is negative, subtracting it adds the battery load to grid import.

Available grid power remains grid capacity minus current grid import. Available station power describes additional power that could be offered to EVs after stopping any current BESS charging and using permitted discharge.

## SoC tick

Add:

```http
POST /api/v1/simulation/tick
Content-Type: application/json

{"elapsedSeconds": 900}
```

The elapsed duration must be positive and finite. Under the service mutex:

```text
energy delta kWh = current BESS power kW × elapsed hours
new stored energy = current stored energy - energy delta
```

SoC is clamped to the configured minimum and `100%`, then allocations and battery dispatch are recomputed before returning the updated station state.

The tick assumes current power stayed constant during the supplied interval. Clamping is a deliberate discrete-simulation simplification; the model does not simulate the exact instant a boundary was reached.

Ticking a station without BESS returns `404 bess_not_configured`. Invalid durations return `400 invalid_tick`.

## Atomicity and lifecycle

- Configuration initializes runtime BESS state from the configured SoC.
- Session and hardware events continue to recompute before returning.
- A tick updates SoC and recomputes under the same mutex.
- Reconfiguring the station resets sessions and BESS runtime state.
- Process restart loses all state, as with the existing grid-only system.

## Focused verification

Tests will demonstrate:

1. BESS configuration validation.
2. Discharge boosts EV supply without exceeding grid, power, or SoC constraints.
3. A BESS at minimum SoC does not discharge.
4. Spare grid power charges the BESS without reducing EV allocations.
5. Explicit ticks update and clamp SoC deterministically.
6. OPS state reports grid import, BESS power, mode, and SoC consistently.
7. The HTTP tick maps success and validation errors correctly.
8. The Docker and Postman flows demonstrate boost, charging, and SoC movement.

The existing grid-only behavior remains unchanged when `bess` is omitted.

# Dynamic Hardware Availability Design

## Goal

Allow Electra OPS to change charger or connector availability at runtime while preserving station safety and immediately redistributing power.

## API

```http
PATCH /api/v1/chargers/{chargerId}
PATCH /api/v1/connectors/{connectorId}
```

Both endpoints accept one strict JSON object:

```json
{"status":"unavailable"}
```

`status` accepts only `available` or `unavailable`.

Successful requests return `200 OK` with the recomputed station state. Invalid JSON or status values return `400 Bad Request`. An unknown station or hardware ID returns `404 Not Found` using the existing JSON error shape.

## Service Behaviour

The existing `Service` remains the only mutable-state owner. Each availability update runs under its mutex so configuration mutation, affected-session removal, allocation recomputation, and snapshot construction are atomic.

When a charger becomes unavailable:

- Update its operational status.
- Remove every active session attached to one of its connectors.
- Recompute allocations for all remaining sessions before returning.

When a connector becomes unavailable:

- Update its operational status.
- Remove the active session attached to that connector, if any.
- Recompute allocations for all remaining sessions before returning.

When hardware becomes available again, no session is created automatically. The hardware simply becomes eligible for future session starts.

Stopped sessions are removed rather than retained with a failure status, matching the existing stop lifecycle and avoiding a new history subsystem.

## OPS State

The existing station response already contains charger and connector status, connector occupancy, active session ID, assigned power, and charger current power. Returning the recomputed station state therefore exposes both the availability change and its power impact without a new response model.

## Error Handling

- Station not configured: `404 station_not_configured`
- Unknown charger: `404 charger_not_found`
- Unknown connector: `404 connector_not_found`
- Invalid or empty request: `400 invalid_request`
- Invalid status: `400 invalid_hardware_status`

Rejected requests do not mutate configuration, sessions, allocations, or timestamps.

## Testing

Focused service tests will verify:

- Unavailable connector removes its session and redistributes power.
- Unavailable charger removes every attached session and redistributes power to another charger.
- Restored hardware accepts a new session.
- Invalid and unknown updates leave state unchanged.

Focused HTTP tests will verify each route, successful OPS response state, and error mapping. Existing allocation tests continue to prove that unavailable hardware receives zero allocation.

## Scope Boundaries

This feature does not add fault categories, retry logic, session history, maintenance workflows, hardware telemetry, or BESS behaviour.

# Strong Acceptance Scenarios Design

## Goal

Make the submission's validation strategy immediately convincing to Electra without duplicating the allocation test suite in Python or building a custom test framework.

## Coverage Strategy

Use three complementary layers:

1. Go allocation and service tests prove precise invariants, error handling, and deterministic behavior.
2. A standard-library Python runner proves the externally visible HTTP behavior against the Docker image.
3. `TEST_SCENARIOS.md` explains why each scenario matters and maps every documented scenario to its automated coverage.

The Docker runner is not a replacement for focused Go tests. It demonstrates that the compiled container preserves the same behavior through routing, JSON decoding, service mutation, allocation recomputation, and response encoding.

## Docker Scenario Set

The runner will execute explicit functions for these cases:

1. **Lifecycle and redistribution** — configure a representative station, start two sessions, observe fair sharing, update demand, restore demand, stop one session, and confirm immediate redistribution.
2. **Effective and shared limits** — prove vehicle, charging-curve, connector, charger, and grid constraints through externally visible allocations.
3. **Cross-charger redistribution** — reproduce the important edge case where a full low-capacity charger must not prevent spare power reaching another charger.
4. **Minimum-power admission** — show a session waiting at `0 kW` when its minimum cannot be met and charging immediately after capacity is freed.
5. **Invalid-operation atomicity** — reject a conflicting or invalid event and confirm the previously valid OPS state is unchanged.
6. **Determinism** — submit equivalent station and session inputs in different stable orders and confirm equivalent allocations.
7. **Container latency** — run repeated configure, start, update, and stop events against the Docker API and fail if any accepted event takes one second or more.
8. **Hardware unavailability** — add this case after the availability endpoints exist; confirm affected sessions end, remaining power is redistributed, and OPS sees unavailable hardware.

## Files and Structure

- `examples/scenarios.json` will hold named input data and expected values for the explicit cases.
- `examples/run_scenarios.py` will contain one readable function per scenario plus small shared HTTP and assertion helpers.
- `TEST_SCENARIOS.md` will gain a compact coverage matrix referencing the relevant Go tests and Docker scenario names.

The runner will continue using only Python's standard library. It will not interpret a generic assertion language, dynamically discover tests, or introduce third-party dependencies.

## Execution

The reviewer-facing flow remains:

```bash
docker compose up --build -d
python3 examples/run_scenarios.py
```

Each scenario resets station state through the configuration endpoint, prints a concise `PASS` line, and exits non-zero with an actionable error on failure.

Source-level verification still includes `go test ./...` and `go test -race ./...`. HTTP acceptance checks always target the rebuilt Docker image.

## Error Handling and Measurement

- Unexpected HTTP status codes include the method, path, expected status, actual status, and response body.
- Missing or malformed scenario data fails with a concise message.
- Floating-point power comparisons use a small tolerance.
- Latency uses a monotonic clock and checks individual accepted HTTP events against the brief's one-second limit.
- Latency output reports the number of events, average duration, and maximum duration for reviewer context.

## Scope Guardrails

- Do not duplicate every allocator table case in Python.
- Do not add mocks, external services, test libraries, or a generic scenario DSL.
- Keep BESS cases out until BESS exists.
- Add availability coverage only after its service and HTTP behavior are implemented.
- Prefer strong representative edge cases over a large scenario count.

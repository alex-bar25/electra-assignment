# Strong Test Scenarios Design

## Goal

Make the submission's validation strategy convincing to Electra by prioritizing allocation behavior under real physical constraints, while keeping the runnable Docker example small and reviewer-friendly.

## Coverage Strategy

Use three complementary artifacts:

1. Focused Go allocation and service tests prove physical invariants, fairness, deterministic behavior, and lifecycle recomputation.
2. The existing standard-library Python runner demonstrates one complete HTTP lifecycle against the compiled Docker image.
3. `TEST_SCENARIOS.md` explains why each scenario matters and maps every documented scenario to its automated coverage.

The Go tests are the primary evidence because grid, charger, connector, vehicle, and charging-curve interactions are the heart of the assignment. The Docker runner remains a packaging and external-interface smoke test rather than a duplicate test framework.

## Core Physical Coverage

The test suite must visibly cover:

1. A single session capped independently by requested power, vehicle maximum, charging curve, connector capacity, charger capacity, and grid capacity.
2. Equal sharing when station power is constrained.
3. Redistribution when one session cannot consume its equal share.
4. Three demand levels producing `50/120/130 kW` under a `300 kW` grid limit.
5. Multiple connectors sharing one charger limit.
6. Redistribution across chargers when one charger reaches its own lower limit.
7. Stable allocations for equivalent input in different orders.
8. Minimum-power waiting and immediate reconsideration after capacity changes.

Most of this coverage already exists. The only new Go behavior test needed before availability work is the explicit three-demand-level case.

## Runnable Docker Example

Keep the current lifecycle runner unchanged in scope. It demonstrates:

- health and station configuration;
- one session receiving its demand;
- constrained fair sharing after a second session starts;
- synchronous demand-update redistribution;
- synchronous stop redistribution; and
- the final OPS station state.

The reviewer-facing commands remain:

```bash
docker compose up --build -d
python3 examples/run_scenarios.py
```

Do not add physical-limit tables, determinism loops, concurrency cases, or latency loops to the Python runner. Those are clearer and more reliable as focused Go tests.

## Latency

Keep `BenchmarkSessionLifecycle` as the evidence for the brief's `<1 second` event requirement. It exercises configure, start, update, and stop through the real Go HTTP handler and reports `ns/op`.

Document its exact command and map it to the latency scenario:

```bash
go test ./internal/api -run '^$' -bench BenchmarkSessionLifecycle -benchtime=100x
```

Docker image construction and the lifecycle smoke test still run at review checkpoints, but deployment benchmarking is not part of the submission's core test story.

## Documentation Coverage Map

Add a compact table to `TEST_SCENARIOS.md` mapping each scenario to its primary Go test and, where relevant, the lifecycle runner. Preserve every existing “Why this scenario matters” explanation.

The map must be honest about incomplete optional work:

- Dynamic availability is mapped after its service and API endpoints are implemented.
- BESS remains explicitly uncovered until BESS is implemented.
- Minimum-power scenarios are labeled as implemented extensions rather than optional future work.

## Scope Guardrails

- Add only the missing three-demand allocation test in this slice.
- Do not duplicate existing allocator cases.
- Do not expand atomicity or concurrency coverage; existing rejection and race-detector checks are sufficient.
- Do not add dependencies, mocks, a scenario DSL, or deployment test infrastructure.
- Add availability tests alongside the availability behavior in the next feature slice.
- Add BESS tests only if BESS is implemented.

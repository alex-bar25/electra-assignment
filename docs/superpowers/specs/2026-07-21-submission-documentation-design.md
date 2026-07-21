# Submission Documentation Design

## Goal

Replace internal planning material with concise reviewer-facing documentation that makes the implementation easy to understand, run, test, and discuss in a technical interview.

The root README must be self-sufficient. Documents under `docs/` provide optional depth without duplicating the README.

## Documentation Structure

### `README.md`

The README is the primary entry point and should be readable in roughly five minutes. It will contain:

1. System purpose and implemented scope
2. Docker quick start and local Go commands
3. Compact HTTP API and error-shape reference
4. High-level Mermaid architecture diagram
5. Package responsibilities and synchronous mutation flow
6. Allocation, minimum-power, availability, and BESS summaries
7. Concurrency and in-memory state model
8. Test, Docker scenario, Postman, and benchmark commands
9. Assumptions and trade-offs
10. Security and explicit out-of-scope items
11. Known limitations and production follow-ups
12. Links to the detailed documents

A reviewer must not need the detailed documents to build, run, test, or understand the main design.

### `docs/ARCHITECTURE.md`

The architecture document explains:

- Why the solution is one Go process serving one station
- Why it uses the standard library HTTP server
- The domain, allocation, service, API, and entry-point boundaries
- Mutex ownership and atomic mutation/recomputation
- Configuration and lifecycle request flows
- Availability-event behavior
- BESS dispatch and explicit simulation ticks
- Error handling and operational state
- Determinism and in-memory storage trade-offs
- Security assumptions and realistic production evolution

It will include small Mermaid diagrams for package boundaries and state-changing request flow.

### `docs/ALLOCATION.md`

The allocation document explains:

- Effective session demand
- Station, charger, connector, vehicle, and charging-curve constraints
- Deterministic max-min water filling
- Redistribution within and across chargers
- Stable ordering
- The default `5 kW` useful minimum
- Start-time/session-ID minimum-power admission
- `waiting_for_power` and reconsideration
- BESS contribution to available station supply
- Hard invariants, worked examples, and algorithmic complexity

### `docs/TESTING.md`

The current `TEST_SCENARIOS.md` content will move here and be refined to include:

- The purpose and expected result of every scenario
- Mapping from scenarios to Go tests and runnable Docker coverage
- Physical-limit, lifecycle, availability, minimum-power, and BESS coverage
- Commands for unit tests, race detection, vet, build, benchmark, Docker runner, and Postman
- The benchmark methodology and a representative result

The old root `TEST_SCENARIOS.md` will not remain as a duplicate.

## Decisions to Record

The final documentation will capture these candidate-owned decisions:

- Complete the core allocation/lifecycle/API/Docker path before optional features.
- Use one in-memory station protected by one mutex.
- Keep the allocator pure and HTTP-independent.
- Recompute synchronously before returning every accepted mutation.
- Use equal deterministic max-min fairness without customer, booking, vehicle-SoC, or session-age priority.
- Use start time and session ID only to admit useful minimums deterministically.
- Default an omitted minimum useful power to `5 kW`; waiting sessions receive `0 kW`.
- End sessions affected by unavailable hardware and redistribute immediately.
- Avoid session-history and failure-tracking subsystems.
- Give EV demand priority over BESS charging.
- Treat positive BESS power as discharge and negative power as charging.
- Assume 100% BESS efficiency and allow SoC to reach, but not cross, its configured minimum.
- Advance BESS energy through explicit deterministic ticks instead of a goroutine.
- Accept coarse tick boundary clamping as a documented simulation simplification.
- Prioritize physical allocation tests over infrastructure and concurrency edge-case volume.
- Use focused Go tests as the source of correctness, with Python and Postman as reviewer demonstrations.
- Validate the packaged system through a freshly built Docker image.

## Trade-offs and Limitations

The documentation will state that:

- Process restart loses station state; persistence or event replay is a production follow-up.
- One mutex favors simplicity and consistency over parallel mutation throughput.
- The allocation algorithm favors explainability over optimization for very large stations.
- An explicit tick is deterministic but is not a real-time battery controller.
- Large ticks clamp SoC at a boundary and recompute afterward rather than simulating the exact boundary-crossing instant.
- Unavailable sessions are removed rather than retained as historical failures.
- Station configuration supports one station instance, not orchestration across stations.

## Security and Out of Scope

The README and architecture document will explicitly state that authentication, authorization, TLS/mTLS, request rate limiting, audit logging, and API network isolation are required for real critical infrastructure but intentionally omitted because the exercise evaluates load-management logic and product decisions.

Other explicit non-goals include external persistence, Kafka, Redis, Kubernetes, OCPP, a frontend, multi-station orchestration, distributed locking, hosted observability, tariff modeling, and detailed battery chemistry or degradation.

## Cleanup

After the reviewer-facing documents are complete and verified:

- Remove `docs/superpowers/` entirely.
- Ensure no internal planning documents remain in the final repository.
- Under `docs/`, preserve only the reviewer-facing `ARCHITECTURE.md`, `ALLOCATION.md`, and `TESTING.md` files.
- Keep the root `CLARIFICATIONS.md` as the record of decisions confirmed with the virtual PM, and link to it from the README.

## Verification

Before completion:

- Run every command documented in the README where practical.
- Build a fresh Docker image and run `examples/run_scenarios.py` against it.
- Validate the Postman and scenario JSON files.
- Check all relative documentation links.
- Run formatting, tests, race detection, vet, build, and the lifecycle benchmark.
- Review the final documentation against `BRIEF.MD`, `CLARIFICATIONS.md`, and `AGENTS.md`.
- Confirm the working tree contains no generated junk or internal planning material.

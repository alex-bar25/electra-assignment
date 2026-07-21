# Allocation Scenario Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the one missing physical-allocation scenario and make the submission's documented test coverage easy for Electra to review.

**Architecture:** Add one focused allocator test for three demand levels. Keep the existing Docker lifecycle runner unchanged, keep the existing HTTP benchmark as latency evidence, and add an honest coverage matrix to `TEST_SCENARIOS.md`.

**Tech Stack:** Go 1.26 testing, existing allocation helpers, Markdown, Docker Compose, Python 3 standard library runner.

## Global Constraints

- Do not change production allocation behavior in this slice.
- Do not add dependencies, mocks, concurrency tests, or Docker benchmark infrastructure.
- Do not duplicate physical-limit cases already covered by existing Go tests.
- Preserve every existing “Why this scenario matters” explanation.
- Preserve the user's unstaged `CLARIFICATIONS.md` changes.
- Continue using the existing Docker runner only as a lifecycle smoke test.
- Stop at one commit and review checkpoint per task.

---

### Task 1: Three-demand allocation scenario

**Files:**
- Modify: `internal/allocation/allocator_test.go`

**Interfaces:**
- Consumes: `stationWithOneCharger`, `testSession`, `Allocate`, and `assertPower` from `internal/allocation/allocator_test.go`.
- Produces: `TestAllocateRedistributesAcrossThreeDemandLevels`.

- [ ] **Step 1: Add the focused scenario test**

Add this test immediately after `TestAllocateSharesAndRedistributesGridPower`:

```go
func TestAllocateRedistributesAcrossThreeDemandLevels(t *testing.T) {
	config := stationWithOneCharger(300, 400, 400, 400, 400)
	sessions := []domain.Session{
		testSession("session-1", "connector-1", 50),
		testSession("session-2", "connector-2", 120),
		testSession("session-3", "connector-3", 300),
	}

	assignments := Allocate(config, sessions)
	assertPower(t, assignments, "session-1", 50)
	assertPower(t, assignments, "session-2", 120)
	assertPower(t, assignments, "session-3", 130)
}
```

This is a coverage-only addition: the allocator already implements the behavior, so no production change is expected.

- [ ] **Step 2: Format and run focused allocation tests**

Run:

```bash
gofmt -w internal/allocation/allocator_test.go
go test ./internal/allocation -run 'TestAllocate(SharesAndRedistributesGridPower|RedistributesAcrossThreeDemandLevels)' -count=1
```

Expected: both redistribution tests pass.

- [ ] **Step 3: Run the full allocator package**

Run:

```bash
go test ./internal/allocation -count=1
```

Expected: all allocation tests pass.

- [ ] **Step 4: Review and commit**

Run `git diff --check` and confirm only `internal/allocation/allocator_test.go` is staged, then commit:

```bash
git add internal/allocation/allocator_test.go
git commit -m "test: cover three demand levels"
```

---

### Task 2: Document automated scenario coverage

**Files:**
- Modify: `TEST_SCENARIOS.md`

**Interfaces:**
- Consumes: existing Go test names, `BenchmarkSessionLifecycle`, and `examples/run_scenarios.py`.
- Produces: an accurate reviewer-facing coverage matrix and correct implemented/optional scenario grouping.

- [ ] **Step 1: Correct the extension headings**

Replace the heading before minimum-power scenarios 14–15:

```markdown
## Implemented Extension Scenarios
```

Insert this heading immediately before BESS scenario 16:

```markdown
## Optional Advanced Scenarios
```

Do not change the scenario text or its “Why this scenario matters” explanations.

- [ ] **Step 2: Add the automated coverage matrix**

Under `## Execution Strategy`, before the explanatory paragraphs, add:

```markdown
### Automated coverage map

| Scenarios | Primary Go coverage | Runnable Docker coverage |
| --- | --- | --- |
| 1–2 | `TestAllocateRespectsEffectiveDemandLimits` | Basic first-session allocation only |
| 3–4 | `TestAllocateSharesAndRedistributesGridPower` | Fair sharing and update redistribution |
| 5 | `TestAllocateRedistributesAcrossThreeDemandLevels` | Covered by focused Go test |
| 6 | `TestAllocateRespectsSharedChargerLimit` | Covered by focused Go test |
| 7 | `TestAllocateRedistributesPastFullCharger` | Covered by focused Go test |
| 8–9 | `TestServiceStopSessionRecomputesBeforeReturning`, `TestServiceUpdateSessionRecomputesBeforeReturning` | Update and stop redistribution |
| 10 | `TestAllocateReturnsZeroForUnavailableHardware` | Dynamic update coverage added with availability endpoints |
| 11 | `TestAllocateProducesStableOutput` | Covered by focused Go test |
| 12 | `TestServiceStartSessionRejectsInvalidOperations`, `TestServiceUpdateSessionRejectsInvalidOperationsAtomically`, `TestStartSessionMapsLifecycleErrors` | Docker runner is intentionally limited to the successful lifecycle |
| 13 | `BenchmarkSessionLifecycle` | Docker lifecycle confirms the packaged HTTP path |
| 14–15 | `TestAllocateWaitsWhenMinimumCannotBeReserved`, `TestServiceUpdateSessionReconsidersWaitingSessions` | Covered by focused Go/service tests |
| 16 | Added only if BESS is implemented | Added only if BESS is implemented |
```

Keep the existing benchmark command under scenario 13 unchanged.

- [ ] **Step 3: Verify every referenced test name exists**

Run:

```bash
rg '^func (Test|Benchmark)' internal/**/*_test.go
```

Expected: every concrete test or benchmark named in the matrix appears in the output.

- [ ] **Step 4: Run full source verification**

Run:

```bash
gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go test ./internal/api -run '^$' -bench BenchmarkSessionLifecycle -benchtime=100x
```

Expected: formatting produces no paths; all tests and vet pass; the benchmark remains below `1,000,000,000 ns/op`.

- [ ] **Step 5: Verify the existing packaged lifecycle**

Run these commands separately:

```bash
docker compose up --build -d
python3 examples/run_scenarios.py
```

Expected: the existing configure/start/share/update/stop/final-state scenario prints only `PASS` lines.

- [ ] **Step 6: Review and commit**

Run `git diff --check` and `git status --short`. Confirm the user's `CLARIFICATIONS.md` remains unstaged, then commit only:

```bash
git add TEST_SCENARIOS.md
git commit -m "docs: map automated scenario coverage"
```

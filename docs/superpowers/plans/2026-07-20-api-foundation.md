# API Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose health, station configuration, and station state through a small standard-library HTTP handler.

**Architecture:** `internal/api` owns routing, JSON decoding/encoding, HTTP status mapping, and request-boundary logs. Handlers call the concrete in-memory service directly; no interface, framework, or duplicated business validation is added.

**Tech Stack:** Go 1.26, `net/http`, `encoding/json`, `log/slog`, `httptest`.

## Global Constraints

- Add only `internal/api/handler.go` and `internal/api/handler_test.go`.
- Use Go method-aware `http.ServeMux` patterns.
- Use `{ "code": "...", "message": "..." }` for every API error.
- Decode station configuration directly into `domain.StationConfig`.
- Use the real service in tests; do not add mocks or interfaces.
- Keep session endpoints outside this slice.

---

### Task 1: Health and unconfigured station state

**Files:**
- Create: `internal/api/handler.go`
- Create: `internal/api/handler_test.go`

**Interfaces:**
- Consumes: `*service.Service`, `*slog.Logger`.
- Produces: `New(*service.Service, *slog.Logger) http.Handler`, `GET /health`, and `GET /api/v1/station`.

- [ ] **Step 1: Write failing tests**

Add tests that construct `New(service.New(), slog.New(slog.DiscardHandler))`, assert `GET /health` returns `200` and `{"status":"ok"}`, and assert an unconfigured `GET /api/v1/station` returns `404` with code `station_not_configured`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/api -run 'TestHealth|TestGetStationBeforeConfiguration' -v`

Expected: FAIL because the `api` package and `New` do not exist.

- [ ] **Step 3: Implement routing and response helpers**

Create a private handler containing the service and logger. Register:

```go
mux.HandleFunc("GET /health", api.health)
mux.HandleFunc("GET /api/v1/station", api.getStation)
```

Add these response types:

```go
type healthResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
```

`getStation` maps `service.ErrStationNotConfigured` to `404`; unexpected errors are logged and return a generic `500` response. `writeJSON` always sets `Content-Type: application/json` before writing the status.

- [ ] **Step 4: Verify GREEN**

Run: `gofmt -w internal/api`

Run: `go test ./internal/api -run 'TestHealth|TestGetStationBeforeConfiguration' -v`

Expected: PASS.

---

### Task 2: Configure and query the station

**Files:**
- Modify: `internal/api/handler.go`
- Modify: `internal/api/handler_test.go`

**Interfaces:**
- Produces: `PUT /api/v1/station/config`.

- [ ] **Step 1: Write failing behavior tests**

Add one main-flow test that sends a valid station configuration, asserts `200`, decodes `service.StationState`, and then confirms `GET /api/v1/station` returns the configured state. Add one invalid-configuration test asserting `400` with code `invalid_station_config`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/api -run 'TestConfigureAndGetStation|TestConfigureStationRejectsInvalidConfiguration' -v`

Expected: FAIL with `404` because the configuration route is not registered.

- [ ] **Step 3: Implement strict JSON configuration handling**

Register:

```go
mux.HandleFunc("PUT /api/v1/station/config", api.configureStation)
```

`configureStation` must:

1. Decode one JSON object with `DisallowUnknownFields`.
2. Return `400 invalid_request` for malformed JSON.
3. Call `Service.Configure` and return `400 invalid_station_config` for domain validation errors.
4. Fetch and return the new `StationState` with `200`.
5. Log the accepted station ID; log rejected requests at warning level.

- [ ] **Step 4: Verify the slice**

Run: `gofmt -w internal/api`

Run: `go test ./internal/api -v`

Run: `go test ./...`

Run: `go test -race ./...`

Run: `go vet ./...`

Run: `go build ./...`

Expected: all commands exit successfully with no failures or diagnostics.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handler.go internal/api/handler_test.go
git commit -m "feat: expose station configuration API"
```

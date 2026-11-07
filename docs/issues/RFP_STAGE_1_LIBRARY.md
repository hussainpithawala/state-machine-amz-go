# Request for Proposal (RFP) — Library Layer

## Stage 1: Repository & Query Extensions for the BFF Proxy Migration

**Target system:** [`state-machine-amz-go`](https://github.com/hussainpithawala/state-machine-amz-go)
**Consuming system:** `state-machine-amz-gin` (HTTP layer)
**Companion document:** `docs/RFP_STAGE_1_DOWNSTREAM.md` (Gin-layer RFP)
**Document Version:** 1.0
**Date:** October 14, 2026
**Status:** Draft / For Review

---

## 1. Executive Summary

The downstream RFP (`RFP_STAGE_1_DOWNSTREAM.md`) specifies five new HTTP endpoints and two extended endpoints on `state-machine-amz-gin`. **None of those handler changes can be implemented without corresponding library-layer additions in `state-machine-amz-go`.** This document isolates every change that must land in the library before the Gin handler work can begin.

In scope: **new `Repository` interface methods, new/extended filter structs, SQL implementations in both `postgres.go` and `gorm_postgres.go`, and corresponding `Manager` facade methods.** No HTTP, JSON serialization, or Gin concerns belong here — the library speaks in Go types and `time.Time`, not wire formats.

**Out of scope:** Any change to Gin handlers, response models, routing, or middleware. Anything that concerns camelCase JSON fields, HTTP status codes, or query-parameter parsing belongs in the downstream RFP.

---

## 2. Design Principles

1. **Library owns the query, Gin owns the wire format.** This library never produces JSON response shape — it exposes Go structs. The Gin handler maps library records to API responses.
2. **Time stays native.** All timestamps are `time.Time`. No unix-seconds accept/return at this layer. The Gin handler converts to/from unix seconds (from query params) and ISO 8601 (in responses).
3. **Both repository backends in lockstep.** Every change to the `Repository` interface requires implementations in both `postgres.go` (raw `database/sql`) and `gorm_postgres.go` (GORM). The `GormPostgresRepository` is the production backend; `PostgresRepository` is the legacy backend. Neither may be skipped.
4. **Filter structs stay additive.** Never remove or rename existing fields — only add. The `ExecutionFilter` and `LinkedExecutionFilter` are consumed by other code paths (batch execution, linked-execution orchestration) that must not break.
5. **COUNT follows the same filter.** Every `List*` method's corresponding `Count*` method MUST apply the identical filter logic. This is a data-integrity requirement for pagination.

---

## 3. Scope of Work — Summary

| # | Area | What changes | Where |
|---|---|---|---|
| 1 | `ExecutionFilter` | Add 6 fields + sort/order | `types.go` |
| 2 | `LinkedExecutionFilter` | Add sort/order | `types.go` |
| 3 | State-history paginated query | New `Repository` method | `types.go`, `postgres.go`, `gorm_postgres.go` |
| 4 | Dashboard stats aggregation | New `Repository` method + model | `types.go`, `models.go`, `postgres.go`, `gorm_postgres.go` |
| 5 | State catalog (distinct states/statuses) | New `Repository` method | `types.go`, `postgres.go`, `gorm_postgres.go` |
| 6 | Linked-execution deletion | New `Repository` method (by IDs + by filter) | `types.go`, `postgres.go`, `gorm_postgres.go` |
| 7 | Execution start-time lookup | New `Repository` method | `types.go`, `postgres.go`, `gorm_postgres.go` |
| 8 | `Repository` interface | Register all new methods | `types.go` |
| 9 | `Manager` facade | Add forwarding methods | `repository.go` |
| 10 | `buildExecutionFilters` | Implement new filter conditions in both backends | `postgres.go`, `gorm_postgres.go` |

---

## 4. Filter Struct Extensions

### 4.1 `ExecutionFilter` — Add Fields

**Current struct** (`pkg/repository/types.go:62`):
```go
type ExecutionFilter struct {
    Status         string
    StateMachineID string
    Name           string    // partial match (ILIKE)
    CurrentState   string
    StartAfter     time.Time
    StartBefore    time.Time
    Limit          int
    Offset         int
}
```

**Extended struct** (additive only — existing fields unchanged):
```go
type ExecutionFilter struct {
    Status         string
    StateMachineID string
    Name           string    // partial match (ILIKE)
    CurrentState   string
    StartAfter     time.Time
    StartBefore    time.Time
    Limit          int
    Offset         int

    // --- NEW fields below ---

    ExecutionID    string    // exact match on execution_id
    StateName      string    // EXISTS subquery: state_history.state_name = ?
    StateStatus    string    // EXISTS subquery: state_history.status = ? (AND with StateName if both set)
    SortBy         string    // "startTime" (default), "endTime", "status"
    Order          string    // "asc", "desc" (default "desc")
}
```

**Design notes:**
- `Name` already supports partial match. The downstream RFP specifies `executionName` as partial — it maps to the existing `Name` field. No new `ExecutionName` field is needed.
- `StartAfter`/`StartBefore` already exist. The downstream RFP's `startTimeFrom`/`startTimeTo` (unix seconds) map to these fields after Gin-layer conversion.
- `ExecutionID` is a new exact-match field. Distinct from `StateMachineID`.
- `StateName` and `StateStatus` combine into a **single** `EXISTS (SELECT 1 FROM state_history sh WHERE sh.execution_id = e.execution_id AND sh.execution_start_time = e.start_time AND ...)` subquery when both are present. They must NOT be emitted as two separate subqueries.
- `SortBy`/`Order` control the `ORDER BY` clause. Default: `ORDER BY start_time DESC`.

### 4.2 `LinkedExecutionFilter` — Add Sort Fields

**Current struct** (`pkg/repository/types.go:80`):
```go
type LinkedExecutionFilter struct {
    SourceStateMachineID   string
    SourceExecutionID      string
    SourceStateName        string
    SourceExecutionStatus  string
    InputTransformerName   string
    TargetStateMachineName string
    TargetExecutionID      string
    CreatedAfter           time.Time
    CreatedBefore          time.Time
    Limit                  int
    Offset                 int
}
```

**Extended struct** (additive only):
```go
type LinkedExecutionFilter struct {
    SourceStateMachineID   string
    SourceExecutionID      string
    SourceStateName        string
    SourceExecutionStatus  string
    InputTransformerName   string
    TargetStateMachineName string
    TargetExecutionID      string
    CreatedAfter           time.Time
    CreatedBefore          time.Time
    Limit                  int
    Offset                 int

    // --- NEW fields below ---

    SortBy string // "createdAt" (default), "sourceStateMachineId", "targetStateMachineName"
    Order  string // "asc", "desc" (default "desc")
}
```

**Note:** Currently `ListLinkedExecutions` hardcodes `ORDER BY le.created_at DESC`. This must become dynamic based on `SortBy`/`Order`, defaulting to current behavior when neither is set.

---

## 5. New Repository Interface Methods

### 5.1 `buildExecutionFilters` — EXISTS Subquery for StateName/StateStatus

**Applies to both: `postgres.go` and `gorm_postgres.go`**

When `filter.StateName` and/or `filter.StateStatus` are set, append a single EXISTS subquery:

```sql
WHERE EXISTS (
    SELECT 1
    FROM state_history sh
    WHERE sh.execution_id = e.execution_id
      AND sh.execution_start_time = e.start_time
      AND sh.state_name = ?       -- if filter.StateName != ""
      AND sh.status = ?           -- if filter.StateStatus != ""
)
```

Both conditions are ANDed within the subquery. If only one is set, emit only that condition.

The `COUNT(*)` query behind `CountExecutions` must apply the same subquery (it uses the same `buildExecutionFilters` helper, so this happens automatically — verify in tests).

The `ORDER BY` clause must respect `SortBy` and `Order`:

| SortBy value | SQL column |
|---|---|
| `startTime` (default) | `start_time` |
| `endTime` | `end_time` |
| `status` | `status` |

Direction is `ASC` or `DESC` (validate: only these two values; default `DESC`).

### 5.2 `ListStateHistory` — Paginated State History (Cursor)

**New method on `Repository` interface:**

```go
// ListStateHistory retrieves state history for an execution with optional cursor pagination.
// If afterSequenceNumber is nil and limit <= 0, returns all records (backward compatible with GetStateHistory).
// Order controls direction: "asc" (default) or "desc".
ListStateHistory(ctx context.Context, executionID string, afterSequenceNumber *int, limit int, order string) ([]*StateHistoryRecord, int64, error)
```

**Returns:**
- `[]*StateHistoryRecord` — the page of records
- `int64` — total count of state-history rows for this execution (unfiltered)
- `error`

**SQL (cursor path):**
```sql
SELECT ... FROM state_history
WHERE execution_id = $1
  AND sequence_number > $2  -- only when afterSequenceNumber is set
ORDER BY sequence_number {ASC|DESC}
LIMIT $3                     -- only when limit > 0
```

**Count query (always):**
```sql
SELECT COUNT(*) FROM state_history WHERE execution_id = $1
```

**Behavior matrix:**
| afterSequenceNumber | limit | Behavior |
|---|---|---|
| nil | 0 or negative | Return all (same as current `GetStateHistory`) |
| set | positive | Cursor page |
| nil | positive | First page (no cursor filter) |

**Note:** `GetStateHistory` should remain as-is for backward compatibility with existing callers (e.g., `persistent.go`). `ListStateHistory` is the new cursor-capable variant.

**Implementation in both `postgres.go` and `gorm_postgres.go`.**

### 5.3 `GetDashboardStats` — Dashboard Aggregation

**New types:**

```go
// DashboardStats holds aggregated dashboard statistics.
type DashboardStats struct {
    StatusCounts     []StatusCount         // all statuses with count > 0 in window30d
    DurationStats    *DurationStats        // over SUCCEEDED executions in window7d; nil when no qualifying rows
    TotalExecutions  int64                 // count in window30d
    TotalStateMachines int64               // all-time count
    RecentFailures   []*DashboardFailure   // top 10 FAILED by start_time DESC
    Timestamp        time.Time             // NOW() from the database
}

type StatusCount struct {
    Status string
    Count  int64
}

type DurationStats struct {
    AvgDuration *float64 // seconds; nil when no qualifying rows
    MaxDuration *float64
    MinDuration *float64
}

type DashboardFailure struct {
    ExecutionID    string
    StateMachineID string
    Name           string
    Status         string
    StartTime      *time.Time
    CurrentState   string
    Error          string
}
```

**New method on `Repository` interface:**
```go
// GetDashboardStats returns dashboard statistics using the DB's NOW() for rolling windows.
// window30d and window7d are computed at the repository layer using CURRENT_TIMESTAMP.
GetDashboardStats(ctx context.Context) (*DashboardStats, error)
```

**SQL queries (single round-trip if possible, otherwise 4 queries):**

1. **Status counts (30 days):**
```sql
SELECT status, COUNT(*) as cnt
FROM executions
WHERE start_time >= NOW() - INTERVAL '30 days'
GROUP BY status
HAVING COUNT(*) > 0
```

2. **Duration stats (7 days, SUCCEEDED only):**
```sql
SELECT
    AVG(EXTRACT(EPOCH FROM (end_time - start_time))) as avg_dur,
    MAX(EXTRACT(EPOCH FROM (end_time - start_time))) as max_dur,
    MIN(EXTRACT(EPOCH FROM (end_time - start_time))) as min_dur
FROM executions
WHERE status = 'SUCCEEDED'
  AND start_time >= NOW() - INTERVAL '7 days'
  AND end_time IS NOT NULL
```

3. **Total executions (30 days):**
```sql
SELECT COUNT(*) FROM executions
WHERE start_time >= NOW() - INTERVAL '30 days'
```

4. **Recent failures:**
```sql
SELECT execution_id, state_machine_id, name, status, start_time, current_state, error
FROM executions
WHERE status = 'FAILED'
ORDER BY start_time DESC
LIMIT 10
```

5. **Total state machines (all time):**
```sql
SELECT COUNT(*) FROM state_machines
```

6. **Timestamp:**
```sql
SELECT NOW()
```

**Rationale for using DB `NOW()`:** Avoids clock-skew issues documented in the downstream RFP §12.3.

**Implementation in both `postgres.go` and `gorm_postgres.go`.**

### 5.4 `GetDistinctStateStatuses` — State Catalog

**New type:**

```go
// StateStatusPair is a distinct (state_name, status) combination observed in state_history.
type StateStatusPair struct {
    StateName string
    Status    string
}
```

**New method on `Repository` interface:**
```go
// GetDistinctStateStatuses returns distinct (state_name, status) pairs from state_history
// for the given state machine, bounded by the most recent `executionLimit` executions.
GetDistinctStateStatuses(ctx context.Context, stateMachineID string, executionLimit int) ([]*StateStatusPair, error)
```

**SQL:**
```sql
SELECT DISTINCT sh.state_name, sh.status
FROM state_history sh
WHERE sh.execution_id IN (
    SELECT execution_id
    FROM executions
    WHERE state_machine_id = $1
    ORDER BY start_time DESC
    LIMIT $2
)
```

**Implementation in both `postgres.go` and `gorm_postgres.go`.**

### 5.5 `DeleteLinkedExecutions` — Bulk Delete

**New method on `Repository` interface:**
```go
// DeleteLinkedExecutions deletes linked-execution records.
// If ids is non-empty, deletes exactly those records (ignores filter).
// If ids is empty and filter is non-nil, deletes all matching records.
// Returns the count of deleted rows and the IDs deleted (when deleting by IDs).
DeleteLinkedExecutions(ctx context.Context, ids []string, filter *LinkedExecutionFilter) (int64, []string, error)
```

**SQL (by IDs):**
```sql
DELETE FROM linked_executions WHERE id = ANY($1)
```
Return the count and the provided ids.

**SQL (by filter):**
Build the same WHERE clause as `ListLinkedExecutions` uses (share the filter-building helper). Execute DELETE instead of SELECT. Return the count, nil for IDs.

**Safety:** If both `ids` is empty and `filter` is nil (or all-zero), return an error — refuse wide delete. This is enforced by the `Manager` wrapper as well.

**Implementation in both `postgres.go` and `gorm_postgres.go`.**

### 5.6 `GetExecutionStartTime` — Earliest State History Timestamp

**New method on `Repository` interface:**
```go
// GetExecutionStartTime returns the execution_start_time from the earliest state-history row
// for the given execution. Returns an error if no state-history rows exist.
GetExecutionStartTime(ctx context.Context, executionID string) (*time.Time, error)
```

**SQL:**
```sql
SELECT execution_start_time
FROM state_history
WHERE execution_id = $1
ORDER BY sequence_number ASC
LIMIT 1
```

If `sql.ErrNoRows`, return an error (the Gin handler will map this to 404).

**Implementation in both `postgres.go` and `gorm_postgres.go`.**

---

## 6. Repository Interface Registration

Add all five new methods to the `Repository` interface in `pkg/repository/types.go`:

```go
type Repository interface {
    // ... existing methods unchanged ...

    // --- Stage 1 additions ---

    // ListStateHistory retrieves paginated state history with cursor support.
    ListStateHistory(ctx context.Context, executionID string, afterSequenceNumber *int, limit int, order string) ([]*StateHistoryRecord, int64, error)

    // GetDashboardStats returns aggregated dashboard statistics.
    GetDashboardStats(ctx context.Context) (*DashboardStats, error)

    // GetDistinctStateStatuses returns distinct state/status pairs observed in state_history.
    GetDistinctStateStatuses(ctx context.Context, stateMachineID string, executionLimit int) ([]*StateStatusPair, error)

    // DeleteLinkedExecutions deletes linked executions by IDs or by filter.
    DeleteLinkedExecutions(ctx context.Context, ids []string, filter *LinkedExecutionFilter) (int64, []string, error)

    // GetExecutionStartTime returns the earliest state-history execution_start_time.
    GetExecutionStartTime(ctx context.Context, executionID string) (*time.Time, error)
}
```

---

## 7. Manager Facade Methods

Add corresponding forwarding methods to `Manager` in `pkg/repository/repository.go`:

```go
func (pm *Manager) ListStateHistory(ctx context.Context, executionID string, afterSequenceNumber *int, limit int, order string) ([]*StateHistoryRecord, int64, error) {
    return pm.repository.ListStateHistory(ctx, executionID, afterSequenceNumber, limit, order)
}

func (pm *Manager) GetDashboardStats(ctx context.Context) (*DashboardStats, error) {
    return pm.repository.GetDashboardStats(ctx)
}

func (pm *Manager) GetDistinctStateStatuses(ctx context.Context, stateMachineID string, executionLimit int) ([]*StateStatusPair, error) {
    return pm.repository.GetDistinctStateStatuses(ctx, stateMachineID, executionLimit)
}

func (pm *Manager) DeleteLinkedExecutions(ctx context.Context, ids []string, filter *LinkedExecutionFilter) (int64, []string, error) {
    return pm.repository.DeleteLinkedExecutions(ctx, ids, filter)
}

func (pm *Manager) GetExecutionStartTime(ctx context.Context, executionID string) (*time.Time, error) {
    return pm.repository.GetExecutionStartTime(ctx, executionID)
}
```

The `Manager` MUST also validate safety preconditions (refuse `DeleteLinkedExecutions` with empty ids and nil/empty filter) before calling the repository.

---

## 8. `buildExecutionFilters` Changes

### 8.1 `postgres.go`

Extend the existing `buildExecutionFilters` method (`postgres.go:1108`) to handle:

| New filter field | SQL condition |
|---|---|
| `ExecutionID` | `execution_id = $N` |
| `StateName` (+ `StateStatus` combined) | `EXISTS (SELECT 1 FROM state_history sh WHERE sh.execution_id = executions.execution_id AND sh.execution_start_time = executions.start_time AND sh.state_name = $N [AND sh.status = $M])` |
| `SortBy` + `Order` | Applied to the `ORDER BY` clause (outside `buildExecutionFilters`; see §8.3) |

**Fix existing bug:** The current `StartBefore` filter emits `start_before <= $N` — this is an invalid column reference. It should be `start_time <= $N`. Fix in this scope.

**Fix existing bug:** The `StartBefore` filter does not increment `argCount` — the `argCount++` is commented out. Uncomment it so the next filter parameter gets the correct `$N` positional index.

### 8.2 `gorm_postgres.go`

Apply the same filter additions to the GORM query builder. The GORM backend constructs queries differently (using `db.Where(...)` chains), so the EXISTS subquery must be expressed as a raw-Where clause:

```go
if filter.StateName != "" || filter.StateStatus != "" {
    existsSQL := "EXISTS (SELECT 1 FROM state_history sh WHERE sh.execution_id = executions.execution_id AND sh.execution_start_time = executions.start_time"
    if filter.StateName != "" {
        existsSQL += fmt.Sprintf(" AND sh.state_name = ?", filter.StateName)
    }
    if filter.StateStatus != "" {
        existsSQL += fmt.Sprintf(" AND sh.status = ?", filter.StateStatus)
    }
    existsSQL += ")"
    db = db.Where(existsSQL)
}
```

### 8.3 `ORDER BY` Clause — Both Backends

`ListExecutions` in both backends currently hardcodes the ORDER BY clause. Make it dynamic:

```go
sortColumn := "start_time" // default
switch filter.SortBy {
case "endTime":
    sortColumn = "end_time"
case "status":
    sortColumn = "status"
}
direction := "DESC" // default
if strings.EqualFold(filter.Order, "asc") {
    direction = "ASC"
}
query += fmt.Sprintf(" ORDER BY %s %s", sortColumn, direction)
```

Same pattern applies to `ListLinkedExecutions` `ORDER BY` clause.

---

## 9. Implementation Notes

### 9.1 File Touch-Map

| File | Changes |
|---|---|
| `pkg/repository/types.go` | Extend `ExecutionFilter`, `LinkedExecutionFilter`. Add `DashboardStats`, `StatusCount`, `DurationStats`, `DashboardFailure`, `StateStatusPair` types. Register 5 new methods on `Repository` interface. |
| `pkg/repository/repository.go` | Add 5 new `Manager` forwarding methods. |
| `pkg/repository/postgres.go` | Implement 5 new methods. Extend `buildExecutionFilters` for new fields. Fix `StartBefore` bugs. Dynamic ORDER BY in `ListExecutions`. |
| `pkg/repository/gorm_postgres.go` | Implement 5 new methods. Extend filter builder for new fields. Dynamic ORDER BY in `ListExecutions` and `ListLinkedExecutions`. |

### 9.2 Existing `GetStateHistory` — Do Not Modify

The existing `GetStateHistory(ctx, executionID)` method is consumed by internal library code (`pkg/statemachine/persistent/persistent.go`, etc.). It must remain unchanged. `ListStateHistory` is the new parallel method.

### 9.3 GORM Postgres Backend — Raw WHERE Clauses

The GORM backend uses `.Where(...)` chaining. For the EXISTS subquery, GORM does not support subqueries via its built-in expression builder for correlated subqueries like this one. Use `db.Where(rawSQL, args...)`.

### 9.4 Dashboard Stats — Prefer Fewer Round-Trips

The six queries in §5.3 can be batched when the backend supports it (e.g., using `db.QueryRowContext` for multiple queries in parallel goroutines). For the initial implementation, sequential queries are acceptable — the dashboard endpoint is not high-frequency.

### 9.5 `LinkedExecutionFilter` — SortBy Column Mapping

| SortBy value | SQL column |
|---|---|
| `createdAt` (default) | `le.created_at` |
| `sourceStateMachineId` | `le.source_state_machine_id` |
| `targetStateMachineName` | `le.target_state_machine_name` |

---

## 10. Acceptance Criteria

An implementation is accepted when:

1. **`go build ./...` succeeds** in `state-machine-amz-go` with no new lint/vet issues.
2. **`go vet ./...` passes** — no unreachable params, no unused variables.
3. **Unit tests exist for:**
   - `buildExecutionFilters` with every new field individually and in combination (including `StateName + StateStatus` together).
   - `ListStateHistory` with all three behavioral modes (all/unbounded, first page, cursor page).
   - `DeleteLinkedExecutions` by IDs, by filter, and the safety check (empty ids + nil filter → error).
   - `GetDashboardStats` with empty data (all counts zero, nil duration stats).
   - `GetExecutionStartTime` with and without rows.
4. **Integration tests** (against testcontainer PostgreSQL) that:
   - Seed ≥2 state machines with different execution counts.
   - Exercise each new `Repository` method end-to-end.
   - Assert filter combinations on `ListExecutions` including state-exists subquery + time range.
   - Assert cursor pagination correctness on `ListStateHistory`.
5. **Both backends tested:** `postgres.go` and `gorm_postgres.go` must pass the same integration test suite. Use the existing test-suite pattern (see `postgres_integration_test.go` and `gorm_postgres_integration_test.go`).
6. **Backward compatibility:** All existing `Repository` interface consumers (including `state-machine-amz-gin`) compile and pass existing tests without modification (i.e., the interface grows, it does not change).
7. **Fix verification:** The `StartBefore` bug fix (§8.1) has a regression test asserting the correct SQL column name.

---

## 11. Relationship to Downstream RFP

This table shows how each downstream RFP item maps to library work:

| Downstream RFP Item | Library Change (this RFP) | Priority |
|---|---|---|
| §5 — ExecutionFilter extensions | §4.1, §5.1, §8 | P0 — blocks handler work |
| §6.1 — `GET /dashboard/stats` | §5.3 — `GetDashboardStats` | P0 |
| §6.2 — `GET /state-machines/:id/states` | §5.4 — `GetDistinctStateStatuses` | P0 |
| §6.3 — `GET /linked-executions` | §4.2 — sort/order on `LinkedExecutionFilter` | P0 — ListLinkedExecutions/CountLinkedExecutions already exist |
| §6.4 — `DELETE /linked-executions` | §5.5 — `DeleteLinkedExecutions` | P0 |
| §6.5 — `GET /executions/:id/start-time` | §5.6 — `GetExecutionStartTime` | P0 |
| §6.6 — History cursor pagination | §5.2 — `ListStateHistory` | P0 |

### Dependency ordering:

1. **Filter struct extensions** (§4) — no dependencies, do first.
2. **`buildExecutionFilters` changes** (§8) — depends on §4.
3. **New Repository methods** (§5.2–5.6) — independent of each other, can be parallelized.
4. **Manager facade** (§7) — depends on §6 (interface registration).
5. **Tests** (§10) — depends on all above.

---

## 12. Bug Fixes Bundled in This Scope

While extending `buildExecutionFilters`, two pre-existing bugs must be fixed:

1. **Invalid column reference for `StartBefore`:** Currently emits `start_before <= $N` — should be `start_time <= $N`. (`postgres.go:1157`)
2. **Missing `argCount++` for `StartBefore`:** The increment is commented out, causing positional parameter misalignment for any filter that follows `StartBefore`. (`postgres.go:1159`)

Both bugs are dormant because the Gin handler currently does not set `StartBefore`. The RFP's new `startTimeTo` parameter will activate this code path, so the fixes must land in the same change.

---

## 13. Versioning & Release

- This is an **additive** change to the public API of `state-machine-amz-go`.
- Suggested version bump: **minor** (e.g., `v1.2.22` → `v1.3.0`).
- The `state-machine-amz-gin` `go.mod` must be updated to require this new version after both RFPs are implemented.

---

## Appendix A — Current Repository Interface Surface (Reference)

For reference, these are the methods currently on the `Repository` interface that are relevant to this RFP:

```
ListExecutions(ctx, *ExecutionFilter) → ([]*ExecutionRecord, error)
CountExecutions(ctx, *ExecutionFilter) → (int64, error)
ListExecutionIDs(ctx, *ExecutionFilter) → ([]string, error)
GetStateHistory(ctx, executionID) → ([]*StateHistoryRecord, error)
GetStateMachine(ctx, stateMachineID) → (*StateMachineRecord, error)
ListStateMachines(ctx, *DefinitionFilter) → ([]*StateMachineRecord, error)
SaveLinkedExecution(ctx, *LinkedExecutionRecord) → error
ListLinkedExecutions(ctx, *LinkedExecutionFilter) → ([]*LinkedExecutionRecord, error)
CountLinkedExecutions(ctx, *LinkedExecutionFilter) → (int64, error)
```

## Appendix B — Validation: SortBy / Order Enum Contract

The library does NOT validate enum strings — it maps known values and silently defaults. This matches existing patterns:

| Input | Mapped to |
|---|---|
| unknown SortBy | default column |
| unknown Order | `DESC` |
| empty SortBy | default column |
| empty Order | default (`desc` → `DESC`) |

The Gin handler is responsible for rejecting invalid enum values at the HTTP boundary and returning 400 Bad Request. The library simply treats unrecognized values as defaults.

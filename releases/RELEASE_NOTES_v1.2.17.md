# Release Notes - v1.2.17

**Release Date:** March 29, 2026

## Overview

This release delivers comprehensive bug fixes to the `ListNonLinkedExecutions` repository method, correcting critical query logic issues that affected result accuracy in chained execution workflows. The fixes ensure proper isolation between mutually exclusive workflow branches and accurate filtering across all dimensions (state, status, state machine).

## Key Highlights

### 🐞 Critical Bug Fixes

**Correlated Subquery Logic**
- Replaced non-correlated derived table with proper `NOT EXISTS` correlated subquery
- Fixed filter scoping: all `linkedExecutionFilter` fields now correctly scoped to `linked_executions` table alone
- Status filter now applied to outer `executions` row, eliminating broken references inside subquery

**Pagination Bug**
- Fixed pagination to read `Limit` and `Offset` from `executionFilter` (was incorrectly reading from `linkedExecutionFilter`)
- Both guard check and value now read from the same source

**Execution Filter Application**
- `executionFilter` clauses (StateMachineID, Status, Name, StartAfter, StartBefore) now applied unconditionally
- No longer nested inside `linkedExecutionFilter` nil check
- Ensures independent filter parameters work correctly in all combinations

**Nil Pointer Panic Prevention**
- Added guard against nil `executionFilter` parameter
- Initializes empty `ExecutionFilter{}` if nil to prevent panic on field access

**Deduplication Logic**
- Fixed duplicate rows when execution has multiple `state_history` entries (e.g., retried states)
- Execution with retried states now appears exactly once in results

### ✅ Comprehensive Test Coverage

Added 7 new integration tests covering all edge cases:

| Test | Purpose |
|------|---------|
| `MutuallyExclusiveBranches_NoOverlap` | Verifies no cross-branch leakage between PaymentCheck/FraudCheck |
| `LinkedFromOneState_DoesNotExcludeFromOtherStateQuery` | Core correctness: link from StateA doesn't exclude from StateB query |
| `StatusFilter_Isolates_ByStateName` | Validates status filter isolation (SUCCEEDED/RUNNING/FAILED) |
| `StateMachineID_Scopes_Results` | Tests state machine ID scoping across multiple machines |
| `Deduplication_MultipleStateHistoryRows` | Ensures single result per execution despite retries |
| `NoStateHistory_NeverReturned` | Verifies INNER JOIN gate behavior |
| `FullScenario_BothBranches_MixedStatuses_MixedLinks` | End-to-end production-like scenario |

## What's Changed

### Repository Layer (`pkg/repository/gorm_postgres.go`)

**Before:**
```go
// Non-correlated subquery with broken status reference
subQuery := r.db.Table("linked_executions le")
if linkedExecutionFilter.SourceExecutionStatus != "" {
    subQuery = subQuery.Where("le.source_execution_id = executions.execution_id AND executions.status = ?", ...)
}
```

**After:**
```go
// Correlated NOT EXISTS subquery with proper filter scoping
subQuery := r.db.Table("linked_executions le").
    Select("1").
    Where("le.source_execution_id = executions.execution_id")
// ... filters applied to le.* fields only

query = query.Where("NOT EXISTS (?)", subQuery)

// Status filter applied to outer executions row
if linkedExecutionFilter.SourceExecutionStatus != "" {
    query = query.Where("executions.status = ?", ...)
}
```

### Test Infrastructure (`pkg/repository/gorm_postgres_integration_test.go`)

- **+669 lines** of new test code
- 7 comprehensive test scenarios
- 6 reusable test helper functions:
  - `collectExecutionIDs()` - Extract execution IDs from result slices
  - `toSet()` - Convert string slice to map for O(1) membership checks
  - `assertDisjoint()` - Verify no overlap between result sets
  - `makeExecution()` - Construct minimal ExecutionRecord for fixtures
  - `makeStateHistory()` - Construct minimal StateHistoryRecord for fixtures
  - `makeLinkedExecution()` - Construct minimal LinkedExecutionRecord for fixtures

## Impact Assessment

### Who Should Upgrade

- **All users** of `ListNonLinkedExecutions` for chained execution workflows
- **Critical** for users with:
  - Mutually exclusive workflow branches (e.g., PaymentCheck vs FraudCheck)
  - Executions that pass through multiple states with links from specific states
  - Retry logic creating multiple `state_history` rows per state
  - Pagination requirements on filtered results

### Behavior Changes

| Scenario | Before (v1.2.16) | After (v1.2.17) |
|----------|------------------|-----------------|
| Execution with link from StateA queried by StateB | ❌ Incorrectly excluded | ✅ Correctly included |
| Execution with 3 retry attempts | ❌ Appears 3 times in results | ✅ Appears exactly once |
| Pagination with Limit/Offset | ❌ Ignored (read from wrong struct) | ✅ Works correctly |
| Nil executionFilter parameter | ❌ Panics | ✅ Handled gracefully |
| Cross-branch queries | ❌ Potential leakage | ✅ Complete isolation |

### Migration Notes

**No breaking changes.** This is a pure bug fix release. The corrected behavior aligns with documented semantics and user expectations.

**No database migrations required.** All changes are at the query logic level.

## Technical Details

### Files Modified

| File | Lines Added | Lines Removed | Net Change |
|------|-------------|---------------|------------|
| `pkg/repository/gorm_postgres.go` | 69 | 47 | +22 |
| `pkg/repository/gorm_postgres_integration_test.go` | 669 | 2 | +667 |
| **Total** | **740** | **53** | **+687** |

### Bug Fix Summary

| Bug # | Issue | Fix |
|-------|-------|-----|
| #1 | Non-correlated subquery with broken status reference | Correlated `NOT EXISTS` with proper `le.*` scoping |
| #2 | Pagination read from wrong filter struct | Read `Limit/Offset` from `executionFilter` |
| #3 | Execution filters nested inside linkedExecutionFilter check | Apply unconditionally |
| #4 | Nil pointer panic on executionFilter access | Guard with nil check, initialize empty struct |
| #5 | Execution filters incorrectly scoped | Separate application of execution vs linked filters |
| #6 | Error message contained Go variable name | Clean error message: `"failed to list non-linked executions"` |
| #7 | Duplicate rows with multiple state_history entries | `SELECT DISTINCT` on all execution columns |

## Verification

Run the new integration tests to verify correct behavior:

```bash
# Run all ListNonLinkedExecutions tests
go test -v ./pkg/repository -run TestListNonLinkedExecutions

# Run with PostgreSQL backend (requires running DB)
go test -v ./pkg/repository -run TestListNonLinkedExecutions -tags=integration
```

## Related Issues

This release addresses multiple correctness issues discovered during comprehensive testing of chained execution workflows with:
- Mutually exclusive branch states
- Cross-state linked execution tracking
- Retry scenarios with multiple history entries
- Complex filter combinations

## Contributors

- Core development and testing
- Integration test suite expansion

---

**Full Changelog:** [CHANGELOG.md](CHANGELOG.md#1217---2026-03-29)

**Previous Release:** [v1.2.16](https://github.com/your-org/state-machine-amz-go/releases/tag/v1.2.16)

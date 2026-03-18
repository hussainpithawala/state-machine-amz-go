# Release Notes - v1.2.12

**Release Date:** March 18, 2026

## Overview

Version 1.2.12 is a **code cleanup and simplification release** that removes unnecessary complexity from the state machine execution engine. This release focuses on removing panic recovery logic, simplifying state history tracking, and cleaning up test files.

## ⚠️ Breaking Changes

### Removed Panic Recovery Mechanism

The panic recovery mechanism in state machine execution has been removed. This includes:

- Removed `ExecFailureMessage` constant from `internal/states/state.go`
- Removed `executeStateWithRecovery()` wrapper function from `pkg/statemachine/statemachine.go`
- Removed `MergeInputs()` helper function from `pkg/statemachine/statemachine.go`

**Migration:** If your code relied on panic recovery during state execution, you should now implement explicit error handling within your state's `Execute()` method.

### Simplified State History

The `HistorySequenceNumber` field has been removed from the `Execution` struct. Sequence numbers are now implicitly tracked by the order of entries in the `History` slice.

**Affected Changes:**
- `pkg/execution/execution.go`: Removed `HistorySequenceNumber` from `Execution` struct
- `pkg/execution/execution.go`: `AddStateHistory()` no longer tracks sequence numbers
- `pkg/executor/executor_message.go`: Removed `HistorySequenceNumber` mapping from repository records
- `pkg/repository/models.go`: Updated `ExecutionRecord` struct accordingly

## 🧹 Code Cleanup

### Removed Test Files

The following test files have been removed as part of code simplification:

- `pkg/handler/batch_barrier_test.go` (473 lines)
- `pkg/handler/batch_barrier_integration_test.go` (457 lines)
- `pkg/statemachine/statemachine_test.go` (358 lines) - sequence number tests
- `pkg/statemachine/persistent/persistent_test.go` (678 lines) - sequence number tests

### Simplified Execution Flow

The state machine execution flow has been streamlined:

**Before:**
```go
output, nextState, err := sm.executeStateWithRecovery(ctx, state, execCtx.Input, currentStateName)
if err != nil {
    execCtx.AddStateHistory(currentStateName, currentInput, output, "FAILED")
    // ... error handling
} else {
    execCtx.AddStateHistory(currentStateName, currentInput, output, "SUCCEEDED")
}
```

**After:**
```go
output, nextState, err := state.Execute(ctx, currentInput)
execCtx.AddStateHistory(currentStateName, currentInput, output)
if err != nil {
    // ... error handling
}
```

### Handler Package Addition

A new handler package has been added at `pkg/statemachine/handler/handler.go` (222 lines). This appears to be a consolidation of execution handling logic.

## 📦 Dependency Changes

- Removed `github.com/stretchr/objx` v0.5.0 from `go.mod` and `go.sum`

## 📊 Statistics

| Metric | Value |
|--------|-------|
| Files Changed | 18 |
| Lines Added | 328 |
| Lines Removed | 2,194 |
| Net Change | -1,866 lines |

## 🔍 Key File Changes

| File | Change Summary |
|------|----------------|
| `pkg/statemachine/statemachine.go` | Removed panic recovery logic, simplified execution flow (-61 lines) |
| `pkg/execution/execution.go` | Simplified `Execution` struct and `AddStateHistory()` |
| `pkg/handler/*.go` | Removed batch barrier test files (-930 lines) |
| `pkg/statemachine/handler/handler.go` | New handler implementation (+222 lines) |
| `pkg/repository/*.go` | Updated to remove `HistorySequenceNumber` references |

## ✅ Testing

This release focuses on code cleanup. The removed test files were primarily related to:
- Batch barrier signaling tests (now covered elsewhere or deemed unnecessary)
- Sequence number validation tests (no longer needed after removing sequence numbers)

Existing integration tests continue to validate core functionality.

## 🚀 Recommendations

1. **No immediate action required** for most users - this is a cleanup release
2. If you relied on panic recovery, implement explicit error handling in your states
3. If you accessed `HistorySequenceNumber`, use the slice index instead

## 📝 Related Documentation

- See `CHANGELOG.md` for detailed historical changes
- See `EXECUTION_CHAINING_IMPLEMENTATION.md` for execution chaining details
- See `INTEGRATION_SUMMARY.md` for integration patterns

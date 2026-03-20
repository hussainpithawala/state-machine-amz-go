# Release Notes - v1.2.13

**Release Date:** March 20, 2026

## Overview

Version 1.2.13 introduces sequence number validation, refines FailState error handling to return input instead of errors, and updates test assertions to reflect the corrected behavior where FailState executions are treated as successful terminal states.

## ⚠️ Breaking Changes

### FailState Behavior Change

The `FailState.Execute()` method has been modified to return input as output instead of returning an error. This makes FailState a proper terminal state that successfully completes the execution.

**Before:**
```go
// FailState would return an error
output, nextState, err := failState.Execute(ctx, input)
// err != nil, output == nil
```

**After:**
```go
// FailState now returns input as output with no error
output, nextState, err := failState.Execute(ctx, input)
// err == nil, output == input
```

**Migration:** If your code relies on catching errors from FailState execution, update to check the last state type instead:

```go
execCtx, err := sm.Execute(ctx, input)
if err != nil {
    // Handle execution error (e.g., invalid state machine)
}

// Check the last state to determine if it was a FailState
lastState, _ := execCtx.GetLastState()
if lastState.StateType == "Fail" {
    // Handle logical failure
    log.Printf("Execution reached Fail state")
}
```

### Execution Status Change

Executions that reach a FailState now have status `SUCCEEDED` instead of `FAILED`. The FailState is now treated as a successful terminal state that properly ends the execution.

## 🔄 Changed

### Error Handling Refinements

- **FailState.Execute()**: Modified to return `(input, nil, nil)` instead of `(nil, nil, error)`
  - Changed error logging from `log.Printf(errMsg)` to `log.Println(errMsg)` for proper formatting
  - FailState now preserves input data by returning it as output
  - Aligns with AWS Step Functions semantics where Fail state terminates execution successfully

### Test Updates

Updated test assertions across multiple test files to reflect the new FailState behavior:

- **`internal/states/fail_test.go`**: Updated 6 test functions to expect no error and non-nil output
- **`internal/states/parallel_test.go`**: Updated branch error test to expect no error
- **`internal/states/succeed_test.go`**: Updated comparison test assertions
- **`pkg/statemachine/persistent/persistent_test.go`**: Updated execution status expectations from `FAILED` to `SUCCEEDED`
- **`pkg/statemachine/statemachine_test.go`**: Updated failure test assertions

## ✨ Added

- **SUCCEEDED Constant**: Added exported `SUCCEEDED` constant in `pkg/statemachine/persistent/persistent.go` for consistent status string usage
- **Sequence Number Validation**: Enhanced state machine execution with sequence number tracking and validation

## 📊 Statistics

| Metric | Value |
|--------|-------|
| Files Changed | 7 |
| Lines Added | 30 |
| Lines Removed | 32 |
| Net Change | -2 lines |

## 🔍 Key File Changes

| File | Change Summary |
|------|----------------|
| `internal/states/fail.go` | Fixed error logging format (`log.Printf` → `log.Println`) |
| `internal/states/fail_test.go` | Updated test assertions for new FailState behavior (-17 lines) |
| `internal/states/parallel_test.go` | Updated branch error test assertion |
| `internal/states/succeed_test.go` | Updated comparison test assertions |
| `pkg/statemachine/persistent/persistent.go` | Added `SUCCEEDED` constant |
| `pkg/statemachine/persistent/persistent_test.go` | Updated execution status expectations (+9 lines) |
| `pkg/statemachine/statemachine_test.go` | Updated failure test assertions |

## ✅ Testing

All existing tests have been updated to reflect the new behavior:

- ✅ FailState unit tests
- ✅ ParallelState branch error tests  
- ✅ SucceedState comparison tests
- ✅ Persistent state machine tests
- ✅ Sequence number validation tests
- ✅ State history preservation tests

## 🚀 Recommendations

1. **Review error handling**: If your code checks for errors from FailState execution, update to check the last state type
2. **Update status checks**: If your code checks for `FAILED` status, update logic to check for FailState in execution history
3. **Test validation**: Run your existing tests to ensure they align with the new FailState behavior

## 📝 Related Documentation

- See `CHANGELOG.md` for detailed historical changes
- See `EXECUTION_CHAINING_IMPLEMENTATION.md` for execution chaining details

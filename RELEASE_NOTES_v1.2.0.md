# Release Notes - v1.2.0

## 🧪 Testing Enhancement - Comprehensive Failure Scenario Test Coverage

**Release Date**: February 2, 2026

### Overview

Version 1.2.0 adds comprehensive unit test coverage for execution failure scenarios in the persistent state machine package. These tests validate that executions are properly marked as `FAILED` with correct error information, end times, and history tracking when failures occur.

### What's New

#### Enhanced Test Coverage for Failure Scenarios

Added two critical test cases to `pkg/statemachine/persistent/persistent_test.go`:

1. **`TestExecute_FailState_MarkedAsFailed`** - Validates failure handling when a Fail state is encountered
2. **`TestExecute_TaskStateError_MarkedAsFailed`** - Validates failure handling when a Task state execution errors

### Technical Details

#### Test Case 1: Fail State Handling

**Location**: `pkg/statemachine/persistent/persistent_test.go:756`

Tests the scenario where an execution encounters a Fail state during workflow execution.

**Workflow:**
```yaml
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    Next: FailState
  FailState:
    Type: Fail
    Error: CustomError
    Cause: This is a test failure
```

**Validations:**
- ✅ Error is returned with correct error message and cause
- ✅ Execution status is set to `FAILED`
- ✅ Execution's Error field is populated with the error
- ✅ EndTime is properly set
- ✅ History entries are recorded for all executed states

#### Test Case 2: Task State Error Handling

**Location**: `pkg/statemachine/persistent/persistent_test.go:819`

Tests the scenario where a Task state execution fails (e.g., database connection error, external service failure).

**Workflow:**
```yaml
StartAt: ProcessTask
States:
  ProcessTask:
    Type: Task
    Resource: arn:aws:lambda:us-east-1:123456789012:function:ProcessData
    Next: SuccessState
  SuccessState:
    Type: Succeed
```

**Mock Handler:**
```go
mockHandler := &mockTaskHandler{
    executeFunc: func(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
        return nil, fmt.Errorf("task execution failed: database connection error")
    },
}
```

**Validations:**
- ✅ Error is returned from the task handler
- ✅ Execution status is set to `FAILED`
- ✅ Execution's Error field contains the task error
- ✅ EndTime is properly set
- ✅ History contains exactly one entry from the failed task
- ✅ Failed state history has correct state name, type, status (`FAILED`), and error

#### Supporting Infrastructure

**Mock Task Handler:**
```go
// pkg/statemachine/persistent/persistent_test.go:17
type mockTaskHandler struct {
    executeFunc func(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error)
}
```

Simple mock implementation that allows flexible error injection for testing various failure scenarios.

### Code References

**Failure Handling Logic:**
The tests validate the behavior implemented in `pkg/statemachine/persistent/persistent.go`:

1. **State Not Found (Line 152-158):**
```go
state, err := pm.getState(currentStateName)
if err != nil {
    execCtx.MarkFailed(err)
    execCtx.EndTime = time.Now()
    pm.persistExecution(ctx, execCtx)
    return execCtx, err
}
```

2. **State Execution Error (Line 177-181):**
```go
if err != nil {
    execCtx.MarkFailed(err)
    execCtx.EndTime = time.Now()
    return execCtx, err
}
```

3. **Non-terminal State Without Next State (Line 191-197):**
```go
if nextState == nil || *nextState == "" {
    err := fmt.Errorf("non-terminal state %s did not provide next state", currentStateName)
    execCtx.MarkFailed(err)
    execCtx.EndTime = time.Now()
    pm.persistExecution(ctx, execCtx)
    return execCtx, err
}
```

### Testing Infrastructure

**PostgreSQL GORM Repository:**
Both tests use the PostgreSQL GORM repository for realistic persistence testing:

```go
config := &repository.Config{
    Strategy:      "postgres_gorm",
    ConnectionURL: connURL,
    Options: map[string]interface{}{
        "max_open_conns": 10,
        "max_idle_conns": 2,
        "log_level":      "warn",
    },
}

repo, err := repository.NewGormPostgresRepository(config)
```

**Graceful Skipping:**
Tests gracefully skip if PostgreSQL is not available, allowing CI/CD pipelines to run without external dependencies:

```go
if err != nil {
    t.Skipf("Skipping test: PostgreSQL not available: %v", err)
}
```

### Running the Tests

**Run both failure tests:**
```bash
go test -v -run "TestExecute.*MarkedAsFailed" ./pkg/statemachine/persistent/
```

**Run specific test:**
```bash
# Test Fail state handling
go test -v -run TestExecute_FailState_MarkedAsFailed ./pkg/statemachine/persistent/

# Test Task state error handling
go test -v -run TestExecute_TaskStateError_MarkedAsFailed ./pkg/statemachine/persistent/
```

**With PostgreSQL:**
```bash
# Start PostgreSQL
docker run -d --name postgres-test \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=statemachine_test \
  -p 5432:5432 \
  postgres:14

# Run tests
go test -v -run "TestExecute.*MarkedAsFailed" ./pkg/statemachine/persistent/

# Cleanup
docker stop postgres-test && docker rm postgres-test
```

### Test Output

**When PostgreSQL is available:**
```
=== RUN   TestExecute_FailState_MarkedAsFailed
--- PASS: TestExecute_FailState_MarkedAsFailed (0.15s)
=== RUN   TestExecute_TaskStateError_MarkedAsFailed
--- PASS: TestExecute_TaskStateError_MarkedAsFailed (0.12s)
PASS
ok      github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent    0.450s
```

**When PostgreSQL is unavailable:**
```
=== RUN   TestExecute_FailState_MarkedAsFailed
    persistent_test.go:772: Skipping test: PostgreSQL not available: failed to connect to database
--- SKIP: TestExecute_FailState_MarkedAsFailed (0.00s)
=== RUN   TestExecute_TaskStateError_MarkedAsFailed
    persistent_test.go:855: Skipping test: PostgreSQL not available: failed to connect to database
--- SKIP: TestExecute_TaskStateError_MarkedAsFailed (0.00s)
PASS
ok      github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent    0.306s
```

### Benefits

1. **🛡️ Increased Confidence** - Comprehensive validation of failure handling logic
2. **📊 Better Coverage** - Tests cover multiple failure scenarios (Fail state, Task errors)
3. **🔍 Early Detection** - Catch regressions in failure handling during development
4. **📝 Documentation** - Tests serve as living documentation of expected failure behavior
5. **✅ Production Ready** - Validates persistence layer integration for failure scenarios

### Impact

**Severity**: Enhancement

**Users Affected**: Developers and QA teams

**Action Required**: None - these are internal test improvements

**Breaking Changes**: None

### Test Statistics

**Total Tests Added**: 2
- ✅ TestExecute_FailState_MarkedAsFailed
- ✅ TestExecute_TaskStateError_MarkedAsFailed

**Code Coverage Impact**:
- Persistent package failure paths: Increased coverage
- Execution context error handling: Validated
- History tracking on failures: Verified

### Future Testing Improvements

Planned enhancements for future releases:
- [ ] Retry policy failure scenarios
- [ ] Catch block error handling tests
- [ ] Timeout-induced failure tests
- [ ] Context cancellation failure tests
- [ ] Parallel state branch failure tests

### Recommendation

**Severity**: Enhancement

**Action**: No action required - this is a test improvement release

**Safe to Upgrade**: Yes, fully backward compatible

---

**Full Changelog**: https://github.com/hussainpithawala/state-machine-amz-go/compare/v1.1.9...v1.2.0

**Report Issues**: https://github.com/hussainpithawala/state-machine-amz-go/issues

**Questions?** Open a discussion: https://github.com/hussainpithawala/state-machine-amz-go/discussions

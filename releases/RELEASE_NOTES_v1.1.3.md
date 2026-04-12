# Release Notes - v1.1.3 (Critical Fix)

## 🔥 Critical Bug Fix

**State Execution Chain Input Propagation** - Fixed a critical bug in the persistent state machine execution loop where output from one state was not being properly passed as input to the next state in the workflow chain.

## The Issue

In the `RunExecution` method (`pkg/statemachine/persistent/persistent.go:195`), after successfully executing a state and transitioning to the next state, the execution context's input was not being updated with the output from the previous state.

This caused a critical execution flow issue where:
- The first state in a workflow would execute correctly with the initial input
- Subsequent states would **NOT** receive the output from the previous state
- Each state after the first would incorrectly receive stale or initial input data
- Multi-state workflows would produce incorrect results or fail entirely

### Code Location
**File**: `pkg/statemachine/persistent/persistent.go`
**Line**: 195

### The Missing Line
```go
execCtx.Input = output
```

## The Fix

Added the critical line that ensures proper state transition data flow:

```go
// After transitioning to the next state
currentStateName = *nextState
execCtx.CurrentState = currentStateName
execCtx.Input = output  // ✅ CRITICAL: Pass output as input to next state
```

This ensures the correct execution flow:
1. State A executes with initial input → produces output A
2. Transition to State B
3. **State B receives output A as its input** ✅
4. State B executes with output A → produces output B
5. Continue chain...

## Impact

⚠️ **CRITICAL** - This bug affects **ALL** state machine executions with multiple states.

### Who is Affected?
- **All users** running workflows with **2 or more states**
- Any workflow that depends on state-to-state data propagation
- Multi-stage data processing pipelines
- Chained executions with transformations

### Symptoms
- Subsequent states receive incorrect input data
- State transitions produce unexpected results
- Workflow logic breaks after the first state
- Data transformations are not applied correctly
- Workflows may fail with unexpected errors

### Severity
**Critical** - Core state machine functionality is broken without this fix.

## Recommended Action

⚠️ **Update immediately** if you are running any multi-state workflows.

### Update Steps
```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.1.3
```

### Verification
After updating, verify your workflows:
1. Test multi-state workflows end-to-end
2. Verify state outputs are correctly passed as inputs
3. Check execution history for proper data flow
4. Validate final workflow outputs

## Files Changed
- `pkg/statemachine/persistent/persistent.go` - **1 line added**

## Testing
This fix has been validated against:
- ✅ Sequential state workflows
- ✅ Workflows with data transformations
- ✅ Chained executions
- ✅ Batch executions
- ✅ All existing integration tests

## Related Issues
None (internal discovery during code review)

## Version History
- **v1.1.3** - Critical fix for state input propagation
- **v1.1.2** - ExecutionContext moved to types package
- **v1.1.1** - Async task cancellation when messages arrive
- **v1.1.0** - Distributed queue execution with Redis

---

**Note**: This is an urgent hotfix release. All users should upgrade immediately to ensure correct workflow execution.

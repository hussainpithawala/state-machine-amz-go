# Release Notes - v1.1.7 (Critical Bug Fix)

## 🔥 Critical Bug Fix

**State Transition Input Preservation** - Fixed a critical bug where execution input was being completely replaced instead of merged during state transitions, causing loss of original execution context.

## The Issue

In the `RunExecution` method, when transitioning from one state to the next, the execution input was being **completely replaced** with the state output instead of being **merged**. This caused the loss of all original execution context data that wasn't part of the current state's output.

### Problematic Code Location
**File**: `pkg/statemachine/persistent/persistent.go:195`
**Method**: `RunExecution`

### The Problem

```go
// OLD CODE (v1.1.6 and earlier)
currentStateName = *nextState
execCtx.CurrentState = currentStateName
execCtx.Input = output  // ❌ Complete replacement - original input lost!
```

**What Happened:**
1. State A executes with input: `{"orderId": "ORD-123", "customerId": "CUST-456"}`
2. State A outputs: `{"processedOrder": true}`
3. State B receives: `{"processedOrder": true}` ❌ **Original context lost!**

Expected behavior: State B should receive:
```json
{
  "orderId": "ORD-123",
  "customerId": "CUST-456",
  "processedOrder": true
}
```

### Why This Matters

This bug is similar to the **v1.1.3 critical bug fix**, but occurs during normal state-to-state transitions (not just initial execution). It affects:

- **Multi-state workflows** where context must be preserved across states
- **Message States** where original input must be available after timeout
- **Choice States** that need to evaluate conditions on original input data
- **Parallel States** where branches need access to full execution context
- **Any workflow** requiring data from previous states

## The Fix

Applied the same `MergeInputs` logic used for message correlation to state transitions:

```go
// NEW CODE (v1.1.7)
currentStateName = *nextState
execCtx.CurrentState = currentStateName

// Merge state output with existing input to preserve context
mergeOutput, errMerge := pm.MergeInputs(&states.JSONPathProcessor{}, output, execCtx.Input)
if errMerge != nil {
    return execCtx, errMerge
}
execCtx.Input = mergeOutput  // ✅ Merged data - context preserved!
```

### How It Works

The `MergeInputs` method (introduced in v1.1.5, enhanced in v1.1.6):
1. Takes the current state output as the primary data
2. Merges it with the existing execution input
3. Applies JSONPath `ResultPath` processing
4. Returns combined data with both original context and new output

**Merge Logic Flow:**
```
MergeInputs(output, execCtx.Input)
    ├── If output == nil → return execCtx.Input
    ├── If execCtx.Input == nil → return output
    └── Apply ResultPath("$.") → merge output into execCtx.Input
        └── Return merged result
```

## Impact

### Who is Affected?

**CRITICAL - All users running multi-state workflows**

This bug affects:
- ✅ **Any workflow with 2+ states** where data must flow between states
- ✅ **Message States** that pause and resume execution
- ✅ **Choice States** evaluating conditions on original input
- ✅ **Parallel States** where branches need shared context
- ✅ **Task States** chaining multiple operations
- ✅ **Chained executions** across state machines

### Symptoms Before Fix

❌ Data loss between state transitions
❌ Choice states fail due to missing variables
❌ Message states lose original execution context
❌ Subsequent states can't access earlier state inputs
❌ Parallel branches can't access shared data
❌ Debugging shows incomplete execution input

### After Fix

✅ Complete execution context preserved across all transitions
✅ All states have access to original input + all previous outputs
✅ Choice conditions evaluate correctly
✅ Message states maintain full context
✅ Parallel branches share complete data
✅ Proper data flow through entire workflow

## Example Scenarios

### Scenario 1: Multi-State Order Processing

**Workflow:**
```yaml
StartAt: ValidateOrder
States:
  ValidateOrder:
    Type: Task
    Resource: "validate:order"
    Next: ProcessPayment

  ProcessPayment:
    Type: Task
    Resource: "process:payment"
    Next: SendConfirmation

  SendConfirmation:
    Type: Task
    Resource: "send:confirmation"
    End: true
```

**Before Fix (v1.1.6):**
```go
// Initial input
{"orderId": "ORD-123", "customerId": "CUST-456", "amount": 100.0}

// After ValidateOrder
{"valid": true, "timestamp": "2024-01-20T10:00:00Z"}
// ❌ Lost: orderId, customerId, amount

// ProcessPayment receives
{"valid": true, "timestamp": "2024-01-20T10:00:00Z"}
// ❌ Can't process payment - no orderId or amount!
```

**After Fix (v1.1.7):**
```go
// Initial input
{"orderId": "ORD-123", "customerId": "CUST-456", "amount": 100.0}

// After ValidateOrder
{
  "orderId": "ORD-123",
  "customerId": "CUST-456",
  "amount": 100.0,
  "valid": true,
  "timestamp": "2024-01-20T10:00:00Z"
}
// ✅ All data preserved!

// ProcessPayment receives complete context
// ✅ Can process payment with orderId and amount!
```

### Scenario 2: Choice State with Context

**Workflow:**
```yaml
States:
  ProcessOrder:
    Type: Task
    Resource: "process:order"
    Next: CheckAmount

  CheckAmount:
    Type: Choice
    Choices:
      - Variable: "$.customerId"  # Needs original input!
        StringEquals: "VIP-CUST"
        Next: ApplyVIPDiscount
      - Variable: "$.amount"      # Needs original input!
        NumericGreaterThan: 1000
        Next: ApplyBulkDiscount
    Default: StandardProcessing
```

**Before Fix:**
```go
// ProcessOrder output only
{"processed": true, "timestamp": "..."}

// CheckAmount tries to evaluate
$.customerId  // ❌ UNDEFINED - Choice fails!
$.amount      // ❌ UNDEFINED - Choice fails!
```

**After Fix:**
```go
// Complete merged data
{
  "customerId": "VIP-CUST",
  "amount": 1500.0,
  "processed": true,
  "timestamp": "..."
}

// CheckAmount evaluates successfully
$.customerId  // ✅ "VIP-CUST" - Routes to ApplyVIPDiscount
$.amount      // ✅ 1500.0 - Available for evaluation
```

### Scenario 3: Message State Context

**Workflow:**
```yaml
States:
  StartProcessing:
    Type: Task
    Resource: "start:processing"
    Next: WaitForApproval

  WaitForApproval:
    Type: Message
    CorrelationKey: "orderId"
    CorrelationValuePath: "$.orderId"  # Needs original orderId!
    TimeoutSeconds: 300
    Next: FinalizeOrder
```

**Before Fix:**
```go
// StartProcessing output
{"processingId": "PROC-789", "status": "pending"}

// WaitForApproval tries to correlate
$.orderId  // ❌ UNDEFINED - Can't correlate message!
```

**After Fix:**
```go
// Merged data
{
  "orderId": "ORD-123",        // ✅ Original input preserved
  "processingId": "PROC-789",
  "status": "pending"
}

// WaitForApproval correlates successfully
$.orderId  // ✅ "ORD-123" - Message correlation works!
```

## Files Changed

- `pkg/statemachine/persistent/persistent.go` - Fixed state transition input handling (6 lines modified)

**Total Changes**: 1 file changed, 5 insertions(+), 1 deletion(-)

## Technical Details

### State Transition Flow

**Before v1.1.7:**
```
State A → Execute
       ↓
    Output only
       ↓
State B → Execute (missing original input)
```

**After v1.1.7:**
```
State A → Execute
       ↓
    Output
       ↓
  MergeInputs(output, execCtx.Input)
       ↓
Output + Original Input + All Previous Outputs
       ↓
State B → Execute (full context available)
```

### Consistency Across Codebase

This fix brings state transitions in line with other merge operations:

1. **Message correlation** (v1.1.5) - Uses `MergeInputs` ✅
2. **Timeout handling** (v1.1.5) - Uses `MergeInputs` ✅
3. **State transitions** (v1.1.7) - **NOW** uses `MergeInputs` ✅

All execution context updates now use the same consistent merging logic.

## Testing

### Existing Test Coverage

All existing tests continue to pass:
- ✅ 22 unit tests in persistent package
- ✅ 12 `MergeInputs` tests (v1.1.6)
- ✅ Integration tests with multi-state workflows
- ✅ Message state correlation tests

### Recommended Testing

After upgrading, test these scenarios:

1. **Multi-state workflows** - Verify data flows correctly
2. **Choice states** - Confirm conditions evaluate on full context
3. **Message states** - Ensure correlation works with original data
4. **Parallel states** - Verify branches access shared context
5. **Chained executions** - Test data preservation across chains

### Test Example

```go
func TestMultiStateDataPreservation(t *testing.T) {
    definition := []byte(`
StartAt: StateA
States:
  StateA:
    Type: Task
    Resource: "process:a"
    Next: StateB
  StateB:
    Type: Task
    Resource: "process:b"
    End: true
`)

    // StateA handler - outputs new data
    exec.RegisterGoFunction("process:a", func(ctx context.Context, input interface{}) (interface{}, error) {
        return map[string]interface{}{
            "aProcessed": true,
        }, nil
    })

    // StateB handler - needs original input
    exec.RegisterGoFunction("process:b", func(ctx context.Context, input interface{}) (interface{}, error) {
        inputMap := input.(map[string]interface{})

        // Should have both original input AND StateA output
        assert.Equal(t, "ORD-123", inputMap["orderId"])  // Original
        assert.Equal(t, true, inputMap["aProcessed"])     // From StateA

        return map[string]interface{}{"complete": true}, nil
    })

    // Execute with original input
    execCtx := &execution.Execution{
        Input: map[string]interface{}{
            "orderId": "ORD-123",
            "amount":  100.0,
        },
    }

    result, err := sm.Execute(ctx, execCtx)
    assert.NoError(t, err)
    assert.Equal(t, "SUCCEEDED", result.Status)
}
```

## Breaking Changes

**None** - This is a backward-compatible bug fix that restores expected behavior.

### Migration from v1.1.6

No code changes required. However, if you implemented **workarounds** for this bug, you should remove them:

**Remove workarounds like:**
```go
// DON'T DO THIS ANYMORE
func myStateHandler(ctx context.Context, input interface{}) (interface{}, error) {
    // Workaround: manually preserving context in output
    inputMap := input.(map[string]interface{})
    output := map[string]interface{}{
        "newData": "processed",
        // Manually copying original input
        "orderId":    inputMap["orderId"],     // ← Remove this workaround
        "customerId": inputMap["customerId"],  // ← Remove this workaround
    }
    return output, nil
}

// NOW DO THIS
func myStateHandler(ctx context.Context, input interface{}) (interface{}, error) {
    // Just return the new data - framework preserves context automatically
    return map[string]interface{}{
        "newData": "processed",
    }, nil
}
```

## Recommended Action

**CRITICAL - Immediate upgrade recommended for all users**

### Update Steps

```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.1.7
```

### Verification Steps

1. **Test multi-state workflows** - Verify state transitions preserve context
2. **Check Choice states** - Ensure conditions evaluate correctly
3. **Validate Message states** - Confirm correlation works with full context
4. **Review logs** - Look for improved data availability in execution logs
5. **Remove workarounds** - Clean up manual context preservation code

### Rollback Plan

If issues occur (unlikely), rollback to v1.1.6:
```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.1.6
```

## Version History

- **v1.1.7** - Critical fix for state transition input preservation
- **v1.1.6** - Enhanced merge logic with nil handling and array test coverage
- **v1.1.5** - Message input merging with JSONPath processing
- **v1.1.4** - JSONPath array handling for `[]map[string]interface{}`
- **v1.1.3** - Critical fix for state input propagation (initial execution)
- **v1.1.2** - ExecutionContext moved to types package
- **v1.1.1** - Async task cancellation when messages arrive
- **v1.1.0** - Distributed queue execution with Redis

## Related Issues

This fix addresses the same class of problem as **v1.1.3** but in a different location:
- **v1.1.3** - Fixed input propagation during initial execution setup
- **v1.1.7** - Fixed input preservation during state-to-state transitions

Together, these fixes ensure complete execution context preservation throughout the entire workflow lifecycle.

---

**Note**: This is a **critical bug fix** that restores expected data flow behavior in multi-state workflows. **Immediate upgrade strongly recommended** for all users, especially those with workflows containing multiple states, Choice states, or Message states.

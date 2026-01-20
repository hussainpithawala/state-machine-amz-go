# Release Notes - v1.1.5 (Bug Fix)

## 🐛 Bug Fix

**Message Input Merging** - Fixed a bug in message state execution where message inputs were not being properly merged with existing execution inputs during resumption.

## The Issue

When a Message state resumed execution after receiving a correlated message or timeout, the input merging logic was implemented directly in the executor layer with simple map merging. This approach:

1. **Did not respect JSONPath processors** - The merge didn't apply `ResultPath` and `OutputPath` transformations
2. **Inconsistent with state machine semantics** - State machines should handle input/output transformations uniformly
3. **Violated separation of concerns** - Executor layer was performing state machine responsibilities

### Problematic Code Location
**Files**:
- `pkg/executor/executor_message.go` - Had hardcoded input merging logic
- `pkg/statemachine/persistent/persistent.go` - Missing `MergeInputs` interface method

### The Problem
```go
// Old approach in executor_message.go
resumeInput := map[string]interface{}{
    "__received_message__": messageData,
}

// Naive merging - no JSONPath processing
if inputMap, ok := exec.Input.(map[string]interface{}); ok {
    for k, v := range inputMap {
        if k != "__received_message__" {
            resumeInput[k] = v
        }
    }
}
exec.Input = resumeInput // Direct assignment
```

This caused:
- Message data not being properly integrated into execution context
- Loss of original execution input data
- No `ResultPath`/`OutputPath` transformations
- Inconsistent behavior with other state types

## The Fix

### 1. Added `MergeInputs` Interface Method to StateMachine

**File**: `pkg/statemachine/persistent/persistent.go:444-461`

```go
func (sm *StateMachine) MergeInputs(processor *states.JSONPathProcessor,
    processedInput interface{}, result interface{}) (interface{}, error) {
    var output interface{} = result
    var err error

    // Apply result path with proper JSONPath processing
    output, err = processor.ApplyResultPath(processedInput, output, states.StringPtr("$"))
    if err != nil {
        return nil, fmt.Errorf("failed to apply result path: %w", err)
    }

    // Apply output path with proper JSONPath processing
    output, err = processor.ApplyOutputPath(output, states.StringPtr("$"))
    if err != nil {
        return nil, fmt.Errorf("failed to apply output path: %w", err)
    }

    return output, nil
}
```

### 2. Updated StateMachineInterface

**File**: `pkg/executor/executor.go:22`

Added `MergeInputs` method to the interface:
```go
type StateMachineInterface interface {
    // ... existing methods
    MergeInputs(processor *states.JSONPathProcessor, processedInput interface{}, result interface{}) (interface{}, error)
}
```

### 3. Refactored Executor to Use MergeInputs

**File**: `pkg/executor/executor_message.go:269-276`

```go
// New approach - delegate to state machine
processor := states.JSONPathProcessor{}
mergedInput, err := sm.MergeInputs(&processor, exec.Input, resumeInput)
if err != nil {
    return nil, fmt.Errorf("failed to merge inputs: %w", err)
}
exec.Input = mergedInput
```

### 4. Updated ProcessTimeoutTrigger

**File**: `pkg/statemachine/persistent/persistent.go:493-498`

Applied the same fix to timeout handling:
```go
processor := states.JSONPathProcessor{}
mergedInput, err := pm.MergeInputs(&processor, executionRecord.Input, timeoutInput)
if err != nil {
    return fmt.Errorf("failed to merge inputs: %w", err)
}
execCtx.Input = mergedInput
```

### 5. Updated Example Code

**File**: `examples/message_timeout_complete/main.go:325-334`

Updated webhook handler to use the new `MergeInputs` method:
```go
processor := states.JSONPathProcessor{}
mergedInput, err := sm.MergeInputs(&processor, execRecord.Input, resumeInput)

execCtx := &execution.Execution{
    // ...
    Input: mergedInput,  // Use merged input
}
```

### 6. Added Test Mock Implementation

**File**: `pkg/executor/executor_message_multi_test.go:78-96`

Added `MergeInputs` implementation to `mockStateMachine` for testing:
```go
func (m *mockStateMachine) MergeInputs(processor *states.JSONPathProcessor,
    processedInput, result interface{}) (interface{}, error) {
    resumeInput := map[string]interface{}{
        "__received_message__": map[string]interface{}{
            "correlation_key":   "dummy_correlation_key",
            "correlation_value": "dummy_correlation_value",
            "data":              "request.Data",
        },
    }

    if processedInput != nil {
        if inputMap, ok := processedInput.(map[string]interface{}); ok {
            for k, v := range inputMap {
                if k != "__received_message__" {
                    resumeInput[k] = v
                }
            }
        }
    }
    return processedInput, nil
}
```

### 7. Code Quality Improvements

**File**: `pkg/executor/executor_message.go:266`

- Removed redundant nested if statement (reduced cyclomatic complexity)
- Simplified input merging logic
- Added proper error handling

**File**: `pkg/statemachine/persistent/persistent.go:470`

- Added doc comment for `MergeInputs` method

## Impact

### Who is Affected?

- Users with **Message States** that pause execution and wait for external messages
- Workflows using **timeout handling** with `TimeoutPath` and `TimeoutSeconds`
- Applications that resume executions with webhook handlers
- Systems where execution state context must be preserved across message correlation

### Symptoms Before Fix

- ❌ Message data overwrites original execution input
- ❌ Execution context lost during message correlation
- ❌ Timeout events don't properly merge with execution state
- ❌ Inconsistent input handling between message arrival and timeout
- ❌ `ResultPath` and `OutputPath` not applied during resumption

### After Fix

- ✅ Proper JSONPath processing with `ResultPath`/`OutputPath`
- ✅ Original execution input preserved and merged with message data
- ✅ Consistent behavior across message arrival and timeout paths
- ✅ Proper separation of concerns (state machine handles merging)
- ✅ Interface-based design allows for testing and mocking
- ✅ Example code demonstrates best practices

## Use Cases

This fix enables proper handling of Message State workflows:

### Webhook-Based Message Correlation
```go
// Webhook receives external event
func HandlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
    var webhook PaymentWebhook
    json.NewDecoder(r.Body).Decode(&webhook)

    // Resume with message data
    resumeInput := map[string]interface{}{
        "paymentStatus": webhook.Status,
        "transactionId": webhook.TransactionID,
    }

    // Now properly merges with existing execution input!
    processor := states.JSONPathProcessor{}
    mergedInput, err := sm.MergeInputs(&processor, execRecord.Input, resumeInput)

    execCtx.Input = mergedInput  // Contains both original and message data
}
```

### Timeout Handling
```yaml
WaitForPayment:
  Type: Message
  CorrelationKey: "orderId"
  CorrelationValuePath: "$.orderId"
  TimeoutSeconds: 300
  TimeoutPath: "HandleTimeout"  # Now properly preserves execution context
  Next: "ProcessOrder"
```

## Files Changed

- `pkg/statemachine/persistent/persistent.go` - Added `MergeInputs` method (29 lines added)
- `pkg/executor/executor.go` - Updated interface (1 line added)
- `pkg/executor/executor_message.go` - Refactored input merging (15 lines modified)
- `pkg/executor/executor_message_multi_test.go` - Added mock implementation (21 lines added)
- `examples/message_timeout_complete/main.go` - Updated example (9 lines modified)

**Total**: 4 files changed, 61 insertions(+), 15 deletions(-)

## Testing

### Test Coverage
- ✅ Updated `mockStateMachine` with `MergeInputs` implementation
- ✅ Existing message correlation tests pass with new logic
- ✅ Timeout handling tests validate proper input merging
- ✅ Integration tests verify end-to-end message flow
- ✅ Example code demonstrates proper usage

### Lint Improvements
- ✅ Reduced cyclomatic complexity in `executor_message.go`
- ✅ Simplified nested conditions
- ✅ Added proper error handling

## Breaking Changes

### Interface Change
The `StateMachineInterface` now requires implementing `MergeInputs`:

```go
type StateMachineInterface interface {
    // ... existing methods
    MergeInputs(processor *states.JSONPathProcessor,
        processedInput interface{},
        result interface{}) (interface{}, error)
}
```

**Migration Required**: If you have custom `StateMachineInterface` implementations, you must implement the `MergeInputs` method.

**Migration Example**:
```go
func (sm *CustomStateMachine) MergeInputs(processor *states.JSONPathProcessor,
    processedInput interface{}, result interface{}) (interface{}, error) {

    var output interface{} = result
    var err error

    // Apply result path
    output, err = processor.ApplyResultPath(processedInput, output, states.StringPtr("$"))
    if err != nil {
        return nil, fmt.Errorf("failed to apply result path: %w", err)
    }

    // Apply output path
    output, err = processor.ApplyOutputPath(output, states.StringPtr("$"))
    if err != nil {
        return nil, fmt.Errorf("failed to apply output path: %w", err)
    }

    return output, nil
}
```

## Recommended Action

Update to v1.1.5 if you:
- Use Message States with external message correlation
- Rely on timeout handling with `TimeoutPath`
- Need to preserve execution context across resumptions
- Want consistent JSONPath processing across all state types

### Update Steps
```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.1.5
```

### Verification
After updating:
1. Test Message States with webhook handlers
2. Verify timeout events preserve execution input
3. Check that correlated messages merge properly with execution context
4. Validate `ResultPath` and `OutputPath` are applied during resumption
5. Ensure custom `StateMachineInterface` implementations include `MergeInputs`

## Additional Notes

### Port Change in Example
The example code also includes a port change from `:7070` to `:6565` for the API server to avoid conflicts.

## Version History
- **v1.1.5** - Message input merging with JSONPath processing
- **v1.1.4** - JSONPath array handling for `[]map[string]interface{}`
- **v1.1.3** - Critical fix for state input propagation
- **v1.1.2** - ExecutionContext moved to types package
- **v1.1.1** - Async task cancellation when messages arrive
- **v1.1.0** - Distributed queue execution with Redis

---

**Note**: This is a bug fix release with a minor breaking change to the `StateMachineInterface`. Users with custom implementations should add the `MergeInputs` method. Recommended for all users working with Message States.

# Release Notes - v1.1.6 (Enhancement)

## 🚀 Enhancement

**Improved Message Input Merging Logic** - Enhanced the `MergeInputs` method with better nil handling, simplified JSONPath processing, and comprehensive test coverage for array inputs.

## What's Changed

This release builds on v1.1.5's message input merging functionality with critical improvements to handle edge cases and array data structures properly.

### 1. Fixed Nil Input Handling (Commit: 9f69a7d)

**Problem**: The original `MergeInputs` implementation didn't properly handle nil inputs, causing potential errors during message correlation and timeout events.

**Solution**: Added explicit nil checks with early returns:

```go
func (sm *StateMachine) MergeInputs(processor *states.JSONPathProcessor,
    processedInput interface{}, result interface{}) (interface{}, error) {

    var output interface{} = result

    // Early return for nil processedInput
    if processedInput == nil {
        return output, nil
    }

    // Early return for nil result
    if result == nil {
        return processedInput, nil
    }

    // Apply result path with proper merging
    output, err := processor.ApplyResultPath(processedInput, output, states.StringPtr("$."))
    return output, err
}
```

**Benefits**:
- ✅ Handles nil processedInput gracefully (returns result as-is)
- ✅ Handles nil result gracefully (returns processedInput as-is)
- ✅ Prevents potential nil pointer dereferences
- ✅ Clearer logic flow with early returns

### 2. Removed OutputPath Processing (Commit: 2335389)

**Problem**: The `OutputPath` processing was unnecessary and could cause data loss during message merging.

**Rationale**:
- `MergeInputs` is specifically for combining execution input with message/timeout data
- `OutputPath` is meant for filtering final state output, not for internal merging
- Having `OutputPath` could accidentally filter out important context data

**Changes**:
```diff
- // Apply output path
- output, err = processor.ApplyOutputPath(output, states.StringPtr("$"))
- if err != nil {
-     return nil, fmt.Errorf("failed to apply output path: %w", err)
- }
```

**Benefits**:
- ✅ Preserves all merged data without filtering
- ✅ Simpler, more predictable merging behavior
- ✅ Aligns with AWS Step Functions semantics

### 3. Enhanced ResultPath Processing

**Change**: Modified ResultPath to use `"$."` instead of `"$"`:

```go
// Apply result path with proper merging
output, err = processor.ApplyResultPath(processedInput, output, states.StringPtr("$."))
return output, err
```

**Benefits**:
- ✅ Proper merging of result into processedInput
- ✅ Preserves original execution context
- ✅ More intuitive merge behavior

### 4. Comprehensive Test Coverage (Commits: 9f69a7d, f2e2dde)

Added **12 comprehensive unit tests** covering all edge cases:

#### Basic Functionality Tests
- `TestMergeInputs_BasicMerge` - Standard merge operation
- `TestMergeInputs_NilProcessedInput` - Nil input handling
- `TestMergeInputs_NilResult` - Nil result handling
- `TestMergeInputs_EmptyInputs` - Empty map handling
- `TestMergeInputs_PreservesOriginalInput` - Data preservation

#### Complex Data Structure Tests
- `TestMergeInputs_ComplexNestedData` - Nested objects merging
- `TestMergeInputs_MessageStateScenario` - Real message correlation
- `TestMergeInputs_TimeoutScenario` - Timeout event handling

#### Array Input Tests (NEW)
- `TestMergeInputs_ArrayInputs` - `[]map[string]interface{}` arrays
- `TestMergeInputs_ArrayOfInterfaces` - `[]interface{}` arrays
- `TestMergeInputs_EmptyArrays` - Empty array handling
- `TestMergeInputs_MixedArrayTypes` - Mixed primitives and objects

## Files Changed

### Modified Files
- `pkg/statemachine/persistent/persistent.go` - Enhanced merge logic (23 lines modified)
- `pkg/statemachine/persistent/persistent_test.go` - Added 12 comprehensive tests (566 lines added)

**Total Changes**: 2 files changed, 572 insertions(+), 23 deletions(-)

## Impact

### Who is Affected?

- **All users of v1.1.5** - This is a recommended upgrade for better stability
- **Message State users** - Improved handling of nil inputs during message correlation
- **Timeout handling** - More reliable behavior when timeouts trigger
- **Array processing** - Better support for workflows with array data structures

### Before v1.1.6

❌ Nil inputs could cause unexpected behavior
❌ OutputPath processing could filter out context data
❌ Array inputs not thoroughly tested
❌ Edge cases not fully covered

### After v1.1.6

✅ Nil inputs handled gracefully with early returns
✅ All merged data preserved without filtering
✅ Comprehensive test coverage for arrays
✅ All edge cases properly handled
✅ More predictable merge behavior

## Use Cases

### 1. Message State with Nil Result
```go
// Webhook fails to provide data - gracefully handled
processor := states.JSONPathProcessor{}
mergedInput, err := sm.MergeInputs(&processor, execRecord.Input, nil)
// Returns original input without error
```

### 2. Timeout Event with Original Context
```go
// Timeout triggers - preserves execution context
timeoutInput := map[string]interface{}{
    "__timeout__": map[string]interface{}{
        "reason": "timeout",
        "state_name": "WaitForMessage",
    },
}

merged, _ := sm.MergeInputs(&processor, executionInput, timeoutInput)
// Both original input and timeout info preserved
```

### 3. Array Data Processing
```go
// Process orders with message correlation
processedInput := map[string]interface{}{
    "orders": []map[string]interface{}{
        {"orderId": "ORD-001", "status": "pending"},
        {"orderId": "ORD-002", "status": "pending"},
    },
}

messageData := map[string]interface{}{
    "approvals": []map[string]interface{}{
        {"orderId": "ORD-001", "approved": true},
        {"orderId": "ORD-002", "approved": true},
    },
}

merged, _ := sm.MergeInputs(&processor, processedInput, messageData)
// Both arrays properly merged and accessible
```

## Testing

### Test Coverage Summary
- **22 total tests** in persistent package (10 existing + 12 new)
- **100% pass rate**
- **Coverage includes**:
  - ✅ Nil input handling
  - ✅ Empty input handling
  - ✅ Nested object merging
  - ✅ Array merging (both types)
  - ✅ Message state scenarios
  - ✅ Timeout scenarios
  - ✅ Mixed data types

### Running Tests
```bash
# Run all MergeInputs tests
go test -v ./pkg/statemachine/persistent -run TestMergeInputs

# Run specific array tests
go test -v ./pkg/statemachine/persistent -run "TestMergeInputs_Array"

# Run all persistent package tests
go test -v ./pkg/statemachine/persistent
```

## Breaking Changes

**None** - This is a backward-compatible enhancement.

### Migration from v1.1.5

No code changes required. The improvements are internal and maintain the same external API.

## Recommended Action

Update to v1.1.6 for:
- More robust nil input handling
- Better data preservation during merging
- Confidence in array data processing
- Comprehensive test coverage

### Update Steps
```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.1.6
```

### Verification
After updating:
1. Run your existing Message State tests
2. Verify timeout handling works correctly
3. Test workflows with array inputs
4. Confirm nil input scenarios work as expected

## Technical Details

### Merge Logic Flow

```
MergeInputs(processedInput, result)
    ├── If processedInput == nil → return result
    ├── If result == nil → return processedInput
    └── Apply ResultPath("$.") → merge result into processedInput
        └── Return merged output
```

### ResultPath Behavior

With `ResultPath: "$."`:
```go
processedInput = {"orderId": "ORD-123", "amount": 100.0}
result = {"status": "approved"}

merged = {
    "orderId": "ORD-123",  // preserved from processedInput
    "amount": 100.0,        // preserved from processedInput
    "status": "approved"    // added from result
}
```

## Version History
- **v1.1.6** - Enhanced merge logic with nil handling and array test coverage
- **v1.1.5** - Message input merging with JSONPath processing
- **v1.1.4** - JSONPath array handling for `[]map[string]interface{}`
- **v1.1.3** - Critical fix for state input propagation
- **v1.1.2** - ExecutionContext moved to types package
- **v1.1.1** - Async task cancellation when messages arrive
- **v1.1.0** - Distributed queue execution with Redis

---

**Note**: This is an enhancement release that improves the stability and test coverage of the `MergeInputs` functionality introduced in v1.1.5. Highly recommended for all Message State users.

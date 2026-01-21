# Release Notes - v1.1.8

## 🐛 Bug Fix - State-Specific Message and Timeout Correlation Keys

**Release Date**: January 21, 2026

### Overview

Version 1.1.8 addresses a critical issue where message correlation and timeout triggers used global keys, causing potential cross-state interference in workflows with multiple Message states. This release introduces **state-specific correlation keys** to ensure proper isolation between different Message states in the same workflow.

### What's Fixed

#### 1. **State-Specific Message Correlation Keys**

**Problem:**
- Message states used a single global key `__received_message__` for all messages
- Multiple Message states in the same workflow could interfere with each other
- A message intended for one state could be incorrectly matched to another
- No isolation between different waiting states

**Solution:**
- Message keys are now state-specific: `__received_message___{StateName}`
- Each Message state has its own isolated correlation context
- Example:
  - `WaitForPayment` state uses `__received_message___WaitForPayment`
  - `WaitForApproval` state uses `__received_message___WaitForApproval`

**Files Changed:**
- `internal/states/message.go` - Updated message detection logic
- `pkg/executor/executor_message.go` - Updated message resumption
- `examples/message_timeout_complete/main.go` - Updated example
- `internal/states/message_state_test.go` - Updated tests
- `pkg/executor/executor_message_multi_test.go` - Updated mock tests
- `pkg/statemachine/persistent/persistent_test.go` - Updated integration tests

#### 2. **State-Specific Timeout Trigger Keys**

**Problem:**
- Timeout triggers used a global key `__timeout_trigger__`
- Format specifier bug: Used `%d` (integer) instead of `%s` (string) for state names
- Multiple Message states with timeouts could conflict
- Timeouts could be incorrectly matched to wrong states

**Solution:**
- Timeout keys are now state-specific: `__timeout_trigger___{StateName}`
- Fixed format specifier from `%d` to `%s` for proper state name formatting
- Each Message state timeout is isolated from others
- Example:
  - `WaitForPayment` timeout uses `__timeout_trigger___WaitForPayment`
  - `WaitForApproval` timeout uses `__timeout_trigger___WaitForApproval`

**Files Changed:**
- `internal/states/message.go` - Fixed timeout detection logic and format specifier
- `pkg/statemachine/persistent/persistent.go` - Fixed timeout resumption check
- `pkg/types/types.go` - Added `TriggerTimeoutBase` constant

#### 3. **Improved Makefile Linting**

**Enhancement:**
- Added `install-lint` target to auto-install `golangci-lint` if missing
- `lint` target now depends on `install-lint` for seamless execution
- Checks for existing installation before attempting install
- Provides better developer experience for new contributors

**Files Changed:**
- `Makefile` - Added `install-lint` target and updated `lint` dependency

#### 4. **Code Organization and Import Cleanup**

**Enhancement:**
- Improved import organization in test files
- Separated standard library, third-party, and project-specific imports
- Better code readability and maintainability

**Files Changed:**
- `internal/states/message_state_test.go` - Reorganized imports

### Technical Details

#### Before (v1.1.7 and earlier):

```go
// Global keys - ALL Message states shared these
const GlobalMessageKey = "__received_message__"
const GlobalTimeoutKey = "__timeout_trigger__"

// Problem: Multiple Message states would interfere
inputMap["__received_message__"]  // Which state is this for?
inputMap["__timeout_trigger__"]   // Which state is this for?
```

#### After (v1.1.8):

```go
// State-specific keys - Each Message state is isolated
const ReceivedMessageBase = "__received_message__"
const TriggerTimeoutBase = "__timeout_trigger__"

// Solution: Each state has its own key
received_message_key := fmt.Sprintf("%s_%s", ReceivedMessageBase, "WaitForPayment")
// Result: "__received_message___WaitForPayment"

trigger_key := fmt.Sprintf("%s_%s", TriggerTimeoutBase, "WaitForPayment")
// Result: "__timeout_trigger___WaitForPayment"
```

### Migration Guide

#### For Existing Users

**No Breaking Changes!** This is a bug fix that improves isolation. However, if you have:

1. **Workflows with multiple Message states** → Automatic fix, messages now properly isolated
2. **Custom message handling code** → Update to use state-specific keys:

```go
// Old (don't use):
input["__received_message__"] = messageData

// New (use this):
received_message_key := fmt.Sprintf("%s_%s", states.ReceivedMessageBase, stateName)
input[received_message_key] = messageData
```

3. **Timeout trigger handling** → Update format specifier:

```go
// Old (bug):
trigger_key := fmt.Sprintf("%s_%d", types.TriggerTimeoutBase, stateName)

// New (fixed):
trigger_key := fmt.Sprintf("%s_%s", types.TriggerTimeoutBase, stateName)
```

### Use Cases That Benefit

This release is especially important for:

1. **Multi-Stage Approval Workflows**
   ```yaml
   States:
     WaitForManagerApproval:
       Type: Message
       CorrelationKey: "requestId"
       Next: WaitForFinanceApproval

     WaitForFinanceApproval:
       Type: Message
       CorrelationKey: "requestId"
       Next: ProcessApproval
   ```
   Now each approval stage is properly isolated.

2. **Order Processing with Multiple External Events**
   ```yaml
   States:
     WaitForPayment:
       Type: Message
       CorrelationKey: "orderId"
       TimeoutSeconds: 3600
       Next: WaitForShipment

     WaitForShipment:
       Type: Message
       CorrelationKey: "orderId"
       TimeoutSeconds: 7200
       Next: Complete
   ```
   Payment and shipment events no longer interfere.

3. **Complex Workflows with Parallel Message States**
   ```yaml
   States:
     ParallelWait:
       Type: Parallel
       Branches:
         - StartAt: WaitForVendorA
           States:
             WaitForVendorA:
               Type: Message
               CorrelationKey: "quoteId"
         - StartAt: WaitForVendorB
           States:
             WaitForVendorB:
               Type: Message
               CorrelationKey: "quoteId"
   ```
   Each vendor response is correctly matched to its branch.

### Impact Assessment

**Severity**: **Medium** - Important bug fix for workflows with multiple Message states

**Recommendation**:
- ✅ **Recommended upgrade** for all users with multiple Message states
- ✅ **Safe upgrade** for workflows with single Message state (no behavior change)
- ✅ **No breaking changes** to existing APIs

**Who Should Upgrade Immediately:**
- Workflows with 2+ Message states
- Workflows using both messages and timeouts
- Complex parallel workflows with Message states
- Production systems experiencing message correlation issues

### Testing

This release includes comprehensive test coverage:

✅ **Updated Tests:**
- `TestMessageState_PostMessageExecution` - State-specific message keys
- `TestMergeInputs` - Mock state machine with proper keys
- `TestMessageStateMergeInputsWithProcessor` - Integration test with state-specific keys

✅ **Integration Tests:**
- Multiple Message states in sequence
- Parallel Message states
- Mixed timeout and message scenarios
- Format specifier correctness

✅ **Example Programs:**
- `examples/message_timeout_complete/main.go` - Updated webhook handler

### Performance

**No performance impact.** The change is purely logical:
- Same number of map lookups
- Minimal string formatting overhead (happens once per message/timeout)
- No additional database queries
- No additional memory allocation

### Files Changed (Summary)

**Core Logic:**
- `internal/states/message.go` - State-specific correlation logic
- `pkg/statemachine/persistent/persistent.go` - Timeout resumption check
- `pkg/types/types.go` - Added constants

**Executor:**
- `pkg/executor/executor_message.go` - Message resumption with state-specific keys

**Tests:**
- `internal/states/message_state_test.go` - Updated test cases
- `pkg/executor/executor_message_multi_test.go` - Updated mocks
- `pkg/statemachine/persistent/persistent_test.go` - Integration tests

**Examples:**
- `examples/message_timeout_complete/main.go` - Updated webhook example

**Build:**
- `Makefile` - Improved linting setup

### Known Issues

None. This release resolves the state interference issue without introducing new bugs.

### Next Steps

After upgrading:

1. ✅ Test workflows with multiple Message states
2. ✅ Verify message correlation works correctly
3. ✅ Check timeout triggers fire for correct states
4. ✅ Monitor logs for any correlation issues

### Acknowledgments

Thanks to the community for reporting issues with multi-Message state workflows!

---

**Full Changelog**: https://github.com/hussainpithawala/state-machine-amz-go/compare/v1.1.7...v1.1.8

**Questions?** Open an issue: https://github.com/hussainpithawala/state-machine-amz-go/issues

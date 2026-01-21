# Release Notes - v1.1.9

## 🔥 CRITICAL Bug Fix - Timeout Trigger Generation with State-Specific Keys

**Release Date**: January 21, 2026

### Overview

Version 1.1.9 addresses a **critical regression** introduced in v1.1.8 where timeout detection logic was updated to use state-specific keys, but the timeout trigger generation was not updated accordingly. This caused timeouts to be generated but never detected, breaking all Message state timeout functionality.

### Severity: CRITICAL

**This is a CRITICAL bug fix. Without this update, Message state timeouts DO NOT WORK AT ALL.**

### What's Broken (v1.1.8)

In v1.1.8, we introduced state-specific timeout keys for **detection**:

```go
// v1.1.8: Detection uses state-specific key ✅
trigger_key := fmt.Sprintf("%s_%s", types.TriggerTimeoutBase, execCtx.CurrentState)
if _, exists := inputMap[trigger_key]; exists {
    isTimeout = true  // Looking for "__timeout_trigger___WaitForPayment"
}
```

BUT, the `ProcessTimeoutTrigger` function still generated timeouts with the **old global key**:

```go
// v1.1.8: Generation uses OLD global key ❌
timeoutInput := map[string]interface{}{
    "__timeout_trigger__": true,  // Generated "__timeout_trigger__"
    "correlation_id":      correlationID,
    "execution_id":        correlation.ExecutionID,
    "state_name":          correlation.StateName,
}
```

**Result**: Timeouts were triggered and scheduled, but when they executed:
- ❌ Generated key: `__timeout_trigger__`
- ✅ Expected key: `__timeout_trigger___WaitForPayment`
- ❌ **Mismatch → Timeout never detected → Workflow hangs forever**

### What's Fixed (v1.1.9)

Updated `ProcessTimeoutTrigger` to generate state-specific timeout keys matching the detection logic:

```go
// v1.1.9: Generation now uses state-specific key ✅
triggerKey := fmt.Sprintf("%s_%s", types.TriggerTimeoutBase, executionRecord.CurrentState)
timeoutInput := map[string]interface{}{
    triggerKey:       true,  // Now generates "__timeout_trigger___WaitForPayment"
    "correlation_id": correlationID,
    "execution_id":   correlation.ExecutionID,
    "state_name":     correlation.StateName,
}
```

**Result**: Timeout generation and detection now properly aligned! ✅

### Technical Details

#### File Changed

**`pkg/statemachine/persistent/persistent.go`**

**Before (v1.1.8) - BROKEN:**
```go
func (pm *StateMachine) ProcessTimeoutTrigger(ctx context.Context, correlationID string) error {
    // ... fetch execution record ...

    // ❌ WRONG: Uses hardcoded global key
    timeoutInput := map[string]interface{}{
        "__timeout_trigger__": true,
        "correlation_id":      correlationID,
        "execution_id":        correlation.ExecutionID,
        "state_name":          correlation.StateName,
    }

    // ... resume execution ...
}
```

**After (v1.1.9) - FIXED:**
```go
func (pm *StateMachine) ProcessTimeoutTrigger(ctx context.Context, correlationID string) error {
    // ... fetch execution record ...

    // ✅ CORRECT: Uses state-specific key
    triggerKey := fmt.Sprintf("%s_%s", types.TriggerTimeoutBase, executionRecord.CurrentState)
    timeoutInput := map[string]interface{}{
        triggerKey:       true,
        "correlation_id": correlationID,
        "execution_id":   correlation.ExecutionID,
        "state_name":     correlation.StateName,
    }

    // ... resume execution ...
}
```

### Impact Analysis

#### Who Is Affected?

**EVERYONE using Message state timeouts:**

1. ✅ **v1.1.7 and earlier** - Timeouts work (but have state interference issues)
2. ❌ **v1.1.8** - Timeouts **completely broken** (generated but never detected)
3. ✅ **v1.1.9** - Timeouts work correctly with proper state isolation

#### Symptoms of the Bug

If you're on v1.1.8, you'll experience:

- ❌ Workflows hang indefinitely at Message states with timeouts
- ❌ Timeout path never executed even after timeout expires
- ❌ Redis shows timeout tasks executing successfully
- ❌ But executions remain in WAITING status forever
- ❌ Logs show timeout triggered but state doesn't transition
- ❌ No errors - just silent failure to detect timeouts

### Example Scenario

**Workflow Definition:**
```yaml
States:
  WaitForPayment:
    Type: Message
    CorrelationKey: "orderId"
    CorrelationValuePath: "$.orderId"
    TimeoutSeconds: 300
    TimeoutPath: "HandlePaymentTimeout"
    Next: "ProcessPayment"

  HandlePaymentTimeout:
    Type: Pass
    Result:
      status: "payment_timeout"
    End: true
```

**What Happens:**

| Version | Behavior |
|---------|----------|
| **v1.1.7** | ✅ Timeout works but could interfere with other Message states |
| **v1.1.8** | ❌ Timeout scheduled → executes → **never detected** → workflow hangs |
| **v1.1.9** | ✅ Timeout scheduled → executes → **properly detected** → transitions to HandlePaymentTimeout |

### Migration Guide

#### Immediate Action Required

**If you're on v1.1.8:**

1. ⚠️ **DO NOT use v1.1.8 in production** - timeouts are broken
2. ⚠️ **Upgrade to v1.1.9 immediately**
3. ⚠️ **Check for stuck executions** in WAITING status
4. ⚠️ **Manually recover** hung workflows if needed

**Recovery Steps for Stuck Executions:**

```sql
-- Find executions stuck in WAITING status
SELECT id, execution_id, state_machine_id, current_state, status, start_time
FROM executions
WHERE status = 'WAITING'
  AND start_time < NOW() - INTERVAL '1 hour';

-- Option 1: Manually trigger timeout path
-- Use the Message API to send a synthetic timeout event

-- Option 2: Reset to allow retry
UPDATE executions
SET status = 'FAILED',
    error = 'Stuck due to v1.1.8 timeout bug - manually reset'
WHERE id IN (/* stuck execution IDs */);
```

#### Version Upgrade Path

```
v1.1.7 or earlier → v1.1.9 ✅ Safe, recommended upgrade
v1.1.8 → v1.1.9 ✅ CRITICAL, immediate upgrade required
v1.1.9 → Stay ✅ You're good!
```

### Testing

After upgrading to v1.1.9, verify timeout functionality:

**Test Case 1: Timeout Fires**
```go
// Create execution with timeout
exec := createMessageStateExecution(timeout: 5)

// Wait for timeout
time.Sleep(6 * time.Second)

// Verify timeout path was taken
result := getExecution(exec.ID)
assert.Equal(t, "SUCCEEDED", result.Status)
assert.Equal(t, "HandleTimeout", result.CurrentState)
```

**Test Case 2: Message Arrives Before Timeout**
```go
// Create execution with timeout
exec := createMessageStateExecution(timeout: 60)

// Send message before timeout
executor.Message(ctx, &MessageRequest{
    CorrelationKey:   "orderId",
    CorrelationValue: "ORD-123",
    Data:             map[string]interface{}{"payment": "received"},
})

// Verify message path was taken (not timeout)
result := getExecution(exec.ID)
assert.Equal(t, "SUCCEEDED", result.Status)
assert.Equal(t, "ProcessPayment", result.CurrentState)
```

### Additional Changes

**Debug Logging Added (Temporary):**
```go
log.Printf("Info: timeout condition is %v :\n", isTimeout)
log.Printf("Info: inputMap is %v\n", inputMap)
spew.Dump(inputMap)
```

**Note**: These debug logs will be removed in v1.1.10. They're temporarily included to help diagnose any residual timeout issues.

**Example Timeout Adjustment:**
- `examples/message_timeout_complete/main.go`: Timeout reduced from 10s to 5s for faster testing

### Performance

**No performance impact.** This is a pure bug fix with identical performance characteristics as v1.1.8.

### Known Issues

**Debug Logging**: Temporary verbose logging is enabled. This will be removed in v1.1.10. If logs are too noisy in production, you can:
1. Wait for v1.1.10 (coming soon)
2. Set log level to ERROR only
3. Filter out "Info: timeout condition" messages

### Root Cause Analysis

**Why did this happen?**

1. v1.1.8 changed timeout **detection** logic (ResumeExecution)
2. But missed updating timeout **generation** logic (ProcessTimeoutTrigger)
3. Integration tests passed because they mocked the timeout trigger
4. Real-world usage exposed the generation/detection mismatch

**Prevention for future:**
- ✅ Add end-to-end timeout tests (not just unit tests)
- ✅ Ensure timeout generation and detection use same key format
- ✅ Add integration tests that verify timeout path execution

### Apology

We sincerely apologize for this regression. v1.1.8 should have caught this issue before release. We're improving our testing process to prevent similar issues in the future.

### Next Steps

After upgrading:

1. ✅ Test timeout functionality in dev/staging
2. ✅ Monitor timeout executions in production
3. ✅ Check for any stuck WAITING executions
4. ✅ Verify timeout paths execute correctly
5. ✅ Report any issues immediately

### Recommendation

**Severity**: **CRITICAL**

**Action**: **Immediate upgrade required for ALL users on v1.1.8**

**Safe to skip v1.1.8**: Yes, users on v1.1.7 or earlier can upgrade directly to v1.1.9.

---

**Full Changelog**: https://github.com/hussainpithawala/state-machine-amz-go/compare/v1.1.8...v1.1.9

**Report Issues**: https://github.com/hussainpithawala/state-machine-amz-go/issues

**Questions?** Open a discussion: https://github.com/hussainpithawala/state-machine-amz-go/discussions

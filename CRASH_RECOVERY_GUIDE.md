# Crash-Resilient Execution Recovery

## Overview

This document describes the crash-resilient execution recovery mechanism implemented in the state machine system. This feature ensures that state machine executions can automatically resume after unexpected crashes or failures, picking up from the last successfully completed state.

## Problem Statement

In the normal course of operation, state machine executions complete successfully. However, in adverse scenarios:

1. **Persistent State Machine crashes** - The application hosting the state machine terminates unexpectedly
2. **Infrastructure failures** - Database connections, network issues, or hardware failures
3. **Resource exhaustion** - Out of memory, disk space, or other system resources

In such scenarios, executions with `RUNNING` status become **orphaned** - they are neither completed nor explicitly failed, and the last persisted state history represents the furthest point of progress.

## Solution Architecture

The crash-resilient recovery system consists of four main components:

### 1. Recovery Metadata Tracking

Each execution now tracks recovery-related information:

```go
type RecoveryMetadata struct {
    LastSuccessfulState       string      // Name of last successfully completed state
    LastSuccessfulStateOutput interface{} // Output from last successful state
    RecoveryAttemptCount      int         // Number of recovery attempts
    LastRecoveryAttemptAt     *time.Time  // Timestamp of last recovery attempt
    MaxRecoveryAttempts       int         // Maximum recovery attempts allowed
    RecoveryStrategy          string      // Recovery strategy (RETRY, SKIP, FAIL, PAUSE)
    CrashDetectedAt           *time.Time  // When crash was detected
}
```

**Key Features:**
- Updated after every successful state execution
- Persisted to the database with each state transition
- Used to determine resumption point during recovery

### 2. Orphaned Execution Detection

The system identifies orphaned executions using:

- **Status-based filtering**: Only `RUNNING` executions are considered
- **Time threshold**: Executions running longer than `OrphanedThreshold` (default: 5 minutes)
- **Efficient queries**: Database-level filtering for performance

```sql
SELECT * FROM executions
WHERE status = 'RUNNING'
  AND start_time < (NOW() - INTERVAL '5 minutes')
ORDER BY start_time ASC
```

### 3. Recovery Strategies

The system supports multiple recovery strategies:

| Strategy | Behavior | Use Case |
|----------|----------|----------|
| `RETRY` | Re-executes from last successful state | Default, idempotent states |
| `SKIP` | Skips problematic state, continues to next | Non-critical states |
| `FAIL` | Marks execution as failed after max attempts | Critical data integrity |
| `PAUSE` | Pauses for manual intervention | Requires human review |

### 4. Background Recovery Scanner

A background goroutine periodically scans for orphaned executions:

```go
// Start recovery scanner
err := stateMachine.StartRecoveryScanner(&recovery.RecoveryConfig{
    Enabled: true,
    ScanInterval: 30 * time.Second,
    OrphanedThreshold: 5 * time.Minute,
    DefaultRecoveryStrategy: recovery.StrategyRetry,
    DefaultMaxRecoveryAttempts: 3,
})
```

## Implementation Details

### Database Schema Changes

The `executions` table now includes:

```sql
ALTER TABLE executions 
ADD COLUMN recovery_metadata JSONB DEFAULT '{}';

ALTER TABLE executions 
ADD COLUMN created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE executions 
ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
```

### Execution Flow

#### Normal Execution with Recovery Tracking

```
┌─────────────────┐
│ Start Execution │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Execute State  │
│     (State A)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Succeeded?    │──No──▶ Handle Error
└────────┬────────┘
         │ Yes
         ▼
┌─────────────────┐
│ Update Recovery │
│    Metadata     │
│  (State A done) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Persist to DB   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Next State?    │──Yes──▶ Continue
└────────┬────────┘
         │ No
         ▼
┌─────────────────┐
│   Complete      │
└─────────────────┘
```

#### Recovery Flow After Crash

```
┌─────────────────┐
│  Background     │
│ Scanner Runs    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Find Orphaned   │
│  Executions     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Check Recovery  │
│    Attempts     │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
  Max       Can
Reached   Recover
    │         │
    │         ▼
    │    ┌─────────────────┐
    │    │ Load Last       │
    │    │ Successful State│
    │    └────────┬────────┘
    │             │
    │             ▼
    │    ┌─────────────────┐
    │    │ Resume Execution│
    │    │  from Checkpoint│
    │    └────────┬────────┘
    │             │
    │             ▼
    │    ┌─────────────────┐
    │    │ Update Recovery │
    │    │    Metadata     │
    │    └─────────────────┘
    │
    ▼
┌─────────────────┐
│ Mark as FAILED  │
│ (Max Attempts)  │
└─────────────────┘
```

## Usage Guide

### Basic Setup

```go
import (
    "github.com/hussainpithawala/state-machine-amz-go/pkg/recovery"
    "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

// Create persistent state machine
sm, err := persistent.New(definition, true, "my-sm", repositoryManager)
if err != nil {
    // handle error
}

// Configure recovery
recoveryConfig := &recovery.RecoveryConfig{
    Enabled: true,
    ScanInterval: 30 * time.Second,
    OrphanedThreshold: 5 * time.Minute,
    DefaultRecoveryStrategy: recovery.StrategyRetry,
    DefaultMaxRecoveryAttempts: 3,
}

// Start recovery scanner
err = sm.StartRecoveryScanner(recoveryConfig)
if err != nil {
    // handle error
}

// Graceful shutdown
defer sm.StopRecoveryScanner()
```

### Custom Recovery Configuration

```go
// Different thresholds for different state machines
criticalSMConfig := &recovery.RecoveryConfig{
    Enabled: true,
    ScanInterval: 10 * time.Second,      // Faster detection
    OrphanedThreshold: 2 * time.Minute,  // Shorter threshold
    DefaultRecoveryStrategy: recovery.StrategyFail,  // Fail fast
    DefaultMaxRecoveryAttempts: 1,       // Single attempt
}

batchSMConfig := &recovery.RecoveryConfig{
    Enabled: true,
    ScanInterval: 60 * time.Second,      // Slower detection
    OrphanedThreshold: 10 * time.Minute, // Longer threshold
    DefaultRecoveryStrategy: recovery.StrategyRetry,
    DefaultMaxRecoveryAttempts: 5,       // More attempts
}
```

### Manual Recovery

```go
// Get recovery manager
recoveryMgr := sm.GetRecoveryManager()

// Find orphaned executions
orphaned, err := recoveryMgr.FindOrphanedExecutions(ctx)
if err != nil {
    // handle error
}

// Recover specific execution
for _, orphan := range orphaned {
    err := recoveryMgr.RecoverExecution(ctx, orphan, func(ctx context.Context, exec *execution.Execution) (*execution.Execution, error) {
        return sm.RunExecution(ctx, exec.Input, exec)
    })
    if err != nil {
        log.Printf("Failed to recover %s: %v", orphan.ExecutionID, err)
    }
}
```

## Best Practices

### 1. Idempotent State Handlers

Design state handlers to be idempotent so they can be safely retried:

```go
// Good: Idempotent operation
func (h *Handler) ProcessPayment(ctx context.Context, input interface{}) (interface{}, error) {
    // Check if already processed
    if alreadyProcessed(input.ID) {
        return getExistingResult(input.ID), nil
    }
    // Process payment
    return processPayment(input)
}

// Bad: Non-idempotent operation
func (h *Handler) ChargeCard(ctx context.Context, input interface{}) (interface{}, error) {
    // Always charges, even if retried
    return chargeCard(input.CardID, input.Amount)
}
```

### 2. Appropriate Thresholds

Set thresholds based on:
- **Expected execution duration**: Longer executions need longer thresholds
- **Business criticality**: Critical executions need shorter thresholds
- **State complexity**: Complex states may need more time

### 3. Monitoring and Alerting

Monitor recovery metrics:
- Number of orphaned executions detected
- Recovery success/failure rates
- Average recovery time
- Executions exceeding max attempts

```go
// Example: Export recovery metrics
func (rm *RecoveryManager) GetMetrics() RecoveryMetrics {
    return RecoveryMetrics{
        OrphanedDetected:    rm.orphanedCount,
        RecoveredSuccessful: rm.successCount,
        RecoveredFailed:     rm.failedCount,
        MaxAttemptsReached:  rm.maxAttemptsCount,
    }
}
```

### 4. Database Maintenance

Regularly clean up old executions:
```sql
-- Archive old completed executions
DELETE FROM executions 
WHERE status IN ('SUCCEEDED', 'FAILED')
  AND end_time < (NOW() - INTERVAL '30 days');
```

## Limitations and Considerations

### 1. State Handler Side Effects

Re-executing states may cause:
- Duplicate external API calls
- Multiple database writes
- Repeated notifications

**Mitigation**: Implement idempotency in state handlers.

### 2. Data Consistency

If a crash occurs mid-transaction:
- External systems may have partial updates
- Database transactions may be incomplete

**Mitigation**: Use distributed transactions or saga patterns.

### 3. Recovery Storm

After a prolonged outage, many executions may need recovery simultaneously.

**Mitigation**: 
- Implement rate limiting in recovery scanner
- Prioritize critical executions
- Use exponential backoff

### 4. Resource Consumption

Recovery scanning consumes database and application resources.

**Mitigation**:
- Tune scan interval based on load
- Use database indexes on `status` and `start_time`
- Consider partitioning executions table

## Testing

### Unit Tests

Test recovery logic in isolation:

```go
func TestRecoveryManager_RecoverExecution(t *testing.T) {
    // Setup
    repo := createTestRepository()
    manager := recovery.NewRecoveryManager(repo, &recovery.RecoveryConfig{
        DefaultRecoveryStrategy: recovery.StrategyRetry,
        DefaultMaxRecoveryAttempts: 3,
    })
    
    // Create orphaned execution
    orphaned := &recovery.OrphanedExecution{
        ExecutionID: "test-exec",
        CurrentState: "StateB",
        StartTime: time.Now().Add(-10 * time.Minute),
    }
    
    // Execute recovery
    err := manager.RecoverExecution(ctx, orphaned, mockRecoveryFunc)
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "SUCCEEDED", getExecutionStatus("test-exec"))
}
```

### Integration Tests

Test with actual database and state machine:

```go
func TestCrashRecovery_Integration(t *testing.T) {
    // Setup real database
    db := setupTestDatabase()
    repo := repository.NewPostgresRepository(db)
    
    // Create state machine with recovery
    sm := createPersistentStateMachine(repo)
    sm.StartRecoveryScanner(defaultConfig)
    
    // Start execution
    execCtx := startExecution(sm, testInput)
    
    // Simulate crash (stop without completing)
    sm.StopRecoveryScanner()
    
    // Wait for threshold
    time.Sleep(6 * time.Minute)
    
    // Restart recovery
    sm.StartRecoveryScanner(defaultConfig)
    
    // Verify execution completed
    waitForExecutionCompletion(execCtx.ID)
    assert.Equal(t, "SUCCEEDED", getExecutionStatus(execCtx.ID))
}
```

## Troubleshooting

### Recovery Not Triggering

**Symptoms**: Orphaned executions remain in RUNNING status

**Check**:
1. Recovery scanner is started: `sm.GetRecoveryManager().IsRunning()`
2. Threshold is appropriate for execution duration
3. No errors in recovery scanner logs

### Repeated Recovery Failures

**Symptoms**: Execution repeatedly fails recovery attempts

**Check**:
1. State handler idempotency
2. External dependency availability
3. Recovery attempt count vs. max attempts
4. Error messages in execution history

### Performance Degradation

**Symptoms**: Database or application slowdown during recovery

**Check**:
1. Scan interval too aggressive
2. Too many orphaned executions
3. Missing database indexes
4. Recovery function complexity

## Future Enhancements

Potential improvements:
- [ ] Configurable per-state-machine recovery policies
- [ ] Recovery priority queues
- [ ] Machine learning-based anomaly detection
- [ ] Distributed recovery coordination
- [ ] Recovery simulation/testing mode
- [ ] Enhanced metrics and observability

## Related Documentation

- [Execution Model](../pkg/execution/execution.go)
- [Repository Interface](../pkg/repository/repository.go)
- [Persistent State Machine](../pkg/statemachine/persistent/persistent.go)
- [Recovery Manager](../pkg/recovery/recovery.go)

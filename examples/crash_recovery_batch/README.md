# Batch/Micro-Batch Crash Recovery Example

This example demonstrates crash recovery for **batch/micro-batch orchestration** scenarios, where an orchestrator manages multiple worker executions with barrier synchronization.

## The Problem

In batch/micro-batch orchestration:

1. **Orchestrator** dispatches micro-batches and waits at a barrier
2. **Workers** process individual tasks and decrement the barrier
3. **Last worker** to complete signals the orchestrator to continue

**Crash Scenario:**
```
Orchestrator: Dispatches micro-batch → Waits at barrier (PAUSED)
Worker 1:    Completes → Barrier: 4/5
Worker 2:    Completes → Barrier: 3/5
Worker 3:    Completes → Barrier: 2/5
Worker 4:    CRASHES before decrementing barrier
Worker 5:    CRASHES before decrementing barrier

Result: Barrier stuck at 2, orchestrator waits forever!
```

**Recovery Challenge:**
- Both orchestrator AND workers need recovery
- Recovered workers must properly signal the barrier
- Orchestrator must resume when barrier completes
- **Critical**: Barrier signaling must work correctly after recovery

## Solution

This example demonstrates:

1. **Recovery Metadata Tracking** - Both orchestrator and workers track recovery state
2. **Independent Resumption** - Each orphaned execution resumes independently
3. **Barrier Integrity** - Redis barrier counter survives crashes
4. **Proper Signaling** - Recovered workers signal orchestrator via barrier mechanism
5. **Orchestrator Continuation** - Orchestrator resumes when barrier reaches zero

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     PostgreSQL Database                      │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐ │
│  │ Orchestrator   │  │   Worker 1     │  │   Worker 2     │ │
│  │  Execution     │  │  Execution     │  │  Execution     │ │
│  │  (PAUSED)      │  │  (SUCCEEDED)   │  │  (ORPHANED)    │ │
│  └────────────────┘  └────────────────┘  └────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                            ↕
┌─────────────────────────────────────────────────────────────┐
│                        Redis                                 │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Barrier: batch:barrier:mb-0 = 2 (stuck)              │   │
│  │ Cursor: batch:cursor:batch-001 = 0                   │   │
│  │ IDs: batch:ids:batch-001 = [exec1, exec2, ...]       │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                            ↕
┌─────────────────────────────────────────────────────────────┐
│                   Recovery Scanner                           │
│  1. Find orphaned executions (RUNNING > threshold)          │
│  2. Resume workers from last successful state               │
│  3. Workers complete → decrement barrier                    │
│  4. Last worker signals orchestrator                        │
│  5. Orchestrator resumes and dispatches next micro-batch    │
└─────────────────────────────────────────────────────────────┘
```

## Running the Example

### Prerequisites

1. **PostgreSQL**: `createdb statemachine_test`
2. **Redis**: `redis-server` (default port 6379)
3. **Go**: 1.21+

### Run

```bash
cd examples/crash_recovery_batch
go run main.go
```

## Example Output

```
=== Crash Recovery for Batch/Micro-Batch Orchestration ===

This example demonstrates:
  1. Orchestrator managing micro-batch executions
  2. Simulated crash of orchestrator and workers
  3. Recovery scanner resuming orphaned executions
  4. Barrier mechanism signaling orchestrator completion
  5. Orchestrator continuing after barrier is cleared

Initializing components...
Connected to PostgreSQL: postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable
Connected to Redis: redis://localhost:6379/0

State machines initialized:
  - Worker SM: crash-recovery-worker-sm
  - Orchestrator SM: micro-batch-orchestrator-v1

=== Phase 1: Creating Source Executions ===

Created 15 source executions

=== Phase 2: Starting Batch Orchestration ===

Batch orchestration started: crash-recovery-batch-001
Total executions: 15, Micro-batch size: 5

=== Phase 3: Simulating Crash ===

Simulating crash scenario:
  - Orchestrator: PAUSED at WaitForMicroBatchCompletion
  - Workers: Mid-execution (some completed, some not)

Simulating 3/5 workers completing before crash...
    Worker 1 completed, barrier remaining: 4
    Worker 2 completed, barrier remaining: 3
    Worker 3 completed, barrier remaining: 2
Barrier remaining after partial completions: 2

>>> CRASH SIMULATED <<<
   - Last 2 workers crashed before decrementing barrier
   - Orchestrator waiting at barrier
   - Barrier counter stuck at non-zero value

Waiting 5s for executions to be considered orphaned...

=== Phase 4: Starting Recovery Scanner ===

Recovery scanner started (interval: 2s, threshold: 4s)

Monitoring recovery process...

[2s] Orchestrator: PAUSED        Barrier: 2
[4s] Orchestrator: PAUSED        Barrier: 2
[6s]     Worker processing data...
    Worker 4 completed, barrier remaining: 1
[6s] Orchestrator: PAUSED        Barrier: 1
[8s]     Worker processing data...
    Worker 5 completed, barrier remaining: 0
[8s] Orchestrator: DISPATCHING   Barrier: CLEARED
[10s] Orchestrator: SUCCEEDED     Barrier: CLEARED

>>> Recovery Complete! Orchestrator finished successfully <<<

=== Final Results ===

Orchestrator Executions:
  ID: mb-orch-crash-recovery-batch-001-...
  Status: SUCCEEDED
  Current State: CompleteBatch
  Recovery Attempts: 1

Worker Executions: 15 total
  Succeeded: 15, Failed: 0

Redis Keys Status:
  Cursor: 15 / 15
  IDs List: 15 items

Barrier Status:
  Micro-batch 0: CLEARED (key deleted)
  Micro-batch 1: CLEARED (key deleted)
  Micro-batch 2: CLEARED (key deleted)
```

## Key Components

### 1. Orchestrator State Machine

```yaml
StartAt: DispatchMicroBatch
States:
  DispatchMicroBatch:
    Type: Task
    Resource: "batch:dispatch"
    Next: WaitForMicroBatchCompletion

  WaitForMicroBatchCompletion:
    Type: Message
    CorrelationKey: "micro_batch_id"
    CorrelationValuePath: "$.dispatchResult.microBatchId"
    Next: EvaluateMicroBatch

  EvaluateMicroBatch:
    Type: Task
    Resource: "batch:evaluate"
    Next: CheckContinue

  CheckContinue:
    Type: Choice
    Choices:
      - Variable: "$.dispatchResult.isBatchComplete"
        BooleanEquals: false
        Next: DispatchMicroBatch
    Default: CompleteBatch

  CompleteBatch:
    Type: Succeed
```

### 2. Worker State Machine

```yaml
StartAt: ProcessData
States:
  ProcessData:
    Type: Task
    Resource: "worker:process-data"
    End: true
```

### 3. Barrier Mechanism

The barrier is a Redis counter initialized to the micro-batch size:

```go
// Initialize barrier
redis.Set(ctx, "batch:barrier:mb-0", 5)

// Each worker decrements
remaining := redis.Decr(ctx, "batch:barrier:mb-0")

// Last worker (remaining == 0) signals orchestrator
if remaining == 0 {
    orchestrator.ResumeExecution()
}
```

### 4. Recovery Flow

```
1. Recovery Scanner detects orphaned executions
   ↓
2. Worker executions resume from last successful state
   ↓
3. Recovered workers complete processing
   ↓
4. Workers decrement barrier counter
   ↓
5. Last worker's barrier decrement → 0
   ↓
6. Barrier logic finds waiting orchestrator via correlation
   ↓
7. Orchestrator resumes with completion payload
   ↓
8. Orchestrator continues to next micro-batch
```

## Critical: Barrier Signaling After Recovery

The key challenge is ensuring that **recovered workers properly signal the orchestrator**.

### Without Recovery (Normal Flow)

```go
// Worker completes normally
func (w *Worker) Complete(ctx context.Context) {
    // Process data
    result := w.process()
    
    // Decrement barrier
    remaining := barrier.Decrement(ctx, mbID)
    
    // Last worker signals orchestrator
    if remaining == 0 {
        orchestrator.ResumeExecution(ctx, mbID)
    }
}
```

### With Recovery

```go
// Worker crashes mid-execution
func (w *Worker) Complete(ctx context.Context) {
    // Process data
    result := w.process()  // ← CRASH HERE
    
    // Never reaches barrier decrement!
    remaining := barrier.Decrement(ctx, mbID)
    
    if remaining == 0 {
        orchestrator.ResumeExecution(ctx, mbID)
    }
}

// Recovery scanner resumes worker
func RecoveryManager.Recover(worker *Worker) {
    // Reload last successful state
    input := worker.GetRecoveryInput()
    
    // Re-execute from checkpoint
    result := worker.process()  // ← Re-executes
    
    // Decrement barrier (THIS IS CRITICAL!)
    remaining := barrier.Decrement(ctx, mbID)
    
    // Signal orchestrator if last worker
    if remaining == 0 {
        orchestrator.ResumeExecution(ctx, mbID)
    }
}
```

### Why This Works

1. **Barrier Persists**: Redis barrier survives application crashes
2. **Idempotent Decrement**: Each worker decrements exactly once
3. **Correlation Tracking**: Message correlation links barrier to orchestrator
4. **Recovery Metadata**: Workers resume from correct checkpoint
5. **Orchestrator Waits**: Message state keeps waiting for barrier signal

## Recovery Metadata in Batch Scenario

### Orchestrator Recovery Metadata

```go
{
    "last_successful_state": "DispatchMicroBatch",
    "last_successful_state_output": {
        "microBatchId": "mb-0",
        "size": 5
    },
    "recovery_attempt_count": 1,
    "crash_detected_at": "2024-01-15T10:30:00Z"
}
```

### Worker Recovery Metadata

```go
{
    "last_successful_state": "",  // Crashed before completing any state
    "recovery_attempt_count": 1,
    "crash_detected_at": "2024-01-15T10:30:00Z"
}
```

## Troubleshooting

### Orchestrator Never Resumes

**Symptoms**: Barrier reaches 0, but orchestrator stays PAUSED

**Check**:
1. Message correlation exists: `redis-cli HGETALL message:correlation:*`
2. Orchestrator execution status: Check database
3. Recovery scanner logs: Look for errors

### Barrier Stuck at Non-Zero

**Symptoms**: Some workers completed, but barrier never reaches 0

**Check**:
1. All workers recovered: Check execution status
2. Redis barrier key exists: `redis-cli GET batch:barrier:mb-0`
3. Worker recovery completed successfully

### Duplicate Barrier Decrement

**Symptoms**: Barrier goes negative

**Solution**: Barrier uses Lua script for atomic decrement with guard:
```lua
if redis.call("EXISTS", key) == 0 then return -1 end
local rem = redis.call("DECR", key)
return rem
```

## Comparison: Single vs Batch Recovery

| Aspect | Single Execution | Batch Orchestration |
|--------|-----------------|---------------------|
| **Recovery Target** | One execution | Orchestrator + multiple workers |
| **Synchronization** | None | Barrier counter |
| **Signaling** | Direct completion | Barrier → correlation → orchestrator |
| **Complexity** | Low | High (multiple moving parts) |
| **Failure Modes** | Simple retry | Barrier stuck, signaling failures |

## Best Practices

### 1. Barrier TTL

Set appropriate TTL to prevent stale barriers:
```go
BarrierTTL = 1 * time.Hour  // Adjust based on batch duration
```

### 2. Recovery Threshold

Set threshold based on expected worker duration:
```go
// Workers take ~30s to complete
orphanedThreshold = 2 * time.Minute  // 4x buffer
```

### 3. Idempotent Workers

Ensure workers can be safely re-executed:
```go
func (w *Worker) Process(ctx context.Context, input interface{}) (interface{}, error) {
    // Check if already processed (idempotency)
    if w.alreadyProcessed(input.ID) {
        return w.getExistingResult(input.ID), nil
    }
    // Process normally
    return w.processData(input)
}
```

### 4. Monitoring

Monitor key metrics:
- Orphaned executions count
- Barrier status for active micro-batches
- Recovery success/failure rates
- Orchestrator wait time

## Related Documentation

- [Single Execution Crash Recovery](../crash_recovery/)
- [Batch Orchestration Guide](../../pkg/batch/)
- [Recovery Manager](../../pkg/recovery/recovery.go)
- [Barrier Implementation](../../pkg/batch/barrier.go)
- [Orchestrator Implementation](../../pkg/batch/orchestrator.go)

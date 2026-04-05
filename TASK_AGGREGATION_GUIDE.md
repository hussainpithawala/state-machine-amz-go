# Task Aggregation with Asynq Groups

## Overview

The state-machine-amz-go framework now supports **Task Aggregation** using Asynq's `GroupOpt` feature. This allows multiple micro-batch tasks to be collected into a group and processed together as a single batch operation rather than individually, reducing queue overhead and improving efficiency.

## How It Works

### Traditional Flow (Without Grouping)
```
enqueueIDs → N × EnqueueExecution → N individual tasks in queue → N separate executions
```

Each task is:
- Enqueued individually (N Redis calls)
- Polled separately by workers
- Executed independently
- Tracking barrier signaled N times

### Group-Based Flow (With Aggregation)
```
enqueueIDs → EnqueueExecutionGroup (N tasks with same groupID)
           → Asynq aggregates into 1 group
           → GroupAggregator creates 1 batch task
           → 1 batch execution processes all N tasks together
           → Barrier signaled for each task internally
```

## Configuration

### 1. Enable Group Aggregation in Queue Config

```go
queueConfig := &queue.Config{
    RedisClientOpt: &asynq.RedisClientOpt{
        Addr: "localhost:6379",
    },
    Concurrency: 10,
    Queues: map[string]int{
        "default": 5,
    },
    RetryPolicy: &queue.RetryPolicy{
        MaxRetry: 3,
        Timeout:  10 * time.Minute,
    },
    // Enable group aggregation
    GroupAggregation: &queue.GroupAggregationConfig{
        Enabled:          true,
        GroupMaxSize:     100,     // Process when 100 tasks collected
        GroupMaxDelay:    30 * time.Second, // Or after 30 seconds
        GroupGracePeriod: 15 * time.Second, // Reset timer on each new task
    },
}

worker, err := queue.NewWorker(queueConfig, execHandler)
```

### 2. Enable Group Enqueue in Orchestrator Input

```go
input := batch.OrchestratorInput{
    BatchID:              "batch-123",
    TotalCount:           1000,
    MicroBatchSize:       100,
    SourceStateName:      "ProcessData",
    OrchestratorSMID:     batch.OrchestratorStateMachineID,
    TargetStateMachineID: "target-workflow-v1",
    FailurePolicy:        batch.DefaultFailurePolicy(),
    
    // Enable group-based enqueue
    UseGroupEnqueue: true, // <-- This enables aggregation
}

doneCh, err := orchestrator.Run(ctx, batchID, sourceIDs, targetMachineID, sourceState, opts, execOpts)
```

## Trigger Conditions

Tasks in a group are processed when **ANY** of these conditions is met:

1. **GroupMaxSize**: The group reaches the specified number of tasks
   - Example: If `GroupMaxSize=100`, the group processes immediately when the 100th task arrives

2. **GroupMaxDelay**: Maximum time since the first task was added
   - Example: If `GroupMaxDelay=30s`, the group processes 30 seconds after the first task, even if not full

3. **GroupGracePeriod**: Time since the last task was added (resets with each new task)
   - Example: If `GroupGracePeriod=15s` and tasks keep arriving every 5 seconds, processing waits until 15 seconds of silence

## Architecture Components

### 1. GroupAggregator (`pkg/queue/aggregator.go`)

The `ExecutionTaskAggregator` implements `asynq.GroupAggregator`:

```go
type ExecutionTaskAggregator struct{}

func (a *ExecutionTaskAggregator) Aggregate(group string, tasks []*asynq.Task) *asynq.Task {
    // Combines all individual task payloads into a single BatchTaskPayload
    batchPayload := &BatchTaskPayload{
        GroupID:      group,
        TaskCount:    len(tasks),
        OriginalType: TypeExecutionTask,
        Tasks:        make([]json.RawMessage, 0, len(tasks)),
    }
    
    // Collect all task payloads
    for _, task := range tasks {
        batchPayload.Tasks = append(batchPayload.Tasks, task.Payload())
    }
    
    // Create aggregated task with type "statemachine:batch"
    return asynq.NewTask(TypeBatchTask, data, asynq.TaskID(fmt.Sprintf("batch-%s", group)))
}
```

### 2. BatchTaskPayload (`pkg/queue/task.go`)

```go
type BatchTaskPayload struct {
    GroupID            string            `json:"group_id"`
    StateMachineID     string            `json:"state_machine_id,omitempty"`
    SourceStateName    string            `json:"source_state_name,omitempty"`
    InputTransformerName string          `json:"input_transformer_name,omitempty"`
    TaskCount          int               `json:"task_count"`
    OriginalType       string            `json:"original_type"`
    Tasks              []json.RawMessage `json:"tasks"` // Individual task payloads
}
```

### 3. Batch Execution Handler (`pkg/handler/execution_handler.go`)

The `HandleBatchExecution` method processes all tasks in the batch:

```go
func (h *ExecutionHandler) HandleBatchExecution(ctx context.Context, payload *queue.BatchTaskPayload) error {
    // Process each individual task in the batch
    for i, taskData := range payload.Tasks {
        var execPayload queue.ExecutionTaskPayload
        json.Unmarshal(taskData, &execPayload)
        
        // Execute this individual task
        h.processBatchTask(ctx, &execPayload)
        
        // Signal barrier completion for micro-batch tracking
        h.processBatchBarrier(ctx, &execPayload, err, h.orchestrator)
    }
    return nil
}
```

## Benefits

1. **Reduced Queue Overhead**: N tasks → 1 batch task (fewer Redis operations)
2. **Better Resource Utilization**: Workers process tasks in batches, reducing polling frequency
3. **Preserved Micro-Batch Tracking**: Barrier signaling still works for each individual task
4. **Backward Compatible**: `UseGroupEnqueue=false` (default) maintains existing behavior
5. **Flexible Triggers**: Configure size-based, time-based, or grace period triggers

## When to Use

### Use Group Aggregation When:
- ✅ Micro-batch size is large (50+ tasks)
- ✅ Tasks have similar execution times
- ✅ You want to reduce Redis load from frequent polling
- ✅ State machine executions are lightweight

### Use Individual Enqueue When:
- ❌ Micro-batch size is small (< 10 tasks)
- ❌ Tasks need immediate individual execution
- ❌ You need fine-grained task-level monitoring
- ❌ Executions are long-running (don't want to block the group)

## Example: Full Setup

```go
// 1. Configure worker with group aggregation
queueConfig := &queue.Config{
    RedisClientOpt: &asynq.RedisClientOpt{Addr: "localhost:6379"},
    Concurrency:    10,
    Queues: map[string]int{
        "target-workflow-v1": 5,
    },
    RetryPolicy: &queue.RetryPolicy{MaxRetry: 3, Timeout: 10 * time.Minute},
    GroupAggregation: &queue.GroupAggregationConfig{
        Enabled:          true,
        GroupMaxSize:     100,
        GroupMaxDelay:    30 * time.Second,
        GroupGracePeriod: 15 * time.Second,
    },
}

execHandler := handler.NewExecutionHandlerWithContext(manager, qc, execAdapter, orchestrator)
worker, err := queue.NewWorker(queueConfig, execHandler)
go func() {
    if err := worker.Run(); err != nil {
        log.Fatalf("Worker failed: %v", err)
    }
}()
defer worker.Shutdown()

// 2. Configure orchestrator with group enqueue
input := batch.OrchestratorInput{
    BatchID:              "batch-abc123",
    TotalCount:           1000,
    MicroBatchSize:       100, // 10 micro-batches of 100 tasks each
    SourceStateName:      "PrepareData",
    OrchestratorSMID:     batch.OrchestratorStateMachineID,
    TargetStateMachineID: "target-workflow-v1",
    FailurePolicy:        batch.DefaultFailurePolicy(),
    UseGroupEnqueue:      true, // Enable group aggregation
}

// 3. Run the orchestrator
// Each micro-batch of 100 tasks will be:
//   - Enqueued as a group (100 individual tasks with same groupID)
//   - Aggregated by Asynq into 1 batch task
//   - Processed together by 1 worker execution
//   - Barrier decremented 100 times (once per task)
doneCh, err := orchestrator.Run(ctx, batchID, sourceIDs, targetMachineID, sourceState, opts, execOpts)
```

## Monitoring and Debugging

### Log Output

With group aggregation enabled, you'll see:

```
Processing batch task: GroupID=batch-abc123:mb0, TaskCount=100, StateMachineID=target-workflow-v1
Processing batch task 1/100: ExecutionName=batch-abc123-mb0-0, SourceExecutionID=exec-001
Batch task execution completed: ID=exec-001, Status=SUCCEEDED
Processing batch task 2/100: ExecutionName=batch-abc123-mb0-1, SourceExecutionID=exec-002
...
Batch execution completed: GroupID=batch-abc123:mb0, Processed=100/100 tasks
```

### Redis Keys

Group tasks are stored in Asynq's internal group keys:
- `asynq:{<group>}` - Sorted set of task IDs in the group
- Individual tasks still maintain their uniqueness via `TaskID`

### Metrics

Micro-batch metrics continue to work unchanged:
- Barrier decrement per task (not per batch)
- Sliding window failure rate calculation
- Threshold-based failure detection and pausing

## Migration Guide

### From Individual to Group-Based

1. **Add GroupAggregation to queue config** (no breaking changes)
2. **Set `UseGroupEnqueue: true`** in orchestrator input
3. **Monitor logs** to verify batch processing
4. **Adjust triggers** (`GroupMaxSize`, `GroupMaxDelay`) based on observed behavior

### Rollback

If issues arise, simply set `UseGroupEnqueue: false` or remove the field (defaults to false).

## Performance Considerations

### Memory
- Aggregated batch task holds all individual payloads in memory
- For 100 tasks × 1KB each = ~100KB batch payload (negligible)

### Execution Time
- Batch execution time ≈ sum of individual execution times
- No parallelism within a batch (sequential processing)
- Configure `Timeout` appropriately (e.g., `100 tasks × 30s = 50m`)

### Concurrency
- Groups are processed one-at-a-time per worker
- Multiple workers can process different groups concurrently
- Set `Concurrency` based on desired parallelism

## Troubleshooting

### Issue: Group never processes
- **Check**: `GroupAggregation.Enabled` is true
- **Check**: `GroupMaxSize` or `GroupMaxDelay` is configured
- **Check**: Tasks are actually being enqueued with the same `GroupID`

### Issue: Tasks processed individually despite grouping
- **Check**: `UseGroupEnqueue: true` is set in `OrchestratorInput`
- **Check**: Worker config has `GroupAggregation` enabled
- **Check**: No errors in aggregator logs

### Issue: Batch execution timeout
- **Increase**: `RetryPolicy.Timeout` to accommodate batch processing time
- **Reduce**: `GroupMaxSize` to process smaller batches faster
- **Check**: Individual task execution times and multiply by batch size

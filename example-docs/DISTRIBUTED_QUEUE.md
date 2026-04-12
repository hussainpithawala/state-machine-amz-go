# Distributed Queue Integration

This document describes the integration of [asynq](https://github.com/hibiken/asynq) for distributed task processing in the state machine framework.

## Overview

The state machine now supports distributed execution using Redis-backed task queues. This enables:

- **Horizontal Scaling**: Multiple worker instances process tasks concurrently
- **Reliable Execution**: Tasks are persisted and retried on failure
- **Decoupled Architecture**: Leaders enqueue tasks, workers process them independently
- **Real-time Monitoring**: Web UI for queue statistics and task management

## Architecture

### Components

1. **Queue Client** (`pkg/queue/client.go`): Enqueues execution tasks to Redis
2. **Queue Worker** (`pkg/queue/worker.go`): Processes execution tasks from Redis
3. **Task Definition** (`pkg/queue/task.go`): Defines task payload structure
4. **Queue Config** (`pkg/queue/config.go`): Configuration for Redis and worker settings

### Execution Flow

#### Leader Mode (Task Enqueueing)

```
StateMachine.ExecuteBatch()
    ↓
executeBatchConcurrent() [detects queueClient]
    ↓
executeBatchViaQueue()
    ↓
For each source execution:
    - Create ExecutionTaskPayload
    - Enqueue to Redis via queueClient
    ↓
Return enqueue results
```

#### Worker Mode (Task Processing)

```
Worker.Run()
    ↓
Polls Redis for tasks
    ↓
For each task:
    - Parse ExecutionTaskPayload
    - Load state machine definition
    - Execute state machine
    - Persist results to repository
    ↓
Retry on failure (configurable)
```

## Usage

### 1. Setup Infrastructure

Start Redis and monitoring UI:

```bash
cd docker-examples
docker-compose up -d
```

Services:
- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- Asynqmon UI: `http://localhost:8080`

### 2. Configure Queue Client

```go
import "github.com/hussainpithawala/state-machine-amz-go/pkg/queue"

queueConfig := &queue.Config{
    RedisAddr:     "localhost:6379",
    RedisPassword: "",
    RedisDB:       0,
    Concurrency:   10,
    Queues: map[string]int{
        "critical": 6,  // Higher priority
        "default":  3,
        "low":      1,  // Lower priority
    },
    RetryPolicy: &queue.RetryPolicy{
        MaxRetry: 3,
        Timeout:  10 * time.Minute,
    },
}

queueClient, err := queue.NewClient(queueConfig)
if err != nil {
    log.Fatal(err)
}
defer queueClient.Close()
```

### 3. Enable Distributed Execution

Attach the queue client to your state machine:

```go
sm, err := persistent.NewFromDefnId(ctx, "my-state-machine", repoManager)
if err != nil {
    log.Fatal(err)
}

// Enable distributed execution
sm.SetQueueClient(queueClient)
```

### 4. Execute Batch (Leader)

When `ExecuteBatch()` is called with a queue client attached, tasks are enqueued instead of executed locally:

```go
filter := &repository.ExecutionFilter{
    StateMachineID: "my-state-machine",
    Status:         "SUCCEEDED",
    Limit:          100,
}

batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "batch-exec-001",
    ConcurrentBatches: 10, // Ignored in queue mode
    StopOnError:       false,
    OnExecutionStart: func(sourceExecID string, index int) {
        log.Printf("Enqueuing task %d", index)
    },
}

results, err := sm.ExecuteBatch(ctx, filter, "ProcessOrder", batchOpts)
```

### 5. Run Workers

Start worker instances to process tasks:

```go
worker, err := queue.NewWorker(queueConfig, repoManager)
if err != nil {
    log.Fatal(err)
}

// Blocks until shutdown signal
if err := worker.Run(); err != nil {
    log.Fatal(err)
}
```

Run multiple workers for horizontal scaling:

```bash
# Terminal 1
go run main.go -mode=worker -concurrency=10

# Terminal 2
go run main.go -mode=worker -concurrency=10

# Terminal 3
go run main.go -mode=worker -concurrency=10
```

## Task Payload Structure

```go
type ExecutionTaskPayload struct {
    StateMachineID    string      // ID of the state machine to execute
    SourceExecutionID string      // Source execution for chained workflows
    SourceStateName   string      // Source state name for output derivation
    ExecutionName     string      // Name for the new execution
    ExecutionIndex    int         // Index in batch execution
    Input             interface{} // Optional input (or derived from source)
    Options           map[string]interface{} // Additional options
}
```

## Queue Priority

Tasks can be enqueued with different priorities:

```go
// Default priority
queueClient.EnqueueExecution(payload)

// Critical priority
queueClient.EnqueueExecutionWithPriority(payload, "critical")

// Low priority
queueClient.EnqueueExecutionWithPriority(payload, "low")
```

Workers process tasks based on queue weights:
- `critical`: 6 (highest priority)
- `default`: 3
- `low`: 1 (lowest priority)

## Monitoring with Asynqmon

Access the web UI at `http://localhost:8080` to view:

- **Queue Statistics**: Tasks pending, active, completed, failed
- **Task Details**: Payload, retry count, error messages
- **Scheduled Tasks**: Tasks scheduled for future processing
- **Failed Tasks**: Inspect and retry failed tasks manually
- **Real-time Updates**: Live statistics and task processing

## Error Handling and Retries

### Retry Policy

Configure retry behavior in `queue.Config`:

```go
RetryPolicy: &queue.RetryPolicy{
    MaxRetry: 3,              // Retry up to 3 times
    Timeout:  10 * time.Minute, // Task timeout
}
```

### Exponential Backoff

Asynq automatically applies exponential backoff between retries:
- 1st retry: ~15 seconds
- 2nd retry: ~1 minute
- 3rd retry: ~5 minutes

### Task Failure

If a task exhausts all retries, it moves to the "dead" queue where it can be:
- Inspected via Asynqmon
- Manually retried
- Archived

## Comparison: Local vs Distributed Execution

| Feature | Local (No Queue Client) | Distributed (With Queue Client) |
|---------|-------------------------|----------------------------------|
| Execution | In-process goroutines | Redis-backed workers |
| Scaling | Limited to single process | Horizontal scaling with multiple workers |
| Reliability | Lost on process crash | Persisted in Redis, survives crashes |
| Monitoring | Manual logging | Web UI (Asynqmon) |
| Retry Logic | Custom implementation | Built-in with exponential backoff |
| Load Balancing | Manual semaphore | Automatic across workers |

## Best Practices

1. **Leader-Worker Separation**: Run leaders and workers as separate processes
2. **Multiple Workers**: Deploy multiple worker instances for redundancy and performance
3. **Queue Priorities**: Use priority queues for critical vs background tasks
4. **Monitoring**: Regularly check Asynqmon for stuck or failed tasks
5. **Graceful Shutdown**: Handle SIGTERM/SIGINT to allow workers to finish current tasks
6. **Resource Limits**: Configure worker concurrency based on available resources
7. **Task Timeouts**: Set appropriate timeouts for long-running state machines

## Example Application

See `examples/distributed_queue/` for a complete example with:
- Leader mode for enqueueing tasks
- Worker mode for processing tasks
- Configuration via command-line flags
- Graceful shutdown handling
- PostgreSQL and Redis integration

## Migration Guide

To migrate existing batch executions to distributed mode:

### Before (Local Execution)

```go
sm, _ := persistent.NewFromDefnId(ctx, "my-sm", repo)
results, _ := sm.ExecuteBatch(ctx, filter, "StateName", batchOpts)
```

### After (Distributed Execution)

```go
// Setup queue client
queueClient, _ := queue.NewClient(queueConfig)
defer queueClient.Close()

// Attach to state machine
sm, _ := persistent.NewFromDefnId(ctx, "my-sm", repo)
sm.SetQueueClient(queueClient) // Enable distributed mode

// Same API, different execution model
results, _ := sm.ExecuteBatch(ctx, filter, "StateName", batchOpts)

// Start workers separately
worker, _ := queue.NewWorker(queueConfig, repo)
worker.Run()
```

## Troubleshooting

### Tasks Not Processing

1. Check Redis connection: `redis-cli ping`
2. Verify workers are running: Check Asynqmon "Servers" tab
3. Check queue configuration: Ensure queue names match

### High Task Failure Rate

1. Review error messages in Asynqmon
2. Check worker logs for stack traces
3. Verify database connectivity
4. Increase task timeout if needed

### Performance Issues

1. Increase worker concurrency
2. Add more worker instances
3. Optimize state machine execution
4. Check Redis performance metrics
5. Review database query performance

## References

- [Asynq Documentation](https://github.com/hibiken/asynq)
- [Asynqmon GitHub](https://github.com/hibiken/asynqmon)
- [Redis Documentation](https://redis.io/documentation)

# Release Notes

## v1.1.0 - Distributed Queue Execution Support (2026-01-04)

### 🚀 Major Features

#### Distributed Queue Execution
- **Redis-backed Task Queue**: Execute state machine workflows across multiple worker nodes
- **Priority-based Queues**: Configure multiple queues with different priority levels
- **Leader-Worker Architecture**: Separate processes for task generation (leader) and execution (workers)
- **Automatic Queue Assignment**: Dynamic queue creation based on state machine IDs
- **Task Retry Policies**: Configurable retry mechanisms with timeout support

#### Repository Enhancements
- **List State Machines**: New `ListStateMachines()` API with filtering support
- **Execution Context Support**: Pass execution context to task handlers via `ExecutionContextAdapter`
- **Performance Optimizations**:
  - Fixed GORM upsert operations for faster execution saves
  - State history now uses `INSERT ... ON CONFLICT DO NOTHING` (avoiding expensive updates)
  - Unique execution ID generation prevents database conflicts

#### Batch Execution Modes
- **Distributed Mode**: Enqueue tasks to Redis queue for distributed processing
- **Concurrent Mode**: Execute tasks concurrently in the same process
- **Sequential Mode**: Execute tasks one by one

### 🔧 API Changes

#### New Repository Methods
```go
// List all state machines with optional filtering
ListStateMachines(ctx context.Context, filter *DefinitionFilter) ([]*StateMachineRecord, error)
```

#### New Handler Methods
```go
// Create handler with execution context support
NewExecutionHandlerWithContext(repoManager *repository.Manager, execCtx states.ExecutionContext)
```

#### Batch Execution Options
```go
type BatchExecutionOptions struct {
    NamePrefix        string
    ConcurrentBatches int
    StopOnError       bool
    Mode              string  // "distributed", "concurrent", "sequential"
}
```

### 📦 Queue Package

#### Queue Configuration
```go
type Config struct {
    RedisAddr     string
    RedisPassword string
    RedisDB       int
    Concurrency   int
    Queues        map[string]int  // queue name -> priority
    RetryPolicy   *RetryPolicy
}
```

#### Queue Client
```go
// Create queue client
queueClient, err := queue.NewClient(config)

// Enqueue execution task
taskInfo, err := queueClient.EnqueueExecution(payload)

// Get queue statistics
stats, err := queueClient.GetStats()
```

#### Queue Worker
```go
// Create worker with execution handler
worker, err := queue.NewWorker(config, handler)

// Start processing tasks
worker.Run()

// Graceful shutdown
worker.Shutdown()
```

### 🐛 Bug Fixes

- **Execution ID Conflicts**: Fixed issue where executions were reusing state machine ID instead of generating unique IDs
- **Slow SQL Queries**: Optimized GORM save operations to use proper upsert instead of full table updates
- **State History Performance**: Changed from 316ms updates to fast inserts with conflict handling

### 📊 Performance Improvements

| Operation | Before | After | Improvement |
|-----------|--------|-------|-------------|
| Save Execution | 265ms (UPDATE) | <10ms (INSERT) | 96% faster |
| Save State History | 316ms (UPSERT) | <10ms (INSERT) | 97% faster |
| Batch Processing | Sequential only | Distributed queues | Scalable |

### 📖 Examples

#### Distributed Queue Example
See `examples/distributed_queue/main.go` for a complete example demonstrating:
- Leader mode: Enqueue 1M+ execution tasks
- Worker mode: Process tasks from distributed queue
- Order processing workflow with 4 states
- Task executor with registered handlers

#### Running the Example
```bash
# Start workers (multiple instances for scale)
go run examples/distributed_queue/main.go -mode=worker -concurrency=10

# Start leader to enqueue tasks
go run examples/distributed_queue/main.go -mode=leader
```

### 🔄 Migration Guide

#### Updating Existing Code

1. **Execution ID Generation**: Executions now get unique IDs automatically
   ```go
   // Before: exec.ID was the state machine ID
   // After: exec.ID is unique per execution (e.g., "sm-id-exec-1234567890")
   ```

2. **Using Distributed Queue**: Add queue client to state machine
   ```go
   queueClient, err := queue.NewClient(queueConfig)
   sm.SetQueueClient(queueClient)

   // Batch execution will now use distributed queue
   results, err := sm.ExecuteBatch(ctx, filter, "", batchOpts)
   ```

3. **Execution Context Support**: Pass task handlers to workers
   ```go
   exec := executor.NewBaseExecutor()
   exec.RegisterGoFunction("task:handler", handlerFunc)

   execAdapter := executor.NewExecutionContextAdapter(exec)
   handler := persistent.NewExecutionHandlerWithContext(repoManager, execAdapter)
   ```

### ⚠️ Breaking Changes

None. This release is backward compatible with v1.0.x.

### 📝 Requirements

- Go 1.21 or later
- PostgreSQL (for persistence)
- Redis (for distributed queue - optional)

### 🙏 Credits

Special thanks to all contributors for testing and feedback!

---

**Full Changelog**: https://github.com/hussainpithawala/state-machine-amz-go/compare/v1.0.8...v1.1.0

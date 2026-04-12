# Asynq Integration Summary

## What Was Implemented

This integration adds distributed task queue capabilities to the state machine framework using [asynq](https://github.com/hibiken/asynq).

## New Components

### 1. Queue Package (`pkg/queue/`)

#### `task.go`
- Task type definitions (`TypeExecutionTask`)
- `ExecutionTaskPayload` struct for task data
- Task creation and parsing functions

#### `config.go`
- Queue configuration with Redis settings
- Retry policy configuration
- Queue priority definitions
- Default configuration factory

#### `client.go`
- Queue client for enqueuing tasks
- Priority-based task enqueueing
- Scheduled task support
- Configurable retry and timeout options

#### `worker.go`
- Worker server for processing tasks
- Task handler registration
- State machine execution from task payload
- Graceful shutdown support

### 2. Modified Files

#### `pkg/statemachine/persistent/persistent.go`

**Added:**
- `queueClient` field to `StateMachine` struct
- `SetQueueClient()` and `GetQueueClient()` methods
- `executeBatchViaQueue()` - enqueues tasks to Redis instead of direct execution
- `executeBatchLocal()` - original local execution (renamed from `executeBatchConcurrent`)

**Modified:**
- `executeBatchConcurrent()` - now routes to queue or local execution based on queue client presence

### 3. Example Application

#### `examples/distributed_queue/main.go`

Complete example demonstrating:
- **Leader Mode**: Enqueues batch execution tasks
- **Worker Mode**: Processes tasks from queue
- Command-line flags for configuration
- Graceful shutdown handling
- PostgreSQL + Redis integration

### 4. Infrastructure

#### `docker-examples/docker-compose.yml`

**Added Services:**
- **Redis**: Task queue backend (port 6379)
- **Asynqmon**: Web UI for monitoring (port 8080)

**Configuration:**
- Persistent Redis storage
- Health checks
- Network configuration for service communication

## Key Features

### Distributed Execution

When a queue client is attached to a state machine, `ExecuteBatch()` automatically switches from local goroutine-based execution to distributed queue-based execution:

```go
// Local execution (original behavior)
sm.ExecuteBatch(ctx, filter, "State", opts)

// Distributed execution (with queue client)
sm.SetQueueClient(queueClient)
sm.ExecuteBatch(ctx, filter, "State", opts) // Now enqueues to Redis
```

### Horizontal Scaling

Run multiple worker instances to process tasks in parallel:

```bash
# Leader - enqueues tasks
./app -mode=leader

# Worker 1
./app -mode=worker -concurrency=10

# Worker 2
./app -mode=worker -concurrency=10

# Worker 3
./app -mode=worker -concurrency=10
```

### Priority Queues

Tasks can be assigned to different priority queues:

- **critical** (weight: 6) - Highest priority
- **default** (weight: 3) - Normal priority
- **low** (weight: 1) - Background tasks

### Monitoring

Asynqmon web UI provides:
- Real-time queue statistics
- Task inspection and debugging
- Manual retry of failed tasks
- Server/worker status
- Historical metrics

### Reliability

Built-in reliability features:
- Tasks persisted in Redis (survive crashes)
- Automatic retry with exponential backoff
- Configurable retry limits and timeouts
- Dead letter queue for exhausted retries

## Usage Flow

### 1. Start Infrastructure

```bash
cd docker-examples
docker-compose up -d
```

### 2. Run Leader (Enqueue Tasks)

```bash
go run examples/distributed_queue/main.go \
  -mode=leader \
  -redis=localhost:6379 \
  -postgres="postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
```

### 3. Run Workers (Process Tasks)

```bash
# Terminal 1
go run examples/distributed_queue/main.go \
  -mode=worker \
  -redis=localhost:6379 \
  -postgres="postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable" \
  -concurrency=10
```

### 4. Monitor

Open http://localhost:8080 in your browser to view:
- Queue depths
- Active tasks
- Completed/failed tasks
- Worker status

## Architecture Diagram

```
┌─────────────┐
│   Leader    │
│  (Client)   │
└──────┬──────┘
       │ Enqueue Tasks
       ↓
┌─────────────┐
│    Redis    │←─────┐
│   (Queue)   │      │
└──────┬──────┘      │
       │             │
       │ Poll Tasks  │ Update Status
       ↓             │
┌─────────────┐      │
│  Worker 1   │──────┤
└─────────────┘      │
┌─────────────┐      │
│  Worker 2   │──────┤
└─────────────┘      │
┌─────────────┐      │
│  Worker 3   │──────┘
└─────────────┘
       │
       │ Persist Results
       ↓
┌─────────────┐
│ PostgreSQL  │
│ (Repository)│
└─────────────┘
```

## API Changes

### New Methods

```go
// StateMachine
func (pm *StateMachine) SetQueueClient(client *queue.Client)
func (pm *StateMachine) GetQueueClient() *queue.Client

// Queue Client
func NewClient(config *Config) (*Client, error)
func (c *Client) EnqueueExecution(payload *ExecutionTaskPayload, opts ...asynq.Option) (*asynq.TaskInfo, error)
func (c *Client) EnqueueExecutionWithPriority(payload *ExecutionTaskPayload, priority string) (*asynq.TaskInfo, error)
func (c *Client) Close() error

// Queue Worker
func NewWorker(config *Config, repositoryManager *repository.Manager) (*Worker, error)
func (w *Worker) Run() error
func (w *Worker) Shutdown()
```

### Backward Compatibility

All existing code continues to work without changes:
- `ExecuteBatch()` without queue client uses local execution
- No breaking changes to existing APIs
- Queue integration is opt-in

## Configuration

### Environment Variables (Example)

```bash
# Redis
export REDIS_ADDR=localhost:6379
export REDIS_PASSWORD=
export REDIS_DB=0

# Worker
export WORKER_CONCURRENCY=10

# PostgreSQL
export POSTGRES_URL="postgres://user:pass@localhost:5432/db"
```

### Command-Line Flags (Example)

```bash
-mode=worker              # Application mode: leader or worker
-redis=localhost:6379     # Redis address
-redis-password=          # Redis password
-redis-db=0               # Redis database number
-concurrency=10           # Worker concurrency
-postgres=postgres://...  # PostgreSQL connection URL
-sm-id=my-state-machine   # State machine ID
```

## Files Created

```
pkg/queue/
  ├── task.go           # Task definitions
  ├── config.go         # Configuration
  ├── client.go         # Queue client
  └── worker.go         # Queue worker

examples/distributed_queue/
  ├── main.go           # Example application
  └── README.md         # Example documentation

docs/
  └── DISTRIBUTED_QUEUE.md  # Integration documentation

docker-examples/
  └── docker-compose.yml    # Updated with Redis + Asynqmon
```

## Dependencies Added

```
github.com/hibiken/asynq v0.25.1
github.com/redis/go-redis/v9 v9.7.0
```

## Testing

To test the integration:

```bash
# 1. Start infrastructure
cd docker-examples && docker-compose up -d

# 2. Run worker
go run examples/distributed_queue/main.go -mode=worker

# 3. Run leader (in another terminal)
go run examples/distributed_queue/main.go -mode=leader

# 4. Check Asynqmon
open http://localhost:8080
```

## Benefits

1. **Scalability**: Horizontal scaling with multiple workers
2. **Reliability**: Task persistence and automatic retries
3. **Observability**: Real-time monitoring via web UI
4. **Flexibility**: Priority queues for workload management
5. **Simplicity**: Minimal code changes, opt-in integration
6. **Production-Ready**: Battle-tested asynq framework

## Next Steps

1. Deploy workers in production environment
2. Configure monitoring alerts based on Asynqmon metrics
3. Tune concurrency settings based on load testing
4. Implement custom task types for specialized workflows
5. Add metrics export (Prometheus integration available)
6. Configure Redis persistence strategy
7. Set up Redis replication for high availability

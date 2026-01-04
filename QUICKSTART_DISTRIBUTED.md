# Quick Start: Distributed Queue Execution

This guide helps you get started with distributed state machine execution using Redis and asynq.

## Prerequisites

- Go 1.19+
- Docker and Docker Compose
- PostgreSQL (or use Docker Compose)
- Redis (or use Docker Compose)

## Step 1: Start Infrastructure

```bash
cd docker-examples
docker-compose up -d
```

This starts:
- **PostgreSQL** on port `5432`
- **Redis** on port `6379`
- **Asynqmon UI** on port `8080` (http://localhost:8080)

## Step 2: Build the Example

```bash
cd examples/distributed_queue
go build -o distributed_queue .
```

## Step 3: Start Worker(s)

Start one or more workers to process tasks:

```bash
# Worker 1
./distributed_queue \
  -mode=worker \
  -redis=localhost:6379 \
  -postgres="postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable" \
  -concurrency=10
```

In separate terminals, start additional workers for horizontal scaling:

```bash
# Worker 2
./distributed_queue -mode=worker -concurrency=10

# Worker 3
./distributed_queue -mode=worker -concurrency=10
```

## Step 4: Run Leader (Enqueue Tasks)

In another terminal, run the leader to enqueue batch execution tasks:

```bash
./distributed_queue \
  -mode=leader \
  -redis=localhost:6379 \
  -postgres="postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable" \
  -sm-id=order-processing-sm
```

## Step 5: Monitor with Asynqmon

Open http://localhost:8080 in your browser to:
- View queue statistics
- Monitor active tasks
- Inspect failed tasks
- Retry failed tasks manually
- View worker status

## Architecture Overview

```
┌──────────┐
│  Leader  │ Enqueues tasks
└────┬─────┘
     │
     ▼
┌─────────┐
│  Redis  │ Task queue
└────┬────┘
     │
     ├──────► Worker 1 (Concurrency: 10)
     ├──────► Worker 2 (Concurrency: 10)
     └──────► Worker 3 (Concurrency: 10)
                  │
                  ▼
            ┌──────────┐
            │PostgreSQL│ Execution results
            └──────────┘
```

## How It Works

### Leader Mode

1. Connects to Redis and PostgreSQL
2. Loads the state machine definition
3. Creates a queue client
4. Calls `ExecuteBatch()` which detects the queue client
5. Instead of executing locally, enqueues tasks to Redis
6. Each task contains:
   - State machine ID
   - Source execution ID (for chained workflows)
   - Execution name
   - Execution parameters

### Worker Mode

1. Connects to Redis and PostgreSQL
2. Creates an execution handler
3. Starts the asynq worker server
4. Polls Redis for tasks
5. For each task:
   - Loads the state machine definition from PostgreSQL
   - Executes the state machine
   - Persists results to PostgreSQL
6. Retries failed tasks automatically (up to 3 times by default)

## Integration into Your Code

### 1. Setup Queue Client

```go
import "github.com/hussainpithawala/state-machine-amz-go/pkg/queue"

queueConfig := &queue.Config{
    RedisAddr:     "localhost:6379",
    RedisPassword: "",
    RedisDB:       0,
    Concurrency:   10,
    Queues: map[string]int{
        "critical": 6,
        "default":  3,
        "low":      1,
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

### 2. Enable Distributed Execution

```go
import "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"

// Load state machine
sm, err := persistent.NewFromDefnId(ctx, "my-state-machine", repoManager)
if err != nil {
    log.Fatal(err)
}

// Attach queue client to enable distributed execution
sm.SetQueueClient(queueClient)

// Now ExecuteBatch will enqueue to Redis instead of executing locally
results, err := sm.ExecuteBatch(ctx, filter, "StateName", batchOpts)
```

### 3. Run Workers

```go
import (
    "github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
    "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

// Create execution handler
handler := persistent.NewExecutionHandler(repoManager)

// Create worker
worker, err := queue.NewWorker(queueConfig, handler)
if err != nil {
    log.Fatal(err)
}

// Start processing tasks (blocks until shutdown)
if err := worker.Run(); err != nil {
    log.Fatal(err)
}
```

## Configuration Options

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-mode` | `leader` | `leader` or `worker` |
| `-redis` | `localhost:6379` | Redis server address |
| `-redis-password` | `""` | Redis password (if required) |
| `-redis-db` | `0` | Redis database number |
| `-concurrency` | `10` | Number of concurrent workers |
| `-postgres` | `postgres://...` | PostgreSQL connection URL |
| `-sm-id` | `order-processing-sm` | State machine ID |

### Environment Variables

You can also use environment variables:

```bash
export REDIS_ADDR=localhost:6379
export REDIS_PASSWORD=mypassword
export REDIS_DB=0
export POSTGRES_URL="postgres://user:pass@localhost:5432/db?sslmode=disable"
export WORKER_CONCURRENCY=10
```

## Common Use Cases

### 1. Local Development

Run everything locally with Docker Compose:

```bash
# Start infrastructure
docker-compose up -d

# Start worker
./distributed_queue -mode=worker

# Run leader
./distributed_queue -mode=leader
```

### 2. Production Deployment

Deploy leader and workers as separate services:

```yaml
# docker-compose.production.yml
services:
  leader:
    image: myapp:latest
    command: ["-mode=leader"]
    environment:
      - REDIS_ADDR=redis:6379
      - POSTGRES_URL=postgres://...

  worker:
    image: myapp:latest
    command: ["-mode=worker", "-concurrency=20"]
    environment:
      - REDIS_ADDR=redis:6379
      - POSTGRES_URL=postgres://...
    deploy:
      replicas: 5  # Run 5 worker instances
```

### 3. Kubernetes Deployment

```yaml
# worker-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: statemachine-worker
spec:
  replicas: 10
  template:
    spec:
      containers:
      - name: worker
        image: myapp:latest
        args: ["-mode=worker", "-concurrency=10"]
        env:
        - name: REDIS_ADDR
          value: "redis-service:6379"
        - name: POSTGRES_URL
          valueFrom:
            secretKeyRef:
              name: db-secret
              key: connection-url
```

## Monitoring and Observability

### Asynqmon Dashboard

Access http://localhost:8080 for:
- Real-time queue statistics
- Task processing rates
- Error rates and failed tasks
- Worker health and status

### Custom Metrics

Asynq supports Prometheus metrics:

```go
import "github.com/hibiken/asynq/x/metrics"

// Enable Prometheus metrics
inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr})
metrics.ServePrometheusMetrics(inspector, ":9090")
```

### Logging

Workers log execution details:
```
2025/01/04 09:16:45 Starting application in WORKER mode...
2025/01/04 09:16:45 Starting worker to process state machine execution tasks...
2025/01/04 09:16:48 Processing execution task: StateMachineID=order-processing-sm, SourceExecutionID=exec-123, ExecutionName=batch-exec-1, Index=0
2025/01/04 09:16:49 Execution completed successfully: ExecutionName=batch-exec-1
```

## Troubleshooting

### Tasks Not Processing

**Check Redis connection:**
```bash
redis-cli -h localhost -p 6379 ping
# Should return: PONG
```

**Check workers are running:**
- Look for worker processes
- Check Asynqmon "Servers" tab
- Verify worker logs

### High Failure Rate

**Inspect failed tasks in Asynqmon:**
1. Go to "Failed" tab
2. Click on a failed task
3. Review error message and stack trace
4. Manually retry after fixing the issue

**Common causes:**
- Database connection issues
- Invalid state machine definitions
- Task timeouts (increase timeout in config)
- Resource exhaustion (reduce concurrency)

### Performance Issues

**Scale horizontally:**
```bash
# Add more workers
for i in {1..5}; do
  ./distributed_queue -mode=worker -concurrency=10 &
done
```

**Optimize concurrency:**
- Monitor CPU and memory usage
- Adjust `-concurrency` flag
- Balance number of workers vs concurrency per worker

**Database optimization:**
- Add indexes on execution tables
- Increase connection pool size
- Use connection pooling (PgBouncer)

## Next Steps

1. Read [DISTRIBUTED_QUEUE.md](docs/DISTRIBUTED_QUEUE.md) for detailed architecture
2. Review [INTEGRATION_SUMMARY.md](INTEGRATION_SUMMARY.md) for API details
3. Customize the example for your state machine definitions
4. Deploy to production with proper monitoring
5. Set up alerting based on Asynqmon metrics

## Resources

- [Asynq Documentation](https://github.com/hibiken/asynq)
- [Asynqmon UI](https://github.com/hibiken/asynqmon)
- [Redis Best Practices](https://redis.io/topics/best-practices)
- [State Machine Framework Docs](docs/)

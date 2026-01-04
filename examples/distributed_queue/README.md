# Distributed Queue Example

This example demonstrates how to run the state machine with distributed task processing using Redis and asynq.

## Architecture

The application can run in two modes:

1. **Leader Mode**: Enqueues execution tasks to Redis queue
2. **Worker Mode**: Processes execution tasks from Redis queue

## Prerequisites

- Redis server running (default: localhost:6379)
- PostgreSQL database running
- Go 1.19 or later

## Running with Docker Compose

From the `docker-examples` directory:

```bash
cd ../../docker-examples
docker-compose up -d
```

This will start:
- PostgreSQL database
- Redis server
- Asynqmon UI (http://localhost:8080)

## Running the Application

### Leader Mode

Enqueues batch execution tasks:

```bash
go run main.go \
  -mode=leader \
  -redis=localhost:6379 \
  -postgres="postgres://user:password@localhost:5432/statemachine?sslmode=disable" \
  -sm-id=order-processing-sm
```

### Worker Mode

Processes tasks from the queue:

```bash
go run main.go \
  -mode=worker \
  -redis=localhost:6379 \
  -postgres="postgres://user:password@localhost:5432/statemachine?sslmode=disable" \
  -concurrency=10
```

You can run multiple workers for horizontal scaling:

```bash
# Terminal 1
go run main.go -mode=worker -concurrency=5

# Terminal 2
go run main.go -mode=worker -concurrency=5

# Terminal 3
go run main.go -mode=worker -concurrency=5
```

## Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `-mode` | `leader` | Application mode: `leader` or `worker` |
| `-redis` | `localhost:6379` | Redis address |
| `-redis-password` | `""` | Redis password |
| `-redis-db` | `0` | Redis database number |
| `-concurrency` | `10` | Worker concurrency (worker mode only) |
| `-postgres` | `postgres://...` | PostgreSQL connection URL |
| `-sm-id` | `order-processing-sm` | State machine ID |

## Monitoring

Access the Asynqmon web UI at http://localhost:8080 to monitor:
- Queue statistics
- Active tasks
- Scheduled tasks
- Failed tasks
- Task history

## How It Works

### Leader Mode

1. Connects to Redis and PostgreSQL
2. Creates a queue client
3. Loads or creates the state machine
4. Enqueues batch execution tasks to Redis
5. Each task contains:
   - State machine ID
   - Source execution ID
   - Execution parameters

### Worker Mode

1. Connects to Redis and PostgreSQL
2. Creates a worker server
3. Polls Redis for tasks
4. For each task:
   - Loads the state machine definition
   - Executes the state machine with the task parameters
   - Updates execution status in PostgreSQL
5. Retries failed tasks based on retry policy

## Benefits of Distributed Execution

- **Scalability**: Run multiple workers to process tasks in parallel
- **Reliability**: Tasks are persisted in Redis and retried on failure
- **Monitoring**: Real-time visibility into task processing
- **Decoupling**: Leaders and workers run independently
- **Load Balancing**: Tasks are automatically distributed across workers

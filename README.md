# state-machine-amz-go

<!-- Badges -->
[![GoDoc](https://pkg.go.dev/badge/github.com/hussainpithawala/state-machine-amz-go.svg)](https://pkg.go.dev/github.com/hussainpithawala/state-machine-amz-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/hussainpithawala/state-machine-amz-go)](https://goreportcard.com/report/github.com/hussainpithawala/state-machine-amz-go)
[![GitHub Release](https://img.shields.io/github/v/release/hussainpithawala/state-machine-amz-go)](https://github.com/hussainpithawala/state-machine-amz-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml/badge.svg)](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml)

A powerful, production-ready state machine implementation for Go that's fully compatible with Amazon States Language. Build complex workflows using YAML/JSON definitions and execute them locally with native Go functions or integrate with external services.

## ✨ Features

- 🚀 **AWS Step Functions Compatible** - Use identical state definitions as AWS Step Functions ([Amazon States Language](https://states-language.net/))
- 💾 **Persistent Execution** - Built-in PostgreSQL persistence with GORM support
- 🔧 **Local Execution** - Execute workflows with native Go functions, no cloud required
- 📁 **YAML/JSON Support** - Human-readable workflow definitions
- 🛡️ **Comprehensive Error Handling** - Retry policies, catch blocks, and timeout management
- ⏱️ **Advanced Control** - Timeouts, heartbeats, and execution tracking
- 🔄 **All State Types** - Task, Parallel, Choice, Wait, Pass, Succeed, Fail, Map, Message
- 📩 **Message Correlation** - Pause workflows and resume with external asynchronous messages
- 🔗 **Execution Chaining** - Chain multiple state machines together for complex multi-stage workflows
- 📦 **Batch Execution** - Execute chained workflows in batch mode with filtering and concurrency control
- 🎛️ **Micro-Batch Orchestration** - Streaming framework with adaptive failure-rate control and pause/resume (NEW in v1.2.11)
- 🌐 **Distributed Queue** - Redis-backed task queue for horizontal scaling across workers
- 🎯 **REST API** - Complete HTTP API via [state-machine-amz-gin](https://github.com/hussainpithawala/state-machine-amz-gin)
- 🏗️ **Clean Architecture** - Separation between state machine logic and persistence
- 📊 **Execution History** - Complete audit trail with state-by-state tracking
- 🧪 **Test-Friendly** - Easy mocking and comprehensive testing support
- 🔌 **Pluggable Storage** - Support for multiple persistence backends (PostgreSQL, GORM, In-Memory)

## 📦 Installation

```bash
go get github.com/hussainpithawala/state-machine-amz-go
```

### Additional Dependencies

For PostgreSQL persistence with GORM:
```bash
go get gorm.io/gorm
go get gorm.io/driver/postgres
```

For PostgreSQL persistence with raw SQL:
```bash
go get github.com/lib/pq
```

For distributed queue and micro-batch orchestration (optional):
```bash
go get github.com/hibiken/asynq
go get github.com/redis/go-redis/v9
```

For REST API framework (optional):
```bash
go get github.com/hussainpithawala/state-machine-amz-gin
```

## 🚀 Quick Start

### 1. Define Your Workflow in YAML

```yaml
# workflows/order_processing.yaml
Comment: "Order Processing Workflow"
StartAt: ProcessOrder
States:
  ProcessOrder:
    Type: Task
    Resource: "arn:aws:lambda:::process:order"
    ResultPath: "$.orderResult"
    Next: ValidatePayment

  ValidatePayment:
    Type: Task
    Resource: "arn:aws:lambda:::validate:payment"
    ResultPath: "$.paymentResult"
    Next: SendNotification

  SendNotification:
    Type: Task
    Resource: "arn:aws:lambda:::send:notification"
    ResultPath: "$.notificationResult"
    End: true
```

### 2. Execute With Persistence

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

func main() {
	ctx := context.Background()

	// 1. Configure persistence (GORM or raw PostgreSQL)
	config := &repository.Config{
		Strategy:      "gorm-postgres", // or "postgres" for raw SQL
		ConnectionURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Options: map[string]interface{}{
			"max_open_conns":    25,
			"max_idle_conns":    5,
			"conn_max_lifetime": 5 * time.Minute,
		},
	}

	// 2. Create persistence manager
	manager, err := repository.NewPersistenceManager(config)
	if err != nil {
		log.Fatal(err)
	}
	defer manager.Close()

	// 3. Initialize database (auto-creates tables)
	if err := manager.Initialize(ctx); err != nil {
		log.Fatal(err)
	}

	// 4. Load workflow definition
	yamlContent := []byte(`...`) // Your YAML here

	// 5. Create persistent state machine
	psm, err := persistent.New(yamlContent, false, "order-workflow-v1", manager)
	if err != nil {
		log.Fatal(err)
	}

	// 6. Register task handlers
	exec := executor.NewBaseExecutor()
	exec.RegisterGoFunction("process:order", processOrder)
	exec.RegisterGoFunction("validate:payment", validatePayment)
	exec.RegisterGoFunction("send:notification", sendNotification)

	// 7. Create execution context
	execCtx := &execution.Execution{
		ID:             "exec-001",
		Name:           "OrderProcessing",
		StateMachineID: "order-workflow-v1",
		Input: map[string]interface{}{
			"orderId": "ORD-12345",
			"amount":  150.00,
		},
		StartTime: time.Now(),
		Status:    "RUNNING",
	}

	// 8. Execute workflow (automatically persisted)
	result, err := psm.Execute(ctx, execCtx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("✓ Execution completed: %s (Status: %s)\n", result.ID, result.Status)

	// 9. Query execution history
	history, _ := psm.GetExecutionHistory(ctx, result.ID)
	for _, h := range history {
		fmt.Printf("[%d] %s: %s\n", h.SequenceNumber, h.StateName, h.Status)
	}
}

func processOrder(ctx context.Context, input interface{}) (interface{}, error) {
	fmt.Println("Processing order...")
	return map[string]interface{}{"processed": true}, nil
}

func validatePayment(ctx context.Context, input interface{}) (interface{}, error) {
	fmt.Println("Validating payment...")
	return map[string]interface{}{"valid": true}, nil
}

func sendNotification(ctx context.Context, input interface{}) (interface{}, error) {
	fmt.Println("Sending notification...")
	return map[string]interface{}{"sent": true}, nil
}
```

### 3. Query and Filter Executions

```go
// List executions with filtering
filter := &repository.Filter{
	StateMachineID: "order-workflow-v1",
	Status:         "SUCCEEDED",
	Limit:          10,
}

executions, _ := psm.ListExecutions(ctx, filter)
count, _ := psm.CountExecutions(ctx, filter)

fmt.Printf("Found %d successful executions\n", count)
```

## 📚 State Types

<details>
<summary><b>Task State</b> - Execute a unit of work</summary>

```yaml
ProcessPayment:
  Type: Task
  Resource: "arn:aws:lambda:::process:payment"
  Next: "NextState"
  TimeoutSeconds: 30
  Retry:
    - ErrorEquals: ["States.TaskFailed"]
      MaxAttempts: 3
      BackoffRate: 2.0
  Catch:
    - ErrorEquals: ["States.ALL"]
      ResultPath: "$.error"
      Next: "HandleError"
```
</details>

<details>
<summary><b>Parallel State</b> - Execute branches concurrently</summary>

```yaml
ParallelProcessing:
  Type: Parallel
  Branches:
    - StartAt: ProcessPayment
      States:
        ProcessPayment:
          Type: Task
          Resource: "arn:aws:lambda:::process:payment"
          End: true
    - StartAt: CheckInventory
      States:
        CheckInventory:
          Type: Task
          Resource: "arn:aws:lambda:::check:inventory"
          End: true
  Next: "CombineResults"
```
</details>

<details>
<summary><b>Choice State</b> - Conditional branching</summary>

```yaml
RouteOrder:
  Type: Choice
  Choices:
    - Variable: "$.order.total"
      NumericGreaterThan: 1000
      Next: "ProcessLargeOrder"
    - Variable: "$.customer.tier"
      StringEquals: "PREMIUM"
      Next: "ProcessPremium"
  Default: "ProcessStandard"
```
</details>

<details>
<summary><b>Wait State</b> - Pause execution</summary>

```yaml
WaitForApproval:
  Type: Wait
  Seconds: 300  # Wait 5 minutes
  Next: "ProcessApproval"
```
</details>

<details>
<summary><b>Pass State</b> - Transform data</summary>

```yaml
TransformData:
  Type: Pass
  Result:
    transformed: true
  ResultPath: "$.metadata"
  Next: "NextState"
```
</details>

<details>
<summary><b>Message State</b> - Pause and wait for external message (with timeout cancellation)</summary>

```yaml
WaitForPayment:
  Type: Message
  CorrelationKey: "orderId"
  CorrelationValuePath: "$.orderId"
  TimeoutSeconds: 3600  # Timeout automatically cancelled if message arrives
  TimeoutPath: "HandleTimeout"  # Only triggered if timeout fires
  Next: "ProcessOrder"
```

**Message State Timeouts**: BPMN-style boundary timer events with Redis-backed async task scheduling. Scheduled timeout tasks are automatically cancelled when correlated messages arrive, preventing unnecessary processing and keeping queues clean.
</details>

## 💾 Persistence Backends

### GORM PostgreSQL (Recommended)

**Best for:** Production applications, rapid development

**Features:**
- ✅ Auto-migration (no manual schema management)
- ✅ Type-safe operations
- ✅ Built-in validations and hooks
- ✅ ~95% performance of raw SQL
- ✅ Clean, maintainable code

```go
config := &repository.Config{
	Strategy:      "gorm-postgres",
	ConnectionURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
	Options: map[string]interface{}{
		"max_open_conns": 25,
		"max_idle_conns": 5,
		"log_level":      "warn", // silent, error, warn, info
	},
}
```

### Raw PostgreSQL

**Best for:** Maximum performance, complex custom queries

```go
config := &repository.Config{
	Strategy:      "postgres",
	ConnectionURL: "postgres://user:pass@localhost:5432/db?sslmode=disable",
}
```

### In-Memory (Testing)

**Best for:** Unit tests, development

```go
config := &repository.Config{
	Strategy: "memory",
}
```

## 🎯 Advanced Features

### Error Handling with Retries

```yaml
FlakyService:
  Type: Task
  Resource: "arn:aws:lambda:::flaky:service"
  Retry:
    - ErrorEquals: ["States.TaskFailed", "States.Timeout"]
      IntervalSeconds: 1
      MaxAttempts: 5
      BackoffRate: 2.0
  Catch:
    - ErrorEquals: ["ServiceUnavailable"]
      Next: "HandleError"
```

### Result Path Manipulation

```yaml
ProcessData:
  Type: Task
  Resource: "arn:aws:lambda:::process:data"
  InputPath: "$.rawData"
  ResultPath: "$.processed"
  OutputPath: "$.processed.result"
  Next: "NextState"
```

### Task Parameters

```yaml
ProcessPayment:
  Type: Task
  Resource: "process:payment"
  Parameters:
    amount.$: "$.order.total"
    currency: "USD"
    customer.$: "$.customer"
```

### Message Pause and Resume (with Async Timeouts and Cancellation)

Enable your workflows to wait for external events with BPMN-style boundary timer events.

Uses Redis-backed async task scheduling for distributed timeout processing with automatic cancellation when messages arrive.

**1. Define a Message State with Timeout:**
```yaml
WaitForApproval:
  Type: Message
  CorrelationKey: "orderId"
  CorrelationValuePath: "$.orderId"
  TimeoutSeconds: 300  # 5 minute timeout
  TimeoutPath: "HandleTimeout"
  Next: "ProcessOrder"
```

**2. Resume via Executor:**
```go
// When the external message arrives (e.g., via webhook or SQS)
// Scheduled timeout is automatically cancelled!
response, err := exec.Message(ctx, &executor.MessageRequest{
    CorrelationKey:   "orderId",
    CorrelationValue: "ORD-123",
    Data:             map[string]interface{}{"approved": true},
})
```

**3. Timeout Handling:**
```yaml
HandleTimeout:
  Type: Pass
  Result:
    status: "timeout"
    message: "Approval not received within 5 minutes"
  End: true
```

**How it works:**
1. **Message State Entered** → Timeout task scheduled in Redis queue with unique ID, timer starts
2. **Message Arrives Before Timeout** → Message is correlated, scheduled timeout task automatically cancelled from Redis ✅
3. **Timeout Expires (No Message)** → Scheduled timeout task executes, workflow transitions to TimeoutPath ⏰
4. **Race Conditions** → Handled gracefully with correlation status tracking (idempotent)
5. **Clean Queues** → No orphaned tasks after message correlation 🧹

**Architecture Benefits:**
- Distributed timeout processing across multiple workers
- Redis persistence for reliability
- Automatic cleanup when messages are correlated

### Execution Chaining

Build complex multi-stage workflows by chaining state machine executions together!

**Chain using final output:**
```go
// Execute State Machine A
execA, _ := stateMachineA.Execute(ctx, inputA)

// Chain State Machine B using A's final output
execB, _ := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
)
```

**Chain using specific state output:**
```go
// Use output from a specific state
execB, _ := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID, "ProcessData"),
)
```

**Chain with transformation:**
```go
// Apply custom transformation to the output
execB, _ := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
    statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
        data := output.(map[string]interface{})
        return map[string]interface{}{
            "processedData": data["result"],
            "timestamp": time.Now(),
        }, nil
    }),
)
```

**Use Cases:**
- Multi-stage data processing pipelines (Ingest → Validate → Enrich → Store)
- Event-driven workflow orchestration
- Microservices choreography
- ETL workflows

**[📖 Full Documentation](examples/chained_postgres_gorm/CHAINED_EXECUTION_README.md)**

### Batch Chained Execution

Execute chained workflows in batch mode by filtering source executions!

**Basic batch execution:**
```go
// Filter source executions
filter := &repository.ExecutionFilter{
    StateMachineID: sourceStateMachine.GetID(),
    Status:         "SUCCEEDED",
    StartAfter:     time.Now().Add(-24 * time.Hour),
    Limit:          100,
}

// Configure batch options
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "batch-processing",
    ConcurrentBatches: 5,  // Process 5 at a time
    StopOnError:       false,
}

// Execute batch
results, _ := targetStateMachine.ExecuteBatch(ctx, filter, "", batchOpts)

// Process results
for _, result := range results {
    if result.Error != nil {
        log.Printf("Execution %d failed: %v", result.Index, result.Error)
    } else {
        log.Printf("Execution %d succeeded: %s", result.Index, result.Execution.Status)
    }
}
```

**With progress monitoring:**
```go
completedCount := 0
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "monitored-batch",
    ConcurrentBatches: 3,
    OnExecutionStart: func(sourceExecutionID string, index int) {
        log.Printf("⏳ Starting execution %d for %s", index, sourceExecutionID)
    },
    OnExecutionComplete: func(sourceExecutionID string, index int, err error) {
        completedCount++
        if err != nil {
            log.Printf("❌ Execution %d failed", index)
        } else {
            log.Printf("✅ Execution %d succeeded", index)
        }
    },
}

results, _ := targetStateMachine.ExecuteBatch(ctx, filter, "", batchOpts)
log.Printf("Batch complete: %d/%d succeeded", completedCount, len(results))
```

**Use Cases:**
- Data pipeline orchestration (process hundreds of data files)
- Order batch processing (fulfill multiple orders)
- Report generation (generate reports for multiple accounts)
- ETL operations (transform data from multiple sources)

**[📖 Full Documentation](examples/batch_chained_postgres_gorm/BATCH_CHAINED_EXECUTION_README.md)**

### Distributed Queue Execution

Scale your state machine executions horizontally with Redis-backed task queues!

**1. Setup Queue Infrastructure:**

```go
// Configure Redis-backed queue
queueConfig := &queue.Config{
    RedisAddr:     "localhost:6379",
    RedisPassword: "",
    RedisDB:       0,
    Concurrency:   10,
    Queues: map[string]int{
        "critical": 6,  // Priority 6 (highest)
        "default":  3,  // Priority 3
        "low":      1,  // Priority 1 (lowest)
    },
    RetryPolicy: &queue.RetryPolicy{
        MaxRetry: 3,
        Timeout:  10 * time.Minute,
    },
}

queueClient, _ := queue.NewClient(queueConfig)
defer queueClient.Close()
```

**2. Leader Mode - Enqueue Tasks:**

```go
// Load state machine
sm, _ := persistent.NewFromDefnId(ctx, "order-processing", repoManager)
sm.SetQueueClient(queueClient)

// Enqueue 1M tasks to distributed queue
for i := 0; i < 1000000; i++ {
    payload := &queue.ExecutionTaskPayload{
        StateMachineID: sm.GetID(),
        ExecutionName:  fmt.Sprintf("order-%d", i),
        Input: map[string]interface{}{
            "orderId": fmt.Sprintf("ORD-%d", i),
            "amount":  100.00 + float64(i),
        },
    }

    queueClient.EnqueueExecution(payload)
}
```

**3. Worker Mode - Process Tasks:**

```go
// Setup task executor with handlers
exec := executor.NewBaseExecutor()
exec.RegisterGoFunction("validate:order", validateOrderHandler)
exec.RegisterGoFunction("process:payment", processPaymentHandler)

// Create execution handler with context
execAdapter := executor.NewExecutionContextAdapter(exec)
handler := handler.NewExecutionHandlerWithContext(repoManager, queueClient, execAdapter, nil)

// Create and start worker
worker, _ := queue.NewWorker(queueConfig, handler)
worker.Run()  // Blocks and processes tasks
```

**4. Batch Execution with Distributed Mode:**

```go
// Execute batch via distributed queue
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:  "distributed-batch",
    Concurrency: 10,
    Mode:        "distributed",  // Tasks go to queue
}

results, _ := sm.ExecuteBatch(ctx, filter, "", batchOpts)
log.Printf("Enqueued %d tasks to distributed queue", len(results))
```

**5. Monitor Queue Statistics:**

```go
stats, _ := queueClient.GetStats()
for queueName, stat := range stats {
    fmt.Printf("Queue %s: Pending=%d, Active=%d, Failed=%d\n",
        queueName, stat.Pending, stat.Active, stat.Failed)
}
```

**Architecture:**
```
┌─────────────┐         ┌─────────────┐
│   Leader    │────────▶│    Redis    │
│  (Enqueue)  │         │    Queue    │
└─────────────┘         └──────┬──────┘
                               │
                    ┌──────────┼──────────┐
                    ▼          ▼          ▼
              ┌─────────┐ ┌─────────┐ ┌─────────┐
              │Worker 1 │ │Worker 2 │ │Worker N │
              │(Process)│ │(Process)│ │(Process)│
              └─────────┘ └─────────┘ └─────────┘
```

**Use Cases:**
- High-throughput order processing (millions of orders/day)
- Distributed ETL pipelines
- Batch job processing across multiple servers
- Microservices task orchestration
- Scalable workflow execution

**[📖 Full Example](examples/distributed_queue/main.go)**

### Micro-Batch Orchestration (NEW v1.2.11)

Process large batches with intelligent failure management, adaptive evaluation, and operator control!

**Key Features:**
- 🎯 **Streaming Batch Processing** - Process millions of records in manageable micro-batches
- 📊 **Adaptive Failure-Rate Control** - Statistical monitoring with EMA (Exponential Moving Average)
- ⏸️ **Pause/Resume Operations** - Operator intervention when failure rates exceed thresholds
- 🔄 **Redis Barrier Coordination** - Lua-based atomic operations for distributed worker sync
- 📈 **Real-time Metrics** - Per-batch and per-micro-batch granular tracking
- 🎛️ **Configurable Thresholds** - Soft (warning) and hard (halt) failure-rate limits

**1. Enable Micro-Batch Mode:**

```go
// Setup Redis for orchestration
rdb := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
})

// Configure batch execution with micro-batch support
filter := &repository.ExecutionFilter{
    StateMachineID: "source-workflow-v1",
    Status:         "SUCCEEDED",
}

batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:     "nightly-processing",
    DoMicroBatch:   true,           // Enable micro-batch mode
    MicroBatchSize: 500,            // Process 500 at a time
    RedisClient:    rdb,            // Redis for coordination
    StopOnError:    false,

    // Progress callbacks
    OnExecutionStart: func(sourceExecID string, index int) {
        log.Printf("Starting execution %d", index)
    },
    OnExecutionComplete: func(sourceExecID string, index int, err error) {
        if err != nil {
            log.Printf("Execution %d failed: %v", index, err)
        }
    },
}

// Execute with micro-batch orchestration
results, err := targetSM.ExecuteBatch(ctx, filter, "", batchOpts)
```

**2. Setup Orchestrator with Factory Functions:**

```go
// Create factory functions for state machine instantiation
smFactory := func(ctx context.Context, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
    return persistent.NewFromDefnId(ctx, smID, mgr)
}

smCreator := func(def []byte, isJSON bool, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
    return persistent.New(def, isJSON, smID, mgr)
}

// Create orchestrator with Redis and state machine
orchestrator, err := batch.NewOrchestrator(ctx, rdb, targetSM, smFactory, smCreator)

// Register orchestrator definition (idempotent)
err = orchestrator.EnsureDefinition(ctx,
    batch.OrchestratorDefinitionJSON(),
    batch.OrchestratorStateMachineID)

// Register in app-level registry for worker access
orchRegistry := registry.New()
orchRegistry.Register(batch.OrchestratorStateMachineID, orchestrator)
```

**3. Operator Signal for Resume:**

```go
// When failure rates trigger a pause, operator can signal resume
err := orchestrator.Signal(ctx, batchID,
    "ops-engineer@company.com",
    "API rate limit resolved - safe to continue")
```

**4. Monitor Progress:**

```go
// Check orchestrator execution status
executions, _ := manager.ListExecutions(ctx, &repository.ExecutionFilter{
    StateMachineID: batch.OrchestratorStateMachineID,
})

for _, exec := range executions {
    log.Printf("Batch %s: %s (state: %s)",
        exec.ExecutionID, exec.Status, exec.CurrentState)
}

// View metrics
processed, _ := rdb.Get(ctx, fmt.Sprintf("metrics:%s:total_processed", batchID)).Int64()
failed, _ := rdb.Get(ctx, fmt.Sprintf("metrics:%s:total_failed", batchID)).Int64()
rate := float64(failed) / float64(processed) * 100
log.Printf("Progress: %d processed, %d failed (%.2f%% failure rate)",
    processed, failed, rate)
```

**Architecture:**
```
┌──────────────────────────────────────────────────────────────┐
│                    Orchestrator State Machine                │
│  Initialize → DispatchMicroBatch → WaitForCompletion →      │
│  → EvaluateAndDecide → [PauseBatch] → Next/Success/Failure  │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
            ┌────────────────────────┐
            │    Redis Barrier       │
            │  - Lua atomic ops      │
            │  - Counter tracking    │
            │  - Metrics storage     │
            └──────────┬─────────────┘
                       │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   ┌─────────┐   ┌─────────┐   ┌─────────┐
   │Worker 1 │   │Worker 2 │   │Worker N │
   │Processes│   │Processes│   │Processes│
   │ + Fires │   │ + Fires │   │ + Fires │
   │ Barrier │   │ Barrier │   │ Barrier │
   └─────────┘   └─────────┘   └─────────┘
```

**Adaptive Evaluator Logic:**
- Tracks failure rate using Exponential Moving Average (EMA)
- **Soft Threshold (5%)**: Log warning, continue processing
- **Hard Threshold (10%)**: Pause batch, wait for operator signal
- Configurable thresholds based on use case
- Graceful degradation and automatic recovery

**Use Cases:**
- **ETL Pipelines**: Process millions of records with failure monitoring
- **Data Migration**: Migrate large datasets with pause/resume capability
- **Bulk Operations**: Process orders, invoices, or reports at scale
- **Nightly Batch Jobs**: Run scheduled data processing with oversight
- **API Rate Limiting**: Pause when hitting external API limits

**Components:**

| Component | Purpose | Lines of Code |
|-----------|---------|---------------|
| `Orchestrator` | Lifecycle management, pause/resume | 937 |
| `Barrier` | Redis Lua coordination | 115 |
| `Evaluator` | Adaptive failure-rate analysis | 76 |
| `Metrics` | Real-time tracking | 141 |
| `Registry` | Orchestrator lookup | 51 |

**[📖 Complete Example](examples/micro-batch-orchestration/main.go)** (861 lines - fully runnable)

**[📖 Bulk Orchestration Example](examples/micro-bulk-orchestration/main.go)** (721 lines)

## 📊 Execution Tracking

### Query Execution History

```go
// Get complete execution history
history, err := psm.GetExecutionHistory(ctx, "exec-001")
for _, h := range history {
	fmt.Printf("[%d] %s: %s (duration: %v, retries: %d)\n",
		h.SequenceNumber, h.StateName, h.Status, 
		h.Duration(), h.RetryCount)
}
```

### List and Filter Executions

```go
// Filter by status and time range
filter := &repository.Filter{
	StateMachineID: "workflow-v1",
	Status:         repository.StatusSucceeded,
	StartAfter:     time.Now().Add(-24 * time.Hour),
	Limit:          10,
	Offset:         0,
}

executions, _ := psm.ListExecutions(ctx, filter)
count, _ := psm.CountExecutions(ctx, filter)
```

### Statistics (GORM only)

```go
if gormRepo, ok := manager.GetRepository().(repository.ExtendedRepository); ok {
	stats, _ := gormRepo.GetStatistics(ctx, "workflow-v1")
	for status, s := range stats.ByStatus {
		fmt.Printf("%s: %d executions (avg: %.2fs, p95: %.2fs)\n",
			status, s.Count, s.AvgDurationSeconds, s.P95Duration)
	}
}
```

## 🏗️ Architecture

```
┌─────────────────────────────────────┐
│   Your Application                  │
└────────────┬────────────────────────┘
             │
             ▼
┌─────────────────────────────────────┐
│   Persistent State Machine          │
│   - Workflow orchestration          │
│   - Automatic persistence           │
└────────────┬────────────────────────┘
             │
    ┌────────┴────────┐
    ▼                 ▼
┌─────────┐    ┌──────────────┐
│Executor │    │ Repository   │
│ Layer   │    │ Manager      │
└─────────┘    └──────┬───────┘
                      │
         ┌────────────┼────────────┐
         ▼            ▼            ▼
    ┌────────┐  ┌─────────┐  ┌────────┐
    │  GORM  │  │Raw SQL  │  │Memory  │
    │Postgres│  │Postgres │  │(Test)  │
    └────────┘  └─────────┘  └────────┘
```

## 🆚 Comparison with AWS Step Functions

| Feature | AWS Step Functions | state-machine-amz-go |
|---------|-------------------|----------------------|
| **Parallel Execution** | ✅ | ✅ |
| **Task Delegation** | Lambda, ECS, etc. | Go Functions |
| **Pricing** | Per state transition | Free (self-hosted) |
| **Latency** | Network round-trip | In-process (μs) |
| **Local Testing** | Requires mock services | Native Go testing |
| **Custom Logic** | Lambda functions | Direct Go code |
| **Persistence** | Managed | PostgreSQL, GORM |
| **Vendor Lock-in** | AWS-specific | Cloud-agnostic |

## 🧪 Testing

The project includes comprehensive unit and integration tests.

**Run Unit Tests (fast, no dependencies):**
```bash
go test ./pkg/queue/...
go test ./pkg/statemachine/handler/...
```

**Run Integration Tests (requires Docker):**
```bash
# Start Postgres and Redis
cd docker-examples
docker-compose up -d postgres redis

# Run integration tests
go test -tags=integration -v ./pkg/queue/...
go test -tags=integration -v ./pkg/statemachine/handler/...

# Cleanup
docker-compose down -v
```

**Example Unit Test:**
```go
func TestOrderWorkflow(t *testing.T) {
	// Use in-memory repository
	config := &repository.Config{Strategy: "memory"}
	manager, _ := repository.NewPersistenceManager(config)
	manager.Initialize(context.Background())

	// Create test workflow
	psm, _ := persistent.New(yamlContent, false, "test-workflow", manager)

	// Register mock handlers
	exec := executor.NewBaseExecutor()
	exec.RegisterGoFunction("process:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		return map[string]interface{}{"success": true}, nil
	})

	// Execute and assert
	execCtx := &execution.Execution{
		ID:     "test-001",
		Name:   "TestExecution",
		Input:  testInput,
		Status: "RUNNING",
	}

	result, err := psm.Execute(context.Background(), execCtx)
	assert.NoError(t, err)
	assert.Equal(t, repository.StatusSucceeded, result.Status)
}
```

**Test Coverage:**
- ✅ 47 integration tests for queue and handler packages
- ✅ Comprehensive timeout event scenario coverage
- ✅ Real Postgres and Redis integration (no mocks)
- ✅ Message-arrives-first vs timeout-triggers-first scenarios
- ✅ High-volume and concurrent operation testing

**[📖 Full Testing Guide](TESTING.md)**

## 📈 Performance

```
Operation                    | GORM      | Raw SQL   | In-Memory
-----------------------------|-----------|-----------|----------
Save Execution              | 1.2ms     | 1.1ms     | 0.02ms
Get Execution               | 0.8ms     | 0.7ms     | 0.01ms
List Executions (10)        | 2.5ms     | 2.3ms     | 0.05ms
Get Execution with History  | 3.2ms     | 2.8ms     | 0.03ms

* Tested on MacBook Pro M1, PostgreSQL 14
```

## 🗺️ Roadmap

- [x] Core state machine implementation
- [x] AWS States Language compatibility
- [x] PostgreSQL persistence (raw SQL)
- [x] GORM integration
- [x] Persistent state machine
- [x] Execution history tracking
- [x] Statistics and analytics
- [x] Message Pause and Resume (MessageState)
- [x] GORM & PostgreSQL correlation support
- [x] Execution Chaining - Chain state machines together
- [x] Batch Chained Execution - Batch processing with filtering
- [x] Distributed Queue Execution - Redis-backed task queues
- [x] REST API Framework - Complete HTTP API via Gin
- [x] Async Task Cancellation - Automatic timeout cancellation
- [x] Micro-Batch Orchestration - Streaming batch processing with adaptive control
- [x] Bulk Orchestration - Large-scale batch operations
- [x] Orchestrator Registry - App-level orchestrator management
- [ ] Visual workflow builder
- [ ] DynamoDB persistence backend
- [ ] Web dashboard for monitoring
- [ ] Grafana/Prometheus metrics
- [ ] Workflow versioning and rollback

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Inspired by [Amazon States Language](https://states-language.net/)
- Built for Go developers who need workflow orchestration without cloud vendor lock-in

## 👨‍💻 Author

**Hussain Pithawala**
- LinkedIn: [hussainpithawala](https://www.linkedin.com/in/hussainpithawala)
- GitHub: [@hussainpithawala](https://github.com/hussainpithawala)

## 🤝 Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 📮 Support

- 📫 [GitHub Issues](https://github.com/hussainpithawala/state-machine-amz-go/issues)
- 💬 [GitHub Discussions](https://github.com/hussainpithawala/state-machine-amz-go/discussions)

---

⭐ **Star this repo** if you find it useful

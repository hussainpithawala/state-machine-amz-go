# state-machine-amz-go

<!-- Badges -->
[![GoDoc](https://pkg.go.dev/badge/github.com/hussainpithawala/state-machine-amz-go.svg)](https://pkg.go.dev/github.com/hussainpithawala/state-machine-amz-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/hussainpithawala/state-machine-amz-go)](https://goreportcard.com/report/github.com/hussainpithawala/state-machine-amz-go)
[![GitHub Release](https://img.shields.io/github/v/release/hussainpithawala/state-machine-amz-go)](https://github.com/hussainpithawala/state-machine-amz-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml/badge.svg)](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml)

A powerful, production-ready state machine implementation for Go that's fully compatible with Amazon States Language. Build complex workflows using YAML/JSON definitions and execute them locally with native Go functions or integrate with external services.

## 🆕 What's New in v1.2.1

**🐛 Bug Fix** - Message state timeout input preservation fixed!

**What's Fixed in v1.2.1**: Fixed a critical bug in Message state timeout handling where `executeTimeout` was incorrectly using `effectiveInput` instead of `originalInput` for ResultPath merging, causing loss of original execution context during timeout scenarios.

**The Fix:**
- ✅ **Input Preservation** - Original execution input now preserved during timeout scenarios
- ✅ **ResultPath Handling** - Timeout results correctly merged with original input
- ✅ **Test Coverage** - New comprehensive timeout execution tests added
- ✅ **Backward Compatible** - No breaking changes, safe to upgrade

**Impact:** Users with Message states that have both InputPath and ResultPath configured with timeout handling.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.2.1.md)**

---

## 🔄 Previous Release - v1.2.0

**🧪 Testing Enhancement** - Comprehensive failure scenario test coverage!

**What's New in v1.2.0**: Added comprehensive unit tests for execution failure scenarios, validating that executions are properly marked as `FAILED` with correct error information, end times, and history tracking when failures occur.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.2.0.md)**

---

## 🔄 Previous Release - v1.1.9

**🔥 CRITICAL Bug Fix** - Timeout trigger generation now uses state-specific keys!

**What's Fixed in v1.1.9**: Fixed a **critical bug** in `ProcessTimeoutTrigger` where timeout events were generated with hardcoded `__timeout_trigger__` key instead of state-specific keys like `__timeout_trigger___WaitForPayment`, breaking the timeout detection logic introduced in v1.1.8.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.9.md)**

---

## 🔄 Previous Release - v1.1.8

**🐛 Bug Fix** - State-specific message and timeout correlation keys!

**What's Fixed in v1.1.8**: Fixed message correlation and timeout trigger classification to use **state-specific keys** instead of global keys, preventing cross-state interference in workflows with multiple Message states.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.8.md)**

---

## 🔄 Previous Release - v1.1.7

**🔥 CRITICAL Bug Fix** - State transition input preservation fixed!

**What's Fixed in v1.1.7**: Fixed a **critical bug** where execution input was being completely replaced instead of merged during state transitions, causing loss of original execution context. This affected all multi-state workflows.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.7.md)**

---

## 🔄 Previous Release - v1.1.6

**🚀 Enhancement** - Improved message input merging with better nil handling and comprehensive test coverage!

Enhanced the `MergeInputs` method with critical improvements for nil handling, simplified JSONPath processing, and comprehensive test coverage including array inputs.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.6.md)**

---

## 🔄 Previous Release - v1.1.5

**🐛 Bug Fix** - Message input merging with JSONPath processing fixed!

**What's Fixed in v1.1.5**: Fixed a bug where message inputs were not properly merged with existing execution inputs during Message state resumption. Added `MergeInputs` interface method for proper separation of concerns.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.5.md)**

---

## 🔄 Previous Release - v1.1.4

**🐛 Bug Fix** - JSONPath array handling for `[]map[string]interface{}` fixed!

**What's Fixed in v1.1.4**: Enhanced JSONPath processing to properly handle `[]map[string]interface{}` array types during value extraction. Previously, only `[]interface{}` arrays were supported, causing Choice state conditions and JSONPath expressions to fail with common Go data structures.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.4.md)**

---

## 🔄 Previous Release - v1.1.3

**🔥 CRITICAL Bug Fix** - State execution chain input propagation fixed!

Resolved a **critical bug** where output from one state was not being passed as input to the next state in multi-state workflows. This caused subsequent states to receive incorrect input data, breaking the entire state machine execution flow.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.3.md)**

---

## 🔄 Previous Release - v1.1.2

**Critical Bug Fix** - ExecutionContext moved to types package for proper context management! 🔧

Resolved a critical issue where `ExecutionContext` and its registry needed to be accessible at the Background context level. The type has been relocated from `internal/states` to `pkg/types` to enable proper context propagation across the application.

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.2.md)**

---

## 🔄 Previous Release - v1.1.1

**Asynchronous Task Cancellation** - Automatic timeout cancellation when messages arrive! 🎯

Building on v1.1.0's BPMN-style boundary timer events with Redis-backed async task scheduling, when a Message state enters a waiting state, it schedules a timeout task in Redis that will trigger if no correlated message arrives within the specified timeout period.

**What's New in v1.1.1**: If the message arrives before the timeout expires, the message is correlated and the scheduled timeout task is automatically cancelled. If no message arrives, the timeout task executes as scheduled. This prevents unnecessary processing and keeps queues clean.

**Key Benefits:**
- 🧹 **Clean Queues** - No orphaned timeout tasks cluttering Redis
- ⚡ **Reduced Load** - Lower CPU and queue processing overhead
- 🎯 **Race Condition Handling** - Proper handling of message-vs-timeout races
- 📊 **Better Observability** - Accurate task counts in Asynqmon

**[📖 Read the full release notes →](RELEASE_NOTES_v1.1.1.md)**

---

## 🚀 Previous Release - v1.1.0

**Distributed Queue Execution** - Scale your state machine executions across multiple workers with Redis-backed task queues!

```go
// Configure distributed queue
queueConfig := &queue.Config{
    RedisAddr:   "localhost:6379",
    Concurrency: 10,
    Queues: map[string]int{
        "critical": 6,  // High priority
        "default":  3,
        "low":      1,
    },
}

queueClient, _ := queue.NewClient(queueConfig)
sm.SetQueueClient(queueClient)

// Execute batch in distributed mode - tasks processed across workers
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:  "distributed-batch",
    Concurrency: 10,
    Mode:        "distributed",  // NEW: distributed, concurrent, sequential
}

results, _ := sm.ExecuteBatch(ctx, filter, "", batchOpts)
```

**Key Features:**
- ⚡ **Leader-Worker Architecture** - Separate task generation from execution
- 🔄 **Priority Queues** - Route critical tasks to high-priority workers
- 📊 **Performance Optimizations** - 96% faster execution saves (265ms → <10ms)
- 🎯 **Queue Statistics** - Monitor pending, active, and failed tasks
- 🔗 **REST API Framework** - New [state-machine-amz-gin](https://github.com/hussainpithawala/state-machine-amz-gin) for HTTP API

Process millions of tasks efficiently! Perfect for high-throughput order processing, ETL pipelines, batch jobs, and distributed workflows.

**[📖 Read the full release notes →](RELEASE_NOTES.md)**

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
- 🌐 **Distributed Queue** - Redis-backed task queue for horizontal scaling across workers (NEW in v1.1.0)
- 🎯 **REST API** - Complete HTTP API via [state-machine-amz-gin](https://github.com/hussainpithawala/state-machine-amz-gin) (NEW in v1.1.0)
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

For distributed queue support (optional):
```bash
go get github.com/hibiken/asynq
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

**Message State Timeouts** (v1.1.0): BPMN-style boundary timer events with Redis-backed async task scheduling
**NEW in v1.1.1**: Scheduled timeout tasks are automatically cancelled when correlated messages arrive, preventing unnecessary processing and keeping queues clean.
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

**Message State Timeouts** (v1.1.0): Uses Redis-backed async task scheduling for distributed timeout processing
**Timeout Cancellation** (v1.1.1): Automatically cancels scheduled tasks when messages arrive

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
1. **Message State Entered** (v1.1.0) → Timeout task scheduled in Redis queue with unique ID, timer starts
2. **Message Arrives Before Timeout** (v1.1.1) → Message is correlated, scheduled timeout task automatically cancelled from Redis ✅
3. **Timeout Expires (No Message)** → Scheduled timeout task executes, workflow transitions to TimeoutPath ⏰
4. **Race Conditions** → Handled gracefully with correlation status tracking (idempotent)
5. **Clean Queues** → No orphaned tasks after message correlation 🧹

**Architecture Benefits:**
- Distributed timeout processing across multiple workers
- Redis persistence for reliability
- Automatic cleanup when messages are correlated (v1.1.1)

### Execution Chaining (NEW in v1.0.8)

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

### Batch Chained Execution (NEW in v1.0.9)

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

### Distributed Queue Execution (NEW in v1.1.0)

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
handler := persistent.NewExecutionHandlerWithContext(repoManager, execAdapter)

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
- [x] **Execution Chaining (v1.0.8)** - Chain state machines together
- [x] **Batch Chained Execution (v1.0.9)** - Batch processing with filtering
- [x] **Distributed Queue Execution (v1.1.0)** - Redis-backed task queues
- [x] **REST API Framework (v1.1.0)** - Complete HTTP API via Gin
- [x] **Async Task Cancellation (v1.1.1)** - Automatic timeout cancellation
- [x] **Critical Bug Fix (v1.1.3)** - State execution chain input propagation
- [x] **JSONPath Array Handling (v1.1.4)** - Support for []map[string]interface{}
- [x] **Message Input Merging (v1.1.5)** - Proper JSONPath processing for Message states
- [x] **Enhanced Merge Logic (v1.1.6)** - Improved nil handling and comprehensive test coverage
- [x] **State Transition Fix (v1.1.7)** - CRITICAL fix for input preservation during state transitions
- [x] **State-Specific Correlation (v1.1.8)** - Isolated message/timeout keys per Message state
- [x] **Timeout Trigger Generation Fix (v1.1.9)** - CRITICAL fix for timeout event generation with state-specific keys
- [x] **Comprehensive Failure Testing (v1.2.0)** - Enhanced test coverage for execution failure scenarios
- [x] **Message State Timeout Fix (v1.2.1)** - Bug fix for timeout input preservation with ResultPath
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

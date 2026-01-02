# state-machine-amz-go

<!-- Badges -->
[![GoDoc](https://pkg.go.dev/badge/github.com/hussainpithawala/state-machine-amz-go.svg)](https://pkg.go.dev/github.com/hussainpithawala/state-machine-amz-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/hussainpithawala/state-machine-amz-go)](https://goreportcard.com/report/github.com/hussainpithawala/state-machine-amz-go)
[![GitHub Release](https://img.shields.io/github/v/release/hussainpithawala/state-machine-amz-go)](https://github.com/hussainpithawala/state-machine-amz-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml/badge.svg)](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml)

A powerful, production-ready state machine implementation for Go that's fully compatible with Amazon States Language. Build complex workflows using YAML/JSON definitions and execute them locally with native Go functions or integrate with external services.

## 🆕 What's New in v1.0.9

**Batch Chained Execution** - Execute chained workflows in batch mode with powerful filtering and concurrency control!

```go
// Filter source executions
filter := &repository.ExecutionFilter{
    StateMachineID: sourceStateMachine.GetID(),
    Status:         "SUCCEEDED",
    StartAfter:     time.Now().Add(-24 * time.Hour),
    Limit:          100,
}

// Execute batch with concurrency
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "batch-processing",
    ConcurrentBatches: 5,  // Process 5 at a time
}

results, _ := targetStateMachine.ExecuteBatch(ctx, filter, "", batchOpts)
```

Launch hundreds or thousands of chained executions automatically! Perfect for data pipeline orchestration, order processing, report generation, and ETL operations.

**[📖 Read the full release notes →](RELEASE_v1.0.9.md)**

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
- 🎯 **Clean Architecture** - Separation between state machine logic and persistence
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
<summary><b>Message State</b> - Pause and wait for external message</summary>

```yaml
WaitForPayment:
  Type: Message
  CorrelationKey: "orderId"
  CorrelationValuePath: "$.orderId"
  TimeoutSeconds: 3600
  Next: "ProcessOrder"
  Catch:
    - ErrorEquals: ["States.Timeout"]
      Next: "HandleTimeout"
```
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

### Message Pause and Resume

Enable your workflows to wait for external events or human interventions.

**1. Define a Message State:**
```yaml
WaitForApproval:
  Type: Message
  CorrelationKey: "orderId"
  CorrelationValuePath: "$.orderId"
  Next: "ProcessOrder"
```

**2. Resume via Executor:**
```go
// When the external message arrives (e.g., via webhook or SQS)
response, err := exec.Message(ctx, &executor.MessageRequest{
    CorrelationKey:   "orderId",
    CorrelationValue: "ORD-123",
    Data:             map[string]interface{}{"approved": true},
})
```

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
- [ ] Visual workflow builder
- [ ] Redis persistence backend
- [ ] DynamoDB persistence backend
- [ ] Distributed execution support
- [ ] Web dashboard for monitoring
- [ ] Grafana/Prometheus metrics

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

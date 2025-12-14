# state-machine-amz-go
<!-- Badges -->
[![GoDoc](https://pkg.go.dev/badge/github.com/hussainpithawala/state-machine-amz-go.svg)](https://pkg.go.dev/github.com/hussainpithawala/state-machine-amz-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/hussainpithawala/state-machine-amz-go)](https://goreportcard.com/report/github.com/hussainpithawala/state-machine-amz-go)
[![GitHub Release](https://img.shields.io/github/v/release/hussainpithawala/state-machine-amz-go)](https://github.com/hussainpithawala/state-machine-amz-go/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml/badge.svg)](https://github.com/hussainpithawala/state-machine-amz-go/actions/workflows/release.yml)

A powerful, flexible state machine implementation for Golang that's compatible with Amazon States Language.
The same YAML/JSON language, which is used for defining state machines in AWS Step Functions.  
Define complex workflows using YAML/JSON and execute them with local Golang methods or external resources.

## Features

- 🚀 **AWS Step Functions Compatible** - Use the same state language as AWS Step Functions
    - 🔄 https://states-language.net/
- 🔧 **Local Method Execution** - Execute Golang methods directly from state tasks
- 📁 **YAML/JSON Support** - Define workflows in human-readable formats
- 🛡️ **Error Handling** - Built-in retry and catch mechanisms
- ⏱️ **Timeout & Heartbeat** - Control task execution timing
- 🔄 **All State Types** - Support for Pass, Task, Choice, Parallel, Wait, Succeed, Fail
- 🧪 **Test-Friendly** - Easy to mock and test workflows

## Installation

```bash
  go get github.com/hussainpithawala/state-machine-amz-go
```

## Quick start

### 1. Define a State Machine in YAML

```yaml
# order_workflow.yaml
Comment: "Order Processing Workflow"
StartAt: ValidateOrder
States:
  ValidateOrder:
    Type: Task
    Resource: "arn:aws:lambda:::validate:order"
    Next: ProcessPayment
    ResultPath: "$.validation"

  ProcessPayment:
    Type: Task
    Resource: "arn:aws:lambda:::process:payment"
    Next: SendNotification
    ResultPath: "$.payment"
    Retry:
      - ErrorEquals: [ "States.TaskFailed" ]
        MaxAttempts: 3

  SendNotification:
    Type: Task
    Resource: "arn:aws:lambda:::send:notification"
    End: true
    ResultPath: "$.notification"
```

### 2. Create and Execute the Workflow

```golang
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/internal/execution"
	"github.com/hussainpithawala/state-machine-amz-go/internal/statemachine"
	"gopkg.in/yaml.v3"
)

func main() {
	// 1. Load YAML definition
	yamlContent := []byte(`...`) // Your YAML here
	var definition statemachine.StateMachineDefinition
	yaml.Unmarshal(yamlContent, &definition)

	// 2. Create state machine
	sm, _ := statemachine.NewStateMachineFromDefinition(&definition)

	// 3. Create executor
	exec := executor.NewBaseExecutor()

	// 4. Register task handlers
	exec.RegisterGoFunction("validate:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("Validating order...")
		// Your validation logic here
		return map[string]interface{}{"valid": true}, nil
	})

	// 5. Create execution
	executionCtx := &execution.Execution{
		ID:   "exec-123",
		Name: "OrderProcessing",
		Input: map[string]interface{}{
			"orderId": "123",
			"amount":  100.00,
		},
		StartTime: time.Now(),
		Status:    "RUNNING",
	}

	// 6. Execute
	ctx := context.Background()
	result, err := exec.Execute(ctx, sm, executionCtx)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Workflow completed: %s\n", result.Status)
}
```

## State Types

### Task State

#### Execute a single unit of work.

```yaml
ProcessPayment:
  Type: Task
  Resource: "arn:aws:lambda:::process:payment"
  Next: "NextState"
  TimeoutSeconds: 30
  Retry:
    - ErrorEquals: [ "States.TaskFailed" ]
      MaxAttempts: 3
      BackoffRate: 2.0
  Catch:
    - ErrorEquals: [ "States.ALL" ]
      Next: "HandleError"
```

### Parallel State

#### Execute multiple branches concurrently.

```yaml
ParallelProcessing:
  Type: Parallel
  Next: "CombineResults"
  Branches:
    - StartAt: "ProcessPayment"
      States: ...
    - StartAt: "CheckInventory"
      States: ...
  ResultPath: "$.parallelResults"
```

## Choice State

#### Evaluate a condition and transition to a different state based on the result.

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

## Wait State

### Wait for a specified time.

```yaml
WaitForApproval:
  Type: Wait
  Seconds: 300  # Wait 5 minutes
  Next: "ProcessApproval"
```

## Pass State

## ## Pass state simply passes its input to the next state in the workflow.

```yaml
TransformData:
  Type: Pass
  Result:
    transformed: true
    timestamp: "2024-01-01T00:00:00Z"
  ResultPath: "$.metadata"
  Next: "NextState"
```

## Task Delegation

### Registering Task Handlers

```golang
exec := executor.NewBaseExecutor()

// Register a handler for a specific resource
exec.RegisterGoFunction("process:payment", func (ctx context.Context, input interface{}) (interface{}, error) {
payment := input.(map[string]interface{})
// Process payment logic
payment["status"] = "COMPLETED"
payment["transactionId"] = generateID()
return payment, nil
})

// Register with custom resource URI
exec.RegisterGoFunction("custom:operation", func (ctx context.Context, input interface{}) (interface{}, error) {
// Custom business logic
return map[string]interface{}{"result": "success"}, nil
})
```

### Using Parameters

```yaml
ProcessPayment:
  Type: Task
  Resource: "process:payment"
  Parameters:
    amount.$: "$.order.total"
    currency: "USD"
    customer.$: "$.customer"
    metadata:
      source: "web"
      timestamp: "2024-01-01"
```

## Error Handling

### Retry Policies

```yaml
FlakyService:
  Type: Task
  Resource: "arn:aws:lambda:::flaky:service"
  Retry:
    - ErrorEquals: [ "States.TaskFailed", "States.Timeout" ]
      IntervalSeconds: 1
      MaxAttempts: 5
      BackoffRate: 2.0
      MaxDelaySeconds: 60
      JitterStrategy: "FULL"
```

## Catch Blocks

```yaml
RiskyOperation:
  Type: Task
  Resource: "arn:aws:lambda:::risky:operation"
  Catch:
    - ErrorEquals: [ "ValidationError" ]
      ResultPath: "$.error"
      Next: "HandleValidationError"
    - ErrorEquals: [ "States.ALL" ]
      Next: "HandleGenericError"
```

## Advanced Examples

### Parallel Order Processing

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
    - StartAt: UpdateCRM
      States:
        UpdateCRM:
          Type: Task
          Resource: "arn:aws:lambda:::update:crm"
          End: true
  Next: "GenerateInvoice"
```

### Timeouts

```yaml
LongRunningTask:
  Type: Task
  Resource: "arn:aws:lambda:::long:task"
  TimeoutSeconds: 300      # 5 minutes
  HeartbeatSeconds: 60     # 1 minute heartbeat
```
### Result Paths
```yaml
ProcessData:
  Type: Task
  Resource: "arn:aws:lambda:::process:data"
  ResultPath: "$.processed"    # Store result in $.processed
  OutputPath: "$.processed"    # Output only processed data
  InputPath: "$.data"          # Use only $.data as input
```

### Choice-Based Routing

```golang
func testChoiceWorkflow() {
    // Different paths based on input conditions
    input := map[string]interface{}{
    "order": map[string]interface{}{
    "total": 1500.00,
    "priority": "HIGH",
    },
    "customer": map[string]interface{}{
    "tier": "PREMIUM",
    "region": "US",
    },
}

// Based on conditions, workflow takes different paths
}    
```

## API Reference

### Executor Interface

```golang
type Executor interface {
// Execute a state machine
Execute(ctx context.Context, sm StateMachineInterface, execCtx *execution.Execution) (*execution.Execution, error)

// Get execution status
GetStatus(executionID string) (*execution.Execution, error)

// Stop an execution
Stop(ctx context.Context, execCtx *execution.Execution) error

// List all executions
ListExecutions() []*execution.Execution

// Register Go function as task handler
RegisterGoFunction(name string, fn func (context.Context, interface{}) (interface{}, error))
}
```

### Execution Context

```golang
type Execution struct {
ID           string
Name         string
Input        interface{}
Output       interface{}
Status       string
StartTime    time.Time
EndTime      time.Time
Error        error
CurrentState string
StateHistory []StateHistory
}
```
## Comparison with AWS Step Functions


|       Feature      |   AWS Step Functions   | state-machine-amz-go |
|:------------------:|:----------------------:|----------------------|
| Parallel Execution | ✅                      | ✅                    |
| Task Delegation    | Lambda, ECS, etc.      | Go Functions         |
| Pricing            | Per state transition   | Free                 |
| Latency            | Network round-trip     | In-process           |
| Local Testing      | Requires mock services | Native Go testing    |
| Custom Logic       | Lambda functions       | Direct Go code       |


## Roadmap
-[ ] Distributed execution support
- [✅] Persistence layer for executions
  - [✅] Support for Postgres
  - [ ] Support for Redis, AWS DynamoDB, etc. 
- [ ] Web dashboard for monitoring

## License
The gopkg is available as open source under the terms of the MIT License.

## Acknowledgments
Inspired by Amazon States Language

Built for Golang developers who need workflow orchestration based on JSON/YAML based state definitions

Designed for both simple and complex business processes

## Author
- Hussain Pithawala (https://www.linkedin.com/in/hussainpithawala)

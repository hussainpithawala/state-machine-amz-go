# Chained Execution Example

This example demonstrates how to chain state machine executions, where one state machine (B) uses the output from another state machine execution (A) as its input.

## Overview

The chained execution feature allows you to:
1. Use the final output from a completed execution as input for a new execution
2. Use the output from a specific state in a previous execution
3. Apply custom transformations to the source output before using it as input

## Use Cases

- **Multi-stage processing pipelines**: Break complex workflows into manageable stages
- **Data transformation workflows**: Process → Validate → Enrich → Store
- **Event-driven architectures**: Trigger downstream workflows based on upstream results
- **Microservices orchestration**: Chain multiple service workflows together

## How It Works

### Basic Chaining (Using Final Output)

```go
// Execute State Machine A
execA, err := stateMachineA.Execute(ctx, inputA)

// Execute State Machine B using A's final output
execB, err := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
)
```

### Using Specific State Output

```go
// Use output from a specific state (e.g., "ProcessData")
execB, err := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID, "ProcessData"),
)
```

### With Input Transformation

```go
// Apply transformation to the source output
execB, err := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
    statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
        data := output.(map[string]interface{})

        // Transform the data
        return map[string]interface{}{
            "processedData": data["result"],
            "metadata": "additional info",
        }, nil
    }),
)
```

## Example Workflow

The example demonstrates a two-stage data processing pipeline:

### State Machine A: Data Processing Pipeline
1. **ProcessData** - Processes raw sensor data
2. **ValidateData** - Validates the processed data

### State Machine B: Data Enrichment Pipeline
1. **EnrichData** - Enriches data with additional information
2. **StoreData** - Stores the final enriched data

The example shows three ways to chain these:
1. Using the final output from State Machine A
2. Using output from the "ProcessData" state specifically
3. Using output with custom transformation

## Running the Example

### Prerequisites

1. PostgreSQL database running
2. Set environment variable:
   ```bash
   export DATABASE_URL="postgres://user:password@localhost:5432/dbname?sslmode=disable"
   ```

### Execute

```bash
cd examples/postgres_gorm
go run chained_execution_example.go
```

## API Reference

### Execution Options

#### `WithSourceExecution(executionID string, stateName ...string)`
Configures execution to use output from another execution.

**Parameters:**
- `executionID`: The ID of the source execution
- `stateName` (optional): The name of a specific state whose output to use. If omitted, uses final execution output.

**Example:**
```go
// Use final output
statemachine.WithSourceExecution("exec-abc123")

// Use specific state output
statemachine.WithSourceExecution("exec-abc123", "ProcessData")
```

#### `WithInputTransformer(transformer func(interface{}) (interface{}, error))`
Sets a transformation function for the chained input.

**Parameters:**
- `transformer`: A function that takes the source output and returns transformed input

**Example:**
```go
statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
    // Transform the output
    data := output.(map[string]interface{})
    return map[string]interface{}{
        "transformed": data["field"],
    }, nil
})
```

### Execution Methods

#### `execution.GetStateOutput(stateName string) (interface{}, error)`
Retrieves the output of a specific state from an execution.

**Parameters:**
- `stateName`: The name of the state

**Returns:**
- Output from the most recent execution of that state
- Error if state not found

#### `execution.GetFinalOutput() (interface{}, error)`
Retrieves the final output of a completed execution.

**Returns:**
- Final execution output
- Error if execution is not complete

### Repository Methods

#### `repository.GetExecutionOutput(ctx, executionID, stateName) (interface{}, error)`
Retrieves output from a persisted execution.

**Parameters:**
- `ctx`: Context
- `executionID`: The execution ID
- `stateName`: Optional state name (empty string for final output)

**Returns:**
- Output from execution or specific state
- Error if not found

## Advanced Patterns

### Conditional Chaining

```go
// Check execution status before chaining
execA, _ := stateMachineA.Execute(ctx, input)

if execA.Status == "SUCCEEDED" {
    // Only chain if A succeeded
    execB, _ := stateMachineB.Execute(ctx, nil,
        statemachine.WithSourceExecution(execA.ID),
    )
}
```

### Multi-level Chaining

```go
// A → B → C
execA, _ := stateMachineA.Execute(ctx, inputA)
execB, _ := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
)
execC, _ := stateMachineC.Execute(ctx, nil,
    statemachine.WithSourceExecution(execB.ID),
)
```

### Parallel Chaining

```go
// Execute A once, then execute multiple state machines in parallel using A's output
execA, _ := stateMachineA.Execute(ctx, input)

// Execute B and C in parallel (goroutines)
var wg sync.WaitGroup
wg.Add(2)

go func() {
    defer wg.Done()
    stateMachineB.Execute(ctx, nil,
        statemachine.WithSourceExecution(execA.ID),
    )
}()

go func() {
    defer wg.Done()
    stateMachineC.Execute(ctx, nil,
        statemachine.WithSourceExecution(execA.ID),
    )
}()

wg.Wait()
```

## Error Handling

```go
execB, err := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
)

if err != nil {
    // Handle chaining errors
    if strings.Contains(err.Error(), "execution") && strings.Contains(err.Error(), "not found") {
        // Source execution doesn't exist
    } else if strings.Contains(err.Error(), "state") && strings.Contains(err.Error(), "not found") {
        // Specified state doesn't exist in source execution
    }
}
```

## Benefits

1. **Modularity**: Break complex workflows into smaller, reusable state machines
2. **Flexibility**: Mix and match different state machines dynamically
3. **Maintainability**: Easier to understand and modify individual stages
4. **Reusability**: State machines can be reused in different chains
5. **Debugging**: Easier to debug individual stages independently

## Limitations

1. Source execution must be persisted in the repository (requires persistent state machine)
2. Only works with persistent state machines (not in-memory)
3. Circular dependencies should be avoided (A → B → A)

## See Also

- [Persistent State Machine Documentation](../../pkg/statemachine/persistent/)
- [Repository Documentation](../../pkg/repository/)
- [Execution Context Documentation](../../pkg/execution/)

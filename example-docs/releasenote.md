# Release v1.0.8 - Execution Chaining

**Release Date**: December 31, 2025
**Branch**: `development` → `master`

We're excited to announce **v1.0.8**, which introduces **Execution Chaining** - a powerful feature that enables you to compose complex workflows by chaining multiple state machine executions together. Build sophisticated multi-stage pipelines by connecting smaller, reusable state machines!

---

## 🎉 What's New

### 1. Execution Chaining

Chain state machine executions together, where one state machine can use the output from another execution as its input.

**Three Ways to Chain:**

#### a) **Use Final Output**
```go
// Execute State Machine A
execA, _ := stateMachineA.Execute(ctx, inputA)

// Chain State Machine B using A's final output
execB, _ := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
)
```

#### b) **Use Specific State Output**
```go
// Use output from a specific state
execB, _ := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID, "ProcessData"),
)
```

#### c) **Transform Before Chaining**
```go
// Apply custom transformation
execB, _ := stateMachineB.Execute(ctx, nil,
    statemachine.WithSourceExecution(execA.ID),
    statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
        // Transform the output
        data := output.(map[string]interface{})
        return map[string]interface{}{
            "processedData": data["result"],
            "timestamp": time.Now(),
        }, nil
    }),
)
```

### 2. Output Retrieval Methods

New methods to easily access execution outputs:

```go
// Get output from a specific state
output, err := execution.GetStateOutput("ProcessData")

// Get final execution output
finalOutput, err := execution.GetFinalOutput()
```

### 3. Repository Enhancements

Both PostgreSQL and GORM repositories now support retrieving execution outputs:

```go
// Retrieve from repository
output, err := repoManager.GetExecutionOutput(ctx, executionID, stateName)
```

**Features:**
- Optimized SQL queries for fast retrieval
- Support for both final and state-specific outputs
- Handles NULL values and JSON unmarshaling gracefully

---

## 📋 Use Cases

### Multi-Stage Processing Pipeline
```
Data Ingestion → Validation → Enrichment → Storage
```

Build complex pipelines by chaining specialized state machines:

```go
// Stage 1: Ingest data
execIngest, _ := ingestSM.Execute(ctx, rawData)

// Stage 2: Validate using ingestion output
execValidate, _ := validateSM.Execute(ctx, nil,
    statemachine.WithSourceExecution(execIngest.ID),
)

// Stage 3: Enrich using validation output
execEnrich, _ := enrichSM.Execute(ctx, nil,
    statemachine.WithSourceExecution(execValidate.ID),
)

// Stage 4: Store using enriched output
execStore, _ := storeSM.Execute(ctx, nil,
    statemachine.WithSourceExecution(execEnrich.ID),
)
```

### Event-Driven Workflows
```
Order Created → Payment Processing → Fulfillment → Notification
```

### Microservices Orchestration
```
Auth Service → User Service → Product Service → Analytics
```

---

## 🚀 Key Benefits

1. **Modularity** - Break complex workflows into smaller, manageable state machines
2. **Reusability** - State machines can be reused in different chains
3. **Flexibility** - Mix and match state machines dynamically at runtime
4. **Maintainability** - Easier to understand, test, and modify individual stages
5. **Debugging** - Debug each stage independently
6. **Composability** - Build sophisticated workflows from simple components

---

## 📖 Example

A complete working example is included demonstrating a two-stage pipeline:

**State Machine A: Data Processing**
- ProcessData → ValidateData

**State Machine B: Data Enrichment**
- EnrichData → StoreData

The example shows:
- ✅ Chaining using final output
- ✅ Chaining using specific state output
- ✅ Chaining with input transformation

**Location:** `examples/chained_postgres_gorm/chained_execution_example.go`

---

## 📚 Documentation

Comprehensive documentation included:

- **User Guide**: `examples/chained_postgres_gorm/CHAINED_EXECUTION_README.md`
  - API reference
  - Usage patterns
  - Advanced scenarios (conditional, multi-level, parallel chaining)
  - Error handling

- **Implementation Guide**: `EXECUTION_CHAINING_IMPLEMENTATION.md`
  - Technical details
  - Component descriptions
  - Testing approach

---

## 🔧 API Reference

### New Execution Options

#### `WithSourceExecution(executionID string, stateName ...string)`
Configure execution to chain from a source execution.

```go
// Use final output
statemachine.WithSourceExecution(execA.ID)

// Use specific state output
statemachine.WithSourceExecution(execA.ID, "ProcessData")
```

#### `WithInputTransformer(transformer func(interface{}) (interface{}, error))`
Apply transformation to source output before using as input.

```go
statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
    // Transform logic
    return transformedData, nil
})
```

### New Execution Methods

#### `execution.GetStateOutput(stateName string) (interface{}, error)`
Retrieve output from a specific state.

#### `execution.GetFinalOutput() (interface{}, error)`
Retrieve final execution output.

### New Repository Method

#### `repository.GetExecutionOutput(ctx, executionID, stateName) (interface{}, error)`
Retrieve output from a persisted execution.

---

## 🔄 Backwards Compatibility

✅ **100% Backwards Compatible**

- All existing code works without modification
- New functionality is opt-in
- No breaking changes to any APIs
- Existing tests continue to pass

---

## 🎯 Requirements

- Requires persistent state machines (uses repository)
- Source execution must be persisted in the database
- Both PostgreSQL and GORM repositories supported

---

## 🧪 Testing

All tests pass with comprehensive coverage:

```
✅ pkg/execution tests: PASS
✅ pkg/executor tests: PASS
✅ pkg/factory tests: PASS
✅ pkg/repository tests: PASS
✅ pkg/statemachine tests: PASS
✅ pkg/statemachine/persistent tests: PASS
✅ Example compiles and runs successfully
✅ All linting checks pass (gocritic)
```

---

## 📦 Installation

```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.0.8
```

---

## 🔮 Future Enhancements

We're considering these enhancements for future releases:

1. In-memory state machine chaining with execution cache
2. Automatic dependency resolution for pipelines
3. Visual workflow builder UI
4. Retry policies for chained executions
5. Conditional chaining based on execution status
6. Parallel fan-out patterns

---

## 🎬 Quick Start

```go
package main

import (
    "context"
    "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
    "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

func main() {
    ctx := context.Background()

    // Execute first state machine
    execA, _ := stateMachineA.Execute(ctx, inputData)

    // Chain second state machine using A's output
    execB, _ := stateMachineB.Execute(ctx, nil,
        statemachine.WithSourceExecution(execA.ID),
        statemachine.WithExecutionName("chained-execution-B"),
    )

    fmt.Printf("Chain completed: %s → %s\n", execA.Status, execB.Status)
}
```

---

## 📝 Advanced Patterns

### Conditional Chaining
```go
if execA.Status == "SUCCEEDED" {
    execB, _ := smB.Execute(ctx, nil,
        statemachine.WithSourceExecution(execA.ID),
    )
}
```

### Multi-level Chaining (A → B → C)
```go
execA, _ := smA.Execute(ctx, inputA)
execB, _ := smB.Execute(ctx, nil, statemachine.WithSourceExecution(execA.ID))
execC, _ := smC.Execute(ctx, nil, statemachine.WithSourceExecution(execB.ID))
```

### Parallel Chaining (A → [B, C])
```go
var wg sync.WaitGroup
wg.Add(2)

go func() {
    defer wg.Done()
    smB.Execute(ctx, nil, statemachine.WithSourceExecution(execA.ID))
}()

go func() {
    defer wg.Done()
    smC.Execute(ctx, nil, statemachine.WithSourceExecution(execA.ID))
}()

wg.Wait()
```

---

## 🛠️ Migration Guide

**No migration required!** This is a purely additive feature.

To start using execution chaining:

1. Ensure you're using persistent state machines
2. Use the new `WithSourceExecution` option
3. Optionally add `WithInputTransformer` for custom logic

---

## 🙏 Acknowledgments

Thanks to the community for the feature request and feedback during development!

---

## 📞 Support

- **Documentation**: See `examples/chained_postgres_gorm/CHAINED_EXECUTION_README.md`
- **Issues**: https://github.com/hussainpithawala/state-machine-amz-go/issues
- **Examples**: Check `examples/chained_postgres_gorm/` directory

---

## 🎊 What's Next?

Stay tuned for upcoming features:
- Enhanced error handling and retry mechanisms
- Workflow visualization tools
- Performance optimizations
- Additional chaining patterns

---

**Happy Chaining!** 🔗

Build powerful, composable workflows with state-machine-amz-go v1.0.8! 🚀

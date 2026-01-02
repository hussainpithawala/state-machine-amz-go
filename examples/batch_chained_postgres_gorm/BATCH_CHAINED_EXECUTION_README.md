# Batch Chained Execution Example

This example demonstrates how to launch chained executions in batches by filtering source executions using various criteria.

## Overview

Batch chained execution allows you to:
1. Filter source executions by state machine ID, status, time ranges, and limits
2. Launch chained executions for all matching source executions automatically
3. Control execution mode (sequential or concurrent)
4. Apply input transformations across all batch executions
5. Monitor progress with callbacks

## Key Features

### 1. Filtering Source Executions

Use `ExecutionFilter` to specify which source executions should trigger chained executions:

```go
filter := &repository.ExecutionFilter{
    StateMachineID: "source-state-machine-id",
    Status:         "SUCCEEDED",  // Only successful executions
    StartAfter:     time.Now().Add(-24 * time.Hour), // Last 24 hours
    StartBefore:    time.Now(),
    Limit:          100,  // Process up to 100 executions
    Offset:         0,    // Skip first N executions
}
```

### 2. Batch Execution Options

Configure batch execution behavior with `BatchExecutionOptions`:

```go
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "batch-processing",  // Prefix for execution names
    ConcurrentBatches: 5,                   // Number of concurrent executions (0 = sequential)
    StopOnError:       false,               // Continue on errors
    OnExecutionStart: func(sourceExecutionID string, index int) {
        fmt.Printf("Starting execution %d for source %s\n", index, sourceExecutionID)
    },
    OnExecutionComplete: func(sourceExecutionID string, index int, err error) {
        if err != nil {
            fmt.Printf("Execution %d failed: %v\n", index, err)
        } else {
            fmt.Printf("Execution %d completed\n", index)
        }
    },
}
```

### 3. Sequential vs Concurrent Execution

**Sequential Execution** (ConcurrentBatches = 1 or 0):
- Executions run one after another
- Predictable order
- Lower resource usage
- Easier to debug

```go
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "batch-sequential",
    ConcurrentBatches: 1,  // Sequential
}
```

**Concurrent Execution** (ConcurrentBatches > 1):
- Multiple executions run in parallel
- Faster overall processing
- Controlled concurrency with semaphore
- Higher throughput

```go
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "batch-concurrent",
    ConcurrentBatches: 5,  // Up to 5 concurrent executions
}
```

## Usage Examples

### Example 1: Basic Batch Execution

Execute chained workflows for all successful source executions:

```go
filter := &repository.ExecutionFilter{
    StateMachineID: sourceStateMachine.GetID(),
    Status:         "SUCCEEDED",
    Limit:          50,
}

batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "batch-process",
    ConcurrentBatches: 3,
}

results, err := targetStateMachine.ExecuteBatch(ctx, filter, "", batchOpts)
```

### Example 2: Batch with Time Range Filter

Process executions from the last hour:

```go
filter := &repository.ExecutionFilter{
    StateMachineID: sourceStateMachine.GetID(),
    Status:         "SUCCEEDED",
    StartAfter:     time.Now().Add(-1 * time.Hour),
    Limit:          100,
}

results, err := targetStateMachine.ExecuteBatch(ctx, filter, "", batchOpts)
```

### Example 3: Batch with Specific State Output

Use output from a specific state in the source execution:

```go
// Use output from "ProcessData" state instead of final output
results, err := targetStateMachine.ExecuteBatch(
    ctx,
    filter,
    "ProcessData",  // Source state name
    batchOpts,
)
```

### Example 4: Batch with Input Transformation

Apply transformation to all inputs:

```go
results, err := targetStateMachine.ExecuteBatch(
    ctx,
    filter,
    "",
    batchOpts,
    statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
        data := output.(map[string]interface{})

        // Transform the input
        return map[string]interface{}{
            "originalData": data,
            "priority":     "high",
            "processedAt":  time.Now().Format(time.RFC3339),
        }, nil
    }),
)
```

### Example 5: Batch with Progress Monitoring

Monitor execution progress with callbacks:

```go
completedCount := 0
failedCount := 0

batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "monitored-batch",
    ConcurrentBatches: 5,
    OnExecutionStart: func(sourceExecutionID string, index int) {
        fmt.Printf("⏳ [%d] Starting: %s\n", index, sourceExecutionID)
    },
    OnExecutionComplete: func(sourceExecutionID string, index int, err error) {
        if err != nil {
            failedCount++
            fmt.Printf("❌ [%d] Failed: %v\n", index, err)
        } else {
            completedCount++
            fmt.Printf("✅ [%d] Success\n", index)
        }
    },
}

results, err := targetStateMachine.ExecuteBatch(ctx, filter, "", batchOpts)

fmt.Printf("\nBatch Summary: %d succeeded, %d failed\n", completedCount, failedCount)
```

### Example 6: Stop on First Error

Stop processing when an execution fails:

```go
batchOpts := &statemachine.BatchExecutionOptions{
    NamePrefix:        "fail-fast-batch",
    ConcurrentBatches: 3,
    StopOnError:       true,  // Stop on first error
}

results, err := targetStateMachine.ExecuteBatch(ctx, filter, "", batchOpts)
if err != nil {
    fmt.Printf("Batch stopped due to error: %v\n", err)
    fmt.Printf("Completed %d executions before stopping\n", len(results))
}
```

## API Reference

### ExecuteBatch Method

```go
func (sm *StateMachine) ExecuteBatch(
    ctx context.Context,
    filter *repository.ExecutionFilter,
    sourceStateName string,
    opts *statemachine.BatchExecutionOptions,
    execOpts ...statemachine.ExecutionOption,
) ([]*BatchExecutionResult, error)
```

**Parameters:**
- `ctx`: Context for cancellation and timeouts
- `filter`: Filter to select source executions
- `sourceStateName`: Optional state name to get output from (empty = final output)
- `opts`: Batch execution configuration
- `execOpts`: Additional execution options applied to all executions

**Returns:**
- Array of `BatchExecutionResult` containing results for each execution
- Error if batch setup fails or if `StopOnError` is true and an execution fails

### ExecutionFilter

```go
type ExecutionFilter struct {
    Status         string    // Filter by status (SUCCEEDED, FAILED, etc.)
    StateMachineID string    // Filter by state machine ID
    Name           string    // Filter by name (supports partial match)
    StartAfter     time.Time // Filter executions started after this time
    StartBefore    time.Time // Filter executions started before this time
    Limit          int       // Maximum number of results
    Offset         int       // Offset for pagination
}
```

### BatchExecutionOptions

```go
type BatchExecutionOptions struct {
    NamePrefix        string // Prefix for generated execution names
    ConcurrentBatches int    // Number of concurrent executions (0 = sequential)
    StopOnError       bool   // Stop processing if an execution fails
    OnExecutionStart  func(sourceExecutionID string, index int)
    OnExecutionComplete func(sourceExecutionID string, index int, err error)
}
```

### BatchExecutionResult

```go
type BatchExecutionResult struct {
    SourceExecutionID string
    Execution         *execution.Execution
    Error             error
    Index             int
}
```

## Performance Considerations

### Concurrency

- **Sequential** (ConcurrentBatches = 1): Safe for shared resources, predictable order
- **Low Concurrency** (2-5): Good balance for most use cases
- **High Concurrency** (10+): Best for I/O-bound operations, requires adequate resources

### Memory Usage

- Each concurrent execution consumes memory for state and history
- Monitor memory usage when processing large batches
- Consider pagination (Limit/Offset) for very large datasets

### Database Load

- High concurrency increases database connections
- Use connection pooling appropriately
- Consider database performance when setting concurrency level

## Best Practices

1. **Start with Sequential Execution**
   - Test your workflow with sequential execution first
   - Add concurrency once you verify correctness

2. **Use Appropriate Filters**
   - Filter by status to avoid processing failed executions
   - Use time ranges to process recent data
   - Apply limits to prevent overwhelming the system

3. **Monitor Progress**
   - Use callbacks to track execution progress
   - Log errors and successes for debugging
   - Implement metrics collection if needed

4. **Handle Errors Gracefully**
   - Use `StopOnError: false` for resilient batch processing
   - Check `BatchExecutionResult` for individual failures
   - Implement retry logic if needed

5. **Optimize Concurrency**
   - Match concurrency to available resources
   - Consider downstream system capacity
   - Monitor system metrics during batch execution

## Running the Example

1. Start PostgreSQL database:
   ```bash
   docker run -d \
     --name postgres-statemachine \
     -e POSTGRES_PASSWORD=postgres \
     -e POSTGRES_DB=statemachine_db \
     -p 5432:5432 \
     postgres:15
   ```

2. Set database connection (optional):
   ```bash
   export DATABASE_URL="postgres://postgres:postgres@localhost:5432/statemachine_db?sslmode=disable"
   ```

3. Run the example:
   ```bash
   go run examples/batch_chained_postgres_gorm/batch_chained_execution_example.go
   ```

## Expected Output

The example demonstrates:
1. Creating multiple source executions (5 orders)
2. Sequential batch execution (all 5 orders processed one by one)
3. Concurrent batch execution (all 5 orders processed with concurrency=3)
4. Time range filtering (limited to 3 most recent)
5. Input transformation (2 executions with transformed input)

You should see progress indicators, execution results, and final summary statistics.

## Use Cases

### Data Pipeline Orchestration
Process multiple data batches through a transformation pipeline.

### Order Processing
Process multiple orders through validation, enrichment, and fulfillment.

### Report Generation
Generate reports for multiple accounts or time periods.

### ETL Operations
Extract, transform, and load data for multiple sources.

### Batch Notifications
Send notifications or alerts based on previous execution results.

## Troubleshooting

### No Executions Processed
- Verify filter criteria match existing executions
- Check that source executions have completed successfully
- Review filter status and time range settings

### Out of Memory
- Reduce `ConcurrentBatches`
- Add pagination with Limit/Offset
- Process in smaller time windows

### Database Connection Errors
- Check database connection pool settings
- Reduce concurrent executions
- Verify database is accessible and has capacity

### Slow Performance
- Increase `ConcurrentBatches` for I/O-bound operations
- Verify database indexes exist on filter columns
- Check for resource constraints (CPU, memory, network)

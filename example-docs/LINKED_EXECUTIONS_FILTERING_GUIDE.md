# Linked Executions Filtering Guide

## Overview

The linked executions framework has been enhanced with powerful filtering capabilities that allow you to precisely control which executions should be triggered based on their linked execution relationships. This guide explains the improvements and how to use them.

## Enhanced LinkedExecutionFilter

The `LinkedExecutionFilter` now includes additional fields to enable fine-grained filtering:

```go
type LinkedExecutionFilter struct {
    SourceStateMachineID   string    // Filter by source state machine ID
    SourceExecutionID      string    // Filter by source execution ID
    SourceStateName        string    // Filter by source state name
    SourceExecutionStatus  string    // NEW: Filter by source execution status (SUCCEEDED, FAILED, etc.)
    InputTransformerName   string    // NEW: Filter by input transformer name
    TargetStateMachineName string    // Filter by target state machine name
    TargetExecutionID      string    // Filter by target execution ID
    CreatedAfter           time.Time // Filter linked executions created after this time
    CreatedBefore          time.Time // Filter linked executions created before this time
    Limit                  int       // Maximum number of results
    Offset                 int       // Offset for pagination
}
```

## Key Improvements

### 1. Status-Based Filtering (`SourceExecutionStatus`)

Filter linked executions based on the status of the source execution:

```go
// Find all linked executions where the source execution succeeded
filter := &repository.LinkedExecutionFilter{
    SourceExecutionStatus: "SUCCEEDED",
}
linkedExecs, err := manager.ListLinkedExecutions(ctx, filter)
```

### 2. Input Transformer Filtering (`InputTransformerName`)

Filter by which input transformer was used:

```go
// Find all linked executions using a specific transformer
filter := &repository.LinkedExecutionFilter{
    InputTransformerName: "OrderToShipment",
}
linkedExecs, err := manager.ListLinkedExecutions(ctx, filter)
```

## Use Cases

### Use Case 1: Trigger Executions for a Source Execution

Find all child executions triggered by a specific source execution:

```go
filter := &repository.LinkedExecutionFilter{
    SourceExecutionID: "exec-order-123",
}
linkedExecs, err := manager.ListLinkedExecutions(ctx, filter)

// Trigger new executions for each linked execution
for _, le := range linkedExecs {
    // Process le.TargetExecutionID
}
```

### Use Case 2: Trigger Executions for Successful Source Executions

Find executions that were triggered by successful source executions:

```go
filter := &repository.LinkedExecutionFilter{
    SourceExecutionID:     "exec-order-123",
    SourceExecutionStatus: "SUCCEEDED",
}
linkedExecs, err := manager.ListLinkedExecutions(ctx, filter)
```

### Use Case 3: Trigger Executions by State Name and Status

Find linked executions from a specific state with a specific status:

```go
filter := &repository.LinkedExecutionFilter{
    SourceExecutionID:     "exec-order-123",
    SourceStateName:       "ProcessPayment",
    SourceExecutionStatus: "SUCCEEDED",
}
linkedExecs, err := manager.ListLinkedExecutions(ctx, filter)
```

### Use Case 4: Precise Filtering with Transformer, State, and Status

Find linked executions using all available filters:

```go
filter := &repository.LinkedExecutionFilter{
    SourceExecutionID:      "exec-order-123",
    SourceStateName:        "ProcessPayment",
    SourceExecutionStatus:  "SUCCEEDED",
    InputTransformerName:   "PaymentToShipment",
}
linkedExecs, err := manager.ListLinkedExecutions(ctx, filter)
```

## ListNonLinkedExecutions: Finding Eligible Executions

The `ListNonLinkedExecutions` function has been refactored to use `LinkedExecutionFilter`, enabling you to find executions that **don't** have specific types of linked executions.

### Basic Usage

Find all executions without any linked executions:

```go
filter := &repository.LinkedExecutionFilter{}
nonLinked, err := manager.ListNonLinkedExecutions(ctx, filter)
```

### Advanced Filtering

#### Find Executions Without Successful Linked Executions

Find executions that haven't triggered any successful child executions:

```go
filter := &repository.LinkedExecutionFilter{
    SourceExecutionStatus: "SUCCEEDED",
}
nonLinked, err := manager.ListNonLinkedExecutions(ctx, filter)
```

#### Find Executions Without Links from Specific State

Find executions that haven't triggered chains from a specific state:

```go
filter := &repository.LinkedExecutionFilter{
    SourceStateName: "ProcessPayment",
}
nonLinked, err := manager.ListNonLinkedExecutions(ctx, filter)
```

#### Find Executions Without Links Using Specific Transformer

```go
filter := &repository.LinkedExecutionFilter{
    InputTransformerName: "PaymentToShipment",
}
nonLinked, err := manager.ListNonLinkedExecutions(ctx, filter)
```

#### Combine Multiple Filters

Find SUCCEEDED executions from a specific state machine that haven't triggered chains from "ProcessPayment" state:

```go
filter := &repository.LinkedExecutionFilter{
    SourceStateMachineID:  "order-processing-sm",
    SourceStateName:       "ProcessPayment",
    SourceExecutionStatus: "SUCCEEDED",
}
nonLinked, err := manager.ListNonLinkedExecutions(ctx, filter)

// Trigger new chains for these executions
for _, exec := range nonLinked {
    targetExec, err := targetSM.Execute(ctx, nil,
        statemachine.WithSourceExecutionID(exec.ExecutionID),
        statemachine.WithSourceStateMachineID(exec.StateMachineID),
        statemachine.WithSourceStateName("ProcessPayment"),
    )
    // Handle error and result
}
```

## Practical Workflow Example

### Scenario: Re-trigger Failed Shipments from Successful Payments

You want to retry shipping for orders where:
- Payment succeeded
- Shipping was triggered from the "ProcessPayment" state
- But shipment execution failed

**Step 1: Find Failed Shipment Executions**

```go
// Find all linked executions where source succeeded but we want to retry
filter := &repository.LinkedExecutionFilter{
    SourceStateName:       "ProcessPayment",
    SourceExecutionStatus: "SUCCEEDED",
    TargetStateMachineName: "shipping-sm",
}
linkedExecs, err := manager.ListLinkedExecutions(ctx, filter)

// Get target execution IDs and check their status
failedShipments := []string{}
for _, le := range linkedExecs {
    targetExec, err := manager.GetExecution(ctx, le.TargetExecutionID)
    if err != nil {
        continue
    }
    if targetExec.Status == "FAILED" {
        failedShipments = append(failedShipments, le.SourceExecutionID)
    }
}
```

**Step 2: Re-trigger Shipment for Failed Cases**

```go
// Re-trigger shipment for each failed case
for _, sourceExecID := range failedShipments {
    targetExec, err := shippingSM.Execute(ctx, nil,
        statemachine.WithSourceExecutionID(sourceExecID),
        statemachine.WithSourceStateMachineID("payment-sm"),
        statemachine.WithSourceStateName("ProcessPayment"),
        statemachine.WithInputTransformer(paymentToShipmentTransformer),
    )
    if err != nil {
        log.Printf("Failed to retry shipment for %s: %v", sourceExecID, err)
        continue
    }
    log.Printf("Retried shipment: %s", targetExec.ID)
}
```

## Implementation Details

### Database Queries

#### ListLinkedExecutions with Status Filter

When using `SourceExecutionStatus`, the query performs a JOIN with the executions table:

```sql
SELECT le.*
FROM linked_executions le
INNER JOIN executions e ON le.source_execution_id = e.execution_id
WHERE e.status = 'SUCCEEDED'
  AND le.source_state_name = 'ProcessPayment'
ORDER BY le.created_at DESC
```

#### ListNonLinkedExecutions

Uses a subquery to filter linked executions, then finds executions NOT in that set:

```sql
SELECT DISTINCT e.*
FROM executions e
LEFT JOIN (
    SELECT le.source_execution_id
    FROM linked_executions le
    WHERE le.source_state_name = 'ProcessPayment'
      AND le.input_transformer_name = 'PaymentToShipment'
) AS filtered_links ON e.execution_id = filtered_links.source_execution_id
WHERE filtered_links.source_execution_id IS NULL
  AND e.status = 'SUCCEEDED'
ORDER BY e.start_time DESC
```

## Performance Considerations

1. **Indexed Fields**: All filter fields are indexed for optimal query performance
2. **JOIN Performance**: Status filtering requires a JOIN with executions table - use only when needed
3. **Pagination**: Always use `Limit` and `Offset` for large result sets
4. **Subquery Optimization**: The database optimizer will typically handle subqueries efficiently

## Best Practices

1. **Start with Broad Filters**: Begin with basic filters and add more specificity as needed
2. **Use Status Filters Judiciously**: Status filtering requires a JOIN - only use when necessary
3. **Combine Filters**: Multiple filters make queries more precise and efficient
4. **Monitor Performance**: For high-volume scenarios, monitor query performance and add indexes as needed
5. **Paginate Results**: Always use pagination for production queries

## API Reference

### ListLinkedExecutions

```go
func (manager *Manager) ListLinkedExecutions(
    ctx context.Context,
    filter *LinkedExecutionFilter,
) ([]*LinkedExecutionRecord, error)
```

Returns linked execution records matching the filter criteria.

### CountLinkedExecutions

```go
func (manager *Manager) CountLinkedExecutions(
    ctx context.Context,
    filter *LinkedExecutionFilter,
) (int64, error)
```

Returns the count of linked executions matching the filter criteria.

### ListNonLinkedExecutions

```go
func (manager *Manager) ListNonLinkedExecutions(
    ctx context.Context,
    filter *LinkedExecutionFilter,
) ([]*ExecutionRecord, error)
```

Returns executions that don't have linked executions matching the filter criteria.

## Testing

Comprehensive tests are available in:
- `pkg/repository/non_linked_executions_test.go`
- `pkg/repository/postgres_integration_test.go`
- `pkg/repository/gorm_postgres_integration_test.go`

Run integration tests:

```bash
cd docker-examples
docker-compose up -d postgres
go test -tags=integration -v ./pkg/repository/ -run TestListNonLinkedExecutions
```

## Migration Notes

### Breaking Changes

None - this is a backward-compatible enhancement.

### Recommended Updates

If you have existing code using `ListLinkedExecutions`, consider adding status and transformer filters for more precise control:

**Before:**
```go
filter := &repository.LinkedExecutionFilter{
    SourceExecutionID: execID,
}
```

**After:**
```go
filter := &repository.LinkedExecutionFilter{
    SourceExecutionID:     execID,
    SourceExecutionStatus: "SUCCEEDED",  // Only successful sources
    InputTransformerName:  "MyTransformer",  // Specific transformer
}
```

## Support

For issues or questions, please file an issue at:
https://github.com/hussainpithawala/state-machine-amz-go/issues

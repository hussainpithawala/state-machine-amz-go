# Crash Recovery Example

This example demonstrates the crash-resilient execution recovery mechanism in the state machine system.

## Overview

The example simulates a real-world scenario where:
1. **First Run**: An execution starts processing through multiple states but is abruptly terminated (simulating a crash)
2. **Second Run**: A background recovery scanner detects the orphaned execution and automatically resumes it from the last successfully completed state

## Workflow

The state machine workflow consists of four states:

```
Initialize → ProcessData → ValidateResults → GenerateReport
```

Each state performs a simulated task with a small delay to mimic real processing.

## Running the Example

### Prerequisites

1. **PostgreSQL Database**: Ensure PostgreSQL is running
2. **Database Setup**: Create a database named `statemachine_test` (or set custom URL)
3. **Go Dependencies**: Install module dependencies

### Setup Database

```bash
# Create database
createdb statemachine_test

# Or using psql
psql -U postgres -c "CREATE DATABASE statemachine_test;"
```

### Run Example

```bash
# Navigate to example directory
cd examples/crash_recovery

# Run with default database
go run main.go

# Or with custom database URL
DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=disable" go run main.go
```

## Example Output

```
=== Crash-Resilient Execution Recovery Example ===

This example demonstrates:
  1. Starting an execution that simulates a crash
  2. Recovery scanner detecting the orphaned execution
  3. Automatic resumption from the last successful state

Connected to database: postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable
Cleaning up previous test data...

=== Phase 1: Starting Initial Execution (will simulate crash) ===

  → [Initialize] Starting initialization...
Initial execution started: crash-recovery-exec-001
Status: RUNNING, Current State: ProcessData

>>> SIMULATING CRASH <<<
   (Execution terminated abruptly without proper cleanup)

Waiting 4s for execution to be considered orphaned...

=== Phase 2: Starting Recovery Scanner ===

Recovery scanner started (interval: 2s, threshold: 3s)

Monitoring execution recovery...
  Status: RUNNING     Current State: ProcessData         Duration: 3s
  → [ProcessData] Processing data items...
  → [ValidateResults] Validating results...
  → [GenerateReport] Generating final report...
  Status: SUCCEEDED   Current State: GenerateReport      Duration: 5s

>>> Execution completed with status: SUCCEEDED <<<

=== Execution History ===
  1. State: Initialize           Status: SUCCEEDED   Duration: 201ms
  2. State: ProcessData          Status: SUCCEEDED   Duration: 302ms
  3. State: ValidateResults      Status: SUCCEEDED   Duration: 201ms
  4. State: GenerateReport       Status: SUCCEEDED   Duration: 200ms

Total executions for state machine: 1

=== Recovery Metadata ===
  Last Successful State: Initialize
  Recovery Attempts: 1
  Crash Detected At: 2024-01-15T10:30:45Z

=== Example completed successfully ===
```

## How It Works

### 1. Initial Execution

```go
// Create execution context
execCtx := &execution.Execution{
    ID:             "crash-recovery-exec-001",
    StateMachineID: "crash-recovery-sm",
    Input:          map[string]interface{}{...},
    Status:         "RUNNING",
}

// Start execution
executionInstance, err := pm.Execute(ctx, execCtx)
```

As the execution progresses:
- Each state updates `RecoveryMetadata.LastSuccessfulState`
- State output is stored in `RecoveryMetadata.LastSuccessfulStateOutput`
- All data is persisted to the database after each state

### 2. Crash Simulation

```go
// Simulate crash by cancelling context abruptly
// In real scenario, process would just terminate
time.Sleep(800 * time.Millisecond)
fmt.Println(">>> SIMULATING CRASH <<<")
```

The execution remains in `RUNNING` status with:
- `CurrentState` = state that was executing during crash
- `RecoveryMetadata.LastSuccessfulState` = last completed state
- `RecoveryMetadata.LastSuccessfulStateOutput` = output from last successful state

### 3. Recovery Scanner

```go
// Configure recovery
recoveryConfig := &recovery.RecoveryConfig{
    Enabled: true,
    ScanInterval: 2 * time.Second,
    OrphanedThreshold: 3 * time.Second,
    DefaultRecoveryStrategy: recovery.StrategyRetry,
    DefaultMaxRecoveryAttempts: 3,
}

// Start scanner
pm.StartRecoveryScanner(recoveryConfig)
```

The scanner:
1. Queries for executions with `status = 'RUNNING'` AND `start_time < (NOW() - threshold)`
2. For each orphaned execution:
   - Checks if recovery attempts < max attempts
   - Loads last successful state output
   - Resumes execution from that point

### 4. Automatic Resumption

```go
// Recovery function wraps RunExecution
recoveryFunc := func(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error) {
    // Use last successful output as input
    execCtx.Input = execCtx.GetRecoveryInput()
    return pm.RunExecution(ctx, execCtx.Input, execCtx)
}
```

The execution:
- Resumes from `RecoveryMetadata.LastSuccessfulState`
- Uses `RecoveryMetadata.LastSuccessfulStateOutput` as input
- Continues through remaining states to completion

## Key Concepts

### Recovery Metadata

```go
type RecoveryMetadata struct {
    LastSuccessfulState       string      // Last completed state name
    LastSuccessfulStateOutput interface{} // Output from last successful state
    RecoveryAttemptCount      int         // Number of recovery attempts
    MaxRecoveryAttempts       int         // Maximum allowed attempts
    RecoveryStrategy          string      // RETRY, SKIP, FAIL, or PAUSE
    CrashDetectedAt           *time.Time  // When crash was detected
}
```

### Recovery Strategies

| Strategy | Behavior | Use Case |
|----------|----------|----------|
| `RETRY` | Re-execute from last successful state | Default, idempotent operations |
| `SKIP` | Skip problematic state | Non-critical states |
| `FAIL` | Mark as failed after max attempts | Critical data integrity |
| `PAUSE` | Pause for manual intervention | Requires human review |

### Configuration Options

```go
type RecoveryConfig struct {
    Enabled                  bool          // Enable/disable recovery
    ScanInterval             time.Duration // How often to scan
    OrphanedThreshold        time.Duration // When to consider orphaned
    DefaultRecoveryStrategy  RecoveryStrategy // Recovery strategy
    DefaultMaxRecoveryAttempts int         // Max recovery attempts
    StateMachineID           string        // Filter by state machine
}
```

## Database Schema

The example uses the `executions` table with recovery metadata:

```sql
CREATE TABLE executions (
    execution_id           TEXT PRIMARY KEY,
    state_machine_id       TEXT NOT NULL,
    name                   TEXT NOT NULL,
    input                  JSONB,
    output                 JSONB,
    status                 TEXT NOT NULL,
    start_time             TIMESTAMP NOT NULL,
    end_time               TIMESTAMP,
    current_state          TEXT NOT NULL,
    error                  TEXT,
    metadata               JSONB DEFAULT '{}',
    history_sequence_number INTEGER NOT NULL,
    recovery_metadata      JSONB DEFAULT '{}',  -- Recovery tracking
    created_at             TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at             TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Customization

### Adjust Timing

```go
// Faster recovery detection
orphanedThreshold = 1 * time.Second
scanInterval = 500 * time.Millisecond

// Slower, more conservative
orphanedThreshold = 10 * time.Minute
scanInterval = 1 * time.Minute
```

### Different Recovery Strategy

```go
// Pause for manual intervention
recoveryConfig.DefaultRecoveryStrategy = recovery.StrategyPause

// Fail fast
recoveryConfig.DefaultRecoveryStrategy = recovery.StrategyFail
recoveryConfig.DefaultMaxRecoveryAttempts = 1
```

### Custom Workflow

Modify the YAML workflow definition:

```yaml
Comment: "Custom workflow"
StartAt: FirstState
States:
  FirstState:
    Type: Task
    Resource: "task:first"
    Next: SecondState
  
  SecondState:
    Type: Task
    Resource: "task:second"
    End: true
```

## Troubleshooting

### Execution Not Recovering

1. **Check threshold**: Ensure `orphanedThreshold` is less than time since crash
2. **Verify scanner**: Confirm `StartRecoveryScanner` was called successfully
3. **Check logs**: Look for recovery scanner log messages

### Database Connection Errors

```bash
# Verify PostgreSQL is running
pg_isready -h localhost -p 5432

# Check database exists
psql -U postgres -l | grep statemachine_test
```

### Recovery Fails Repeatedly

1. **Check idempotency**: Ensure task handlers can be safely retried
2. **Review logs**: Look for error messages in recovery attempts
3. **Increase threshold**: Give execution more time before considering orphaned

## Related Documentation

- [Crash Recovery Guide](../../CRASH_RECOVERY_GUIDE.md)
- [Recovery Manager](../../pkg/recovery/recovery.go)
- [Persistent State Machine](../../pkg/statemachine/persistent/persistent.go)
- [PostgreSQL Messages Example](../postgres_messages/)

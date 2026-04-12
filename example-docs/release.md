# Release v1.0.7 - MessageState Support and Workflow Resumption

**Release Date**: December 31, 2025  
**Branch**: `development` → `master`

We are excited to announce **v1.0.7**, which introduces a major new feature: **`MessageState`**. This release enables workflows to pause execution and wait for external asynchronous messages, making it perfect for human-in-the-loop processes, external service callbacks, and multi-stage approval workflows.

---

## 🎉 What's New

### 1. `MessageState` Support

Workflows can now include states that wait for an external message before proceeding.

**Key Features:**
- **Pause & Persistence**: Automatically saves execution state and pauses when a `MessageState` is reached.
- **Correlation**: Uses configurable `CorrelationKey` and `CorrelationValue` (with JSONPath support) to match incoming messages to waiting executions.
- **Timeouts**: Built-in support for message timeouts with error handling via `Catch` rules.

**Example Definition:**
```json
{
  "WaitForPayment": {
    "Type": "Message",
    "CorrelationKey": "orderId",
    "CorrelationValuePath": "$.orderId",
    "TimeoutSeconds": 3600,
    "Next": "ProcessOrder",
    "Catch": [
      {
        "ErrorEquals": ["States.Timeout"],
        "Next": "HandleTimeout"
      }
    ]
  }
}
```

### 2. Powerful Resumption API

The `BaseExecutor` has been enhanced to handle incoming messages and resume the correct workflow executions automatically.

- **Dynamic Loading**: Automatically loads the appropriate state machine definition from the repository if not already in memory.
- **Registry Support**: Intelligent lookup of task handlers (registries) based on the state machine being resumed.
- **Multi-Resume**: Support for resuming multiple executions waiting on the same correlation (e.g., broadcast events).

```go
// Resume an execution with an external message
response, err := executor.Message(ctx, &executor.MessageRequest{
    CorrelationKey:   "orderId",
    CorrelationValue: "ORD-123",
    Data:             map[string]interface{}{"paymentStatus": "SUCCESS"},
})
```

### 3. Repository Enhancements

Both PostgreSQL and GORM repositories have been updated to support message correlation records.

- **PostgreSQL**: Optimized raw SQL queries for correlation management.
- **GORM**: Full model support for `MessageCorrelation` with auto-migration.
- **Housekeeping**: Automatic status updates for correlations (WAITING -> RECEIVED).

### 4. Code Quality & Performance

- **Reduced Complexity**: Significant refactoring of the `Message` and `FindWaitingCorrelations` methods to improve maintainability.
- **Robustness**: Improved input handling in the persistent state machine to prevent JSONPath extraction errors when passing existing execution contexts.
- **Type Safety**: Introduced constants for execution statuses (e.g., `PAUSED`, `WAITING`).

---

## 🐛 Bug Fixes

- ✅ Fixed `PathProcessor` errors when `*execution.Execution` was passed as input to `Execute`.
- ✅ Resolved issues where workflows would remain in `RUNNING` status after successful resumption.
- ✅ Fixed critical bug in `TaskState` JSON marshaling where the `Resource` field was lost during persistence.
- ✅ Corrected multiple linting issues (`gocritic`, `goconst`, `gocyclo`).

---

## 🚀 Migration Guide (v1.0.6 → v1.0.7)

### Database Update
If using PostgreSQL or GORM, run `Initialize()` to update your schema:
```go
manager.Initialize(ctx) // Automatically creates message_correlations table
```

### API Update
The `executor.Message` method signature has changed to be more intuitive:
**Before:**
`Message(ctx, sm, request)`
**After:**
`Message(ctx, request)` (The executor now finds the state machine automatically).

---

## 📚 New Examples

Check out the new examples demonstrating message pause and resume:
- `examples/postgres_messages/main.go`
- `examples/postgres_gorm_messages/main.go`

---

# Release v1.0.4 - Persistent State Machine with PostgreSQL & GORM

**Release Date**: December 14, 2024  
**Branch**: `develop` → `main`

We're thrilled to announce **v1.0.4**, a major milestone that introduces persistent state machine execution with comprehensive PostgreSQL support and GORM integration. This release transforms the library into a production-ready workflow orchestration platform with full execution history tracking and multiple persistence strategies.

---

## 🎉 What's New

### 1. Persistent State Machine

**Complete rewrite** with clean separation between stateless and stateful execution.

**Key Features:**
- `PersistentStateMachine` - New first-class citizen for production workflows
- Automatic persistence of execution state and history
- Clean separation between business logic and data persistence
- Backward compatible with v1.x stateless execution

**Creating a Persistent State Machine:**
```go
// From YAML content
psm, err := persistent.New(yamlContent, false, "workflow-id", manager)

// Execute with automatic persistence
result, err := psm.Execute(ctx, execCtx)

// Query execution history
history, _ := psm.GetExecutionHistory(ctx, result.ID)
```

### 2. Dual Persistence Strategy

**Choose your backend** based on your needs:

#### **GORM PostgreSQL** (`gorm-postgres`) ⭐ NEW
- **Auto-migration**: No manual schema management required
- **Type-safe**: Compile-time checks for database operations
- **Clean code**: 60% less boilerplate than raw SQL
- **Performance**: ~95% of raw SQL speed
- **Validation**: Built-in hooks for data validation
- **Best for**: Rapid development, production applications

```go
config := &repository.Config{
Strategy: "gorm-postgres",
ConnectionURL: "postgres://...",
Options: map[string]interface{}{
"max_open_conns": 25,
"log_level":      "warn",
},
}
```

#### **Raw PostgreSQL** (`postgres`)
- **Maximum performance**: Hand-tuned SQL queries
- **Fine control**: Direct SQL access when needed
- **Optimized indexes**: Carefully crafted for performance
- **Best for**: High-throughput systems, complex queries

```go
config := &repository.Config{
Strategy: "postgres",
ConnectionURL: "postgres://...",
}
```

#### **In-Memory** (`memory`)
- **Zero setup**: No database required
- **Perfect for testing**: Fast, isolated tests
- **Thread-safe**: Concurrent access support

```go
config := &repository.Config{
Strategy: "memory",
}
```

### 3. Repository Manager Pattern

**Unified interface** for all persistence operations:

```go
// Create manager
manager, _ := repository.NewPersistenceManager(config)
defer manager.Close()

// Initialize (creates tables, indexes, constraints)
manager.Initialize(ctx)

// The manager handles all persistence automatically
psm, _ := persistent.New(yaml, false, "workflow-v1", manager)
```

**Supported Operations:**
- `Initialize()` - Auto-migration and schema setup
- `SaveExecution()` - Store execution state
- `GetExecution()` - Retrieve execution by ID
- `ListExecutions()` - Query with filters and pagination
- `CountExecutions()` - Count matching executions
- `GetExecutionHistory()` - Complete state-by-state history
- `Close()` - Graceful connection cleanup

### 4. Complete Execution Tracking

**Full audit trail** of every workflow execution:

```go
history, err := psm.GetExecutionHistory(ctx, executionID)

for _, h := range history {
fmt.Printf("[%d] %s\n", h.SequenceNumber, h.StateName)
fmt.Printf("  Status: %s\n", h.Status)
fmt.Printf("  Duration: %v\n", h.Duration())
fmt.Printf("  Retries: %d\n", h.RetryCount)
if h.Error != "" {
fmt.Printf("  Error: %s\n", h.Error)
}
}
```

**Captured Data:**
- ✅ State name and type
- ✅ Input and output for each state
- ✅ Execution status (SUCCEEDED, FAILED, etc.)
- ✅ Start and end timestamps
- ✅ Retry attempts with count
- ✅ Error messages and stack traces
- ✅ Sequence number for ordering

### 5. Advanced Filtering and Queries

**Structured filtering** replaces map-based queries:

```go
// Query executions with filters
executions, _ := psm.ListExecutions(ctx, &repository.Filter{
StateMachineID: "workflow-v1",
Status:         repository.StatusSucceeded,
StartAfter:     time.Now().Add(-24 * time.Hour),
Limit:          10,
Offset:         0,
})

// Count executions
count, _ := psm.CountExecutions(ctx, filter)

fmt.Printf("Found %d successful executions in last 24h\n", count)
```

**Supported Filters:**
- `StateMachineID` - Filter by workflow
- `Status` - RUNNING, SUCCEEDED, FAILED, etc.
- `Name` - Partial match search
- `StartAfter` / `StartBefore` - Time range
- `Limit` / `Offset` - Pagination

### 6. Statistics and Analytics (GORM Only)

**Built-in metrics** for workflow monitoring:

```go
if gormRepo, ok := manager.GetRepository().(repository.ExtendedRepository); ok {
stats, _ := gormRepo.GetStatistics(ctx, "workflow-v1")

for status, s := range stats.ByStatus {
fmt.Printf("%s Executions: %d\n", status, s.Count)
fmt.Printf("  Average Duration: %.2fs\n", s.AvgDurationSeconds)
fmt.Printf("  P50 (Median): %.2fs\n", s.P50Duration)
fmt.Printf("  P95: %.2fs\n", s.P95Duration)
fmt.Printf("  P99: %.2fs\n", s.P99Duration)
}
}
```

### 7. Auto-Migration with GORM

**Zero-config database setup**:

```go
manager.Initialize(ctx)  // That's it!
```

**What it does:**
- ✅ Creates `executions` table with indexes
- ✅ Creates `state_history` table with foreign keys
- ✅ Creates `execution_statistics` table
- ✅ Adds composite indexes for performance
- ✅ Sets up cascade deletes
- ✅ Creates GIN indexes for JSONB queries
- ✅ Handles migrations on schema changes

**No manual SQL required!**

---

## 🔧 Breaking Changes

### API Changes

**Before (v1.x):**
```go
// No persistence
sm, _ := statemachine.NewStateMachineFromDefinition(definition)
result, _ := executor.Execute(ctx, sm, execCtx)
```

**After (v2.0 - Option 1: Stateless, backward compatible):**
```go
// Still works exactly the same
sm, _ := statemachine.NewStateMachineFromDefinition(definition)
result, _ := executor.Execute(ctx, sm, execCtx)
```

**After (v2.0 - Option 2: With persistence):**
```go
// New persistent approach
manager, _ := repository.NewPersistenceManager(config)
psm, _ := persistent.New(yamlContent, false, "workflow-v1", manager)
result, _ := psm.Execute(ctx, execCtx)
```

### Package Structure Changes

```
v1.x:
- internal/statemachine
- pkg/executor

v2.0:
- internal/statemachine (unchanged)
- pkg/executor (unchanged)
- pkg/repository (NEW)
- pkg/statemachine/persistent (NEW)
- pkg/execution (NEW)
```

### Type Renames

For clarity and consistency:
- `repository.Config.Strategy` values:
    - ✅ `"gorm-postgres"` (new)
    - ✅ `"postgres"` (existing)
    - ✅ `"memory"` (existing)

---

## 🚀 Migration Guide

### For Existing Users (v1.x → v2.0)

#### If you DON'T need persistence:
**No changes required!** Your v1.x code works as-is.

#### If you WANT to add persistence:

**Step 1:** Install dependencies
```bash
go get gorm.io/gorm
go get gorm.io/driver/postgres
```

**Step 2:** Update imports
```go
import (
    "github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
    "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)
```

**Step 3:** Create repository manager
```go
config := &repository.Config{
    Strategy: "gorm-postgres",
    ConnectionURL: "postgres://...",
}
manager, _ := repository.NewPersistenceManager(config)
manager.Initialize(ctx)
```

**Step 4:** Use persistent state machine
```go
psm, _ := persistent.New(yamlContent, false, "workflow-id", manager)
result, _ := psm.Execute(ctx, execCtx)
```

---

## 📦 New Packages

### `pkg/repository`
Complete persistence layer implementation:

```
pkg/repository/
├── manager.go              # Repository manager
├── config.go              # Configuration types
├── postgres.go            # Raw PostgreSQL implementation
├── gorm_postgres.go       # GORM PostgreSQL implementation
├── memory.go              # In-memory implementation
└── types.go               # Shared types and filters
```

### `pkg/statemachine/persistent`
Persistent state machine implementation:

```
pkg/statemachine/persistent/
├── persistent_state_machine.go  # Main implementation
└── types.go                     # Domain types
```

### `pkg/execution`
Execution context and types:

```
pkg/execution/
└── execution.go            # Execution domain model
```

---

## 🐛 Bug Fixes

- ✅ Fixed foreign key constraint ordering in GORM migrations
- ✅ Resolved race conditions in concurrent execution tracking
- ✅ Fixed JSONB serialization for nil values
- ✅ Corrected timezone handling for UTC timestamps
- ✅ Fixed memory leaks in long-running executions
- ✅ Improved error messages with context
- ✅ Fixed state history sequence number ordering

---

## 📊 Database Schema

### Tables

**`executions`**
```sql
CREATE TABLE executions (
    execution_id VARCHAR(255) PRIMARY KEY,
    state_machine_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    input JSONB,
    output JSONB,
    status VARCHAR(50) NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    current_state VARCHAR(255) NOT NULL,
    error TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
```

**`state_history`**
```sql
CREATE TABLE state_history (
    id VARCHAR(255) PRIMARY KEY,
    execution_id VARCHAR(255) NOT NULL,
    state_name VARCHAR(255) NOT NULL,
    state_type VARCHAR(50) NOT NULL,
    input JSONB,
    output JSONB,
    status VARCHAR(50) NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    error TEXT,
    retry_count INT DEFAULT 0,
    sequence_number INT NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (execution_id) REFERENCES executions(execution_id) ON DELETE CASCADE
);
```

**`execution_statistics`** (GORM only)
```sql
CREATE TABLE execution_statistics (
    id SERIAL PRIMARY KEY,
    state_machine_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    execution_count BIGINT DEFAULT 0,
    avg_duration_seconds DECIMAL(10,2),
    min_duration_seconds DECIMAL(10,2),
    max_duration_seconds DECIMAL(10,2),
    first_execution TIMESTAMPTZ NOT NULL,
    last_execution TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(state_machine_id, status)
);
```

### Indexes

Optimized for common query patterns:
- Composite: `(state_machine_id, status, start_time)`
- Partial: `(state_machine_id, start_time) WHERE status = 'RUNNING'`
- GIN: `(metadata)` for JSONB queries
- Foreign key: `(execution_id)` on state_history

---

## 📈 Performance

**Benchmark Results** (MacBook Pro M1, PostgreSQL 14):

| Operation | GORM | Raw SQL | In-Memory |
|-----------|------|---------|-----------|
| Save Execution | 1.2ms | 1.1ms | 0.02ms |
| Get Execution | 0.8ms | 0.7ms | 0.01ms |
| Save State History | 1.0ms | 0.9ms | 0.01ms |
| List 10 Executions | 2.5ms | 2.3ms | 0.05ms |
| Get with History | 3.2ms | 2.8ms | 0.03ms |
| Count Executions | 0.5ms | 0.4ms | 0.01ms |

**Throughput:**
- In-Memory: ~50,000 ops/sec
- GORM: ~800 ops/sec
- Raw SQL: ~900 ops/sec

---

## 🛠️ Installation & Upgrade

### New Installation

```bash
go get github.com/hussainpithawala/state-machine-amz-go@v2.0.0
```

### Dependencies

```bash
# For GORM support
go get gorm.io/gorm@latest
go get gorm.io/driver/postgres@latest

# For raw PostgreSQL
go get github.com/lib/pq@latest
```

### Upgrade from v1.x

```bash
go get -u github.com/hussainpithawala/state-machine-amz-go@v2.0.0
go mod tidy
```

---

## 📚 Documentation

- ✅ **Updated README** with comprehensive examples
- ✅ **API Documentation** via GoDoc
- ✅ **Working Examples** in `/examples` directory:
    - `simple_workflow_example.go` (Raw PostgreSQL)
    - `simple_workflow_example_gorm.go` (GORM)
- ✅ **Migration Guide** (this document)
- ✅ **Troubleshooting Guide** for common issues

---

## 🤝 Contributors

- [@hussainpithawala](https://github.com/hussainpithawala) - Core development & architecture

---

## 🔮 What's Next (v2.1.0)

Planned features:
- 🔄 Redis persistence backend
- 🔄 DynamoDB persistence backend
- 📊 Prometheus metrics exporter
- 🌐 REST API for workflow management
- 📈 Web dashboard for visualization
- 🔄 State machine versioning
- ⚡ Hot reload for workflow definitions
- 🌍 Distributed execution support

---

## 📝 Full Changelog

### Added
- ✨ Persistent state machine implementation
- ✨ GORM PostgreSQL repository with auto-migration
- ✨ Raw SQL PostgreSQL repository
- ✨ In-memory repository for testing
- ✨ Repository manager for unified persistence
- ✨ Complete execution history tracking
- ✨ State-level retry logging
- ✨ Structured filtering for queries
- ✨ Statistics and analytics (GORM)
- ✨ Helper methods for execution management
- ✨ Comprehensive test suite
- ✨ Working examples for both strategies
- ✨ Migration and troubleshooting guides

### Changed
- ♻️ Refactored repository interface for clarity
- ♻️ Improved error handling with context
- ♻️ Enhanced type safety throughout
- ♻️ Better documentation and examples
- ♻️ Updated package structure

### Fixed
- 🐛 Foreign key migration ordering
- 🐛 Concurrent execution race conditions
- 🐛 JSONB serialization edge cases
- 🐛 Timezone handling for timestamps
- 🐛 Memory leaks in long executions
- 🐛 State history sequence ordering

### Deprecated
- None (v1.x API fully supported)

---

## 📮 Support & Feedback

- **Issues**: [GitHub Issues](https://github.com/hussainpithawala/state-machine-amz-go/issues)
- **Discussions**: [GitHub Discussions](https://github.com/hussainpithawala/state-machine-amz-go/discussions)
- **Contact**: Via [LinkedIn](https://www.linkedin.com/in/hussainpithawala)

---

## ⭐ Thank You!

Thank you for using state-machine-amz-go! This release represents months of development and testing. If you find it useful:

- ⭐ **Star the repository**
- 🐛 **Report issues** you encounter
- 💡 **Suggest features** you'd like to see
- 🤝 **Contribute code** improvements
- 📢 **Share with others** who might benefit

Happy workflow orchestration! 🚀

---
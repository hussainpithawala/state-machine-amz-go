# Release Notes - v1.2.11

**Release Date**: March 8, 2026
**Branch**: `distro_batch`
**Base Branch**: `master`

---

## 🎯 Overview

Release v1.2.11 introduces a comprehensive **Micro-Batch Orchestration Framework** that enables streaming processing of large-scale batch operations with adaptive failure-rate control. This release represents a significant architectural enhancement to the state machine framework, adding Redis-backed coordination, intelligent batch processing, and robust error handling capabilities.

---

## 🚀 Major Features

### 1. Micro-Batch Orchestration Framework

The centerpiece of this release is a complete streaming framework for processing large batches of executions with intelligent failure management.

#### Key Components:

**Orchestrator (`pkg/batch/orchestrator.go` - 937 lines)**
- Complete lifecycle management for micro-batch executions
- Automatic orchestrator state machine creation and management
- Resume/pause capabilities with operator signaling
- Factory pattern support for state machine creation
- Comprehensive error handling and recovery

**Redis Barrier System (`pkg/batch/barrier.go` - 115 lines)**
- Lua-based atomic operations for batch coordination
- Distributed counter management across worker processes
- Automatic barrier triggering when micro-batches complete
- TTL-based cleanup to prevent stale state

**Adaptive Evaluator (`pkg/batch/evaluator.go` - 76 lines)**
- Statistical failure-rate monitoring using exponential moving average (EMA)
- Configurable soft (warning) and hard (halt) thresholds
- Rolling window analysis for adaptive decision-making
- Supports graceful degradation and automatic recovery

**Metrics System (`pkg/batch/metrics.go` - 141 lines)**
- Real-time batch processing metrics
- Per-batch and per-micro-batch granular tracking
- Redis-backed persistence with TTL management
- Tracks: processed count, failures, failure rate, current batch index

**Type System (`pkg/batch/types.go` - 220 lines)**
- Comprehensive type definitions for batch operations
- Support for orchestrator configuration and state management
- Integration types for executor context

#### Workflow Definitions:
- **Micro-Batch Orchestrator** (`pkg/batch/workflows/microbatch_orchestrator.json`)
  - State machine ID: `micro-batch-orchestrator-v1`
  - States: Initialize → DispatchMicroBatch → WaitForCompletion → EvaluateAndDecide → PauseBatch (conditional) → Success/Failure

- **Bulk Orchestrator** (`pkg/batch/workflows/bulk_orchestrator.json`)
  - State machine ID: `micro-bulk-orchestrator-v1`
  - Optimized for large-scale bulk operations

#### Integration Points:
- Seamless integration with existing state machine execution flow
- Worker-driven coordination via queue task handlers
- Operator intervention support through signal API
- Automatic continuation worker for orchestrator resumption

---

### 2. Bulk Orchestration Support

Added dedicated bulk orchestrator capabilities for large-scale batch operations that don't require micro-batch granularity.

**Features:**
- Simplified orchestration for full-batch operations
- Optimized for scenarios where pause/resume isn't required
- Consistent API with micro-batch orchestrator
- Shared infrastructure (metrics, barriers, evaluator)

---

### 3. Orchestrator Registry System

New app-level registry (`pkg/registry/registry.go` - 51 lines) for managing orchestrator instances.

**Capabilities:**
- Thread-safe orchestrator registration and lookup
- Enables queue workers to signal orchestrators without global variables
- Supports multiple concurrent orchestrator instances
- Clean separation of concerns between worker and orchestrator logic

---

## 🔧 Enhancements

### State Machine Improvements

#### Persistent State Machine Extensions
- **New File**: `pkg/statemachine/persistent/persistent_microbatch.go` (184 lines)
  - Dedicated micro-batch execution logic
  - Integration with orchestrator lifecycle
  - Automatic batch preparation and ID management

- **Enhanced**: `pkg/statemachine/persistent/persistent.go` (+255 lines)
  - Resource cleanup improvements
  - Simplified orchestration logic
  - Better error propagation
  - Refactored resumption and continuation handling

#### Core State Machine
- **Enhanced**: `pkg/statemachine/statemachine.go` (+34 lines)
  - Added `ExecuteBatch` method with micro-batch support
  - New `BatchExecutionOptions` for fine-grained control
  - Progress callback support (OnExecutionStart, OnExecutionComplete)
  - Flexible configuration (micro-batch size, error handling, Redis client)

### Execution Handler Enhancements

**Modified**: `pkg/handler/execution_handler.go` (+38 lines, -21 lines)
- Added orchestrator parameter to `NewExecutionHandlerWithContext`
- Enhanced message handling with enriched metadata
- Improved workflow context handling
- Automatic micro-batch signaling integration

**Breaking Change**: Execution handler constructor signature updated
```go
// Old
handler.NewExecutionHandlerWithContext(manager, queueClient, execAdapter)

// New
handler.NewExecutionHandlerWithContext(manager, queueClient, execAdapter, orchestrator)
```

### Executor Improvements

**Enhanced**: `pkg/executor/executor_message.go` (+99 lines, -3 lines)
- Improved message handling capabilities
- Better execution context management
- Enhanced error reporting
- Support for orchestrator payloads

### Queue Worker Updates

**Enhanced**: `pkg/queue/worker.go` (+15 lines, -3 lines)
- Better integration with orchestrator registry
- Improved task handler registration
- Enhanced error handling and logging

### JSONPath Processing

**Fixed**: `internal/states/jsonpath.go` (+22 lines, -10 lines)
- Resolved JSONPath processing bugs
- Improved path resolution logic
- Better error messages

**New Tests**: `internal/states/jsonpath_setvalue_test.go` (228 lines)
- Comprehensive test coverage for set value operations
- Edge case handling validation

**Enhanced Tests**: `internal/states/jsonpath_test.go` (+19 lines)
- Additional test cases for improved coverage

### Wait State Enhancements

**Enhanced**: `internal/states/wait.go` (+49 lines, -8 lines)
- Improved wait state handling
- Better timeout management
- Enhanced serialization support

**New Tests**: `internal/states/wait_marshal_test.go` (241 lines)
- Comprehensive marshalling/unmarshalling tests
- Validation of wait state persistence

### Validation System

**Enhanced**: `internal/validator/validator.go` (+175 lines)
- Improved state machine validation logic
- Better error reporting and detection
- Enhanced validation rules

**New Tests**: `internal/validator/validator_test.go` (231 lines)
- Unit tests for all validation scenarios
- Edge case coverage

---

## 📦 New Examples

### Micro-Batch Orchestration Example
**File**: `examples/micro-batch-orchestration/main.go` (861 lines)

A complete, runnable demonstration of the micro-batch streaming framework featuring:
- Source and target workflow definitions
- Redis barrier coordination
- Adaptive failure-rate monitoring
- Pause/resume simulation
- Operator signaling demonstration
- Queue worker pool setup
- Orchestrator continuation worker
- Comprehensive logging and metrics

**Environment Variables:**
```bash
REDIS_ADDR          # Default: localhost:6379
POSTGRES_URL        # Default: uses in-memory repository
SOURCE_EXEC_COUNT   # Default: 5000
MICRO_BATCH_SIZE    # Default: 500
WORKER_CONCURRENCY  # Default: 10
PAUSE_AT_BATCH      # Default: 2 (set -1 to disable)
```

### Bulk Orchestration Example
**File**: `examples/micro-bulk-orchestration/main.go` (721 lines)

Demonstrates bulk orchestration patterns for large-scale operations without micro-batch granularity.

---

## 🐛 Bug Fixes

### JSONPath Bug Fix
- **Branch**: `fix-jsonpath-bug`
- **Impact**: Resolved path resolution issues affecting nested object access
- **Files Modified**: `internal/states/jsonpath.go`

### Async Execution Continue Fix
- **Branch**: `fix_async_exec_continue`
- **Impact**: Fixed continuation bugs in asynchronous execution flows
- **Files Modified**: Multiple executor and handler files

### Timeout Map Fix
- **Branch**: `timeout_map_fix`
- **Impact**: Resolved timeout tracking issues in long-running executions
- **Files Modified**: State machine and executor files

### Merge Output Message Fix
- **Branch**: `merge_output_message_fix`
- **Impact**: Fixed message merging logic in output processing
- **Files Modified**: Executor message handling

---

## 🏗️ Infrastructure Changes

### CI/CD Improvements
**Modified**: `.github/workflows/ci.yml`
- Removed `test-all` dependency from Build job
- Faster build pipeline execution
- Maintained test coverage through `test-examples`

### Database Configuration
**Enhanced**: `Makefile`
- Added automatic test database creation
- Improved PostgreSQL setup for local development
- Better Docker container orchestration

**Updated**: `examples/micro-batch-orchestration/main.go`
- Default local PostgreSQL configuration
- Reduced worker concurrency for local development
- Better environment variable defaults

### TTL Constant Refactoring
**Modified**: Multiple files across packages
- Standardized TTL handling using `time.Duration`
- Removed redundant `TTL` suffix from constant names
- Updated all Redis calls to use new format
- Improved code readability and type safety

---

## 📊 Statistics

### Code Changes
- **Total Lines Changed**: 5,549 added, 77 removed (net +5,472)
- **Files Modified**: 34
- **New Files Created**: 14
- **Test Coverage Added**: 1,073 lines

### Package Breakdown
| Package | Lines Added | Lines Removed | Net Change |
|---------|-------------|---------------|------------|
| `pkg/batch/` | 2,406 | 0 | +2,406 |
| `examples/` | 1,582 | 0 | +1,582 |
| `pkg/statemachine/persistent/` | 439 | 71 | +368 |
| `internal/validator/` | 406 | 0 | +406 |
| `internal/states/` | 318 | 18 | +300 |
| `pkg/executor/` | 108 | 3 | +105 |
| `pkg/handler/` | 59 | 21 | +38 |
| Other packages | 231 | 24 | +207 |

### Test Coverage
- **New Test Files**: 4
- **Total Test Lines**: 1,073
- **Test Distribution**:
  - Batch tests: 373 lines
  - Wait state tests: 241 lines
  - Validator tests: 231 lines
  - JSONPath tests: 228 lines

---

## 🔄 Migration Guide

### For Existing Users

#### 1. Update Execution Handler Initialization
If you're using `NewExecutionHandlerWithContext`, update the call:

```go
// Before
handler := handler.NewExecutionHandlerWithContext(repoManager, queueClient, execAdapter)

// After - if not using orchestrator
handler := handler.NewExecutionHandlerWithContext(repoManager, queueClient, execAdapter, nil)

// After - if using orchestrator
handler := handler.NewExecutionHandlerWithContext(repoManager, queueClient, execAdapter, orchestrator)
```

#### 2. Add Dependencies
Update `go.mod` to include new dependencies:
```bash
go get github.com/redis/go-redis/v9
go get github.com/hibiken/asynq
```

#### 3. Setup Redis (if using micro-batch features)
Ensure Redis is available for orchestration:
```bash
docker run -d -p 6379:6379 redis:latest
```

---

## 🔐 Security Considerations

- Redis connection credentials should be managed through environment variables
- Orchestrator state includes execution metadata - ensure appropriate access controls
- TTL-based cleanup prevents indefinite data retention in Redis
- All Lua scripts are validated and sandboxed by Redis

---

## ⚡ Performance Notes

### Optimization Highlights
- Redis Lua scripts for atomic operations minimize round-trips
- Adaptive evaluator uses lightweight EMA calculations
- Metrics recording is batched to reduce Redis calls
- Worker concurrency is configurable per deployment

### Recommended Settings
- **Development**: 4-10 workers, 50-100 micro-batch size
- **Production**: 20-50 workers, 500-1000 micro-batch size
- **High-throughput**: 50-100 workers, 1000-2000 micro-batch size

---

## 🔮 Future Enhancements

While not part of this release, the architecture supports future additions:
- Multi-region orchestration coordination
- Dynamic batch size adjustment based on failure rates
- Web UI for orchestrator monitoring and control
- Prometheus metrics integration
- Webhook-based operator notifications

---

## 📚 Documentation

### Key Files to Review
- `pkg/batch/orchestrator.go` - Core orchestration logic
- `examples/micro-batch-orchestration/main.go` - Complete working example
- `pkg/batch/types.go` - Type definitions and API contracts
- `pkg/batch/workflows/` - Orchestrator state machine definitions

### Example Usage
See the micro-batch orchestration example for a complete, production-ready implementation pattern.

---

## 🙏 Contributors

This release includes work from the following branches:
- `distro_batch` (main branch)
- `fix-jsonpath-bug`
- `fix_async_exec_continue`
- `timeout_map_fix`
- `merge_output_message_fix`

---

## 📝 Commit History

```
057d06f Remove `test-all` dependency from Build job in CI workflow
d7b7364 Enrich orchestrator response payload with additional metadata
c5fd2b8 Enrich orchestrator response payload with additional metadata
d2b1a59 Update BulkOrchestratorStateMachineID to `micro-bulk-orchestrator-v1`
47e4558 Add bulk orchestrator and micro-batch streaming framework integration
f110dd8 Refactor TTL constants to remove redundant suffix
8bfddf7 Update PostgreSQL test configuration and enhance Makefile
04264f3 Improve resource cleanup and simplify orchestration logic
0939563 Refactor orchestrator resumption logic and consolidate continuation
44990a5 Introduce continuation worker for orchestrator resumption
056cdda Add orchestrator support to execution handler
56a6e33 Add support for nested merges and target machine ID
6748098 Refactor TTL constants to use `time.Duration`
2539680 Update `main.go` for default local PostgreSQL config
7ea1cd0 Add comprehensive test coverage for `JSONPathProcessor` and `WaitState`
137ec66 Add unit tests for state machine validator
2dd3a46 Add example for micro-batch orchestration
a720ff3 Add adaptive micro-batch processing with Redis-backed orchestration
```

---

## ⚠️ Breaking Changes

1. **Execution Handler Constructor**: The `NewExecutionHandlerWithContext` function now requires an orchestrator parameter (can be `nil` if not using orchestration features)

2. **Dependency Updates**: New required dependencies for Redis client and Asynq

---

## ✅ Upgrade Checklist

- [ ] Update `go.mod` dependencies
- [ ] Update execution handler initialization calls
- [ ] Setup Redis if using micro-batch features
- [ ] Review and update CI/CD configuration
- [ ] Test existing workflows for compatibility
- [ ] Review new examples for integration patterns
- [ ] Update deployment configurations with new environment variables

---

For questions or issues with this release, please refer to the repository issues or contact the maintainers.

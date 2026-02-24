# Changelog

All notable changes to this project will be documented in this file.
## [1.2.10] - 2026-02-24

### Changed
- **Batch Processing Refinements**: Fully removed sequential execution logic and refined error messaging.
  - Completely removed commented-out sequential execution code from batch processing.
  - Enhanced error messages for improved clarity and debugging.

## [1.2.9] - 2026-02-24

### Changed
- **Batch Processing Execution Mode**: Removed sequential execution path in batch processing logic.
  - Commented out conditional logic for sequential execution.
  - Defaulted batch processing to always use concurrent execution for improved performance.

## [1.2.8] - 2026-02-23

### Changed
- Refactored input transformer handling and enhanced execution payload
  - Removed redundant inline logic for input transformer name assignment.
  - Added `InputTransformerName` and `ApplyUnique` fields to `ExecutionTaskPayload`.
  - Updated execution task creation and processing to include the new fields.
  - Enhanced state machine execution configuration with input transformer and uniqueness support.

## [1.2.7] - 2026-02-23

### Added
- `ListNonLinkedExecutions` function for querying executions without specific linked conditions
- `WithUniqueness` option to enforce unique execution parameters for batch executions
- `SourceExecutionStatus` and `InputTransformerName` filters in `LinkedExecutionFilter`
- Rerun functionality for filtered batch executions
- Comprehensive integration tests for `ListNonLinkedExecutions` with various filtering scenarios
- Documentation for usage and performance considerations of enhanced filtering

### Changed
- Refactored `ListNonLinkedExecutions` to modularize query building and execution
- Expanded `ListNonLinkedExecutions` to support separate `ExecutionFilter` and `LinkedExecutionFilter` parameters
- Enhanced input transformations for batch executions with additional fields
- Updated execution logic to support improved summary outputs
- Improved SQL queries to handle JOIN operations for status-based filtering
- Improved separation of concerns, readability, and maintainability across the codebase

### Improved
- SQL query construction abstracted into dedicated builder and helper methods
- Enhanced filtering flexibility and query readability
- Simplified main functions by delegating responsibility to specialized components
- Updated helpers for result scanning and JSON unmarshalling


## [1.2.6] - 2026-02-22

### Added
- **Linked Execution Framework**: Comprehensive system for tracking and querying relationships between state machine executions.
  - `LinkedExecutionRecord` model and database schema for `linked_executions` table.
  - `SaveLinkedExecution()` repository method to persist linked execution records.
  - `ListLinkedExecutions()` repository method with filtering and pagination support.
  - `CountLinkedExecutions()` repository method for counting linked executions.
  - `LinkedExecutionFilter` type with multiple filter criteria (source/target execution IDs, state machine IDs, state names, timestamps).
  - Automatic linked execution tracking when using `WithSourceExecutionID()` in chained executions.
  - Database indexes on `source_state_machine_id`, `source_execution_id`, and `target_execution_id` for efficient querying.
- **Comprehensive Test Coverage**:
  - 330+ lines of linked execution tests in `pkg/statemachine/persistent/linked_execution_test.go`.
  - 265 lines of PostgreSQL integration tests for linked executions.
  - 265 lines of GORM integration tests for linked executions.
  - Test scenarios covering creation, transformer handling, multiple chains, idempotency, filtering, pagination, and counting.
- **Example Updates**: Enhanced `examples/chained_postgres_gorm/main.go` with linked execution queries and demonstrations.

### Changed
- **Code Quality Improvements**:
  - Refactored function signatures to use placeholder parameters (`_`) for unused arguments across state implementations and tests.
  - Added missing comments for public methods and constants to improve code documentation.
  - Replaced `log.Fatalf` with `log.Printf` in examples and core logic to avoid abrupt exits and enable graceful error handling.
  - Introduced deferred error handling for resource closures (repository managers, Redis clients, connections).
  - Standardized error handling and message key generation across state implementations.
  - Enhanced example applications with better input management and error handling.
- **Linting & CI/CD Configuration**:
  - Updated `.golangci.yml`: Fixed version format (string), moved `examples` exclusions from `skip-dirs` to `issues.exclude-dirs`, removed `revive` linter, disabled default linters.
  - Updated `.github/workflows/ci.yml`: Updated Go version, switched to manual installation of `golangci-lint` v2.4.0, replaced GolangCI action with direct script execution.
- **Repository Implementations**:
  - Extended PostgreSQL repository with linked execution methods (+233 lines in `postgres.go`).
  - Extended GORM repository with linked execution methods (+149 lines in `gorm_postgres.go`).
  - Updated repository Manager to expose new linked execution methods.

### Fixed
- Addressed linter warnings: removed redundant type declarations and unused variables.
- Improved logging consistency when errors occur during cleanup operations.
- Enhanced marshaling and validation logic for state configurations.
- Updated `.gitignore` to exclude `pkg/.DS_Store`.

## [1.2.5] - 2026-02-10

### Changed
- **Input-Output Consistency**: Updated state machine execution to ensure consistent input-output flow.
  - Set `execCtx.Output` to `execCtx.Input` when pausing execution to maintain input-output consistency.
  - Modified state resume logic to use the previous execution's output as the next input.
  - Switched state-machine repository and timeout handling to rely on `Output` instead of `Input`.
- **Execution Handler Functions**: Added new execution handler functions to `executor.BaseExecutor` for improved execution control.
- **Example Refinements**: Enhanced `examples/message_timeout_complete/main.go` with:
  - Refined state machine definitions for better clarity.
  - Added execution context to API setup methods.
  - Improved timeout handling demonstrations.
- **CI/CD Updates**: Upgraded Go version to 1.24 in GitHub workflows (`ci.yml` and `release.yml`).

### Fixed
- Resolved input-output mismatch issues during state transitions and execution pauses.
- Improved timeout handling consistency across state executions.

## [1.2.4] - 2026-02-10

### Added
- **TLS/SSL Support for Redis**: Full TLS support for secure Redis connections.
  - New `RedisClientOpt` field in queue `Config` struct for flexible Redis configuration.
  - Support for TLS certificates (CA, client cert, and key) in Redis connections.
  - `InsecureSkipVerify` option for development/testing environments.
  - Connection pooling and timeout configurations (DialTimeout, ReadTimeout, WriteTimeout, PoolSize).
- New secure Redis examples demonstrating TLS configuration:
  - `examples/test_secure_redis/` - Example with TLS-enabled Redis.
  - `examples/test_tls_conn/` - Example testing TLS connections.
- Docker Compose configuration for secure Redis instance (`redis-secured` service).
  - Runs on ports 7379 (non-TLS) and 7380 (TLS).
  - Configured with password authentication (`redispassword`).
  - TLS certificate volume mounting.
  - Health checks for TLS connections.

### Changed
- **Breaking Change**: Refactored queue `Config` struct to use `asynq.RedisClientOpt`.
  - Removed individual fields: `RedisAddr`, `RedisPassword`, `RedisDB`, `TlsConfig`.
  - Replaced with single `RedisClientOpt *asynq.RedisClientOpt` field.
  - Provides more flexibility and direct access to all Asynq Redis options.
- Updated `DefaultConfig()` to include secure defaults:
  - Default password: `redispassword`.
  - Connection timeouts: 10s dial, 30s read/write.
  - Pool size: 20 connections.
  - TLS with `InsecureSkipVerify` enabled by default.
- Updated all examples to use new `RedisClientOpt` configuration:
  - `examples/distributed_queue/main.go`
  - `examples/message_timeout_complete/main.go`
- Enhanced `.gitignore` to exclude TLS certificates and Redis data directories.
- Updated `Makefile`:
  - Added `REDIS_PASSWORD`, `REDIS_SECURED_ADDR`, `REDIS_SECURED_PASSWORD` variables.
  - Updated health checks for both standard and secure Redis instances.
  - Excluded new TLS examples from CI test runs.
- Updated CI workflow (`.github/workflows/ci.yml`):
  - Excluded `test_tls_conn` and `test_secure_redis` from automated example runs.
- Updated Go version to 1.24.0 in `go.mod`.
- Updated dependencies:
  - `github.com/redis/go-redis/v9` from v9.7.0 to v9.7.3.
  - `golang.org/x` packages updated to latest versions.

### Fixed
- Added proper name handling in `StateMachine.SaveDefinition()`:
  - Only sets `record.ID` if `stateMachineID` is non-empty.
  - Only sets `record.Name` if `statemachine.Name` is non-empty.
- Improved `StateMachine.ToRecord()` to use `Name` field when available, falling back to `ID`.
- Added `Name` field to `StateMachine` and `rawStateMachine` structs for proper JSON serialization.
- Fixed timeout duration in `ExampleTestTimeoutScenario()` from 12s to 10s for consistency.
- Fixed Makefile syntax error in `test-examples` target (environment variable export).
- Docker Compose formatting improvements (consistent array bracket style).
- All queue configuration tests updated to use new `RedisClientOpt` structure.
- Added new test `TestConfig_GetRedisClientOptWithTls` to verify TLS configuration.

### Security
- Added TLS/SSL encryption support for Redis connections.
- Secure password authentication for Redis.
- Certificate-based authentication support (CA, client certs).
- Configurable TLS verification for different security requirements.

## [1.0.9] - 2026-01-02

### Added
- **Batch Chained Execution**: Launch chained executions in batch mode with powerful filtering capabilities.
  - `ExecuteBatch()` method in persistent state machine for batch execution of chained workflows.
  - `ListExecutionIDs()` method in Repository interface for efficient execution ID retrieval.
  - `BatchExecutionOptions` struct for configuring batch execution behavior (concurrency, callbacks, error handling).
  - `BatchExecutionResult` struct for tracking individual batch execution results.
  - Support for sequential and concurrent batch execution modes.
  - Progress monitoring with `OnExecutionStart` and `OnExecutionComplete` callbacks.
  - Flexible filtering by state machine ID, status, time ranges, and pagination.
- Comprehensive documentation in `examples/batch_chained_postgres_gorm/BATCH_CHAINED_EXECUTION_README.md`.
- Working example demonstrating 5 batch execution patterns in `examples/batch_chained_postgres_gorm/batch_chained_execution_example.go`.
- Integration tests for `ListExecutionIDs` in both PostgreSQL and GORM implementations (7 test scenarios each).

### Changed
- Extended Repository interface with `ListExecutionIDs()` method.
- Implemented `ListExecutionIDs()` in PostgreSQL and GORM repositories with optimized DISTINCT queries.
- Enhanced Manager with `ListExecutionIDs()` delegation method.

### Improved
- **Testing Strategy**: Removed all fake/mock repository implementations (~750 lines).
  - Simplified `repository_test.go` by removing `fakeStrategy` (85% code reduction).
  - Simplified `persistent_test.go` by removing `fakeRepository` (74% code reduction).
  - Maintained comprehensive integration tests for actual database operations.
  - Improved test clarity and maintainability.
  - Better CI/CD pipeline integration with real database testing.

### Fixed
- Added missing `timePtr` helper function in integration tests.

## [1.0.8] - 2025-12-31

### Added
- **Execution Chaining**: New feature allowing state machine executions to be chained together.
  - `GetStateOutput(stateName)` method in `execution.Execution` to retrieve output from specific states.
  - `GetFinalOutput()` method in `execution.Execution` to retrieve final execution output.
  - `WithSourceExecution(executionID, stateName...)` execution option to chain executions.
  - `WithInputTransformer(transformer)` execution option to transform chained inputs.
  - `GetExecutionOutput(ctx, executionID, stateName)` method in Repository interface and implementations.
- Comprehensive documentation for execution chaining in `examples/chained_postgres_gorm/CHAINED_EXECUTION_README.md`.
- Working example demonstrating three chaining patterns in `examples/chained_postgres_gorm/chained_execution_example.go`.
- Technical implementation guide in `EXECUTION_CHAINING_IMPLEMENTATION.md`.

### Changed
- Extended `ExecutionConfig` struct with `SourceExecutionID`, `SourceStateName`, and `InputTransformer` fields.
- Enhanced `persistent.StateMachine.Execute()` to detect and handle chained executions.
- Updated PostgreSQL and GORM repositories with `GetExecutionOutput` implementation.

### Fixed
- Resolved gocritic linting issues with parameter type combining across repository and test files.

## [1.0.7] - 2025-12-31

### Added
- New `MessageState` type for asynchronous workflow pausing and resumption.
- `Message` method in `BaseExecutor` for resuming executions via external messages.
- Dynamic state machine and registry loading during execution resumption.
- PostgreSQL and GORM support for message correlation storage.
- New integration tests for multi-execution resumption and dynamic loading.
- Examples for message functionality in `examples/postgres_messages` and `examples/postgres_gorm_messages`.

### Changed
- Updated `BaseExecutor.Message` signature to remove the redundant `StateMachineInterface` parameter.
- Refactored `Message` and `FindWaitingCorrelations` to reduce cyclomatic complexity.
- Improved input handling in `persistent.StateMachine.Execute` to support existing execution contexts.

### Fixed
- Fixed critical bug in `TaskState` JSON serialization where the `Resource` field was lost.
- Fixed workflow status remaining `RUNNING` after successful resumption.
- Resolved various linting issues (`gocritic`, `goimports`, `goconst`).
- Improved SQL query logging by avoiding "record not found" errors for expected missing records.

## [1.0.4] - 2024-12-14

### Added
- Persistent state machine implementation.
- GORM PostgreSQL repository with auto-migration.
- Raw SQL PostgreSQL repository.
- In-memory repository for testing.
- Repository manager for unified persistence.
- Complete execution history tracking.
- Statistics and analytics (GORM).

### Fixed
- Foreign key migration ordering.
- Concurrent execution race conditions.
- JSONB serialization edge cases.
- Timezone handling for timestamps.

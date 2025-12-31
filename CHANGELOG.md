# Changelog

All notable changes to this project will be documented in this file.

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

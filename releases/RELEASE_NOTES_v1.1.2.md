# Release Notes - v1.1.2 (Urgent Fix)

## Bug Fixes
- **Moved ExecutionContext to types package** - Fixed a critical issue where `ExecutionContext` and its registry needed to be accessible at the Background context level. The `ExecutionContext` type has been relocated from `internal/states` to `pkg/types` to enable proper context management across the application.

## Changes
- Relocated `ExecutionContext` type definition to `pkg/types/types.go`
- Updated all references to `ExecutionContext` throughout the codebase:
  - `internal/states/task.go`
  - `pkg/executor/executor.go` and tests
  - `pkg/handler/execution_handler.go`
  - `pkg/statemachine/handler/handler.go`
- Updated all examples to reflect the new import path:
  - `examples/batch_chained_postgres_gorm`
  - `examples/chained_postgres_gorm`
  - `examples/postgres`
  - `examples/postgres_gorm`
  - `examples/postgres_gorm_messages`
  - `examples/postgres_messages`

## Impact
This is an urgent fix that ensures proper context propagation. Users of previous versions should update immediately to avoid context management issues.

## Files Changed
- 12 files changed, 65 insertions(+), 41 deletions(-)

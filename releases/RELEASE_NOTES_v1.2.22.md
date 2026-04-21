# Release Notes v1.2.22

**Release Date**: 2026-04-21

## Overview

Version 1.2.22 focuses on database performance optimizations and schema refinement. This release introduces new composite indexes specifically designed to enhance query performance for state machine execution and state history, along with a refactoring of schema constraints for better maintainability.

## What's New

### 🚀 Performance Improvements

#### Enhanced Query Performance with Composite Indexes
- Added four new composite indexes to optimize execution and state history queries:
  - `idx_state_machine_current_state_start_time`: Optimizes queries filtering by current state and start time.
  - `idx_state_machine_current_state_start_time_status`: Enhances queries involving state, start time, and status.
  - `idx_state_machine_status_start_time`: Optimizes status-based lookups ordered by time.
  - `idx_state_machine_status_start_time_current_state`: Comprehensive index for multi-column filtering.

### 🧹 Refactoring

#### Schema Constraints Refactoring
- Streamlined `CHECK` and `FOREIGN KEY` handling within the database schema.
- Improved schema consistency and reliability for state machine persistent storage.
- Simplified constraint management for easier database maintenance and migrations.

## Migration Guide

### Database Changes

**Required**: Run `Initialize()` on your GORM repository to apply the new indexes and schema changes:

```go
if err := manager.Initialize(ctx); err != nil {
    log.Fatal(err)
}
```

Or execute the following SQL manually:

```sql
-- New Composite Indexes
CREATE INDEX IF NOT EXISTS idx_state_machine_current_state_start_time
    ON executions(state_machine_id, current_state, start_time DESC);

CREATE INDEX IF NOT EXISTS idx_state_machine_current_state_start_time_status
    ON executions(state_machine_id, current_state, start_time DESC, status);

CREATE INDEX IF NOT EXISTS idx_state_machine_status_start_time
    ON executions(state_machine_id, status, start_time DESC);

CREATE INDEX IF NOT EXISTS idx_state_machine_status_start_time_current_state
    ON executions(state_machine_id, status, start_time DESC, current_state);
```

### Breaking Changes

None. This release is fully backward-compatible.

## Technical Details

### Changes Summary
- **Files Modified**: 
  - `pkg/repository/gorm_postgres.go`
  - `pkg/repository/postgres.go`
- **Key Changes**:
  - Implementation of new composite indexes in `Initialize()` methods.
  - Refactored constraint handling in PostgreSQL-based repositories.

### Key Commits
- `ff65b3b84e0867f83f54a3436ca27c872ac093e3`: feat(indexing): add new composite indexes for enhanced query performance and schema constraints refactoring

## Compatibility

- **PostgreSQL**: 12+
- **GORM**: 1.25+
- **Go**: 1.21+
- **Backward Compatibility**: ✅ Fully compatible with existing code

## Testing

All database-related tests pass with the new schema changes:
- ✅ `TestGormPostgresIntegrationSuite`
- ✅ `TestPostgresIntegrationSuite`
- ✅ Index creation and query plan verification

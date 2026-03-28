# Release Notes - v1.2.16

**Release Date:** March 28, 2026

---

## Overview

Version 1.2.16 delivers critical fixes to the `ListNonLinkedExecutions` repository method, ensuring accurate filtering of executions based on state history. This release improves query correctness and adds comprehensive test coverage for state-based filtering scenarios.

---

## ⚠️ Important Notice

### Behavior Change in `ListNonLinkedExecutions`

This release introduces a **behavior change** to the `ListNonLinkedExecutions` method:

**Before:** The method could return executions without any entries in the `state_history` table.

**After:** The method now returns **only** executions that have corresponding entries in the `state_history` table.

This change ensures that the method correctly identifies executions eligible for chained execution workflows. If your application relies on querying executions without state history, please use the `ListExecutions` method directly.

---

## 🚀 What's New

### State History Filtering

The `ListNonLinkedExecutions` method now supports filtering executions by specific state names in the state history:

```go
// Find executions that have reached "ApprovalState" but have no linked executions from that state
filter := &LinkedExecutionFilter{
    SourceStateName: "ApprovalState",
}
executions, err := repo.ListNonLinkedExecutions(ctx, &ExecutionFilter{}, filter)
```

This enables precise control over chained execution workflows by identifying executions at specific workflow stages.

---

## 🐛 Bug Fixes

### Repository Layer

| Component | Issue | Resolution |
|-----------|-------|------------|
| `gorm_postgres.go` | `ListNonLinkedExecutions` returned executions without state history | Added `INNER JOIN state_history` to filter correctly |
| `gorm_postgres.go` | State name filtering not applied | Added `state_history.state_name = ?` condition |
| `postgres.go` | Query returned duplicate rows | Changed to `SELECT DISTINCT ON (e.execution_id)` |
| `postgres.go` | Missing columns in result set | Extended to include all 15 execution columns |
| `postgres.go` | NULL timestamp handling errors | Fixed with `sql.NullTime` for `created_at`/`updated_at` |
| `gorm_postgres.go` | Schema migration failures on existing tables | Improved error handling with warnings |

---

## 🧪 Testing

### New Integration Tests

Two comprehensive test suites have been added to validate state history filtering:

1. **`TestListNonLinkedExecutions_WithStateHistoryFilter`**
   - Validates filtering by specific state names
   - Verifies empty results for non-existent states
   - Confirms executions without state history are excluded

2. **`TestListNonLinkedExecutions_WithStateHistoryAndLinkedExecution`**
   - Tests combined state history + linked execution filtering
   - Ensures correct exclusion of executions with linked executions

### Test Infrastructure

- Consolidated test files to leverage existing GORM integration test suite
- Improved test reliability with proper setup/teardown and data cleanup

---

## 🔧 Infrastructure Updates

### Linting Tool Upgrade

- **golangci-lint** upgraded from v1 to **v2.5.0**
- Updated `Makefile` with version check and automatic reinstallation
- Ensures consistent code quality across the codebase

---

## 📦 Installation

### Upgrade Steps

1. **Update your dependency:**
   ```bash
   go get github.com/hussainpithawala/state-machine-amz-go@v1.2.16
   ```

2. **No database migrations required** - All changes are backward-compatible at the schema level.

3. **Review behavior changes** - Ensure your usage of `ListNonLinkedExecutions` aligns with the new filtering behavior.

### System Requirements

- Go 1.21 or later
- PostgreSQL 13 or later
- golangci-lint v2.5.0 (for development)

---

## 📖 Documentation

### Usage Examples

#### Filter by State Name
```go
// Find all executions that reached "ProcessPayment" state
// but haven't triggered downstream linked executions
filter := &repository.LinkedExecutionFilter{
    SourceStateName: "ProcessPayment",
}

executions, err := repo.ListNonLinkedExecutions(
    ctx, 
    &repository.ExecutionFilter{}, 
    filter,
)
if err != nil {
    log.Fatalf("Failed to list executions: %v", err)
}

for _, exec := range executions {
    log.Printf("Execution %s ready for next step", exec.ExecutionID)
}
```

#### Combined Filtering
```go
// Find SUCCEEDED executions at "ValidateOrder" state
// without linked executions to the "ShipOrder" state machine
filter := &repository.LinkedExecutionFilter{
    SourceStateName: "ValidateOrder",
    SourceExecutionStatus: "SUCCEEDED",
}

executions, err := repo.ListNonLinkedExecutions(ctx, nil, filter)
```

---

## 🔍 Known Issues

None identified in this release.

---

## 📞 Support

For issues, questions, or contributions:
- **GitHub Issues:** [Report a bug](https://github.com/hussainpithawala/state-machine-amz-go/issues)
- **Documentation:** [View docs](https://github.com/hussainpithawala/state-machine-amz-go#readme)

---

## 🙏 Acknowledgments

Thank you to all contributors who helped identify and fix the `ListNonLinkedExecutions` filtering issues.

---

**Full Changelog:** [CHANGELOG_v1.2.16.md](./CHANGELOG_v1.2.16.md)

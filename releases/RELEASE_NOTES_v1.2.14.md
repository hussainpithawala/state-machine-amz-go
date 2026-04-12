# Release Notes v1.2.14

**Release Date**: March 21, 2026  
**Version**: v1.2.14  
**Type**: Minor Release (New Features)

---

## 🎯 Overview

Version 1.2.14 introduces **Crash-Resilient Execution Recovery**, a comprehensive mechanism that automatically detects and recovers state machine executions from unexpected crashes, infrastructure failures, or resource exhaustion. This feature ensures business continuity by resuming interrupted executions from their last known good state.

---

## ✨ New Features

### 1. Crash-Resilient Execution Recovery

A complete recovery system that handles execution interruptions gracefully:

#### Recovery Metadata Tracking
- Tracks last successful state and output after each state execution
- Monitors recovery attempts and enforces configurable limits
- Records crash detection timestamps for audit trails

```go
type RecoveryMetadata struct {
    LastSuccessfulState       string      // Last completed state
    LastSuccessfulStateOutput interface{} // Output for resumption
    RecoveryAttemptCount      int         // Number of recovery attempts
    MaxRecoveryAttempts       int         // Configurable limit
    RecoveryStrategy          string      // RETRY, SKIP, FAIL, or PAUSE
    CrashDetectedAt           *time.Time  // When crash was detected
}
```

#### Background Recovery Scanner
- Periodically scans for orphaned `RUNNING` executions
- Configurable scan interval and orphan threshold
- Automatic recovery with strategy-based handling

```go
// Configure recovery
config := &recovery.RecoveryConfig{
    Enabled: true,
    ScanInterval: 30 * time.Second,
    OrphanedThreshold: 5 * time.Minute,
    DefaultRecoveryStrategy: recovery.StrategyRetry,
    DefaultMaxRecoveryAttempts: 3,
}

// Start scanner
err := stateMachine.StartRecoveryScanner(config)
```

#### Recovery Strategies

| Strategy | Behavior | Use Case |
|----------|----------|----------|
| `RETRY` | Re-execute from last successful state | Default, idempotent operations |
| `SKIP` | Skip problematic state | Non-critical states |
| `FAIL` | Mark as failed after max attempts | Critical data integrity |
| `PAUSE` | Pause for manual intervention | Requires human review |

### 2. Batch/Micro-Batch Recovery

Special handling for batch orchestration scenarios:

- **Orchestrator Recovery**: Resumes paused orchestrators waiting at barriers
- **Worker Recovery**: Recovers crashed workers and properly signals completion
- **Barrier Integrity**: Redis barrier counters survive crashes
- **Automatic Signaling**: Recovered workers trigger orchestrator resumption

**Critical Fix**: Prevents orchestrators from waiting forever when workers crash before decrementing the barrier.

### 3. Repository Enhancements

#### New Methods
```go
// Find executions stuck in RUNNING status beyond threshold
FindOrphanedExecutions(ctx, stateMachineID, threshold) ([]*ExecutionRecord, error)
```

#### Schema Extensions
- `recovery_metadata` JSONB column for recovery tracking
- `created_at` and `updated_at` timestamp columns

---

## 📦 New Files

### Core Implementation
- `pkg/recovery/recovery.go` - Recovery manager and scanner (405 lines)

### Examples
- `examples/crash_recovery/main.go` - Single execution recovery demo (474 lines)
- `examples/crash_recovery/README.md` - Usage guide
- `examples/crash_recovery_batch/main.go` - Batch recovery demo (590 lines)
- `examples/crash_recovery_batch/README.md` - Architecture and troubleshooting

### Documentation
- `CRASH_RECOVERY_GUIDE.md` - Comprehensive guide (479 lines)

---

## 🔧 API Changes

### New Types

```go
// pkg/execution/execution.go
type RecoveryMetadata struct { ... }

// pkg/recovery/recovery.go
type RecoveryManager struct { ... }
type RecoveryConfig struct { ... }
type RecoveryStrategy string
```

### New Methods

```go
// pkg/execution/execution.go
func (e *Execution) UpdateRecoveryMetadata(stateName string, output interface{})
func (e *Execution) MarkRecoveryAttempt()
func (e *Execution) CanRecover() bool
func (e *Execution) GetRecoveryInput() interface{}
func (e *Execution) PrepareForRecovery(maxAttempts int, strategy string)
func (e *Execution) MarkCrashDetected()

// pkg/statemachine/persistent/persistent.go
func (pm *StateMachine) SetRecoveryManager(manager *recovery.RecoveryManager)
func (pm *StateMachine) StartRecoveryScanner(config *recovery.RecoveryConfig) error
func (pm *StateMachine) StopRecoveryScanner() error

// pkg/repository/repository.go
func (pm *Manager) FindOrphanedExecutions(ctx, stateMachineID, threshold) ([]*ExecutionRecord, error)
```

---

## 🚀 Usage Examples

### Basic Setup

```go
import (
    "github.com/hussainpithawala/state-machine-amz-go/pkg/recovery"
    "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

// Create persistent state machine
sm, err := persistent.New(definition, true, "my-sm", repositoryManager)

// Configure and start recovery
recoveryConfig := &recovery.RecoveryConfig{
    Enabled: true,
    ScanInterval: 30 * time.Second,
    OrphanedThreshold: 5 * time.Minute,
    DefaultRecoveryStrategy: recovery.StrategyRetry,
    DefaultMaxRecoveryAttempts: 3,
}

err = sm.StartRecoveryScanner(recoveryConfig)
defer sm.StopRecoveryScanner()
```

### Batch Recovery

```go
// Batch orchestration with automatic recovery
orchestrator, _ := batch.NewOrchestrator(ctx, redisClient, parentSM, ...)

// Recovery scanner handles both orchestrator and workers
err := orchestratorSM.StartRecoveryScanner(recoveryConfig)

// Workers automatically signal orchestrator after recovery
```

---

## 🗄️ Database Migration

**Required**: Execute the following SQL to add recovery metadata support:

```sql
-- Add recovery metadata column
ALTER TABLE executions 
ADD COLUMN recovery_metadata JSONB DEFAULT '{}';

-- Add timestamp columns
ALTER TABLE executions 
ADD COLUMN created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE executions 
ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

-- Add index for orphan detection
CREATE INDEX idx_executions_status_start_time 
ON executions(status, start_time) 
WHERE status = 'RUNNING';
```

---

## 📊 Statistics

| Metric | Value |
|--------|-------|
| **Lines Added** | ~1,900+ |
| **Lines Changed** | ~100 (existing files) |
| **New Files** | 6 |
| **Modified Files** | 15+ |
| **New Package** | `pkg/recovery/` |
| **New Examples** | 2 |
| **Documentation** | 3 comprehensive guides |

---

## 🐛 Bug Fixes

- **Linter Compliance**: Fixed all golangci-lint warnings (errcheck, staticcheck, prealloc, gocritic)
- **Batch Example**: Fixed orchestrator definition loading order
- **Gitignore**: Corrected malformed entries for compiled binaries

---

## ⚠️ Breaking Changes

### Database Schema Changes

This release requires database migration to add recovery metadata columns. See **Database Migration** section above.

### Backward Compatibility

- **Runtime**: Existing code continues to work without changes
- **Recovery**: Opt-in feature - must explicitly call `StartRecoveryScanner()`
- **Metadata**: Optional field - defaults to empty JSON object

---

## 🎯 Use Cases

### 1. Infrastructure Failures
- Application crashes during execution
- Database connection interruptions
- Network partitions

### 2. Resource Exhaustion
- Out of memory conditions
- Disk space exhaustion
- CPU throttling

### 3. Batch Processing
- Worker crashes mid-micro-batch
- Orchestrator failures at barrier
- Queue processing interruptions

### 4. Long-Running Workflows
- Multi-hour/day executions
- Human-in-the-loop pauses
- External dependency waits

---

## 📚 Documentation

### Guides
- [Crash Recovery Guide](CRASH_RECOVERY_GUIDE.md) - Architecture, usage, troubleshooting
- [Single Execution Example](examples/crash_recovery/README.md) - Basic recovery scenario
- [Batch Recovery Example](examples/crash_recovery_batch/README.md) - Complex orchestration recovery

### Code References
- [Recovery Manager](pkg/recovery/recovery.go)
- [Execution Model](pkg/execution/execution.go)
- [Persistent State Machine](pkg/statemachine/persistent/persistent.go)
- [Repository Interface](pkg/repository/repository.go)

---

## 🔍 Testing

### Run Examples

```bash
# Single execution recovery
cd examples/crash_recovery
go run main.go

# Batch/micro-batch recovery
cd examples/crash_recovery_batch
go run main.go
```

### Prerequisites
- PostgreSQL database
- Redis instance (for batch example)

---

## 🙏 Acknowledgments

This release addresses critical production requirements for:
- High-availability state machine executions
- Batch processing with guaranteed completion
- Infrastructure resilience patterns

---

## 📞 Support

For issues or questions:
1. Review [CRASH_RECOVERY_GUIDE.md](CRASH_RECOVERY_GUIDE.md)
2. Check example READMEs in `examples/crash_recovery/` and `examples/crash_recovery_batch/`
3. Open an issue on GitHub

---

**Full Changelog**: [v1.2.13...v1.2.14](https://github.com/hussainpithawala/state-machine-amz-go/compare/v1.2.13...v1.2.14)

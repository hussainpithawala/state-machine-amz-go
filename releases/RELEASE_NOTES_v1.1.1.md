# Release Notes - v1.1.1

## Asynchronous Task Cancellation Support

**Release Date:** TBD
**Type:** Feature Enhancement

---

## Overview

Version 1.1.1 adds support for asynchronous task cancellation in Message states with BPMN-style boundary timer events.

**Background (v1.1.0)**: Message states with `TimeoutSeconds` use Redis-backed async task scheduling to implement timeout behavior. When a Message state enters a waiting state, a timeout task is scheduled in Redis that will trigger if no correlated message arrives within the specified timeout period.

**Message Flow**:
- **Message arrives before timeout** → Message is correlated, workflow continues
- **No message arrives** → Timeout task executes, workflow transitions to TimeoutPath

**What's New (v1.1.1)**: When the message arrives before the timeout expires and is successfully correlated, the scheduled timeout task is now automatically cancelled from the Redis queue. This prevents unnecessary processing and resource consumption by removing obsolete timeout tasks.

## Key Features

### ✨ Automatic Timeout Cancellation
- Timeout tasks are automatically cancelled when correlated messages arrive
- Prevents duplicate processing and resource waste
- Maintains clean task queues with no orphaned timeout events

### 🎯 Enhanced Message State Handling
- Message states now properly handle race conditions between message arrival and timeout
- Correlation status tracking ensures idempotent timeout processing
- Graceful handling when timeouts fire after message completion

### 🔧 Queue Client Improvements
- New `CancelTimeout(correlationID)` method for programmatic cancellation
- Task ID-based cancellation using correlation IDs for precise targeting
- Inspector integration for task lifecycle management

## What's New

### API Additions

**Queue Client**
```go
// Cancel a scheduled timeout task
cancelled, err := queueClient.CancelTimeout(correlationID)
```

**Enhanced Timeout Scheduling**
- Timeout tasks now include unique task IDs based on correlation IDs
- 24-hour retention for completed timeout tasks (debugging support)
- Improved error handling for already-processed correlations

### Test Coverage

Added comprehensive integration tests with real Redis and Postgres:
- 47 new test cases covering timeout event scenarios
- Message-arrives-first vs timeout-triggers-first scenarios
- Concurrent operations and high-volume stress testing
- Race condition verification

## Technical Details

### Implementation Highlights

This builds on the existing BPMN-style boundary timer events for Message states:

1. **Async Task Scheduling** (v1.1.0): Message states schedule timeout tasks in Redis using asynq
2. **Task Identification**: Timeout tasks use `timeout-{correlationID}` as unique task IDs for targeted cancellation
3. **Status Tracking**: Correlation records track `WAITING`, `COMPLETED`, and `TIMEOUT` states
4. **Idempotency**: Timeout handlers check correlation status before processing
5. **Cancellation** (NEW): Uses asynq Inspector API to delete scheduled tasks from Redis when messages arrive

### Timeout Lifecycle

**Complete flow with async task scheduling:**

```
1. Message State Entered (v1.1.0)
   → Timeout task scheduled in Redis queue with unique ID
   → Correlation record created with status WAITING
   → Timer starts counting down

2a. Message Arrives Before Timeout (v1.1.1 enhancement)
   → Message is correlated with the waiting execution
   → Correlation marked COMPLETED
   → Scheduled timeout task automatically cancelled from Redis queue ✨
   → Workflow continues to Next state

2b. Timeout Expires (No Message Arrived)
   → Scheduled timeout task executes from Redis queue
   → Worker processes the timeout task
   → Checks correlation status (still WAITING)
   → Correlation marked TIMEOUT
   → Workflow transitions to TimeoutPath

2c. Race Condition Handling (Idempotency)
   → If timeout task executes after message arrived
   → Checks correlation status (already COMPLETED)
   → Skips timeout processing gracefully
```

**The async scheduling (introduced in v1.1.0) enables:**
- Distributed timeout processing across multiple workers
- Redis-backed task persistence and reliability
- Priority queue support for timeout tasks

**The cancellation (NEW in v1.1.1) prevents:**
- Orphaned timeout tasks after message arrival
- Unnecessary timeout processing
- Queue clutter and wasted resources

## Breaking Changes

None. This release is fully backward compatible.

## Migration Guide

No migration required. Existing Message states with timeouts will automatically benefit from the cancellation support.

## Configuration

No new configuration required. Timeout cancellation is automatic when using Message states with `TimeoutSeconds` specified.

## Testing

### Run Unit Tests
```bash
go test ./pkg/queue/...
go test ./pkg/statemachine/handler/...
```

### Run Integration Tests (requires Docker)
```bash
cd docker-examples
docker-compose up -d postgres redis
go test -tags=integration -v ./pkg/queue/...
go test -tags=integration -v ./pkg/statemachine/handler/...
```

See `TESTING.md` for detailed testing documentation.

## Performance Impact

- **Reduced Queue Load**: Cancelled timeouts free up queue capacity
- **Lower CPU Usage**: No processing of obsolete timeout events
- **Cleaner Metrics**: Asynqmon dashboard shows accurate task counts

## Known Limitations

- Cancellation is best-effort; timeouts in active processing cannot be cancelled
- Tasks cancelled concurrently may have race conditions (last one wins)
- Very short timeouts (<100ms) may fire before cancellation completes

## Dependencies

- `github.com/hibiken/asynq` v0.25.1+ (for Inspector API)
- Redis 6.0+ recommended for optimal performance

## Documentation

- `TESTING.md` - Comprehensive testing guide
- `TEST_COVERAGE_SUMMARY.md` - Detailed test scenario documentation
- Code examples in `examples/` directory

## Contributors

- Core implementation and testing

## What's Next (v1.2.0)

Planned features for the next release:
- Worker timeout processing improvements
- Enhanced retry policies for timeout tasks
- Metrics and observability enhancements
- Additional timeout path configuration options

---

## Full Changelog

### Added
- Asynchronous timeout cancellation support for Message states
- `CancelTimeout(correlationID)` method in queue client
- Unique task IDs for timeout tasks based on correlation IDs
- 24-hour retention for completed timeout tasks
- Comprehensive integration test suite (47 tests)
- `TESTING.md` and `TEST_COVERAGE_SUMMARY.md` documentation

### Enhanced
- Message state timeout handling with race condition protection
- Correlation status tracking (`WAITING` → `COMPLETED`/`TIMEOUT`)
- Error handling for already-processed correlations
- Queue client task inspection capabilities

### Fixed
- Orphaned timeout tasks when messages arrive first
- Duplicate timeout processing edge cases
- Resource cleanup in high-volume scenarios

---

**Upgrade Command:**
```bash
go get github.com/hussainpithawala/state-machine-amz-go@v1.1.1
```

**Questions or Issues?**
Please report issues at: https://github.com/hussainpithawala/state-machine-amz-go/issues

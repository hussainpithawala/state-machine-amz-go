# Release Notes - v1.2.5

## 🔄 Input-Output Consistency & Enhanced Timeout Handling

We're pleased to announce v1.2.5, a focused release that improves input-output consistency during state machine execution and enhances timeout handling behavior.

---

## 🎉 What's New

### Input-Output Consistency Improvements

This release addresses critical input-output flow consistency during state machine execution:

**Key Changes:**

- **Execution Pause Consistency**: When execution is paused (e.g., at a `MessageState`), `execCtx.Output` is now set to `execCtx.Input` to maintain data consistency
- **State Resume Logic**: Modified state resume behavior to use the previous execution's output as the next input, ensuring proper data flow across state transitions
- **Timeout Handling**: Switched state-machine repository and timeout handling to rely on `Output` instead of `Input` for more predictable behavior

**Impact:**

These changes ensure that data flows consistently through your state machine, especially when:
- Execution is paused at message states
- States are resumed after timeouts
- Chained executions are used with message states

**Example Scenario:**

```go
// Before v1.2.5: Input might be lost or inconsistent during pause/resume
// After v1.2.5: Output is properly preserved and flows to next state

// When a MessageState pauses execution:
// execCtx.Output = execCtx.Input (maintains consistency)

// When execution resumes:
// nextState.Input = previousState.Output (proper data flow)
```

### Enhanced Execution Handler Functions

Added new execution handler functions to `executor.BaseExecutor`:

- Improved control over execution lifecycle
- Better support for pausing and resuming executions
- Enhanced error handling during state transitions

### Refined Examples

Updated `examples/message_timeout_complete/main.go` with:

- **Clearer State Machine Definitions**: Improved readability and structure of example state machines
- **Execution Context in API Setup**: Added execution context to API setup methods for better demonstration
- **Enhanced Timeout Demonstrations**: Better examples showing how timeouts work with the new consistency model

---

## 🔧 Infrastructure Improvements

### CI/CD Updates

- **Go Version**: Upgraded to Go 1.24 in GitHub workflows (`ci.yml` and `release.yml`)
- Ensures compatibility with the latest Go features and improvements

### Test Coverage

- Updated test coverage summary to reflect changes in execution handling

---

## 🐛 Bug Fixes

### Input-Output Mismatch

**Issue**: During state transitions, particularly when pausing and resuming executions, input and output data could become inconsistent.

**Fix**:
- Execution context now properly maintains output state when pausing
- Resume logic correctly uses previous output as next input
- Timeout handling consistently uses output data

**Impact**: State machines with message states and timeouts now behave more predictably and maintain data integrity across pause/resume cycles.

### Timeout Handling

**Issue**: Timeout handling was inconsistent in how it accessed execution data.

**Fix**:
- Unified timeout handling to use `Output` instead of `Input`
- Ensures timeout-triggered state transitions receive correct data

---

## 📦 Files Changed

- `.github/workflows/ci.yml` - Go version upgrade
- `.github/workflows/release.yml` - Go version upgrade
- `TEST_COVERAGE_SUMMARY.md` - Updated coverage metrics
- `examples/message_timeout_complete/main.go` - Enhanced example with better demonstrations
- `pkg/statemachine/persistent/persistent.go` - Core consistency and timeout handling improvements

---

## 📈 Migration Path

This is a **patch release** with no breaking changes. Simply update your dependency:

```bash
go get -u github.com/hussainpithawala/state-machine-amz-go@v1.2.5
```

### No Code Changes Required

Your existing code will continue to work without modifications. However, you may notice:

1. **More Consistent Behavior**: State machines with message states will have more predictable data flow
2. **Better Timeout Handling**: Timeouts will correctly pass output data to subsequent states
3. **Improved Reliability**: Reduced potential for data loss during pause/resume cycles

### Recommended Testing

If you use the following features, we recommend testing your state machines after upgrade:

- **MessageState with timeouts**
- **Chained executions with message states**
- **Complex workflows with multiple pause/resume cycles**

The improved consistency should make your workflows more reliable, but testing will verify that your specific use cases benefit as expected.

---

## 🧪 Testing

All existing tests pass with the new changes:

- Integration tests for message states ✅
- Timeout handling tests ✅
- Execution chaining tests ✅
- Persistent state machine tests ✅

---

## 📚 Documentation

- Updated `examples/message_timeout_complete/main.go` with comprehensive inline comments
- Enhanced API setup methods to include execution context
- Improved state machine definition examples for clarity

---

## 🎯 Use Cases Improved

This release particularly benefits these use cases:

1. **Long-Running Workflows**: Workflows that pause at message states and resume later
2. **Timeout-Heavy Processes**: State machines with multiple timeout configurations
3. **Data-Critical Applications**: Applications where data consistency across states is crucial
4. **Chained Executions**: Using output from one execution as input to another with message states

---

## 🔍 Technical Details

### Input-Output Flow Changes

**Before v1.2.5:**
```
State 1 (Input) -> Process -> State 1 (Output)
    ↓ (Pause at MessageState)
State 2 (Input might be inconsistent) -> Process
```

**After v1.2.5:**
```
State 1 (Input) -> Process -> State 1 (Output)
    ↓ (Pause: Output = Input for consistency)
State 2 (Input = Previous Output) -> Process
```

### Repository Changes

The persistent state machine repository now:
- Stores output data consistently during pauses
- Uses output data for timeout-triggered transitions
- Maintains proper data flow during resume operations

---

## 🙏 Contributors

Thank you to everyone who contributed to this release!

---

## 📝 Full Changelog

See [CHANGELOG.md](CHANGELOG.md) for the complete list of changes.

---

## 🚀 Getting Started

### Installation

```bash
go get -u github.com/hussainpithawala/state-machine-amz-go@v1.2.5
```

### Quick Start

Check out our enhanced examples:

```bash
# Message state with timeout example
cd examples/message_timeout_complete
go run main.go
```

---

## 🐛 Known Issues

None at this time.

---

## 🔮 What's Next (v1.3.0)

We're planning for the next release:

- Enhanced observability and tracing
- Additional state types
- Performance optimizations for high-throughput scenarios
- Redis Cluster support
- Enhanced monitoring dashboards

Stay tuned for updates!

---

**Full Release**: [v1.2.5](https://github.com/hussainpithawala/state-machine-amz-go/releases/tag/v1.2.5)

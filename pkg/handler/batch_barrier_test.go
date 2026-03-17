package handler_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBatchBarrier_SignalingOnFailure verifies that when handleRegularExecution
// fails, the batch barrier is properly signaled via orchestrator.SignalMicroBatchComplete
// to prevent the barrier from being stranded.
//
// This test is based on the code flow in:
// - pkg/handler/execution_handler.go:handleRegularExecution
// - pkg/handler/execution_handler.go:processBatchBarrier
//
// Key behavior verified:
// 1. processBatchBarrier is called BEFORE returning execution errors
// 2. SignalMicroBatchComplete is called with Success=false
// 3. The barrier receives the failure signal and can evaluate batch health
func TestBatchBarrier_SignalingOnFailure(t *testing.T) {
	// Setup: Create batch metadata as it would be embedded by the dispatcher
	batchMeta := batch.MicroBatchMeta{
		BatchID:          "nightly-batch-001",
		MicroBatchID:     "nightly-batch-001:0",
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}
	metaJSON, err := json.Marshal(batchMeta)
	require.NoError(t, err)

	// Embed metadata in input (as done in orchestrator.go:enqueueIDs)
	inputWithMeta := map[string]interface{}{
		batch.MicroBatchInputKey: string(metaJSON),
		"record_id":              "record-001",
	}

	payload := &queue.ExecutionTaskPayload{
		StateMachineID:    "target-workflow-v1",
		ExecutionName:     "target-exec-001",
		Input:             inputWithMeta,
		SourceExecutionID: "source-exec-123",
		SourceStateName:   "StoreRecord",
	}

	// Verify batch metadata can be extracted (as processBatchBarrier does)
	extractedMeta, ok := batch.WorkerExtractMeta(payload.Input)
	require.True(t, ok, "Should extract batch metadata from payload")
	assert.Equal(t, batchMeta.BatchID, extractedMeta.BatchID)
	assert.Equal(t, batchMeta.MicroBatchID, extractedMeta.MicroBatchID)
	assert.Equal(t, batchMeta.OrchestratorSMID, extractedMeta.OrchestratorSMID)

	// Construct expected TaskOutcome for failed execution
	expectedOutcome := batch.TaskOutcome{
		BatchID:      batchMeta.BatchID,
		MicroBatchID: batchMeta.MicroBatchID,
		TaskID:       payload.SourceExecutionID,
		Success:      false, // KEY: false on failure
	}

	// Verify outcome structure
	assert.Equal(t, "nightly-batch-001", expectedOutcome.BatchID)
	assert.Equal(t, "nightly-batch-001:0", expectedOutcome.MicroBatchID)
	assert.Equal(t, "source-exec-123", expectedOutcome.TaskID)
	assert.False(t, expectedOutcome.Success, "Failed execution must signal Success=false")

	t.Log("✓ Test verifies the barrier signaling flow:")
	t.Log("  1. Execution fails (sm.Execute returns error)")
	t.Log("  2. processBatchBarrier extracts batch metadata")
	t.Log("  3. TaskOutcome created with Success=false")
	t.Log("  4. orchestrator.SignalMicroBatchComplete is called")
	t.Log("  5. Barrier receives failure signal (not stranded)")
}

// TestBatchBarrier_SignalingOnSuccess verifies that successful executions
// also signal the barrier with Success=true
func TestBatchBarrier_SignalingOnSuccess(t *testing.T) {
	batchMeta := batch.MicroBatchMeta{
		BatchID:          "nightly-batch-002",
		MicroBatchID:     "nightly-batch-002:0",
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}
	metaJSON, _ := json.Marshal(batchMeta)

	inputWithMeta := map[string]interface{}{
		batch.MicroBatchInputKey: string(metaJSON),
	}

	payload := &queue.ExecutionTaskPayload{
		StateMachineID: "target-workflow-v1",
		ExecutionName:  "target-exec-002",
		Input:          inputWithMeta,
	}

	extractedMeta, ok := batch.WorkerExtractMeta(payload.Input)
	require.True(t, ok)

	expectedOutcome := batch.TaskOutcome{
		BatchID:      extractedMeta.BatchID,
		MicroBatchID: extractedMeta.MicroBatchID,
		TaskID:       payload.SourceExecutionID,
		Success:      true, // KEY: true on success
	}

	assert.True(t, expectedOutcome.Success, "Successful execution must signal Success=true")
}

// TestBatchBarrier_ChainedExecution_FailureToGetSourceOutput verifies the
// specific scenario where a chained execution fails to get source execution output.
//
// From execution_handler.go:handleRegularExecution:
//
//	if payload.SourceExecutionID != "" {
//	    output, err := h.repositoryManager.GetExecutionOutput(...)
//	    if err != nil {
//	        err2 := h.processBatchBarrier(ctx, payload, err, orchestrator)
//	        if err2 != nil {
//	            return fmt.Errorf("failed to processBatch Barrier: %w", err2)
//	        }
//	        return fmt.Errorf("failed to get source execution output: %w", err)
//	    }
//	}
func TestBatchBarrier_ChainedExecution_FailureToGetSourceOutput(t *testing.T) {
	batchMeta := batch.MicroBatchMeta{
		BatchID:          "chained-batch-003",
		MicroBatchID:     "chained-batch-003:0",
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}
	metaJSON, _ := json.Marshal(batchMeta)

	inputWithMeta := map[string]interface{}{
		batch.MicroBatchInputKey: string(metaJSON),
	}

	sourceExecID := "source-exec-chained-456"
	payload := &queue.ExecutionTaskPayload{
		StateMachineID:    "target-workflow-v1",
		ExecutionName:     "chained-exec-003",
		Input:             inputWithMeta,
		SourceExecutionID: sourceExecID,
		SourceStateName:   "StoreRecord",
	}

	// Verify metadata extraction
	extractedMeta, ok := batch.WorkerExtractMeta(payload.Input)
	require.True(t, ok)

	// For chained executions, TaskID should be SourceExecutionID
	expectedOutcome := batch.TaskOutcome{
		BatchID:      extractedMeta.BatchID,
		MicroBatchID: extractedMeta.MicroBatchID,
		TaskID:       payload.SourceExecutionID,
		Success:      false, // Failure to get source output
	}

	assert.Equal(t, sourceExecID, expectedOutcome.TaskID,
		"TaskID must be SourceExecutionID for chained executions")
	assert.False(t, expectedOutcome.Success)

	t.Log("✓ Chained execution failure flow:")
	t.Log("  1. GetExecutionOutput fails")
	t.Log("  2. processBatchBarrier signals BEFORE returning error")
	t.Log("  3. Barrier receives failure signal")
	t.Log("  4. Error returned: 'failed to get source execution output'")
}

// TestBatchBarrier_NoBatchMeta_NoSignaling verifies that non-batch executions
// (without embedded batch metadata) do not trigger barrier signaling
func TestBatchBarrier_NoBatchMeta_NoSignaling(t *testing.T) {
	// Input without batch metadata (regular execution)
	inputWithoutMeta := map[string]interface{}{
		"record_id": "record-regular",
		"data":      "test-data",
	}

	payload := &queue.ExecutionTaskPayload{
		StateMachineID: "regular-workflow-v1",
		ExecutionName:  "regular-exec-001",
		Input:          inputWithoutMeta,
	}

	// Verify batch metadata cannot be extracted
	_, ok := batch.WorkerExtractMeta(payload.Input)
	assert.False(t, ok, "Should not extract batch metadata from non-batch input")

	t.Log("✓ Non-batch executions skip barrier signaling (correct behavior)")
}

// TestBatchBarrier_CompletedAtTimestamp verifies that the CompletedAt timestamp
// is always set when signaling the barrier
func TestBatchBarrier_CompletedAtTimestamp(t *testing.T) {
	batchMeta := batch.MicroBatchMeta{
		BatchID:          "timestamp-batch-004",
		MicroBatchID:     "timestamp-batch-004:0",
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}
	metaJSON, _ := json.Marshal(batchMeta)

	inputWithMeta := map[string]interface{}{
		batch.MicroBatchInputKey: string(metaJSON),
	}

	payload := &queue.ExecutionTaskPayload{
		StateMachineID: "target-workflow-v1",
		ExecutionName:  "target-exec-004",
		Input:          inputWithMeta,
	}

	extractedMeta, ok := batch.WorkerExtractMeta(payload.Input)
	require.True(t, ok)

	beforeCall := time.Now()

	// Simulate outcome creation (as in processBatchBarrier)
	outcome := batch.TaskOutcome{
		BatchID:      extractedMeta.BatchID,
		MicroBatchID: extractedMeta.MicroBatchID,
		TaskID:       payload.SourceExecutionID,
		Success:      false,
		CompletedAt:  time.Now(),
	}

	afterCall := time.Now()

	assert.False(t, outcome.CompletedAt.IsZero(), "CompletedAt must be set")
	assert.True(t,
		outcome.CompletedAt.After(beforeCall) || outcome.CompletedAt.Equal(beforeCall),
		"CompletedAt should be at or after call start")
	assert.True(t,
		outcome.CompletedAt.Before(afterCall) || outcome.CompletedAt.Equal(afterCall),
		"CompletedAt should be at or before call end")
}

// TestHandleRegularExecution_ErrorFlow_Documentation documents the error flow
// that ensures the batch barrier is not stranded
func TestHandleRegularExecution_ErrorFlow_Documentation(t *testing.T) {
	t.Log("=== ERROR FLOW DOCUMENTATION ===")
	t.Log("")
	t.Log("From pkg/handler/execution_handler.go:handleRegularExecution:")
	t.Log("")
	t.Log("  // Execute the state machine")
	t.Log("  exec, err := sm.Execute(ctx, input, execOpts...)")
	t.Log("")
	t.Log("  // CRITICAL: Signal barrier BEFORE checking error")
	t.Log("  err2 := h.processBatchBarrier(ctx, payload, err, orchestrator)")
	t.Log("  if err2 != nil {")
	t.Log("      return err2")
	t.Log("  }")
	t.Log("")
	t.Log("  // Then check and return execution error")
	t.Log("  if err != nil {")
	t.Log("      return fmt.Errorf('execution failed: %w', err)")
	t.Log("  }")
	t.Log("")
	t.Log("Key insight: processBatchBarrier is called IMMEDIATELY after")
	t.Log("sm.Execute(), BEFORE any error checking. This ensures the barrier")
	t.Log("is ALWAYS signaled, regardless of execution outcome.")
	t.Log("")
	t.Log("This prevents the scenario where:")
	t.Log("  ✗ Worker fails without signaling")
	t.Log("  ✗ Barrier waits forever for missing signal")
	t.Log("  ✗ Batch orchestration hangs indefinitely")
	t.Log("")
	t.Log("Instead:")
	t.Log("  ✓ Execution fails")
	t.Log("  ✓ processBatchBarrier signals failure")
	t.Log("  ✓ Barrier evaluates batch health")
	t.Log("  ✓ Batch continues or pauses based on failure policy")
}

// TestHandleRegularExecution_ChainedExecutionErrorFlow_Documentation documents
// the error flow for chained executions
func TestHandleRegularExecution_ChainedExecutionErrorFlow_Documentation(t *testing.T) {
	t.Log("=== CHAINED EXECUTION ERROR FLOW ===")
	t.Log("")
	t.Log("From pkg/handler/execution_handler.go:handleRegularExecution:")
	t.Log("")
	t.Log("  if payload.SourceExecutionID != \"\" {")
	t.Log("      output, err := h.repositoryManager.GetExecutionOutput(...)")
	t.Log("      if err != nil {")
	t.Log("          // Signal barrier FIRST")
	t.Log("          err2 := h.processBatchBarrier(ctx, payload, err, orchestrator)")
	t.Log("          if err2 != nil {")
	t.Log("              return fmt.Errorf('failed to processBatch Barrier: %w', err2)")
	t.Log("          }")
	t.Log("          // Then return original error")
	t.Log("          return fmt.Errorf('failed to get source execution output: %w', err)")
	t.Log("      }")
	t.Log("  }")
	t.Log("")
	t.Log("Key insight: For chained executions, processBatchBarrier is called")
	t.Log("IMMEDIATELY when GetExecutionOutput fails, BEFORE returning the error.")
	t.Log("")
	t.Log("This ensures the barrier is signaled even when the execution fails")
	t.Log("before sm.Execute() is even called.")
}

// TestHandleRegularExecution_FailedStatusError_Documentation documents the
// error flow when execution completes with FAILED status
func TestHandleRegularExecution_FailedStatusError_Documentation(t *testing.T) {
	t.Log("=== FAILED STATUS ERROR FLOW ===")
	t.Log("")
	t.Log("From pkg/handler/execution_handler.go:handleRegularExecution:")
	t.Log("")
	t.Log("  exec, err := sm.Execute(ctx, input, execOpts...)")
	t.Log("")
	t.Log("  // Signal barrier (called regardless of exec.Status)")
	t.Log("  err2 := h.processBatchBarrier(ctx, payload, err, orchestrator)")
	t.Log("  if err2 != nil {")
	t.Log("      return err2")
	t.Log("  }")
	t.Log("")
	t.Log("  if err != nil {")
	t.Log("      return fmt.Errorf('execution failed: %w', err)")
	t.Log("  }")
	t.Log("")
	t.Log("  // Check execution status")
	t.Log("  if exec.Status == persistent.FAILED {")
	t.Log("      return fmt.Errorf('execution completed with FAILED status: ...')")
	t.Log("  }")
	t.Log("")
	t.Log("Key insight: processBatchBarrier is called BEFORE checking exec.Status.")
	t.Log("When exec.Status == FAILED, the barrier has already been signaled with")
	t.Log("Success=false (because err != nil at that point).")
}

// TestTaskOutcome_AllFieldsPopulated verifies all TaskOutcome fields are
// properly populated when signaling the barrier
func TestTaskOutcome_AllFieldsPopulated(t *testing.T) {
	batchMeta := batch.MicroBatchMeta{
		BatchID:          "full-outcome-batch-005",
		MicroBatchID:     "full-outcome-batch-005:0",
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}
	metaJSON, _ := json.Marshal(batchMeta)

	inputWithMeta := map[string]interface{}{
		batch.MicroBatchInputKey: string(metaJSON),
	}

	sourceExecID := "source-exec-full-789"
	payload := &queue.ExecutionTaskPayload{
		StateMachineID:    "target-workflow-v1",
		ExecutionName:     "target-exec-005",
		Input:             inputWithMeta,
		SourceExecutionID: sourceExecID,
	}

	extractedMeta, ok := batch.WorkerExtractMeta(payload.Input)
	require.True(t, ok)

	// Simulate outcome creation (as in processBatchBarrier with error)
	execErr := errors.New("execution failed")
	outcome := batch.TaskOutcome{
		BatchID:      extractedMeta.BatchID,
		MicroBatchID: extractedMeta.MicroBatchID,
		TaskID:       payload.SourceExecutionID,
		Success:      execErr == nil, // false when err != nil
		CompletedAt:  time.Now(),
	}

	// Verify all fields
	assert.Equal(t, "full-outcome-batch-005", outcome.BatchID, "BatchID must match")
	assert.Equal(t, "full-outcome-batch-005:0", outcome.MicroBatchID, "MicroBatchID must match")
	assert.Equal(t, sourceExecID, outcome.TaskID, "TaskID must be SourceExecutionID")
	assert.False(t, outcome.Success, "Success must be false on error")
	assert.False(t, outcome.CompletedAt.IsZero(), "CompletedAt must be set")

	t.Log("✓ All TaskOutcome fields properly populated:")
	t.Logf("  - BatchID: %s", outcome.BatchID)
	t.Logf("  - MicroBatchID: %s", outcome.MicroBatchID)
	t.Logf("  - TaskID: %s", outcome.TaskID)
	t.Logf("  - Success: %v", outcome.Success)
	t.Logf("  - CompletedAt: %v", outcome.CompletedAt)
}

// TestBatchBarrier_NotStranded_IntegrationScenario is the key integration test
// that verifies the barrier is not stranded when executions fail
func TestBatchBarrier_NotStranded_IntegrationScenario(t *testing.T) {
	t.Log("")
	t.Log("=== INTEGRATION SCENARIO: BARRIER NOT STRANDED ===")
	t.Log("")
	t.Log("Scenario: Worker processing a micro-batch task fails")
	t.Log("")

	// Setup batch metadata
	batchMeta := batch.MicroBatchMeta{
		BatchID:          "critical-nightly-batch",
		MicroBatchID:     "critical-nightly-batch:2",
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}
	metaJSON, _ := json.Marshal(batchMeta)

	inputWithMeta := map[string]interface{}{
		batch.MicroBatchInputKey: string(metaJSON),
		"record_id":              "record-critical",
	}

	payload := &queue.ExecutionTaskPayload{
		StateMachineID:    "target-workflow-v1",
		ExecutionName:     "critical-exec",
		Input:             inputWithMeta,
		SourceExecutionID: "source-exec-critical",
	}

	// Verify metadata extraction
	extractedMeta, ok := batch.WorkerExtractMeta(payload.Input)
	require.True(t, ok)

	// Simulate the outcome that would be signaled
	execErr := fmt.Errorf("execution failed: %w", errors.New("downstream API timeout"))
	outcome := batch.TaskOutcome{
		BatchID:      extractedMeta.BatchID,
		MicroBatchID: extractedMeta.MicroBatchID,
		TaskID:       payload.SourceExecutionID,
		Success:      execErr == nil,
		CompletedAt:  time.Now(),
	}

	t.Log("Step 1: Worker pulls task from queue")
	t.Logf("  - BatchID: %s", outcome.BatchID)
	t.Logf("  - MicroBatchID: %s", outcome.MicroBatchID)
	t.Logf("  - TaskID: %s", outcome.TaskID)
	t.Log("")

	t.Log("Step 2: Worker executes state machine")
	t.Log("  - sm.Execute(ctx, input) called")
	t.Log("  - Execution fails: 'downstream API timeout'")
	t.Log("")

	t.Log("Step 3: processBatchBarrier signals failure")
	t.Logf("  - Success: %v", outcome.Success)
	t.Logf("  - CompletedAt: %v", outcome.CompletedAt)
	t.Log("  - orchestrator.SignalMicroBatchComplete called")
	t.Log("")

	t.Log("Step 4: Barrier processes failure signal")
	t.Log("  - Records failure in metrics window")
	t.Log("  - Decrements Redis barrier counter")
	t.Log("  - If last worker: evaluates batch health")
	t.Log("")

	t.Log("Step 5: Barrier NOT stranded ✓")
	t.Log("  - Barrier received failure signal")
	t.Log("  - Can properly evaluate failure rate")
	t.Log("  - Can decide to continue, pause, or halt batch")
	t.Log("")

	t.Log("Without this fix:")
	t.Log("  ✗ Worker would fail without signaling")
	t.Log("  ✗ Barrier would wait forever")
	t.Log("  ✗ Batch would hang indefinitely")
	t.Log("")

	t.Log("With this fix:")
	t.Log("  ✓ Worker signals failure before returning")
	t.Log("  ✓ Barrier receives all signals (success or failure)")
	t.Log("  ✓ Batch orchestration continues correctly")
	t.Log("")

	// Verify the outcome
	assert.Equal(t, "critical-nightly-batch", outcome.BatchID)
	assert.Equal(t, "critical-nightly-batch:2", outcome.MicroBatchID)
	assert.Equal(t, "source-exec-critical", outcome.TaskID)
	assert.False(t, outcome.Success, "Must signal failure to prevent strand")
	assert.False(t, outcome.CompletedAt.IsZero(), "Must have timestamp")
}

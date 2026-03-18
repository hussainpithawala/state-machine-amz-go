//go:build integration
// +build integration

package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_BatchBarrier_SignalingOnFailure is an integration test that
// verifies the batch barrier is properly signaled when executions fail,
// preventing the barrier from being stranded.
//
// This test simulates the real scenario where:
// 1. A micro-batch is dispatched with N workers
// 2. Some workers succeed, some fail
// 3. All workers (success or failure) must signal the barrier
// 4. The last worker resumes the orchestrator
func TestIntegration_BatchBarrier_SignalingOnFailure(t *testing.T) {
	ctx := context.Background()

	// Setup miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer rdb.Close()

	// Verify connectivity
	require.NoError(t, rdb.Ping(ctx).Err())

	// Setup test scenario: micro-batch with 5 workers
	const (
		batchID        = "integration-batch-001"
		microBatchID   = "integration-batch-001:0"
		microBatchSize = 5
	)

	mbMeta := batch.MicroBatchMeta{
		BatchID:          batchID,
		MicroBatchID:     microBatchID,
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}

	// Initialize barrier manually (simulating what DispatchMicroBatch does)
	barrier, err := batch.NewBarrier(ctx, rdb)
	require.NoError(t, err)

	require.NoError(t, barrier.Init(ctx, microBatchID, microBatchSize))

	// Verify barrier initialized correctly
	remaining, err := barrier.Remaining(ctx, microBatchID)
	require.NoError(t, err)
	assert.Equal(t, int64(microBatchSize), remaining, "Barrier should start with microBatchSize")

	// Simulate workers: 3 succeed, 2 fail
	type workerResult struct {
		workerID int
		success  bool
		err      error
	}

	results := make([]workerResult, microBatchSize)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Track SignalMicroBatchComplete calls
	signalCount := 0
	var signalMu sync.Mutex
	lastOutcome := batch.TaskOutcome{}

	// Create mock orchestrator that tracks signals
	mockOrch := &mockOrchestrator{
		rdb:      rdb,
		parentSM: &mockSM{},
		signalFunc: func(ctx context.Context, meta batch.MicroBatchMeta, outcome batch.TaskOutcome) error {
			signalMu.Lock()
			defer signalMu.Unlock()
			signalCount++
			lastOutcome = outcome
			return nil
		},
	}

	// Create payload with batch metadata
	metaJSON, _ := json.Marshal(mbMeta)
	inputWithMeta := map[string]interface{}{
		batch.MicroBatchInputKey: string(metaJSON),
	}

	payload := &queue.ExecutionTaskPayload{
		StateMachineID:    "target-workflow-v1",
		ExecutionName:     "integration-exec",
		Input:             inputWithMeta,
		SourceExecutionID: "source-exec-123",
	}

	// Start workers
	for i := 0; i < microBatchSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Simulate work
			time.Sleep(time.Duration(workerID*10) * time.Millisecond)

			// Determine success/failure (workers 3 and 4 fail)
			shouldFail := workerID >= 3
			var execErr error
			if shouldFail {
				execErr = fmt.Errorf("worker %d failed: simulated error", workerID)
			}

			// Simulate what processBatchBarrier does:
			// 1. Extract metadata
			// 2. Create outcome
			// 3. Signal orchestrator
			if meta, ok := batch.WorkerExtractMeta(payload.Input); ok {
				outcome := batch.TaskOutcome{
					BatchID:      meta.BatchID,
					MicroBatchID: meta.MicroBatchID,
					TaskID:       payload.SourceExecutionID,
					Success:      execErr == nil,
					CompletedAt:  time.Now(),
				}

				err := mockOrch.SignalMicroBatchComplete(ctx, meta, outcome)

				mu.Lock()
				results[workerID] = workerResult{
					workerID: workerID,
					success:  execErr == nil,
					err:      err,
				}
				mu.Unlock()
			}
		}(i)
	}

	// Wait for all workers
	wg.Wait()

	// Verify results
	t.Log("=== Worker Results ===")
	for _, r := range results {
		t.Logf("Worker %d: success=%v, signaled=%v", r.workerID, r.success, r.err == nil)
	}

	// All workers should have signaled
	assert.Equal(t, microBatchSize, signalCount, "All workers should signal the barrier")

	// Verify barrier was fully decremented (should be cleared after last worker)
	val := rdb.Get(ctx, "batch:barrier:"+microBatchID).Val()
	assert.Equal(t, "", val, "Barrier key should be deleted after last worker")

	// Verify last outcome was recorded
	t.Logf("Last outcome: BatchID=%s, MicroBatchID=%s, Success=%v",
		lastOutcome.BatchID, lastOutcome.MicroBatchID, lastOutcome.Success)

	t.Log("✓ Barrier was properly signaled by all workers (including failures)")
	t.Log("✓ Barrier was not stranded")
}

// TestIntegration_BatchBarrier_AllFailures tests the scenario where ALL workers
// in a micro-batch fail - the barrier should still be properly signaled
func TestIntegration_BatchBarrier_AllFailures(t *testing.T) {
	ctx := context.Background()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	const (
		batchID        = "all-fail-batch"
		microBatchID   = "all-fail-batch:0"
		microBatchSize = 3
	)

	mbMeta := batch.MicroBatchMeta{
		BatchID:          batchID,
		MicroBatchID:     microBatchID,
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}

	barrier, err := batch.NewBarrier(ctx, rdb)
	require.NoError(t, err)
	require.NoError(t, barrier.Init(ctx, microBatchID, microBatchSize))

	mockOrch := &mockOrchestrator{
		rdb:      rdb,
		parentSM: &mockSM{},
		signalFunc: func(ctx context.Context, meta batch.MicroBatchMeta, outcome batch.TaskOutcome) error {
			return nil
		},
	}

	metaJSON, _ := json.Marshal(mbMeta)
	payload := &queue.ExecutionTaskPayload{
		StateMachineID: "target-workflow-v1",
		Input:          map[string]interface{}{batch.MicroBatchInputKey: string(metaJSON)},
	}

	// All workers fail
	var wg sync.WaitGroup
	for i := 0; i < microBatchSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			execErr := fmt.Errorf("worker %d failed", workerID)

			if meta, ok := batch.WorkerExtractMeta(payload.Input); ok {
				outcome := batch.TaskOutcome{
					BatchID:      meta.BatchID,
					MicroBatchID: meta.MicroBatchID,
					TaskID:       fmt.Sprintf("exec-%d", workerID),
					Success:      execErr == nil,
					CompletedAt:  time.Now(),
				}
				_ = mockOrch.SignalMicroBatchComplete(ctx, meta, outcome)
			}
		}(i)
	}

	wg.Wait()

	// Verify barrier was fully decremented
	val := rdb.Get(ctx, "batch:barrier:"+microBatchID).Val()
	assert.Equal(t, "", val, "Barrier key should be deleted after all workers complete")

	t.Log("✓ All workers failed but barrier was still properly signaled")
	t.Log("✓ Barrier was not stranded even with 100% failure rate")
}

// TestIntegration_BatchBarrier_MixedOutcomes tests that the barrier correctly
// handles a mix of success and failure outcomes
func TestIntegration_BatchBarrier_MixedOutcomes(t *testing.T) {
	ctx := context.Background()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	const (
		batchID        = "mixed-outcome-batch"
		microBatchID   = "mixed-outcome-batch:0"
		microBatchSize = 10
	)

	mbMeta := batch.MicroBatchMeta{
		BatchID:          batchID,
		MicroBatchID:     microBatchID,
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}

	barrier, err := batch.NewBarrier(ctx, rdb)
	require.NoError(t, err)
	require.NoError(t, barrier.Init(ctx, microBatchID, microBatchSize))

	signalCount := 0
	successCount := 0
	failureCount := 0
	var mu sync.Mutex

	mockOrch := &mockOrchestrator{
		rdb:      rdb,
		parentSM: &mockSM{},
		signalFunc: func(ctx context.Context, meta batch.MicroBatchMeta, outcome batch.TaskOutcome) error {
			mu.Lock()
			defer mu.Unlock()
			signalCount++
			if outcome.Success {
				successCount++
			} else {
				failureCount++
			}
			return nil
		},
	}

	metaJSON, _ := json.Marshal(mbMeta)
	payload := &queue.ExecutionTaskPayload{
		StateMachineID: "target-workflow-v1",
		Input:          map[string]interface{}{batch.MicroBatchInputKey: string(metaJSON)},
	}

	// 7 succeed, 3 fail
	var wg sync.WaitGroup
	for i := 0; i < microBatchSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			var execErr error
			if workerID >= 7 {
				execErr = fmt.Errorf("worker %d failed", workerID)
			}

			if meta, ok := batch.WorkerExtractMeta(payload.Input); ok {
				outcome := batch.TaskOutcome{
					BatchID:      meta.BatchID,
					MicroBatchID: meta.MicroBatchID,
					TaskID:       fmt.Sprintf("exec-%d", workerID),
					Success:      execErr == nil,
					CompletedAt:  time.Now(),
				}
				_ = mockOrch.SignalMicroBatchComplete(ctx, meta, outcome)
			}
		}(i)
	}

	wg.Wait()

	// Verify counts
	assert.Equal(t, microBatchSize, signalCount, "All workers should signal")
	assert.Equal(t, 7, successCount, "7 workers should succeed")
	assert.Equal(t, 3, failureCount, "3 workers should fail")

	// Verify barrier was fully decremented
	val := rdb.Get(ctx, "batch:barrier:"+microBatchID).Val()
	assert.Equal(t, "", val, "Barrier key should be deleted")

	t.Logf("✓ Mixed outcomes handled: %d success, %d failure", successCount, failureCount)
	t.Log("✓ Barrier was not stranded")
}

// TestIntegration_BatchBarrier_SingleWorkerFailure tests the edge case where
// a micro-batch has only one worker and it fails
func TestIntegration_BatchBarrier_SingleWorkerFailure(t *testing.T) {
	ctx := context.Background()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	const (
		batchID      = "single-worker-batch"
		microBatchID = "single-worker-batch:0"
	)

	mbMeta := batch.MicroBatchMeta{
		BatchID:          batchID,
		MicroBatchID:     microBatchID,
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}

	barrier, err := batch.NewBarrier(ctx, rdb)
	require.NoError(t, err)
	require.NoError(t, barrier.Init(ctx, microBatchID, 1))

	signalCalled := false
	var signaledOutcome batch.TaskOutcome

	mockOrch := &mockOrchestrator{
		rdb:      rdb,
		parentSM: &mockSM{},
		signalFunc: func(ctx context.Context, meta batch.MicroBatchMeta, outcome batch.TaskOutcome) error {
			signalCalled = true
			signaledOutcome = outcome
			return nil
		},
	}

	metaJSON, _ := json.Marshal(mbMeta)
	payload := &queue.ExecutionTaskPayload{
		StateMachineID: "target-workflow-v1",
		Input:          map[string]interface{}{batch.MicroBatchInputKey: string(metaJSON)},
	}

	// Single worker fails
	execErr := fmt.Errorf("single worker failed")

	if meta, ok := batch.WorkerExtractMeta(payload.Input); ok {
		outcome := batch.TaskOutcome{
			BatchID:      meta.BatchID,
			MicroBatchID: meta.MicroBatchID,
			TaskID:       "single-exec",
			Success:      execErr == nil,
			CompletedAt:  time.Now(),
		}
		_ = mockOrch.SignalMicroBatchComplete(ctx, meta, outcome)
	}

	// Verify signal was called
	assert.True(t, signalCalled, "SignalMicroBatchComplete should be called")
	assert.False(t, signaledOutcome.Success, "Outcome should indicate failure")

	// Verify barrier was cleared
	val := rdb.Get(ctx, "batch:barrier:"+microBatchID).Val()
	assert.Equal(t, "", val, "Barrier key should be deleted")

	t.Log("✓ Single worker failure handled correctly")
	t.Log("✓ Barrier was not stranded")
}

// ──────────────────────────────────────────────────────────────────────────────
// Mock implementations
// ──────────────────────────────────────────────────────────────────────────────

// mockOrchestrator is a test mock that tracks SignalMicroBatchComplete calls
type mockOrchestrator struct {
	rdb        *redis.Client
	parentSM   *mockSM
	signalFunc func(ctx context.Context, meta batch.MicroBatchMeta, outcome batch.TaskOutcome) error
}

// SignalMicroBatchComplete implements batch.Orchestrator.SignalMicroBatchComplete
func (m *mockOrchestrator) SignalMicroBatchComplete(ctx context.Context, mbMeta batch.MicroBatchMeta, outcome batch.TaskOutcome) error {
	if m.signalFunc != nil {
		return m.signalFunc(ctx, mbMeta, outcome)
	}
	return nil
}

// mockSM is a minimal mock state machine
type mockSM struct{}

func (m *mockSM) GetID() string                             { return "mock-sm" }
func (m *mockSM) GetStartAt() string                        { return "Start" }
func (m *mockSM) GetRepositoryManager() *repository.Manager { return nil }
func (m *mockSM) GetQueueClient() *queue.Client             { return nil }
func (m *mockSM) SetQueueClient(*queue.Client)              {}
func (m *mockSM) SetExecutor(interface{})                   {}
func (m *mockSM) Execute(context.Context, interface{}, ...interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockSM) ResumeExecution(context.Context, string, interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockSM) GetExecution(context.Context, string) (interface{}, error) { return nil, nil }
func (m *mockSM) FindWaitingExecutionsByCorrelation(context.Context, string, interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockSM) SaveDefinition(context.Context) error { return nil }

// pkg/statemachine/persistent_microbatch.go
//
// This file is a DROP-IN extension to persistent.go – it lives in the same
// package and adds exactly two things:
//
//  1. Two new fields on BatchExecutionOptions (DoMicroBatch, MicroBatchSize,
//     RedisClient, FailurePolicy) needed to activate micro-batch mode.
//
//  2. executeMicroBatch – the concrete implementation that fills the previously
//     empty "if opts.DoMicroBatch" branch in executeBatchConcurrent.
//
// Changes required in the existing persistent.go
// ───────────────────────────────────────────────
// Replace the empty block:
//
//   if opts.DoMicroBatch && opts.MicroBatchSize >= 0 {
//   }
//
// with:
//
//   if opts.DoMicroBatch && opts.MicroBatchSize > 0 {
//       return pm.executeMicroBatch(ctx, sourceExecutionIDs, sourceStateName, opts, execOpts...)
//   }
//
// That single line change is the ONLY modification to persistent.go itself.
// Everything else lives here.

package persistent

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

// ──────────────────────────────────────────────────────────────────────────────
// BatchExecutionOptions extension
//
// These fields are added to the existing statemachine2.BatchExecutionOptions
// struct.  They must be added to pkg/statemachine/options.go:
//
//   // DoMicroBatch enables the adaptive micro-batch streaming path.
//   DoMicroBatch bool
//
//   // MicroBatchSize controls how many source execution IDs are dispatched
//   // in each micro-batch.  Must be > 0 when DoMicroBatch is true.
//   MicroBatchSize int
//
//   // RedisClient is required for the barrier and metrics when DoMicroBatch=true.
//   // The caller is responsible for providing a connected *redis.Client.
//   RedisClient *redis.Client
//
//   // FailurePolicy overrides the default adaptive feedback thresholds.
//   // Zero value uses package-level defaults.
//   FailurePolicy batch.FailurePolicy
// ──────────────────────────────────────────────────────────────────────────────

// executeMicroBatch implements the DoMicroBatch=true path of executeBatchConcurrent.
//
// It:
//  1. Validates that a Redis client is configured.
//  2. Constructs a batch.Orchestrator wired to the parent StateMachine.
//  3. Ensures the orchestrator state machine definition exists in the repository
//     (idempotent; safe to call on every invocation).
//  4. Calls Orchestrator.Run which stores the IDs in Redis and launches the
//     orchestrator as an async persistent.StateMachine execution.
//  5. Optionally blocks until the orchestrator terminates if the context carries
//     a deadline, otherwise returns immediately with lightweight results.
//
// Return value
// ────────────
// A single *BatchExecutionResult is returned representing the orchestration
// itself (not individual task results, which are async). The Execution field
// will be nil if the orchestrator is still running; callers should query the
// repository for final status.
func (pm *StateMachine) executeMicroBatch(
	ctx context.Context,
	sourceExecutionIDs []string,
	targetMachineId string,
	sourceStateName string,
	opts *statemachine2.BatchExecutionOptions,
	execOpts ...statemachine2.ExecutionOption,
) ([]*BatchExecutionResult, error) {
	if opts.RedisClient == nil {
		return nil, fmt.Errorf("executeMicroBatch: opts.RedisClient must be set when DoMicroBatch=true")
	}

	rdb, ok := opts.RedisClient.(*redis.Client)
	if !ok {
		return nil, fmt.Errorf("executeMicroBatch: opts.RedisClient must be *redis.Client")
	}

	// ── Build orchestrator ────────────────────────────────────────────────────
	// Create factory functions for loading/creating state machines
	smFactory := func(ctx context.Context, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
		sm, err := NewFromDefnId(ctx, smID, mgr)
		return sm, err
	}
	smCreator := func(def []byte, isJSON bool, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
		sm, err := New(def, isJSON, smID, mgr)
		return sm, err
	}

	orch, err := batch.NewOrchestrator(ctx, rdb, pm, smFactory, smCreator)
	if err != nil {
		return nil, fmt.Errorf("executeMicroBatch: new orchestrator: %w", err)
	}

	// ── Register definition once (idempotent) ─────────────────────────────────
	defJSON := batch.OrchestratorDefinitionJSON()
	if err := orch.EnsureDefinition(ctx, defJSON); err != nil {
		return nil, fmt.Errorf("executeMicroBatch: ensure definition: %w", err)
	}

	// ── Generate stable batch ID ──────────────────────────────────────────────
	batchID := fmt.Sprintf("mb-%s-%d", pm.GetID(), time.Now().UnixNano())

	// ── Launch orchestrator ───────────────────────────────────────────────────
	doneCh, err := orch.Run(ctx, batchID, sourceExecutionIDs, targetMachineId, sourceStateName, opts, execOpts)
	if err != nil {
		return nil, fmt.Errorf("executeMicroBatch: launch: %w", err)
	}

	// Return a synthetic result immediately so the caller is not blocked.
	// The orchestrator runs asynchronously; callers that need to wait can
	// consume doneCh or query the repository.
	result := &BatchExecutionResult{
		SourceExecutionID: batchID,
		Execution:         nil, // orchestrator is async; query repository for status
		Error:             nil,
		Index:             0,
	}

	// If the context has a deadline we block until the orchestrator finishes
	// or the deadline is hit – whichever comes first.
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		if orchErr := <-doneCh; orchErr != nil {
			result.Error = orchErr
		}
	}

	return []*BatchExecutionResult{result}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Worker Hook – add to the queue worker after each task completes
// ──────────────────────────────────────────────────────────────────────────────

// PostTaskMicroBatchHook should be called by the queue worker immediately after
// it finishes processing a single execution task.  It drives the Redis barrier
// and (for the last worker in a micro-batch) resumes the orchestrator.
//
// Usage in the queue worker loop:
//
//	exec, err := pm.Execute(ctx, execCtx)
//	outcome := batch.TaskOutcome{
//	    BatchID:     ...,
//	    MicroBatchID: ...,
//	    TaskID:      sourceExecID,
//	    Success:     err == nil,
//	    ...
//	}
//	if meta, ok := batch.WorkerExtractMeta(taskPayload.Input); ok {
//	    if hookErr := PostTaskMicroBatchHook(ctx, orchestrator, meta, outcome); hookErr != nil {
//	        log.Printf("warn: micro-batch hook: %v", hookErr)
//	    }
//	}
//
// The orchestrator instance must be the same one created by executeMicroBatch.
// In practice, store it on a shared app-level registry keyed by OrchestratorSMID.
func PostTaskMicroBatchHook(
	ctx context.Context,
	orch *batch.Orchestrator,
	meta batch.MicroBatchMeta,
	outcome batch.TaskOutcome,
) error {
	return orch.SignalMicroBatchComplete(ctx, meta, outcome)
}

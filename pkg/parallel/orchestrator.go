package parallel

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

const failFastRedisKey = "parallel:%s:failed"

// ParallelOrchestrator manages distributed parallel branch execution using
// the Redis barrier pattern for fork-join synchronization.
type ParallelOrchestrator struct {
	rdb       *redis.Client
	barrier   *batch.Barrier
	repo      *repository.Manager
	queueCli  *queue.Client
	smFactory batch.StateMachineFactory
}

// NewOrchestrator creates a new ParallelOrchestrator.
func NewOrchestrator(ctx context.Context, rdb *redis.Client, repo *repository.Manager, queueCli *queue.Client, smFactory batch.StateMachineFactory) (*ParallelOrchestrator, error) {
	barrier, err := batch.NewBarrier(ctx, rdb)
	if err != nil {
		return nil, fmt.Errorf("parallel/orchestrator: create barrier: %w", err)
	}
	return &ParallelOrchestrator{
		rdb:       rdb,
		barrier:   barrier,
		repo:      repo,
		queueCli:  queueCli,
		smFactory: smFactory,
	}, nil
}

// DispatchBranches implements types.ParallelDispatcher. It accepts branches as
// interface{} (must be []states.Branch) and dispatches them as queued executions.
func (o *ParallelOrchestrator) DispatchBranches(
	ctx context.Context,
	parallelID, parentExecID, parentSMID, parentStateName string,
	branches interface{},
	input interface{},
	failurePolicy string,
) ([]string, error) {
	typedBranches, ok := branches.([]states.Branch)
	if !ok {
		return nil, fmt.Errorf("parallel/orchestrator: branches must be []states.Branch")
	}
	return o.dispatchBranchesTyped(ctx, parallelID, parentExecID, parentSMID, parentStateName, typedBranches, input, failurePolicy)
}

// dispatchBranchesTyped creates branch state machine definitions, saves them to
// the repository, initializes the Redis barrier, and enqueues execution tasks
// for each branch.
func (o *ParallelOrchestrator) dispatchBranchesTyped(
	ctx context.Context,
	parallelID string,
	parentExecID string,
	parentSMID string,
	parentStateName string,
	branches []states.Branch,
	input interface{},
	failurePolicy string,
) (branchExecIDs []string, err error) {
	branchCount := len(branches)

	if err := o.barrier.Init(ctx, parallelID, branchCount); err != nil {
		return nil, fmt.Errorf("parallel/orchestrator: init barrier: %w", err)
	}

	branchExecIDs = make([]string, branchCount)

	for i, branch := range branches {
		defnJSON, smID, err := CreateBranchDefinition(parentSMID, parentStateName, i, branch)
		if err != nil {
			return nil, fmt.Errorf("parallel/orchestrator: create branch definition %d: %w", i, err)
		}

		record := &repository.StateMachineRecord{
			ID:         smID,
			Name:       smID,
			Definition: string(defnJSON),
			Version:    "1.0",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		if err := o.repo.SaveStateMachine(ctx, record); err != nil {
			return nil, fmt.Errorf("parallel/orchestrator: save branch SM %d: %w", i, err)
		}

		branchMeta := map[string]interface{}{
			"parallel_id":       parallelID,
			"branch_index":      i,
			"parent_exec_id":    parentExecID,
			"parent_sm_id":      parentSMID,
			"parent_state_name": parentStateName,
			"failure_policy":    failurePolicy,
		}

		branchInput := map[string]interface{}{
			ParallelBranchInputKey: branchMeta,
		}
		if inputMap, ok := input.(map[string]interface{}); ok {
			for k, v := range inputMap {
				branchInput[k] = v
			}
		}

		execName := fmt.Sprintf("%s-branch-%d", parallelID, i)
		branchExecIDs[i] = execName

		payload := &queue.ExecutionTaskPayload{
			StateMachineID: smID,
			ExecutionName:  execName,
			Input:          branchInput,
		}

		taskInfo, err := o.queueCli.EnqueueExecution(payload)
		if err != nil {
			return nil, fmt.Errorf("parallel/orchestrator: enqueue branch %d: %w", i, err)
		}

		parallelRecord := &repository.ParallelExecutionRecord{
			ID:              fmt.Sprintf("pe-%s-%d", parallelID, i),
			ParallelID:      parallelID,
			ParentExecID:    parentExecID,
			ParentSMID:      parentSMID,
			ParentStateName: parentStateName,
			BranchIndex:     i,
			BranchExecID:    execName,
			BranchSMID:      smID,
			Status:          StatusRunning,
			FailurePolicy:   failurePolicy,
		}
		if err := o.repo.SaveParallelExecution(ctx, parallelRecord); err != nil {
			log.Printf("warn: parallel/orchestrator: save parallel execution record %d: %v", i, err)
		}

		log.Printf("parallel/orchestrator: dispatched branch %d: TaskID=%s, SMID=%s", i, taskInfo.ID, smID)
	}

	return branchExecIDs, nil
}

// SignalBranchComplete is called by a queue worker after a branch execution
// completes. It updates the branch status, decrements the barrier, and if
// this is the last worker, triggers parent resumption.
func (o *ParallelOrchestrator) SignalBranchComplete(
	ctx context.Context,
	meta *ParallelBranchMeta,
	branchOutput interface{},
	branchErr error,
) error {
	status := StatusSucceeded
	if branchErr != nil {
		status = StatusFailed
	}

	if err := o.repo.UpdateParallelBranchStatus(ctx, meta.ParallelID, meta.BranchIndex, status, branchOutput); err != nil {
		log.Printf("warn: parallel/orchestrator: update branch status: %v", err)
	}

	if branchErr != nil && meta.FailurePolicy == FailurePolicyFailFast {
		o.rdb.Set(ctx, fmt.Sprintf(failFastRedisKey, meta.ParallelID), "1", batch.BarrierTTL)
	}

	result, err := o.barrier.Decrement(ctx, meta.ParallelID)
	if err != nil {
		return fmt.Errorf("parallel/orchestrator: decrement barrier: %w", err)
	}

	if !result.IsLastWorker {
		log.Printf("parallel/orchestrator: branch %d complete for %s, %d remaining", meta.BranchIndex, meta.ParallelID, result.Remaining)
		return nil
	}

	log.Printf("parallel/orchestrator: all branches complete for %s, resuming parent", meta.ParallelID)
	return o.resumeParent(ctx, meta)
}

// IsFailFast checks the Redis flag to determine if a parallel execution has
// been marked for fail-fast. Workers can check this before starting execution.
func (o *ParallelOrchestrator) IsFailFast(ctx context.Context, parallelID string) bool {
	val, err := o.rdb.Get(ctx, fmt.Sprintf(failFastRedisKey, parallelID)).Result()
	return err == nil && val == "1"
}

// resumeParent collects all branch outputs and resumes the parent execution.
func (o *ParallelOrchestrator) resumeParent(ctx context.Context, meta *ParallelBranchMeta) error {
	records, err := o.repo.GetParallelExecutions(ctx, meta.ParallelID)
	if err != nil {
		return fmt.Errorf("parallel/orchestrator: get branch records: %w", err)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].BranchIndex < records[j].BranchIndex
	})

	var firstError string
	branchOutputs := make([]interface{}, len(records))
	for i, rec := range records {
		branchOutputs[i] = rec.BranchOutput
		if rec.Status == StatusFailed && firstError == "" {
			firstError = fmt.Sprintf("branch %d failed", rec.BranchIndex)
		}
	}

	correlations, err := o.repo.FindWaitingExecutionsByCorrelation(ctx, ParallelCorrelationKey, meta.ParallelID)
	if err != nil || len(correlations) == 0 {
		return fmt.Errorf("parallel/orchestrator: find parent execution by correlation %q: %w (found %d)", meta.ParallelID, err, len(correlations))
	}

	parentRecord := correlations[0]

	sm, err := o.smFactory(ctx, meta.ParentSMID, o.repo)
	if err != nil {
		return fmt.Errorf("parallel/orchestrator: load parent SM: %w", err)
	}

	resumeInput := map[string]interface{}{
		"parallel_results": branchOutputs,
		"parallel_id":      meta.ParallelID,
	}

	if firstError != "" {
		resumeInput["parallel_error"] = firstError
	}

	execCtx := &execution.Execution{
		ID:                    parentRecord.ExecutionID,
		StateMachineID:        parentRecord.StateMachineID,
		Name:                  parentRecord.Name,
		Status:                parentRecord.Status,
		CurrentState:          parentRecord.CurrentState,
		Input:                 resumeInput,
		StartTime:             *parentRecord.StartTime,
		HistorySequenceNumber: parentRecord.HistorySequenceNumber,
	}

	_, err = sm.ResumeExecution(ctx, execCtx)
	if err != nil {
		return fmt.Errorf("parallel/orchestrator: resume parent: %w", err)
	}

	return nil
}

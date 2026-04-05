package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
	"github.com/redis/go-redis/v9"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	_ "github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

// ──────────────────────────────────────────────────────────────────────────────
// Local task resource names
// These must match what is declared in the state machine JSON under "Resource".
// They are registered on the executor via RegisterGoFunction.
// ──────────────────────────────────────────────────────────────────────────────

const (
	ResourceDispatch        = "batch:dispatch"
	ResourceEvaluate        = "batch:evaluate"
	ResourceCheckResume     = "batch:check-resume"
	ResourceDispatchBulk    = "batch:dispatch-bulk"
	ResourceEvaluateBulk    = "batch:evaluate-bulk"
	ResourceCheckResumeBulk = "batch:check-resume-bulk"
)

// OrchestratorStateMachineID is the ID under which the orchestrator state
// machine definition is stored in the repository.
const OrchestratorStateMachineID = "micro-batch-orchestrator-v1"

// BulkOrchestratorStateMachineID is the ID under which the bulk orchestrator
// state machine definition is stored in the repository.
const BulkOrchestratorStateMachineID = "micro-bulk-orchestrator-v1"

// ──────────────────────────────────────────────────────────────────────────────
// Orchestrator
// ──────────────────────────────────────────────────────────────────────────────

// Orchestrator wires together the barrier, metrics, evaluator, and resume
// controller and presents them as local task handlers that can be registered
// on an executor.NewBaseExecutor() and run inside state-machine-amz-go.
type Orchestrator struct {
	rdb       *redis.Client
	barrier   *Barrier
	metrics   *MetricsRecorder
	evaluator *Evaluator
	resume    *ResumeController

	// parentSM is the caller's StateMachine.  It is used to:
	//   (a) look up the waiting orchestrator execution after the barrier fires, and
	//   (b) call ResumeExecution to advance the orchestrator.
	parentSM StateMachine

	// smFactory is used to load state machines from the repository
	smFactory StateMachineFactory

	// smCreator is used to create state machines from JSON/YAML definitions
	smCreator StateMachineCreator
}

// NewOrchestrator creates an Orchestrator and pre-loads the Lua scripts into
// Redis.  The parentSM reference is the same StateMachine that called
// executeBatchConcurrent. The smFactory is used to load state machines from the repository,
// and smCreator is used to create state machines from definitions.
func NewOrchestrator(
	ctx context.Context,
	rdb *redis.Client,
	parentSM StateMachine,
	smFactory StateMachineFactory,
	smCreator StateMachineCreator,
) (*Orchestrator, error) {
	barrier, err := NewBarrier(ctx, rdb)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: %w", err)
	}
	metrics := NewMetricsRecorder(rdb, DefaultWindowN)
	return &Orchestrator{
		rdb:       rdb,
		barrier:   barrier,
		metrics:   metrics,
		evaluator: NewEvaluator(metrics),
		resume:    NewResumeController(rdb),
		parentSM:  parentSM,
		smFactory: smFactory,
		smCreator: smCreator,
	}, nil
}

// RegisterHandlers registers all three local task handlers on exec so the
// orchestrator state machine can call them as Go functions.
func (o *Orchestrator) RegisterHandlers(exec *executor.BaseExecutor) {
	exec.RegisterGoFunction(ResourceDispatch, o.handleDispatch)
	exec.RegisterGoFunction(ResourceEvaluate, o.handleEvaluate)
	exec.RegisterGoFunction(ResourceCheckResume, o.handleCheckResume)
}

// Signal is a delegating method that forwards to ResumeController.Signal
// to manually resume a paused batch after operator intervention.
func (o *Orchestrator) Signal(ctx context.Context, batchID, operator, notes string) error {
	return o.resume.Signal(ctx, batchID, operator, notes)
}

// ──────────────────────────────────────────────────────────────────────────────
// Run – called by persistent_microbatch.go
// ──────────────────────────────────────────────────────────────────────────────

// Run stores the source execution IDs in Redis, constructs the orchestrator
// input, and launches the orchestrator state machine as an execution of
// the parent's StateMachine using the microbatch orchestrator
// definition loaded from the repository.
//
// The orchestrator runs until the first WaitForMicroBatchCompletion Message
// state, at which point Execute returns with status=PAUSED.  Subsequent
// micro-batches are driven by worker calls to SignalMicroBatchComplete, which
// each call parentSM.ResumeExecution.
//
// The function returns immediately after the first Execute call; the caller
// must poll the orchestrator execution status or register a completion callback
// to know when the full batch has finished.  A convenience channel
// `doneCh` is returned so callers can block if needed.
func (o *Orchestrator) Run(
	ctx context.Context,
	batchID string,
	sourceExecutionIDs []string,
	targetMachineId string,
	sourceStateName string,
	opts *statemachine2.BatchExecutionOptions,
	execOpts []statemachine2.ExecutionOption,
) (<-chan error, error) {
	// ── 1. Store IDs in Redis ─────────────────────────────────────────────────
	if err := o.storeIDs(ctx, batchID, sourceExecutionIDs); err != nil {
		return nil, err
	}

	mbSize := opts.MicroBatchSize
	if mbSize <= 0 {
		mbSize = DefaultMicroBatchSize
	}

	// Assemble the failure policy from opts if it carries one; otherwise use
	// framework defaults.  (BatchExecutionOptions can be extended with a
	// FailurePolicy field; for now we use a safe default.)
	policy := FailurePolicy{
		WindowN:                DefaultWindowN,
		WindowM:                DefaultWindowM,
		SevereFailureThreshold: DefaultSevereFailureThreshold,
		SoftFailureThreshold:   DefaultSoftFailureThreshold,
	}

	input := OrchestratorInput{
		BatchID:              batchID,
		TotalCount:           len(sourceExecutionIDs),
		MicroBatchSize:       mbSize,
		SourceStateName:      sourceStateName,
		OrchestratorSMID:     OrchestratorStateMachineID,
		TargetStateMachineID: targetMachineId,
		FailurePolicy:        policy,
	}

	// ── 2. Build the orchestrator state machine ───────────────────────────────
	// The definition must already be saved in the repository under
	// OrchestratorStateMachineID (done once at application startup via
	// Orchestrator.EnsureDefinition).
	orchestratorSM, err := o.smFactory(ctx, OrchestratorStateMachineID, o.parentSM.GetRepositoryManager())
	if err != nil {
		return nil, fmt.Errorf("orchestrator: load definition: %w", err)
	}

	// Register local task handlers on a fresh executor and attach it.
	exec := executor.NewBaseExecutor()
	o.RegisterHandlers(exec)
	orchestratorSM.SetExecutor(exec)

	// Share the parent's queue client so DispatchMicroBatch can enqueue tasks.
	if qc := o.parentSM.GetQueueClient(); qc != nil {
		orchestratorSM.SetQueueClient(qc)
	}

	// ── 3. Launch async – the orchestrator pauses at the first Message state ──
	doneCh := make(chan error, 1)
	go func() {
		execName := fmt.Sprintf("mb-orch-%s-%d", batchID, time.Now().UnixNano())
		execCtx := execution.NewContext(execName, orchestratorSM.GetStartAt(), input)
		execCtx.ID = fmt.Sprintf("%s-baseExecutor-%d", OrchestratorStateMachineID, time.Now().UnixNano())
		execCtx.StateMachineID = OrchestratorStateMachineID

		ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

		result, err := orchestratorSM.Execute(ctx, execCtx)
		if err != nil {
			doneCh <- err
			return
		}
		// If the orchestrator paused (PAUSED) it will be resumed by worker
		// signals. We watch the repository for terminal status.
		if result.Status == "PAUSED" {
			doneCh <- o.waitForTermination(ctx, orchestratorSM, result.ID)
			return
		}
		doneCh <- nil
	}()

	return doneCh, nil
}

// RunBulk stores the bulk input data in Redis, constructs the bulk orchestrator
// input, and launches the bulk orchestrator state machine as an execution of
// the parent's StateMachine using the bulk orchestrator definition.
func (o *Orchestrator) RunBulk(
	ctx context.Context,
	batchID string,
	inputs []interface{},
	targetMachineID string,
	opts *statemachine2.BulkExecutionOptions,
	execOpts []statemachine2.ExecutionOption,
) (<-chan error, error) {
	// ── Prevent duplicate batch orchestrations ─────────────────────────────────
	// Use Redis SETNX to ensure only one orchestrator execution can run for a
	// given batchID. This prevents duplicate processing when RunBulk is called
	// multiple times (e.g., due to retries, restarts, or caller bugs).
	batchLockKey := fmt.Sprintf("batch:lock:%s", batchID)
	locked, err := o.rdb.SetNX(ctx, batchLockKey, "locked", 24*time.Hour).Result()
	if err != nil {
		return nil, fmt.Errorf("batch:run-bulk: acquire lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("batch:run-bulk: batch %s is already running", batchID)
	}

	if err := o.storeBulkInputs(ctx, batchID, inputs); err != nil {
		return nil, err
	}

	mbSize := opts.MicroBatchSize
	if mbSize <= 0 {
		mbSize = DefaultMicroBatchSize
	}

	policy := getFailurePolicy(opts.FailurePolicyConfig)

	config := &statemachine2.ExecutionConfig{}
	for _, execOpt := range execOpts {
		execOpt(config)
	}

	input := BulkOrchestratorInput{
		BatchID:              batchID,
		TotalCount:           len(inputs),
		MicroBatchSize:       mbSize,
		TargetStateMachineID: targetMachineID,
		OrchestratorSMID:     BulkOrchestratorStateMachineID,
		ExecutionNamePrefix:  opts.NamePrefix,
		InputTransformerName: config.InputTransformerName,
		ApplyUnique:          config.ApplyUnique,
		FailurePolicy:        policy,
	}

	orchestratorSM, err := o.smFactory(ctx, BulkOrchestratorStateMachineID, o.parentSM.GetRepositoryManager())
	if err != nil {
		return nil, fmt.Errorf("bulk orchestrator: load definition: %w", err)
	}

	exec := executor.NewBaseExecutor()
	o.RegisterBulkHandlers(exec)
	orchestratorSM.SetExecutor(exec)

	if qc := o.parentSM.GetQueueClient(); qc != nil {
		orchestratorSM.SetQueueClient(qc)
	}

	doneCh := make(chan error, 1)
	go func() {
		execName := fmt.Sprintf("bulk-orch-%s-%d", batchID, time.Now().UnixNano())
		execCtx := execution.NewContext(execName, "DispatchBulkMicroBatch", input)
		execCtx.ID = fmt.Sprintf("%s-baseExecutor-%d", BulkOrchestratorStateMachineID, time.Now().UnixNano())
		execCtx.StateMachineID = BulkOrchestratorStateMachineID

		ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

		result, err := orchestratorSM.Execute(ctx, execCtx)
		if err != nil {
			doneCh <- err
			return
		}
		if result.Status == "PAUSED" {
			doneCh <- o.waitForTermination(ctx, orchestratorSM, result.ID)
			return
		}
		doneCh <- nil
	}()

	return doneCh, nil
}

// storeBulkInputs writes the bulk input data into Redis as JSON strings.
func (o *Orchestrator) storeBulkInputs(ctx context.Context, batchID string, inputs []interface{}) error {
	if len(inputs) == 0 {
		return nil
	}

	key := keyBulkInputsList(batchID)
	pipe := o.rdb.Pipeline()

	for _, input := range inputs {
		inputJSON, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("marshal input: %w", err)
		}
		pipe.RPush(ctx, key, inputJSON)
	}
	pipe.Expire(ctx, key, IDsListTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func getFailurePolicy(config interface{}) FailurePolicy {
	if config == nil {
		return FailurePolicy{
			WindowN:                DefaultWindowN,
			WindowM:                DefaultWindowM,
			SevereFailureThreshold: DefaultSevereFailureThreshold,
			SoftFailureThreshold:   DefaultSoftFailureThreshold,
		}
	}
	if policy, ok := config.(FailurePolicy); ok {
		return policy
	}
	return FailurePolicy{
		WindowN:                DefaultWindowN,
		WindowM:                DefaultWindowM,
		SevereFailureThreshold: DefaultSevereFailureThreshold,
		SoftFailureThreshold:   DefaultSoftFailureThreshold,
	}
}

// waitForTermination polls the repository until the orchestrator execution
// reaches a terminal status (SUCCEEDED or FAILED) or the context is cancelled.
func (o *Orchestrator) waitForTermination(ctx context.Context, sm StateMachine, execID string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			rec, err := sm.GetExecution(ctx, execID)
			if err != nil {
				continue
			}
			switch rec.Status {
			case "SUCCEEDED":
				return nil
			case "FAILED":
				return fmt.Errorf("orchestrator execution %s FAILED", execID)
			}
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Common dispatch helpers
// ──────────────────────────────────────────────────────────────────────────────

// dispatchSlice represents a slice of micro-batch work returned by
// computeDispatchSlice for both ID-based and input-based dispatching.
type dispatchSlice struct {
	mbIndex    int
	mbID       string
	actualSize int
	cursorVal  int
}

// computeDispatchSlice calculates which micro-batch should be dispatched next.
// Returns nil if all work has been dispatched.
func (o *Orchestrator) computeDispatchSlice(
	ctx context.Context,
	batchID string,
	totalCount int,
	microBatchSize int,
) (*dispatchSlice, error) {
	mbSize := microBatchSize
	if mbSize <= 0 {
		mbSize = DefaultMicroBatchSize
	}

	cursorVal, err := o.rdb.Get(ctx, keyCursor(batchID)).Int()
	if err == redis.Nil {
		cursorVal = 0
	} else if err != nil {
		return nil, fmt.Errorf("read cursor for batch %s: %w", batchID, err)
	}

	if cursorVal >= totalCount {
		return nil, nil // All work dispatched
	}

	mbIndex := cursorVal / mbSize
	mbID := microBatchID(batchID, mbIndex)

	return &dispatchSlice{
		mbIndex:   mbIndex,
		mbID:      mbID,
		cursorVal: cursorVal,
	}, nil
}

// checkIdempotency checks whether a micro-batch has already been dispatched.
// Returns true if the batch was already dispatched (caller should skip).
func (o *Orchestrator) checkIdempotency(
	ctx context.Context,
	batchID string,
	mbIndex int,
	prefix string,
) (dispatchedKey string, alreadyDispatched bool, error error) {
	dispatchedKey = fmt.Sprintf("batch:dispatched:%s:mb%d", batchID, mbIndex)
	dispatched, err := o.rdb.Get(ctx, dispatchedKey).Result()
	if err == nil && dispatched == "1" {
		return dispatchedKey, true, nil
	} else if err != nil && err != redis.Nil {
		return dispatchedKey, false, fmt.Errorf("check dispatched key: %w", err)
	}
	return dispatchedKey, false, nil
}

// markDispatched atomically marks a micro-batch as dispatched and advances the cursor.
func (o *Orchestrator) markDispatched(
	ctx context.Context,
	batchID string,
	dispatchedKey string,
	newCursor int,
) error {
	pipe := o.rdb.Pipeline()
	pipe.Set(ctx, dispatchedKey, "1", IDsListTTL)
	pipe.Set(ctx, keyCursor(batchID), newCursor, MetricsTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("mark dispatched and advance cursor: %w", err)
	}
	return nil
}

// buildDispatchResult constructs the standard dispatch result map.
func buildDispatchResult(
	batchID, orchestratorID string,
	totalCount, mbSize int,
	mbID string,
	mbIndex, actualSize int,
	alreadyDispatched bool,
	extraFields map[string]interface{},
) map[string]interface{} {
	result := map[string]interface{}{
		"batchID":        batchID,
		"orchestratorID": orchestratorID,
		"totalCount":     totalCount,
		"microBatchSize": mbSize,
		"dispatchResult": map[string]interface{}{
			"isBatchComplete":   false,
			"microBatchId":      mbID,
			"microBatchIndex":   mbIndex,
			"size":              actualSize,
			"dispatchedAt":      time.Now().UTC(),
			"alreadyDispatched": alreadyDispatched,
		},
	}

	// Merge any extra fields specific to the dispatch type
	for k, v := range extraFields {
		result[k] = v
	}

	return result
}

// ──────────────────────────────────────────────────────────────────────────────
// Task Handlers (local Go functions registered on the executor)
// ──────────────────────────────────────────────────────────────────────────────

// handleDispatch is the local handler for the DispatchMicroBatch Task state.
//
// Input:  OrchestratorInput (as map[string]interface{} from the state machine)
// Output: DispatchResult    (written to $.dispatchResult via ResultPath)
func (o *Orchestrator) handleDispatch(ctx context.Context, rawInput interface{}) (interface{}, error) {
	var input OrchestratorInput
	if err := remarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("batch:dispatch: unmarshal input: %w", err)
	}

	mbSize := input.MicroBatchSize
	if mbSize <= 0 {
		mbSize = DefaultMicroBatchSize
	}

	// ── Compute dispatch slice ────────────────────────────────────────────────
	slice, err := o.computeDispatchSlice(ctx, input.BatchID, input.TotalCount, mbSize)
	if err != nil {
		return nil, fmt.Errorf("batch:dispatch: compute slice: %w", err)
	}

	if slice == nil {
		return map[string]interface{}{
			"batchID":              input.BatchID,
			"orchestratorID":       input.OrchestratorSMID,
			"totalCount":           input.TotalCount,
			"microBatchSize":       input.MicroBatchSize,
			"sourceStateMachineID": input.SourceStateMachineID,
			"sourceStateName":      input.SourceStateName,
			"dispatchResult": map[string]interface{}{
				"isBatchComplete": true,
			},
		}, nil
	}

	// ── Read IDs from Redis ───────────────────────────────────────────────────
	end := int64(slice.cursorVal + mbSize - 1)
	if int(end) >= input.TotalCount {
		end = int64(input.TotalCount - 1)
	}
	ids, err := o.rdb.LRange(ctx, keyIDsList(input.BatchID), int64(slice.cursorVal), end).Result()
	if err != nil {
		return nil, fmt.Errorf("batch:dispatch: lrange: %w", err)
	}
	slice.actualSize = len(ids)

	// ── Idempotency guard ─────────────────────────────────────────────────────
	dispatchedKey, alreadyDispatched, err := o.checkIdempotency(ctx, input.BatchID, slice.mbIndex, "dispatch")
	if err != nil {
		return nil, fmt.Errorf("batch:dispatch: %w", err)
	}
	if alreadyDispatched {
		newCursor := slice.cursorVal + slice.actualSize
		if err := o.rdb.Set(ctx, keyCursor(input.BatchID), newCursor, MetricsTTL).Err(); err != nil {
			return nil, fmt.Errorf("batch:dispatch: advance cursor (skip dispatched): %w", err)
		}
		return buildDispatchResult(
			input.BatchID, input.OrchestratorSMID, input.TotalCount, mbSize,
			slice.mbID, slice.mbIndex, slice.actualSize, true,
			map[string]interface{}{
				"sourceStateMachineID": input.SourceStateMachineID,
				"sourceStateName":      input.SourceStateName,
			},
		), nil
	}

	// ── Initialise barrier and enqueue ────────────────────────────────────────
	if err := o.barrier.Init(ctx, slice.mbID, slice.actualSize); err != nil {
		return nil, fmt.Errorf("batch:dispatch: init barrier: %w", err)
	}

	if err := o.enqueueIDs(ctx, input, ids, slice.mbID, slice.mbIndex); err != nil {
		return nil, fmt.Errorf("batch:dispatch: enqueue: %w", err)
	}

	// ── Mark dispatched ───────────────────────────────────────────────────────
	newCursor := slice.cursorVal + slice.actualSize
	if err := o.markDispatched(ctx, input.BatchID, dispatchedKey, newCursor); err != nil {
		return nil, fmt.Errorf("batch:dispatch: %w", err)
	}

	return buildDispatchResult(
		input.BatchID, input.OrchestratorSMID, input.TotalCount, mbSize,
		slice.mbID, slice.mbIndex, slice.actualSize, false,
		map[string]interface{}{
			"sourceStateMachineID": input.SourceStateMachineID,
			"sourceStateName":      input.SourceStateName,
		},
	), nil
}

// handleEvaluate is the local handler for the EvaluateMicroBatch Task state.
//
// Input:  OrchestratorInput (with $.dispatchResult populated)
// Output: EvaluationResult  (written to $.evaluation via ResultPath)
func (o *Orchestrator) handleEvaluate(ctx context.Context, rawInput interface{}) (interface{}, error) {
	var input OrchestratorInput
	if err := remarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("batch:evaluate: unmarshal: %w", err)
	}
	if input.DispatchResult == nil {
		return nil, fmt.Errorf("batch:evaluate: missing dispatchResult in input")
	}

	return o.evaluator.Evaluate(ctx, input.BatchID, input.DispatchResult.MicroBatchIndex, input.FailurePolicy)
}

// handleCheckResume is the local handler for the CheckResumeSignal Task state.
//
// Input:  OrchestratorInput
// Output: ResumeCheckResult (written to $.resumeCheck via ResultPath)
func (o *Orchestrator) handleCheckResume(ctx context.Context, rawInput interface{}) (interface{}, error) {
	var input OrchestratorInput
	if err := remarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("batch:check-resume: unmarshal: %w", err)
	}
	return o.resume.Check(ctx, input.BatchID)
}

// handleDispatchBulk is the local handler for the DispatchBulkMicroBatch Task state.
//
// Input:  BulkOrchestratorInput
// Output: DispatchResult (written to $.dispatchResult via ResultPath)
func (o *Orchestrator) handleDispatchBulk(ctx context.Context, rawInput interface{}) (interface{}, error) {
	var input BulkOrchestratorInput
	if err := remarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("batch:dispatch-bulk: unmarshal input: %w", err)
	}

	mbSize := input.MicroBatchSize
	if mbSize <= 0 {
		mbSize = DefaultMicroBatchSize
	}

	// ── Compute dispatch slice ────────────────────────────────────────────────
	slice, err := o.computeDispatchSlice(ctx, input.BatchID, input.TotalCount, mbSize)
	if err != nil {
		return nil, fmt.Errorf("batch:dispatch-bulk: compute slice: %w", err)
	}

	if slice == nil {
		return map[string]interface{}{
			"batchID":              input.BatchID,
			"orchestratorID":       input.OrchestratorSMID,
			"totalCount":           input.TotalCount,
			"microBatchSize":       mbSize,
			"targetStateMachineID": input.TargetStateMachineID,
			"executionNamePrefix":  input.ExecutionNamePrefix,
			"inputTransformerName": input.InputTransformerName,
			"applyUnique":          input.ApplyUnique,
			"failurePolicy":        input.FailurePolicy,
			"dispatchResult": map[string]interface{}{
				"isBatchComplete": true,
			},
		}, nil
	}

	// ── Read inputs from Redis ────────────────────────────────────────────────
	end := int64(slice.cursorVal + mbSize - 1)
	if int(end) >= input.TotalCount {
		end = int64(input.TotalCount - 1)
	}
	inputs, err := o.rdb.LRange(ctx, keyBulkInputsList(input.BatchID), int64(slice.cursorVal), end).Result()
	if err != nil {
		return nil, fmt.Errorf("batch:dispatch-bulk: lrange: %w", err)
	}
	slice.actualSize = len(inputs)

	// ── Idempotency guard ─────────────────────────────────────────────────────
	dispatchedKey, alreadyDispatched, err := o.checkIdempotency(ctx, input.BatchID, slice.mbIndex, "dispatch-bulk")
	if err != nil {
		return nil, fmt.Errorf("batch:dispatch-bulk: %w", err)
	}
	if alreadyDispatched {
		newCursor := slice.cursorVal + slice.actualSize
		if err := o.rdb.Set(ctx, keyCursor(input.BatchID), newCursor, MetricsTTL).Err(); err != nil {
			return nil, fmt.Errorf("batch:dispatch-bulk: advance cursor (skip dispatched): %w", err)
		}
		return buildDispatchResult(
			input.BatchID, input.OrchestratorSMID, input.TotalCount, mbSize,
			slice.mbID, slice.mbIndex, slice.actualSize, true,
			map[string]interface{}{
				"targetStateMachineID": input.TargetStateMachineID,
				"executionNamePrefix":  input.ExecutionNamePrefix,
				"inputTransformerName": input.InputTransformerName,
				"applyUnique":          input.ApplyUnique,
				"failurePolicy":        input.FailurePolicy,
			},
		), nil
	}

	// ── Initialise barrier ────────────────────────────────────────────────────
	if err := o.barrier.Init(ctx, slice.mbID, slice.actualSize); err != nil {
		return nil, fmt.Errorf("batch:dispatch-bulk: init barrier: %w", err)
	}

	// ── Mark dispatched and enqueue ───────────────────────────────────────────
	// Mark dispatched BEFORE enqueue for at-most-once semantics. If we crash
	// after this point, the micro-batch is skipped (preferable to duplicates).
	newCursor := slice.cursorVal + slice.actualSize
	if err := o.markDispatched(ctx, input.BatchID, dispatchedKey, newCursor); err != nil {
		return nil, fmt.Errorf("batch:dispatch-bulk: %w", err)
	}

	if err := o.enqueueBulkInputs(ctx, input, inputs, slice.mbID, slice.mbIndex); err != nil {
		return nil, fmt.Errorf("batch:dispatch-bulk: enqueue: %w", err)
	}

	return buildDispatchResult(
		input.BatchID, input.OrchestratorSMID, input.TotalCount, mbSize,
		slice.mbID, slice.mbIndex, slice.actualSize, false,
		map[string]interface{}{
			"targetStateMachineID": input.TargetStateMachineID,
			"executionNamePrefix":  input.ExecutionNamePrefix,
			"inputTransformerName": input.InputTransformerName,
			"applyUnique":          input.ApplyUnique,
			"failurePolicy":        input.FailurePolicy,
		},
	), nil
}

// handleEvaluateBulk is the local handler for the EvaluateBulkMicroBatch Task state.
//
// Input:  BulkOrchestratorInput (with $.dispatchResult populated)
// Output: EvaluationResult (written to $.evaluation via ResultPath)
func (o *Orchestrator) handleEvaluateBulk(ctx context.Context, rawInput interface{}) (interface{}, error) {
	var input BulkOrchestratorInput
	if err := remarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("batch:evaluate-bulk: unmarshal: %w", err)
	}
	if input.DispatchResult == nil {
		return nil, fmt.Errorf("batch:evaluate-bulk: missing dispatchResult in input")
	}

	return o.evaluator.Evaluate(ctx, input.BatchID, input.DispatchResult.MicroBatchIndex, input.FailurePolicy)
}

// enqueueBulkInputs pushes one task per input to the queue for bulk execution.
func (o *Orchestrator) enqueueBulkInputs(
	ctx context.Context,
	input BulkOrchestratorInput,
	inputs []string,
	mbID string,
	mbIndex int,
) error {
	qc := o.parentSM.GetQueueClient()
	if qc == nil {
		return fmt.Errorf("no queue client configured")
	}

	meta := MicroBatchMeta{
		BatchID:          input.BatchID,
		MicroBatchID:     mbID,
		OrchestratorSMID: input.OrchestratorSMID,
	}
	metaJSON, _ := json.Marshal(meta)

	mbStart := mbIndex * input.MicroBatchSize

	for idx, inputJSON := range inputs {
		var inputData interface{}
		if err := json.Unmarshal([]byte(inputJSON), &inputData); err != nil {
			return fmt.Errorf("unmarshal input %d: %w", idx, err)
		}

		// Embed micro-batch metadata in input so worker can extract it
		inputMap, ok := inputData.(map[string]interface{})
		if !ok {
			inputMap = map[string]interface{}{}
		}
		inputMap[MicroBatchInputKey] = string(metaJSON)

		globalIdx := mbStart + idx
		payload := &queue.ExecutionTaskPayload{
			StateMachineID:       input.TargetStateMachineID,
			SourceExecutionID:    "",
			SourceStateName:      "",
			InputTransformerName: input.InputTransformerName,
			ApplyUnique:          input.ApplyUnique,
			ExecutionName:        fmt.Sprintf("%s-%d", input.ExecutionNamePrefix, globalIdx),
			ExecutionIndex:       globalIdx,
			Input:                inputMap,
		}

		if _, err := qc.EnqueueExecution(payload); err != nil {
			return fmt.Errorf("enqueue input %d: %w", idx, err)
		}
	}

	return nil
}

//	func cursorForIndex(mbStart int, mbIndex int, mbSize int) int {
//		return mbStart + (mbIndex * mbSize)
//	}

// keyBulkInputsList returns the Redis key for the bulk inputs list.
func keyBulkInputsList(batchID string) string {
	return fmt.Sprintf("batch:bulk:inputs:%s", batchID)
}

// RegisterBulkHandlers registers the bulk local task handlers on exec so the
// bulk orchestrator state machine can call them as Go functions.
func (o *Orchestrator) RegisterBulkHandlers(exec *executor.BaseExecutor) {
	exec.RegisterGoFunction(ResourceDispatchBulk, o.handleDispatchBulk)
	exec.RegisterGoFunction(ResourceEvaluateBulk, o.handleEvaluateBulk)
	exec.RegisterGoFunction(ResourceCheckResumeBulk, o.handleCheckResume)
}

// ──────────────────────────────────────────────────────────────────────────────
// Worker-side: SignalMicroBatchComplete
// ──────────────────────────────────────────────────────────────────────────────

// SignalMicroBatchComplete is called by queue workers after every individual
// task execution.  It:
//  1. Records the task outcome in the metrics window.
//  2. Atomically decrements the Redis barrier.
//  3. If this is the last worker (barrier → 0): snapshots the micro-batch rate,
//     finds the waiting orchestrator execution via MessageCorrelation, and calls
//     parentSM.ResumeExecution to advance the orchestrator state machine.
//
// The mbMeta is extracted from the task payload by WorkerExtractMeta.
func (o *Orchestrator) SignalMicroBatchComplete(
	ctx context.Context,
	mbMeta MicroBatchMeta,
	outcome TaskOutcome,
) error {
	// ── 1. Record outcome ─────────────────────────────────────────────────────
	if err := o.metrics.RecordOutcome(ctx, outcome); err != nil {
		// Non-fatal; emit a metric in production rather than blocking.
		fmt.Printf("warn: batch metrics record: %v\n", err)
	}

	// ── 2. Decrement barrier ──────────────────────────────────────────────────
	result, err := o.barrier.Decrement(ctx, mbMeta.MicroBatchID)
	if err != nil {
		return fmt.Errorf("signal: barrier decrement: %w", err)
	}
	if result.Remaining == -1 {
		// Key was already gone (duplicate delivery / retry after barrier cleared).
		return nil
	}
	if !result.IsLastWorker {
		return nil // Others are still running; nothing more to do.
	}

	// ── 3. Winner: snapshot rate + resume orchestrator ────────────────────────
	mbIndex := parseMicroBatchIndex(mbMeta.BatchID, mbMeta.MicroBatchID)
	if _, err := o.metrics.SnapshotMicroBatchRate(ctx, mbMeta.BatchID, mbIndex); err != nil {
		fmt.Printf("warn: snapshot mb rate: %v\n", err)
	}

	return o.ResumeOrchestrator(ctx, mbMeta)
}

// ResumeOrchestrator finds the waiting orchestrator execution and resumes it.
func (o *Orchestrator) ResumeOrchestrator(ctx context.Context, mbMeta MicroBatchMeta) error {
	// Find the orchestrator execution that is paused at WaitForMicroBatchCompletion
	// with CorrelationKey="micro_batch_id", CorrelationValue=mbMeta.MicroBatchID.
	executions, err := o.parentSM.FindWaitingExecutionsByCorrelation(
		ctx,
		MicroBatchCorrelationKey,
		mbMeta.MicroBatchID,
	)
	if err != nil {
		return fmt.Errorf("signal: find waiting executions: %w", err)
	}
	if len(executions) == 0 {
		return fmt.Errorf("signal: no waiting orchestrator for micro_batch_id=%s", mbMeta.MicroBatchID)
	}

	for _, rec := range executions {
		// Reconstruct the execution context with the completion payload as
		// additional input (merged by MergeInputs in RunExecution).
		// IMPORTANT: Include the ReceivedMessage marker so the Message state
		// knows this is a resumption and advances to the Next state.

		// Since WaitForMicroBatchCompletion/WaitForBulkMicroBatchCompletion has no ResultPath,
		// the Message state will use messageData.Data as the entire output. We MUST preserve the
		// complete orchestrator state from rec.Input (which is stored under the "$" key).
		// Note: We do NOT include the marker itself in data to avoid circular references.
		var stateData interface{}
		if inputMap, ok := rec.Input.(map[string]interface{}); ok {
			if dollarData, exists := inputMap["$"]; exists {
				stateData = dollarData
			} else {
				stateData = rec.Input
			}
		} else {
			stateData = rec.Input
		}

		// Use the correct message state marker based on whether this is bulk or not
		messageStateName := "WaitForMicroBatchCompletion"
		if mbMeta.OrchestratorSMID == BulkOrchestratorStateMachineID {
			messageStateName = "WaitForBulkMicroBatchCompletion"
		}

		completionPayload := map[string]interface{}{
			// Message state marker - tells WaitForMicroBatchCompletion/WaitForBulkMicroBatchCompletion to advance
			// Key format: __received_message___<StateName>
			fmt.Sprintf("__received_message___%s", messageStateName): map[string]interface{}{
				"correlation_key":   MicroBatchCorrelationKey,
				"correlation_value": mbMeta.MicroBatchID,
				"received_at":       time.Now().Unix(),
				// Preserve only the actual state data (without markers)
				"data": stateData,
			},
		}
		execCtx := &execution.Execution{
			ID:             rec.ExecutionID,
			StateMachineID: rec.StateMachineID,
			Name:           rec.Name,
			Status:         rec.Status,
			CurrentState:   rec.CurrentState,
			Input:          rec.Input,                                             // Keep original input
			Output:         mergeCompletionPayload(rec.Output, completionPayload), // Merge marker into OUTPUT (ResumeExecution uses Output as input)
		}
		if rec.StartTime != nil {
			execCtx.StartTime = *rec.StartTime
		}

		factory, err := o.smFactory(ctx, mbMeta.OrchestratorSMID, o.parentSM.GetRepositoryManager())
		if err != nil {
			return err
		}

		// Register handlers and queue client for this resumed execution
		exec := executor.NewBaseExecutor()
		// Use bulk handlers if this is the bulk orchestrator
		if mbMeta.OrchestratorSMID == BulkOrchestratorStateMachineID {
			o.RegisterBulkHandlers(exec)
		} else {
			o.RegisterHandlers(exec)
		}
		factory.SetExecutor(exec)
		if qc := o.parentSM.GetQueueClient(); qc != nil {
			factory.SetQueueClient(qc)
		}

		// Resume the orchestrator in a goroutine so it can run to completion
		// without blocking the worker that called SignalMicroBatchComplete.
		// Use context.Background() to ensure the goroutine isn't cancelled
		// when the worker's context is done.
		go func(sm StateMachine, exec *executor.BaseExecutor, execCtx *execution.Execution) {
			// Create a fresh context with the executor adapter
			resumeCtx := context.WithValue(context.Background(), types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

			result, err := sm.ResumeExecution(resumeCtx, execCtx)
			if err != nil {
				fmt.Printf("error: orchestrator resume failed for %s: %v\n", execCtx.ID, err)
				return
			}
			fmt.Printf("orchestrator execution %s completed with status: %s at state: %s\n",
				result.ID, result.Status, result.CurrentState)
		}(factory, exec, execCtx)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// EnsureDefinition – idempotent startup registration
// ──────────────────────────────────────────────────────────────────────────────

// EnsureDefinition saves the orchestrator state machine definition into the
// repository if it does not already exist.  Call once at application startup
// before any micro-batch execution is initiated.
//
//	definitionJSON – the content of workflows/microbatch_orchestrator.json
func (o *Orchestrator) EnsureDefinition(ctx context.Context, definitionJSON []byte, defnId string) error {
	sm, err := o.smCreator(definitionJSON, true, defnId, o.parentSM.GetRepositoryManager())
	if err != nil {
		return fmt.Errorf("orchestrator: parse definition: %w", err)
	}
	return sm.SaveDefinition(ctx)
}

// ──────────────────────────────────────────────────────────────────────────────
// Private helpers
// ──────────────────────────────────────────────────────────────────────────────

// storeIDs writes the full sourceExecutionIDs slice into Redis as a LIST so
// the dispatch handler can LRANGE slices without loading all IDs into memory
// inside the state machine document.
func (o *Orchestrator) storeIDs(ctx context.Context, batchID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	key := keyIDsList(batchID)
	pipe := o.rdb.Pipeline()
	// RPUSH in chunks of 1 000 to avoid huge single commands.
	const chunkSize = 1_000
	for start := 0; start < len(ids); start += chunkSize {
		end := start + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		slice := make([]interface{}, len(ids[start:end]))
		for i, id := range ids[start:end] {
			slice[i] = id
		}
		pipe.RPush(ctx, key, slice...)
	}
	pipe.Expire(ctx, key, IDsListTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// enqueueIDs pushes one task per sourceExecutionID to the queue.  Each payload
// carries MicroBatchMeta so the queue worker can drive the barrier.
func (o *Orchestrator) enqueueIDs(
	ctx context.Context,
	input OrchestratorInput,
	ids []string,
	mbID string,
	mbIndex int,
) error {
	qc := o.parentSM.GetQueueClient()
	if qc == nil {
		return fmt.Errorf("no queue client configured")
	}

	meta := MicroBatchMeta{
		BatchID:          input.BatchID,
		MicroBatchID:     mbID,
		OrchestratorSMID: input.OrchestratorSMID,
	}
	metaJSON, _ := json.Marshal(meta)

	for idx, sourceExecID := range ids {
		// Embed the MicroBatchMeta inside the task Input map so the worker
		// can extract it without any change to ExecutionTaskPayload's struct.
		taskInput := map[string]interface{}{
			MicroBatchInputKey: string(metaJSON),
		}

		payload := &queue.ExecutionTaskPayload{
			StateMachineID:    input.TargetStateMachineID,
			SourceExecutionID: sourceExecID,
			SourceStateName:   input.SourceStateName,
			ExecutionName:     fmt.Sprintf("%s-mb%d-%d", input.BatchID, mbIndex, idx),
			ExecutionIndex:    idx,
			Input:             taskInput,
			ApplyUnique:       true, // Ensure each execution is enqueued only once within 24h
		}

		if _, err := qc.EnqueueExecution(payload); err != nil {
			return fmt.Errorf("enqueue id %s: %w", sourceExecID, err)
		}
	}
	return nil
}

// remarshal round-trips through JSON so we can cleanly convert the
// interface{} that the state machine passes to our typed structs.
func remarshal(src, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// parseMicroBatchIndex extracts the integer index from a micro-batch ID.
// Format: "<batchID>:<index>"
func parseMicroBatchIndex(batchID, mbID string) int {
	var idx int
	_, _ = fmt.Sscanf(mbID, batchID+":%d", &idx)
	return idx
}

// mergeCompletionPayload merges the completion payload into the existing output.
func mergeCompletionPayload(existing interface{}, completion map[string]interface{}) interface{} {
	base, ok := existing.(map[string]interface{})
	if !ok {
		base = make(map[string]interface{})
	}
	for k, v := range completion {
		base[k] = v
	}
	return base
}

// WorkerExtractMeta attempts to read MicroBatchMeta from a task input map.
// Returns (meta, true) if found; (zero, false) otherwise.
func WorkerExtractMeta(taskInput interface{}) (MicroBatchMeta, bool) {
	inputMap, ok := taskInput.(map[string]interface{})
	if !ok {
		return MicroBatchMeta{}, false
	}
	raw, ok := inputMap[MicroBatchInputKey]
	if !ok {
		return MicroBatchMeta{}, false
	}
	rawStr, ok := raw.(string)
	if !ok {
		return MicroBatchMeta{}, false
	}
	var meta MicroBatchMeta
	if err := json.Unmarshal([]byte(rawStr), &meta); err != nil {
		return MicroBatchMeta{}, false
	}
	return meta, true
}

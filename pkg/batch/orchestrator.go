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
	ResourceDispatch    = "batch:dispatch"
	ResourceEvaluate    = "batch:evaluate"
	ResourceCheckResume = "batch:check-resume"
)

// OrchestratorStateMachineID is the ID under which the orchestrator state
// machine definition is stored in the repository.
const OrchestratorStateMachineID = "micro-batch-orchestrator-v1"

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

	// ── Read cursor ───────────────────────────────────────────────────────────
	cursorVal, err := o.rdb.Get(ctx, keyCursor(input.BatchID)).Int()
	if err == redis.Nil {
		cursorVal = 0
	} else if err != nil {
		return nil, fmt.Errorf("batch:dispatch: cursor %s: %w", input.BatchID, err)
	}

	if cursorVal >= input.TotalCount {
		return map[string]interface{}{
			"isBatchComplete": true,
		}, nil
	}

	// ── Slice next micro-batch IDs from Redis list ────────────────────────────
	end := int64(cursorVal + mbSize - 1)
	if int(end) >= input.TotalCount {
		end = int64(input.TotalCount - 1)
	}
	ids, err := o.rdb.LRange(ctx, keyIDsList(input.BatchID), int64(cursorVal), end).Result()
	if err != nil {
		return nil, fmt.Errorf("batch:dispatch: lrange: %w", err)
	}

	mbIndex := cursorVal / mbSize
	mbID := microBatchID(input.BatchID, mbIndex)
	actualSize := len(ids)

	// ── Initialise Redis barrier ──────────────────────────────────────────────
	if err := o.barrier.Init(ctx, mbID, actualSize); err != nil {
		return nil, fmt.Errorf("batch:dispatch: init barrier: %w", err)
	}

	// ── Enqueue each ID to the queue ──────────────────────────────────────────
	if err := o.enqueueIDs(ctx, input, ids, mbID, mbIndex); err != nil {
		return nil, fmt.Errorf("batch:dispatch: enqueue: %w", err)
	}

	// ── Advance cursor ────────────────────────────────────────────────────────
	newCursor := cursorVal + actualSize
	if err := o.rdb.Set(ctx, keyCursor(input.BatchID), newCursor, MetricsTTLSeconds).Err(); err != nil {
		return nil, fmt.Errorf("batch:dispatch: advance cursor: %w", err)
	}

	return map[string]interface{}{
		"batchID":        input.BatchID,
		"orchestratorID": input.OrchestratorSMID,
		"dispatchResult": map[string]interface{}{
			"isBatchComplete": false,
			"microBatchId":    mbID,
			"microBatchIndex": mbIndex,
			"size":            actualSize,
			"dispatchedAt":    time.Now().UTC(),
		},
	}, nil
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

	return o.resumeOrchestrator(ctx, mbMeta, outcome)
}

// resumeOrchestrator finds the waiting orchestrator execution and resumes it.
func (o *Orchestrator) resumeOrchestrator(ctx context.Context, mbMeta MicroBatchMeta, outcome TaskOutcome) error {
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
		completionPayload := map[string]interface{}{
			"completedMicroBatchId": mbMeta.MicroBatchID,
			"completedAt":           time.Now().UTC(),
			"isComplete":            true,
			// Message state marker - tells WaitForMicroBatchCompletion to advance
			// Key format: __received_message___<StateName>
			"__received_message___WaitForMicroBatchCompletion": map[string]interface{}{
				"correlation_key":   MicroBatchCorrelationKey,
				"correlation_value": mbMeta.MicroBatchID,
				"received_at":       time.Now().Unix(),
			},
		}
		execCtx := &execution.Execution{
			ID:             rec.ExecutionID,
			StateMachineID: rec.StateMachineID,
			Name:           rec.Name,
			Status:         rec.Status,
			CurrentState:   rec.CurrentState,
			Input:          rec.Input,                                             // Keep original input
			Output:         mergeCompletionPayload(rec.Output, completionPayload), // Merge completion into output
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
		o.RegisterHandlers(exec)
		factory.SetExecutor(exec)
		if qc := o.parentSM.GetQueueClient(); qc != nil {
			factory.SetQueueClient(qc)
		}

		// Set up context with executor
		ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

		_, err = factory.ResumeExecution(ctx, execCtx)
		if err != nil {
			return fmt.Errorf("signal: resume orchestrator %s: %w", rec.ExecutionID, err)
		}
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
func (o *Orchestrator) EnsureDefinition(ctx context.Context, definitionJSON []byte) error {
	sm, err := o.smCreator(definitionJSON, true, OrchestratorStateMachineID, o.parentSM.GetRepositoryManager())
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
	pipe.Expire(ctx, key, IDsListTTLSeconds)
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
	fmt.Sscanf(mbID, batchID+":%d", &idx)
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

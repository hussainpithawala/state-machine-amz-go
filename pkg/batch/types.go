// Package batch implements an adaptive feedback-controlled micro-batch streaming
// framework that integrates natively with state-machine-amz-go.
//
// Architecture overview
// ─────────────────────
// executeBatchConcurrent (persistent.go)
//
//	└── executeMicroBatch            ← new entry point in persistent_microbatch.go
//	     └── Orchestrator.Run        ← creates + drives the orchestrator StateMachine
//	          ├── batch:dispatch     ← local Task: slice IDs, init barrier, enqueue
//	          ├── WaitForMicroBatchCompletion  ← Message state (pause/resume)
//	          ├── batch:evaluate     ← local Task: compute failure rates
//	          ├── batch:check-resume ← local Task: poll Redis resume flag
//	          └── (loop or halt)
//
// The barrier (Redis counter) is decremented by every queue worker that finishes
// a micro-batch task.  The last worker calls ResumeExecution on the orchestrator
// via the framework's MessageCorrelation mechanism.
package batch

import (
	"context"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

// StateMachine is the interface required by the batch orchestrator.
// It avoids a direct dependency on persistent.StateMachine, breaking the import cycle.
type StateMachine interface {
	GetRepositoryManager() *repository.Manager
	GetQueueClient() *queue.Client
	GetStartAt() string
	SetExecutor(exec *executor.BaseExecutor)
	SetQueueClient(client *queue.Client)
	Execute(ctx context.Context, input interface{}, opts ...statemachine.ExecutionOption) (*execution.Execution, error)
	GetExecution(ctx context.Context, executionID string) (*repository.ExecutionRecord, error)
	ResumeExecution(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error)
	SaveDefinition(ctx context.Context) error
	FindWaitingExecutionsByCorrelation(ctx context.Context, correlationKey string, correlationValue interface{}) ([]*repository.ExecutionRecord, error)
}

// StateMachineFactory is a function that creates a StateMachine from a state machine ID (load from repository)
type StateMachineFactory func(ctx context.Context, stateMachineID string, manager *repository.Manager) (StateMachine, error)

// StateMachineCreator is a function that creates a StateMachine from a definition (parse from JSON/YAML)
type StateMachineCreator func(definition []byte, isJSON bool, stateMachineID string, manager *repository.Manager) (StateMachine, error)

// ─── State-machine I/O contracts ─────────────────────────────────────────────
// These structs are serialised as JSON and flow through the orchestrator
// state machine's input document.  Each Task state writes its output into a
// sub-key via ResultPath (e.g. "$.dispatchResult").

// OrchestratorInput is the root input document for the orchestrator execution.
// IDs are NOT carried inline; they live in Redis (keyIDsList) to avoid
// bloating the state machine's persisted JSON for multi-million-ID batches.
type OrchestratorInput struct {
	// BatchID is the stable identifier linking all Redis keys, the orchestrator
	// execution, and individual task payloads together.
	BatchID string `json:"batch_id"`

	// TotalCount is the number of source execution IDs to process.
	TotalCount int `json:"total_count"`

	// MicroBatchSize is the desired size of each micro-batch (e.g. 1 000).
	MicroBatchSize int `json:"micro_batch_size"`

	// SourceStateName is passed through to each individual task execution so
	// it can derive its input from the correct state of the source execution.
	SourceStateName string `json:"source_state_name"`

	// OrchestratorSMID is the state machine ID that was used to create the
	// orchestrator execution.  Workers use this together with BatchID to locate
	// the waiting orchestrator and resume it.
	OrchestratorSMID string `json:"orchestrator_sm_id"`

	// FailurePolicy controls both statistical-control thresholds.
	FailurePolicy FailurePolicy `json:"failure_policy"`

	// ─── Fields populated by individual states (ResultPath) ─────────────────
	DispatchResult *DispatchResult    `json:"dispatchResult,omitempty"`
	Evaluation     *EvaluationResult  `json:"evaluation,omitempty"`
	ResumeCheck    *ResumeCheckResult `json:"resumeCheck,omitempty"`
}

// FailurePolicy encodes both statistical control parameters.
type FailurePolicy struct {
	// WindowN is the rolling window of individual task outcomes for the
	// instantaneous failure rate.  Default: 500.
	WindowN int `json:"window_n"`

	// WindowM is the number of consecutive micro-batches used for the rolling
	// average failure rate.  Default: 5.
	WindowM int `json:"window_m"`

	// SevereFailureThreshold (0–1) triggers an immediate BatchFailed halt.
	// Default: 0.50 (halt if ≥50 % of last N tasks failed).
	SevereFailureThreshold float64 `json:"severe_failure_threshold"`

	// SoftFailureThreshold (0–1) triggers a PauseBatch for human review.
	// Default: 0.20 (pause if rolling avg across M micro-batches ≥ 20 %).
	SoftFailureThreshold float64 `json:"soft_failure_threshold"`
}

// DispatchResult is the output of the DispatchMicroBatch Task state.
// It is written to $.dispatchResult by the batch:dispatch handler.
type DispatchResult struct {
	IsBatchComplete bool      `json:"isBatchComplete"`
	MicroBatchID    string    `json:"microBatchId,omitempty"`
	MicroBatchIndex int       `json:"microBatchIndex,omitempty"`
	Size            int       `json:"size,omitempty"`
	DispatchedAt    time.Time `json:"dispatchedAt,omitempty"`
}

// EvaluationResult is the output of the EvaluateMicroBatch Task state.
// It is written to $.evaluation by the batch:evaluate handler.
type EvaluationResult struct {
	SevereFailure             bool    `json:"severeFailure"`
	BelowThreshold            bool    `json:"belowThreshold"`
	InstantaneousFailureRate  float64 `json:"instantaneousFailureRate"`
	RollingAverageFailureRate float64 `json:"rollingAverageFailureRate"`
	TotalProcessed            int64   `json:"totalProcessed"`
	TotalFailed               int64   `json:"totalFailed"`
}

// ResumeCheckResult is the output of the CheckResumeSignal Task state.
// It is written to $.resumeCheck by the batch:check-resume handler.
type ResumeCheckResult struct {
	ShouldResume bool       `json:"shouldResume"`
	ResumedAt    *time.Time `json:"resumedAt,omitempty"`
	ResumedBy    string     `json:"resumedBy,omitempty"`
}

// ─── Queue/worker domain types ────────────────────────────────────────────────

// MicroBatchMeta is embedded in queue.ExecutionTaskPayload.Input (as a nested
// map key "micro_batch_meta") when a task belongs to a micro-batch streaming
// run.  Workers inspect this field after completing execution to drive the
// Redis barrier.
//
// Embedding in Input (rather than adding a new field to ExecutionTaskPayload)
// means NO change to the queue package struct is required.
type MicroBatchMeta struct {
	BatchID          string `json:"batch_id"`
	MicroBatchID     string `json:"micro_batch_id"`
	OrchestratorSMID string `json:"orchestrator_sm_id"`
}

// MicroBatchInputKey is the key under which MicroBatchMeta is stored inside
// the task payload's Input map.
const MicroBatchInputKey = "__micro_batch_meta__"

// ─── Internal domain types ────────────────────────────────────────────────────

// TaskOutcome is recorded by every worker after processing one source execution.
type TaskOutcome struct {
	BatchID      string
	MicroBatchID string
	TaskID       string
	Success      bool
	Error        string
	Duration     time.Duration
	CompletedAt  time.Time
}

// BarrierResult is returned by the Lua decrement script.
type BarrierResult struct {
	Remaining    int64
	IsLastWorker bool
}

// ResumeSignal is the value stored in Redis when an operator unblocks a paused batch.
type ResumeSignal struct {
	ResumedAt time.Time `json:"resumed_at"`
	ResumedBy string    `json:"resumed_by"`
	Notes     string    `json:"notes,omitempty"`
}

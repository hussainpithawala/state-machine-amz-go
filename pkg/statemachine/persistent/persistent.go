// pkg/statemachine/persistent.go (Modified)
package persistent

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

const FAILED = "FAILED"
const PAUSED = "PAUSED"

// StateMachine Persistence.StateMachine represents a state machine with an optional repositoryManager
type StateMachine struct {
	statemachine      *statemachine2.StateMachine
	repositoryManager *repository.Manager
	stateMachineID    string
}

// Option allows configuring the state machine
type Option func(*StateMachine)

// New creates a new state machine with options
func New(definition []byte, isJson bool, stateMachineId string, manager *repository.Manager) (*StateMachine, error) {
	sm, err := statemachine2.New(definition, isJson)
	if err != nil {
		return nil, fmt.Errorf("failed to create state machine: %w", err)
	}

	var smId string

	if stateMachineId == "" {
		smId = fmt.Sprintf("sm-%d", time.Now().Unix())
	} else {
		smId = stateMachineId
	}

	return &StateMachine{
		statemachine:      sm,
		stateMachineID:    smId,
		repositoryManager: manager,
	}, nil
}

// NewFromDefnId creates a new state machine by fetching its definition from the repository
func NewFromDefnId(ctx context.Context, stateMachineID string, manager *repository.Manager) (*StateMachine, error) {
	record, err := manager.GetStateMachine(ctx, stateMachineID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch state machine definition: %w", err)
	}

	return New([]byte(record.Definition), true, record.ID, manager)
}

// Execute runs an execution of the state machine with repositoryManager
func (pm *StateMachine) Execute(ctx context.Context, input interface{}, opts ...statemachine2.ExecutionOption) (*execution.Execution, error) {
	var execCtx *execution.Execution

	// Process execution options
	config := &statemachine2.ExecutionConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Handle chained executions - derive input from source execution if specified
	if config.SourceExecutionID != "" {
		derivedInput, err := pm.deriveInputFromExecution(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("failed to derive input from source execution: %w", err)
		}
		input = derivedInput
	}

	// If input is already an Execution context, use it
	if existingExec, ok := input.(*execution.Execution); ok {
		execCtx = existingExec
		// If ID is not set, use stateMachineID
		if execCtx.ID == "" {
			execCtx.ID = pm.stateMachineID
		}
		// If StartAt is not set, use state machine's StartAt
		if execCtx.CurrentState == "" {
			execCtx.CurrentState = pm.statemachine.StartAt
		}
		input = execCtx.Input
	} else {
		// Create execution context
		execName := fmt.Sprintf("execution-%d", time.Now().Unix())
		if config.Name != "" {
			execName = config.Name
		}

		execCtx = execution.NewContext(execName, pm.statemachine.StartAt, input)
		execCtx.ID = pm.stateMachineID
	}

	// Save initial execution state if repositoryManager is enabled
	pm.persistExecution(ctx, execCtx)

	return pm.RunExecution(ctx, input, execCtx)
}

// deriveInputFromExecution retrieves output from a source execution and applies optional transformation
func (pm *StateMachine) deriveInputFromExecution(ctx context.Context, config *statemachine2.ExecutionConfig) (interface{}, error) {
	// Get output from the source execution
	output, err := pm.repositoryManager.GetExecutionOutput(ctx, config.SourceExecutionID, config.SourceStateName)
	if err != nil {
		return nil, err
	}

	// Apply transformation if provided
	if config.InputTransformer != nil {
		return config.InputTransformer(output)
	}

	return output, nil
}

// RunExecution executes the state machine with repositoryManager hooks
func (pm *StateMachine) RunExecution(ctx context.Context, input interface{}, execCtx *execution.Execution) (*execution.Execution, error) {
	if input != nil {
		execCtx.Input = input
	}
	currentStateName := execCtx.CurrentState

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			execCtx.Status = "CANCELLED"
			execCtx.Error = ctx.Err()
			execCtx.EndTime = time.Now()
			pm.persistExecution(ctx, execCtx)
			return execCtx, ctx.Err()
		default:
		}

		// Get current state
		state, exists := pm.statemachine.States[currentStateName]
		if !exists {
			err := fmt.Errorf("state not found: %s", currentStateName)
			execCtx.Status = FAILED
			execCtx.Error = err
			execCtx.EndTime = time.Now()
			pm.persistExecution(ctx, execCtx)
			return execCtx, err
		}

		// Execute state and record history
		history := &execution.StateHistory{
			StateName:      currentStateName,
			StateType:      state.GetType(),
			Input:          execCtx.Input,
			StartTime:      time.Now(),
			SequenceNumber: len(execCtx.History),
		}

		// Execute the state
		output, nextState, err := state.Execute(ctx, execCtx.Input)

		// Check if it's a message state that needs to pause
		if msgResult, ok := output.(*states.MessageStateResult); ok && msgResult.Status == "WAITING" {
			// Update history for the WAITING state
			history.EndTime = time.Now()
			history.Output = output
			history.Status = "WAITING"
			execCtx.History = append(execCtx.History, *history)
			saveHistory(ctx, execCtx, pm, history)

			// Save MessageCorrelationRecord
			correlationRecord := &repository.MessageCorrelationRecord{
				ID:                 fmt.Sprintf("corr-%s-%s", execCtx.ID, currentStateName),
				ExecutionID:        execCtx.ID,
				ExecutionStartTime: &execCtx.StartTime,
				StateMachineID:     pm.stateMachineID,
				StateName:          currentStateName,
				CorrelationKey:     msgResult.CorrelationData.CorrelationKey,
				CorrelationValue:   msgResult.CorrelationData.CorrelationValue,
				CreatedAt:          time.Now().Unix(),
				Status:             "WAITING",
			}
			if msgResult.CorrelationData.TimeoutAt != nil {
				correlationRecord.TimeoutAt = msgResult.CorrelationData.TimeoutAt
			}

			if err := pm.repositoryManager.SaveMessageCorrelation(ctx, correlationRecord); err != nil {
				fmt.Printf("Warning: failed to save message correlation: %v\n", err)
			}

			// Pause the execution
			execCtx.Status = PAUSED
			execCtx.CurrentState = currentStateName
			pm.persistExecution(ctx, execCtx)

			return execCtx, nil
		}

		// Update history
		history.EndTime = time.Now()
		history.Output = output

		if err != nil {
			history.Status = FAILED
			history.Error = err
			execCtx.Status = FAILED
			execCtx.Error = err
			execCtx.EndTime = time.Now()
		} else {
			history.Status = "SUCCEEDED"
			execCtx.Input = output // Next state's input
		}

		// Append to history
		execCtx.History = append(execCtx.History, *history)

		// Persist state history immediately after execution
		saveHistory(ctx, execCtx, pm, history)

		// Update execution record
		execCtx.CurrentState = currentStateName
		pm.persistExecution(ctx, execCtx)

		if err != nil {
			// Handle err by returning and closing the execution at this point only
			// We have already persisted the state of the execution and hence we can
			// return back
			return execCtx, err
		}

		// Check if this is a terminal state
		if state.IsEnd() {
			execCtx.Status = "SUCCEEDED"
			execCtx.Output = output
			execCtx.EndTime = time.Now()
			pm.persistExecution(ctx, execCtx)
			return execCtx, nil
		}

		// Move to next state
		if *nextState == "" {
			err := fmt.Errorf("non-terminal state %s did not provide next state", currentStateName)
			execCtx.Status = FAILED
			execCtx.Error = err
			execCtx.EndTime = time.Now()
			pm.persistExecution(ctx, execCtx)
			return execCtx, err
		}

		currentStateName = *nextState
		execCtx.CurrentState = currentStateName
	}
}

func saveHistory(ctx context.Context, execCtx *execution.Execution, sm *StateMachine, history *execution.StateHistory) {
	if persistErr := sm.repositoryManager.SaveStateHistory(ctx, execCtx, history); persistErr != nil {
		// Log error but don't fail the execution
		fmt.Printf("Warning: failed to persist state history: %v\n", persistErr)
	}
}

// persistExecution is a helper to persist execution state
func (pm *StateMachine) persistExecution(ctx context.Context, execCtx *execution.Execution) {
	if err := pm.repositoryManager.SaveExecution(ctx, execCtx); err != nil {
		fmt.Printf("Warning: failed to persist final execution state: %v\n", err)
	}
}

// GetExecutionHistory retrieves execution history from repositoryManager
func (pm *StateMachine) GetExecutionHistory(ctx context.Context, executionID string) ([]*repository.StateHistoryRecord, error) {
	return pm.repositoryManager.GetStateHistory(ctx, executionID)
}

// GetExecution retrieves an execution from repositoryManager
func (pm *StateMachine) GetExecution(ctx context.Context, executionID string) (*repository.ExecutionRecord, error) {
	return pm.repositoryManager.GetExecution(ctx, executionID)
}

// GetDefinition retrieves a state machine definition from repositoryManager
func (pm *StateMachine) GetDefinition(ctx context.Context, stateMachineID string) (*repository.StateMachineRecord, error) {
	return pm.repositoryManager.GetStateMachine(ctx, stateMachineID)
}

// ListExecutions lists executions from repositoryManager
func (pm *StateMachine) ListExecutions(ctx context.Context, filter *repository.ExecutionFilter) ([]*repository.ExecutionRecord, error) {
	if filter != nil {
		filter.StateMachineID = pm.stateMachineID
	}

	return pm.repositoryManager.ListExecutions(ctx, filter)
}

func (pm *StateMachine) CountExecutions(ctx context.Context, filter *repository.ExecutionFilter) (int64, error) {
	if filter != nil {
		filter.StateMachineID = pm.stateMachineID
	}
	return pm.repositoryManager.CountExecutions(ctx, filter)
}

// SaveDefinition persists the state machine definition to the repository
func (pm *StateMachine) FindWaitingExecutionsByCorrelation(ctx context.Context, correlationKey string, correlationValue interface{}) ([]*repository.ExecutionRecord, error) {
	return pm.repositoryManager.FindWaitingExecutionsByCorrelation(ctx, correlationKey, correlationValue)
}

func (pm *StateMachine) ResumeExecution(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error) {
	// Ensure execution is in a valid state for resumption
	if execCtx.Status != PAUSED && execCtx.Status != "RUNNING" {
		return nil, fmt.Errorf("cannot resume execution with status: %s", execCtx.Status)
	}

	// Update correlation status to RECEIVED
	correlationID := fmt.Sprintf("corr-%s-%s", execCtx.ID, execCtx.CurrentState)
	if err := pm.repositoryManager.UpdateCorrelationStatus(ctx, correlationID, "RECEIVED"); err != nil {
		fmt.Printf("Warning: failed to update correlation status: %v\n", err)
	}

	// Update status to RUNNING if it was PAUSED
	if execCtx.Status == PAUSED {
		execCtx.Status = "RUNNING"
		pm.persistExecution(ctx, execCtx)
	}

	// Continue execution from the current state
	return pm.RunExecution(ctx, execCtx.Input, execCtx)
}

func (pm *StateMachine) GetRepositoryManager() *repository.Manager {
	return pm.repositoryManager
}

func (pm *StateMachine) GetID() string {
	return pm.stateMachineID
}

func (pm *StateMachine) GetStartAt() string {
	return pm.statemachine.GetStartAt()
}

func (pm *StateMachine) GetState(name string) (states.State, error) {
	return pm.statemachine.GetState(name)
}

func (pm *StateMachine) IsTimeout(startTime time.Time) bool {
	return pm.statemachine.IsTimeout(startTime)
}

func (pm *StateMachine) SaveDefinition(ctx context.Context) error {
	record, err := pm.statemachine.ToRecord()
	if err != nil {
		return err
	}
	// Use the explicit stateMachineID if it was provided to New
	record.ID = pm.stateMachineID
	return pm.repositoryManager.SaveStateMachine(ctx, record)
}

// pkg/statemachine/persistent.go (Modified)
package persistent

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

const FAILED = "FAILED"

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

// Execute runs an execution of the state machine with repositoryManager
func (pm *StateMachine) Execute(ctx context.Context, input interface{}, opts ...statemachine2.ExecutionOption) (*execution.Execution, error) {
	// Create execution context
	execName := fmt.Sprintf("execution-%d", time.Now().Unix())
	if len(opts) > 0 {
		config := &statemachine2.ExecutionConfig{}
		for _, opt := range opts {
			opt(config)
		}
		if config.Name != "" {
			execName = config.Name
		}
	}

	execCtx := execution.NewContext(execName, pm.statemachine.StartAt, input)

	// Save initial execution state if repositoryManager is enabled
	execCtx.ID = pm.stateMachineID
	pm.persistExecution(ctx, execCtx)

	return pm.RunExecution(ctx, execCtx)
}

// RunExecution executes the state machine with repositoryManager hooks
func (pm *StateMachine) RunExecution(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error) {
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

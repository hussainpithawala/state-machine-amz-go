// pkg/statemachine/statemachine.go (Modified)
package impl

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

// StateMachine represents a state machine with an optional repository
type StateMachine struct {
	StartAt          string
	States           map[string]states.State
	repository       *repository.Manager
	enableRepository bool
	stateMachineID   string
}

// StateMachineOption allows configuring the state machine
type StateMachineOption func(*StateMachine)

// WithRepository enables repository for the state machine
func WithRepository(rm *repository.Manager) StateMachineOption {
	return func(sm *StateMachine) {
		sm.repository = rm
		sm.enableRepository = true
	}
}

// WithStateMachineID sets the state machine ID for tracking
func WithStateMachineID(id string) StateMachineOption {
	return func(sm *StateMachine) {
		sm.stateMachineID = id
	}
}

// NewStateMachine creates a new state machine with options
func NewStateMachine(startAt string, states map[string]states.State, opts ...StateMachineOption) *StateMachine {
	sm := &StateMachine{
		StartAt:          startAt,
		States:           states,
		enableRepository: false,
		stateMachineID:   fmt.Sprintf("sm-%d", time.Now().Unix()),
	}

	for _, opt := range opts {
		opt(sm)
	}

	return sm
}

// Execute runs an execution of the state machine with repository
func (sm *StateMachine) Execute(ctx context.Context, input interface{}, opts ...ExecutionOption) (*execution.Execution, error) {
	// Create execution context
	execName := fmt.Sprintf("execution-%d", time.Now().Unix())
	if len(opts) > 0 {
		config := &executionConfig{}
		for _, opt := range opts {
			opt(config)
		}
		if config.Name != "" {
			execName = config.Name
		}
	}

	execCtx := execution.NewContext(execName, sm.StartAt, input)

	// Save initial execution state if repository is enabled
	if sm.enableRepository && sm.repository != nil {
		execCtx.ID = sm.stateMachineID
		if err := sm.repository.SaveExecution(ctx, execCtx); err != nil {
			return nil, fmt.Errorf("failed to save initial execution: %w", err)
		}
	}

	return sm.RunExecution(ctx, execCtx)
}

// RunExecution executes the state machine with repository hooks
func (sm *StateMachine) RunExecution(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error) {
	currentStateName := execCtx.CurrentState

	for {
		// Check context cancellation
		select {
		case <-ctx.Done():
			execCtx.Status = "CANCELLED"
			execCtx.Error = ctx.Err()
			execCtx.EndTime = time.Now()
			sm.persistExecution(ctx, execCtx)
			return execCtx, ctx.Err()
		default:
		}

		// Get current state
		state, exists := sm.States[currentStateName]
		if !exists {
			err := fmt.Errorf("state not found: %s", currentStateName)
			execCtx.Status = "FAILED"
			execCtx.Error = err
			execCtx.EndTime = time.Now()
			sm.persistExecution(ctx, execCtx)
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
			history.Status = "FAILED"
			history.Error = err
			execCtx.Status = "FAILED"
			execCtx.Error = err
			execCtx.EndTime = time.Now()
		} else {
			history.Status = "SUCCEEDED"
			execCtx.Input = output // Next state's input
		}

		// Append to history
		execCtx.History = append(execCtx.History, *history)

		// Persist state history immediately after execution
		if sm.enableRepository && sm.repository != nil {
			if persistErr := sm.repository.SaveStateHistory(ctx, execCtx, history); persistErr != nil {
				// Log error but don't fail the execution
				fmt.Printf("Warning: failed to persist state history: %v\n", persistErr)
			}

			// Update execution record
			execCtx.CurrentState = currentStateName
			if persistErr := sm.repository.SaveExecution(ctx, execCtx); persistErr != nil {
				fmt.Printf("Warning: failed to persist execution: %v\n", persistErr)
			}
		}

		// Handle execution errors
		if err != nil {
			sm.persistExecution(ctx, execCtx)
			return execCtx, err
		}

		// Check if this is a terminal state
		if state.IsEnd() {
			execCtx.Status = "SUCCEEDED"
			execCtx.Output = output
			execCtx.EndTime = time.Now()
			sm.persistExecution(ctx, execCtx)
			return execCtx, nil
		}

		// Move to next state
		if *nextState == "" {
			err := fmt.Errorf("non-terminal state %s did not provide next state", currentStateName)
			execCtx.Status = "FAILED"
			execCtx.Error = err
			execCtx.EndTime = time.Now()
			sm.persistExecution(ctx, execCtx)
			return execCtx, err
		}

		currentStateName = *nextState
		execCtx.CurrentState = currentStateName
	}
}

// persistExecution is a helper to persist execution state
func (sm *StateMachine) persistExecution(ctx context.Context, execCtx *execution.Execution) {
	if sm.enableRepository && sm.repository != nil {
		if err := sm.repository.SaveExecution(ctx, execCtx); err != nil {
			fmt.Printf("Warning: failed to persist final execution state: %v\n", err)
		}
	}
}

// GetExecutionHistory retrieves execution history from repository
func (sm *StateMachine) GetExecutionHistory(ctx context.Context, executionID string) ([]*repository.StateHistoryRecord, error) {
	if !sm.enableRepository || sm.repository == nil {
		return nil, fmt.Errorf("repository is not enabled")
	}

	return sm.repository.GetStateHistory(ctx, executionID)
}

// GetExecution retrieves an execution from repository
func (sm *StateMachine) GetExecution(ctx context.Context, executionID string) (*repository.ExecutionRecord, error) {
	if !sm.enableRepository || sm.repository == nil {
		return nil, fmt.Errorf("repository is not enabled")
	}

	return sm.repository.GetExecution(ctx, executionID)
}

// ListExecutions lists executions from repository
func (sm *StateMachine) ListExecutions(ctx context.Context, filters map[string]interface{}, limit, offset int) ([]*repository.ExecutionRecord, error) {
	if !sm.enableRepository || sm.repository == nil {
		return nil, fmt.Errorf("repository is not enabled")
	}

	if filters == nil {
		filters = make(map[string]interface{})
	}
	filters["state_machine_id"] = sm.stateMachineID

	return sm.repository.ListExecutions(ctx, filters, limit, offset)
}

// ExecutionOption configures execution options
type ExecutionOption func(*executionConfig)

type executionConfig struct {
	Name string
}

// WithExecutionName sets the execution name
func WithExecutionName(name string) ExecutionOption {
	return func(c *executionConfig) {
		c.Name = name
	}
}

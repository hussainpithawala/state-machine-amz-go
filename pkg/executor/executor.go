package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/execution"
	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// State defines the interface for state access
type State interface {
	Execute(ctx context.Context, input interface{}) (interface{}, *string, error)
	GetName() string
	GetType() string
	GetNext() *string
	IsEnd() bool
}

// StateMachineInterface defines the minimal interface needed by executor
type StateMachineInterface interface {
	GetStartAt() string
	GetState(name string) (State, error)
	IsTimeout(startTime time.Time) bool
}

// Executor defines the interface for executing state machines
type Executor interface {
	// Execute executes a state machine with the given context and input
	Execute(ctx context.Context, sm StateMachineInterface, execCtx *execution.Execution) (*execution.Execution, error)

	// GetStatus returns the status of an execution
	GetStatus(executionID string) (*execution.Execution, error)

	// Stop stops an execution
	Stop(ctx context.Context, execCtx *execution.Execution) error

	// ListExecutions returns all executions
	ListExecutions() []*execution.Execution
}

// BaseExecutor provides common executor functionality
type BaseExecutor struct {
	executions map[string]*execution.Execution
	registry   *StateRegistry
}

// StateRegistry registers and manages state handlers
type StateRegistry struct {
	taskHandlers map[string]func(context.Context, interface{}) (interface{}, error)
}

// NewStateRegistry creates a new state registry
func NewStateRegistry() *StateRegistry {
	return &StateRegistry{
		taskHandlers: make(map[string]func(context.Context, interface{}) (interface{}, error)),
	}
}

// RegisterTaskHandler registers a handler for a task state
func (r *StateRegistry) RegisterTaskHandler(resourceURI string, handler func(context.Context, interface{}) (interface{}, error)) {
	r.taskHandlers[resourceURI] = handler
}

// GetTaskHandler retrieves a task handler
func (r *StateRegistry) GetTaskHandler(resourceURI string) (func(context.Context, interface{}) (interface{}, error), bool) {
	handler, exists := r.taskHandlers[resourceURI]
	return handler, exists
}

// NewBaseExecutor creates a new BaseExecutor
func NewBaseExecutor() *BaseExecutor {
	return &BaseExecutor{
		executions: make(map[string]*execution.Execution),
		registry:   NewStateRegistry(),
	}
}

// Execute executes a state machine
func (e *BaseExecutor) Execute(ctx context.Context, sm StateMachineInterface, execCtx *execution.Execution) (*execution.Execution, error) {
	// Validate inputs
	if sm == nil {
		return nil, fmt.Errorf("state machine cannot be nil")
	}

	if execCtx == nil {
		return nil, fmt.Errorf("execution context cannot be nil")
	}

	// Store execution
	e.executions[execCtx.ID] = execCtx

	// Set initial status
	execCtx.Status = "RUNNING"

	// Execute state machine
	currentStateName := sm.GetStartAt()
	currentInput := execCtx.Input

	for {
		// Check for timeout
		if sm.IsTimeout(execCtx.StartTime) {
			execCtx.Status = "TIMED_OUT"
			execCtx.EndTime = time.Now()
			return execCtx, fmt.Errorf("state machine execution timed out")
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			execCtx.Status = "ABORTED"
			execCtx.EndTime = time.Now()
			return execCtx, ctx.Err()
		default:
		}

		// Get current state
		state, err := sm.GetState(currentStateName)
		if err != nil {
			execCtx.Status = "FAILED"
			execCtx.EndTime = time.Now()
			execCtx.Error = err
			return execCtx, err
		}

		// Update execution context
		execCtx.CurrentState = currentStateName

		// Execute the state
		output, nextState, err := state.Execute(ctx, currentInput)

		// Record state history
		execCtx.AddStateHistory(currentStateName, currentInput, output)

		// Handle state execution result
		if err != nil {
			execCtx.Status = "FAILED"
			execCtx.EndTime = time.Now()
			execCtx.Error = err
			execCtx.Output = output
			return execCtx, err
		}

		// Check if this is an end state
		if state.IsEnd() || nextState == nil {
			// Execution completed successfully
			execCtx.Status = "SUCCEEDED"
			execCtx.EndTime = time.Now()
			execCtx.Output = output
			break
		}

		// Move to next state
		currentStateName = *nextState
		currentInput = output
	}

	return execCtx, nil
}

// GetStatus returns the status of an execution
func (e *BaseExecutor) GetStatus(executionID string) (*execution.Execution, error) {
	execCtx, exists := e.executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution '%s' not found", executionID)
	}

	return execCtx, nil
}

// Stop stops an execution
func (e *BaseExecutor) Stop(_ context.Context, execCtx *execution.Execution) error {
	if execCtx == nil {
		return fmt.Errorf("execution context cannot be nil")
	}

	execCtx.Status = "ABORTED"
	execCtx.EndTime = time.Now()

	// Remove from active executions
	delete(e.executions, execCtx.ID)

	return nil
}

// ListExecutions returns all executions
func (e *BaseExecutor) ListExecutions() []*execution.Execution {
	executions := make([]*execution.Execution, 0, len(e.executions))
	for _, exec := range e.executions {
		executions = append(executions, exec)
	}
	return executions
}

// RegisterGoFunction registers a Go function as a task handler
func (e *BaseExecutor) RegisterGoFunction(name string, fn func(context.Context, interface{}) (interface{}, error)) {
	resourceURI := fmt.Sprintf("arn:aws:states:::lambda:function:%s", name)
	e.registry.RegisterTaskHandler(resourceURI, fn)
}

// ExecuteGoTask executes a Go task function
func (e *BaseExecutor) ExecuteGoTask(_ context.Context, taskState states.State, input interface{}) (interface{}, error) {
	// This would be implemented when we add TaskState
	// For now, return a placeholder
	return input, nil
}

package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/execution"
)

// StateMachineInterface defines the minimal interface needed by executor
type StateMachineInterface interface {
	GetStartAt() string
	GetState(name string) (interface{}, error)
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
}

// BaseExecutor provides common executor functionality
type BaseExecutor struct {
	executions map[string]*execution.Execution
}

// NewBaseExecutor creates a new BaseExecutor
func NewBaseExecutor() *BaseExecutor {
	return &BaseExecutor{
		executions: make(map[string]*execution.Execution),
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

	// For now, return a simple implementation
	execCtx.Status = "SUCCEEDED"
	execCtx.EndTime = time.Now()
	execCtx.Output = execCtx.Input

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
func (e *BaseExecutor) Stop(ctx context.Context, execCtx *execution.Execution) error {
	if execCtx == nil {
		return fmt.Errorf("execution context cannot be nil")
	}

	execCtx.Status = "ABORTED"
	execCtx.EndTime = time.Now()

	// Remove from active executions
	delete(e.executions, execCtx.ID)

	return nil
}

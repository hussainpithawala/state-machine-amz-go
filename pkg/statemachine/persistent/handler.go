package persistent

import (
	"context"
	"fmt"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

// ExecutionHandler implements queue.ExecutionHandler interface
// It handles execution tasks from the queue by loading the state machine and executing it
type ExecutionHandler struct {
	repositoryManager *repository.Manager
}

// NewExecutionHandler creates a new execution handler
func NewExecutionHandler(repositoryManager *repository.Manager) *ExecutionHandler {
	return &ExecutionHandler{
		repositoryManager: repositoryManager,
	}
}

// HandleExecution processes a state machine execution task
func (h *ExecutionHandler) HandleExecution(ctx context.Context, payload *queue.ExecutionTaskPayload) error {
	// Load the state machine definition
	sm, err := NewFromDefnId(ctx, payload.StateMachineID, h.repositoryManager)
	if err != nil {
		return fmt.Errorf("failed to load state machine: %w", err)
	}

	// Build execution options
	execOpts := []statemachine2.ExecutionOption{
		statemachine2.WithExecutionName(payload.ExecutionName),
	}

	// Add source execution if provided
	if payload.SourceExecutionID != "" {
		execOpts = append(execOpts,
			statemachine2.WithSourceExecution(payload.SourceExecutionID, payload.SourceStateName),
		)
	}

	// Execute the state machine
	exec, err := sm.Execute(ctx, payload.Input, execOpts...)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	if exec.Status == FAILED {
		return fmt.Errorf("execution completed with FAILED status: %v", exec.Error)
	}

	return nil
}

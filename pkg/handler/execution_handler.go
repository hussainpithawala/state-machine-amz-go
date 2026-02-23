// pkg/handler/execution_handler.go
package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

// ExecutionHandler implements queue.ExecutionHandler interface
// It handles all types of execution tasks from the queue including regular, chained, and timeout executions
type ExecutionHandler struct {
	repositoryManager *repository.Manager
	queueClient       *queue.Client
	executionContext  types.ExecutionContext
}

// NewExecutionHandler creates a new execution handler
func NewExecutionHandler(repositoryManager *repository.Manager, queueClient *queue.Client) *ExecutionHandler {
	return &ExecutionHandler{
		repositoryManager: repositoryManager,
		queueClient:       queueClient,
		executionContext:  nil,
	}
}

// NewExecutionHandlerWithContext creates a new execution handler with an execution context
func NewExecutionHandlerWithContext(repositoryManager *repository.Manager, queueClient *queue.Client, execCtx types.ExecutionContext) *ExecutionHandler {
	return &ExecutionHandler{
		repositoryManager: repositoryManager,
		queueClient:       queueClient,
		executionContext:  execCtx,
	}
}

// HandleExecution processes a state machine execution task from the queue
// This method handles:
// 1. Regular executions with direct input
// 2. Chained executions from source executions
// 3. Timeout-triggered executions
func (h *ExecutionHandler) HandleExecution(ctx context.Context, payload *queue.ExecutionTaskPayload) error {
	// Load the state machine definition
	sm, err := persistent.NewFromDefnId(ctx, payload.StateMachineID, h.repositoryManager)
	if err != nil {
		return fmt.Errorf("failed to load state machine: %w", err)
	}

	// Set queue client for timeout scheduling support
	sm.SetQueueClient(h.queueClient)

	// Add execution context if available
	if h.executionContext != nil {
		ctx = context.WithValue(ctx, types.ExecutionContextKey, h.executionContext)
	}

	// Route to appropriate handler based on execution type
	if payload.IsTimeout {
		return h.handleTimeoutExecution(ctx, sm, payload)
	}

	// Handle regular or chained execution
	return h.handleRegularExecution(ctx, sm, payload)
}

// handleRegularExecution handles regular or chained executions
func (h *ExecutionHandler) handleRegularExecution(ctx context.Context, sm *persistent.StateMachine, payload *queue.ExecutionTaskPayload) error {
	var input interface{}

	// Determine input source
	if payload.SourceExecutionID != "" {
		// Chained execution - derive input from source execution
		output, err := h.repositoryManager.GetExecutionOutput(ctx, payload.SourceExecutionID, payload.SourceStateName)
		if err != nil {
			return fmt.Errorf("failed to get source execution output: %w", err)
		}
		input = output
	} else if payload.Input != nil {
		// Direct input provided
		if inputBytes, ok := payload.Input.([]byte); ok {
			if err := json.Unmarshal(inputBytes, &input); err != nil {
				return fmt.Errorf("failed to unmarshal input: %w", err)
			}
		} else {
			input = payload.Input
		}
	}

	// Build execution options
	execOpts := []statemachine2.ExecutionOption{
		statemachine2.WithExecutionName(payload.ExecutionName),
	}

	// Add source execution option if this is a chained execution
	if payload.SourceExecutionID != "" {
		execOpts = append(execOpts,
			statemachine2.WithSourceExecution(payload.SourceExecutionID, payload.SourceStateName),
		)
		execOpts = append(execOpts,
			statemachine2.WithUniqueness(payload.ApplyUnique))
		execOpts = append(execOpts, statemachine2.WithInputTransformerName(payload.InputTransformerName))
	}

	// Execute the state machine
	exec, err := sm.Execute(ctx, input, execOpts...)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Check execution status
	if exec.Status == persistent.FAILED {
		return fmt.Errorf("execution completed with FAILED status: %v", exec.Error)
	}

	fmt.Printf("Execution completed: ID=%s, Status=%s\n", exec.ID, exec.Status)
	return nil
}

// handleTimeoutExecution handles timeout-triggered executions
// This occurs when a Message state times out waiting for a correlation
func (h *ExecutionHandler) handleTimeoutExecution(ctx context.Context, sm *persistent.StateMachine, payload *queue.ExecutionTaskPayload) error {
	// Validate payload
	if payload.CorrelationID == "" {
		return fmt.Errorf("correlation ID is required for timeout execution")
	}

	// Get the correlation record
	correlation, err := h.repositoryManager.GetMessageCorrelation(ctx, payload.CorrelationID)
	if err != nil {
		return fmt.Errorf("failed to get correlation record: %w", err)
	}

	// Check if correlation is still waiting
	if correlation.Status != "WAITING" {
		// Already processed (message received), skip timeout processing
		fmt.Printf("Correlation %s already processed with status %s, skipping timeout\n",
			payload.CorrelationID, correlation.Status)
		return fmt.Errorf("correlation already processed")
	}

	// Verify execution exists
	_, err = h.repositoryManager.GetExecution(ctx, payload.ExecutionID)
	if err != nil {
		return fmt.Errorf("failed to get execution: %w", err)
	}

	// Process timeout trigger
	fmt.Printf("Processing timeout trigger: ExecutionID=%s, CorrelationID=%s\n",
		payload.ExecutionID, payload.CorrelationID)

	return sm.ProcessTimeoutTrigger(ctx, payload.CorrelationID)
}

// HandleTimeout processes a dedicated timeout task
// This is called when a timeout boundary event is triggered
func (h *ExecutionHandler) HandleTimeout(ctx context.Context, payload *queue.TimeoutTaskPayload) error {
	// Load the state machine definition
	sm, err := persistent.NewFromDefnId(ctx, payload.StateMachineID, h.repositoryManager)
	if err != nil {
		return fmt.Errorf("failed to load state machine: %w", err)
	}

	// Set queue client for timeout scheduling support
	sm.SetQueueClient(h.queueClient)

	// Add execution context if available
	if h.executionContext != nil {
		ctx = context.WithValue(ctx, types.ExecutionContextKey, h.executionContext)
	}

	// Get the correlation record
	correlation, err := h.repositoryManager.GetMessageCorrelation(ctx, payload.CorrelationID)
	if err != nil {
		return fmt.Errorf("failed to get correlation record: %w", err)
	}

	// Check if correlation is still waiting
	if correlation.Status != "WAITING" {
		// Already processed (message received), skip timeout processing
		fmt.Printf("Correlation %s already processed with status %s, skipping timeout\n",
			payload.CorrelationID, correlation.Status)
		return fmt.Errorf("correlation already processed")
	}

	// Verify execution exists and is in valid state
	execRecord, err := h.repositoryManager.GetExecution(ctx, payload.ExecutionID)
	if err != nil {
		return fmt.Errorf("failed to get execution: %w", err)
	}

	if execRecord.Status != persistent.PAUSED {
		fmt.Printf("Execution %s is not in PAUSED state (current: %s), skipping timeout\n",
			payload.ExecutionID, execRecord.Status)
		return fmt.Errorf("execution not in PAUSED state")
	}

	// Process timeout trigger
	fmt.Printf("Processing timeout boundary event: ExecutionID=%s, StateName=%s, CorrelationID=%s\n",
		payload.ExecutionID, payload.StateName, payload.CorrelationID)

	return sm.ProcessTimeoutTrigger(ctx, payload.CorrelationID)
}

// SetExecutionContext sets the execution context for the handler
func (h *ExecutionHandler) SetExecutionContext(execCtx types.ExecutionContext) {
	h.executionContext = execCtx
}

// GetRepositoryManager returns the repository manager
func (h *ExecutionHandler) GetRepositoryManager() *repository.Manager {
	return h.repositoryManager
}

// GetQueueClient returns the queue client
func (h *ExecutionHandler) GetQueueClient() *queue.Client {
	return h.queueClient
}

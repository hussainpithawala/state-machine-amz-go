package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
)

// MessageRequest represents an incoming message to resume execution
type MessageRequest struct {
	CorrelationKey   string      `json:"correlation_key"`
	CorrelationValue interface{} `json:"correlation_value"`
	Data             interface{} `json:"data"`
}

// MessageResponse represents the result of processing a message
type MessageResponse struct {
	ExecutionID     string    `json:"execution_id"`
	StateMachineID  string    `json:"state_machine_id"`
	Status          string    `json:"status"`
	MessageReceived bool      `json:"message_received"`
	ResumedAt       time.Time `json:"resumed_at"`
	Error           string    `json:"error,omitempty"`
}

// Message processes an incoming message and resumes paused executions
// This is the main API for handling external messages that trigger state transitions
func (e *BaseExecutor) Message(ctx context.Context, sm StateMachineInterface, request *MessageRequest) (*MessageResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("message request cannot be nil")
	}

	if request.CorrelationKey == "" {
		return nil, fmt.Errorf("correlation key cannot be empty")
	}

	// Find paused executions waiting for this message
	var executions []*execution.Execution
	var err error

	// Try to find in-memory first (for backward compatibility/simple use cases)
	executions, err = e.findWaitingExecutions(ctx, request.CorrelationKey, request.CorrelationValue)
	if err != nil {
		return nil, fmt.Errorf("failed to find waiting executions in-memory: %w", err)
	}

	// If not found in-memory and we have a persistent state machine, try the repository
	if len(executions) == 0 && sm != nil {
		execRecords, err := sm.FindWaitingExecutionsByCorrelation(ctx, request.CorrelationKey, request.CorrelationValue)
		if err != nil {
			return nil, fmt.Errorf("failed to find waiting executions in repository: %w", err)
		}

		for _, rec := range execRecords {
			// Convert repository.ExecutionRecord to execution.Execution
			exec := &execution.Execution{
				ID:             rec.ExecutionID,
				StateMachineID: rec.StateMachineID,
				Name:           rec.Name,
				Status:         rec.Status,
				Input:          rec.Input,
				Output:         rec.Output,
				CurrentState:   rec.CurrentState,
				Metadata:       rec.Metadata,
			}
			if rec.StartTime != nil {
				exec.StartTime = *rec.StartTime
			}
			if rec.EndTime != nil {
				exec.EndTime = *rec.EndTime
			}
			executions = append(executions, exec)
		}
	}

	if len(executions) == 0 {
		return &MessageResponse{
			Status:          "NO_MATCH",
			MessageReceived: false,
			Error:           "no waiting execution found for correlation key/value",
		}, nil
	}

	// Process the first matching execution
	exec := executions[0]

	// Resume the execution with the message data
	response, err := e.resumeExecutionWithMessage(ctx, sm, exec, request)
	if err != nil {
		return &MessageResponse{
			ExecutionID:     exec.ID,
			StateMachineID:  exec.StateMachineID,
			Status:          "ERROR",
			MessageReceived: true,
			ResumedAt:       time.Now(),
			Error:           err.Error(),
		}, err
	}

	return response, nil
}

// findWaitingExecutions finds executions waiting for a message with the given correlation
func (e *BaseExecutor) findWaitingExecutions(ctx context.Context, correlationKey string, correlationValue interface{}) ([]*execution.Execution, error) {
	// Query the persistence layer for executions with:
	// 1. Status = "PAUSED"
	// 2. CurrentState has a matching correlation key/value

	// This is a simplified implementation - you'll need to extend your repository
	// interface to support this query

	// For now, we'll search through in-memory executions
	// In production, this should be a database query
	var waitingExecutions []*execution.Execution

	for _, exec := range e.executions {
		if exec.Status == "PAUSED" {
			// Check if the current state has matching correlation
			if e.matchesCorrelation(exec, correlationKey, correlationValue) {
				waitingExecutions = append(waitingExecutions, exec)
			}
		}
	}

	return waitingExecutions, nil
}

// matchesCorrelation checks if an execution's current state matches the correlation data
func (e *BaseExecutor) matchesCorrelation(exec *execution.Execution, correlationKey string, correlationValue interface{}) bool {
	// Extract correlation data from execution context
	// This would be stored when the message state pauses execution

	if exec.Metadata == nil {
		return false
	}

	// Check for correlation data in metadata
	if corrData, exists := exec.Metadata["correlation_data"]; exists {
		if corrMap, ok := corrData.(map[string]interface{}); ok {
			storedKey, keyOk := corrMap["correlation_key"].(string)
			storedValue := corrMap["correlation_value"]

			if keyOk && storedKey == correlationKey {
				// Compare correlation values
				return compareCorrelationValues(storedValue, correlationValue)
			}
		}
	}

	return false
}

// compareCorrelationValues compares two correlation values
func compareCorrelationValues(stored, incoming interface{}) bool {
	// Simple equality check - can be extended for more complex matching
	return fmt.Sprintf("%v", stored) == fmt.Sprintf("%v", incoming)
}

// resumeExecutionWithMessage resumes a paused execution with the received message
func (e *BaseExecutor) resumeExecutionWithMessage(ctx context.Context, sm StateMachineInterface, exec *execution.Execution, request *MessageRequest) (*MessageResponse, error) {
	// Create input for resumption that includes the message data
	resumeInput := map[string]interface{}{
		"__received_message__": map[string]interface{}{
			"correlation_key":   request.CorrelationKey,
			"correlation_value": request.CorrelationValue,
			"data":              request.Data,
		},
	}

	// Merge with existing input if needed
	if exec.Input != nil {
		if inputMap, ok := exec.Input.(map[string]interface{}); ok {
			for k, v := range inputMap {
				if k != "__received_message__" {
					resumeInput[k] = v
				}
			}
		}
	}

	// Update execution status and input
	exec.Input = resumeInput

	// Use the state machine to resume execution
	_, err := sm.ResumeExecution(ctx, exec)
	if err != nil {
		return nil, fmt.Errorf("failed to resume execution: %w", err)
	}

	response := &MessageResponse{
		ExecutionID:     exec.ID,
		StateMachineID:  exec.StateMachineID,
		Status:          "RESUMED",
		MessageReceived: true,
		ResumedAt:       time.Now(),
	}

	return response, nil
}

// ResumeExecution resumes a paused execution from its current state
func (e *BaseExecutor) ResumeExecution(ctx context.Context, sm StateMachineInterface, execCtx *execution.Execution) (*execution.Execution, error) {
	if execCtx == nil {
		return nil, fmt.Errorf("execution context cannot be nil")
	}

	if execCtx.Status != "PAUSED" && execCtx.Status != "RUNNING" {
		return nil, fmt.Errorf("cannot resume execution in status: %s", execCtx.Status)
	}

	// Resume from the current state
	// This uses the state machine's RunExecution but starts from the current state
	execCtx.Status = "RUNNING"

	// Use the state machine's execution logic
	result, err := sm.RunExecution(ctx, execCtx.Input, execCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to resume execution: %w", err)
	}

	return result, nil
}

// PauseExecution pauses an execution at the current state
func (e *BaseExecutor) PauseExecution(ctx context.Context, execCtx *execution.Execution, correlationData map[string]interface{}) error {
	if execCtx == nil {
		return fmt.Errorf("execution context cannot be nil")
	}

	// Update execution status
	execCtx.Status = "PAUSED"

	// Store correlation data in metadata
	if execCtx.Metadata == nil {
		execCtx.Metadata = make(map[string]interface{})
	}
	execCtx.Metadata["correlation_data"] = correlationData
	execCtx.Metadata["paused_at"] = time.Now()

	// Keep the execution in memory for potential resume
	// In production, this should be persisted to the database
	e.executions[execCtx.ID] = execCtx

	return nil
}

// ListWaitingExecutions returns all executions waiting for messages
func (e *BaseExecutor) ListWaitingExecutions(ctx context.Context) ([]*execution.Execution, error) {
	var waiting []*execution.Execution

	for _, exec := range e.executions {
		if exec.Status == "PAUSED" {
			waiting = append(waiting, exec)
		}
	}

	return waiting, nil
}

// GetWaitingExecutionByCorrelation finds a waiting execution by correlation key
func (e *BaseExecutor) GetWaitingExecutionByCorrelation(ctx context.Context, correlationKey string, correlationValue interface{}) (*execution.Execution, error) {
	executions, err := e.findWaitingExecutions(ctx, correlationKey, correlationValue)
	if err != nil {
		return nil, err
	}

	if len(executions) == 0 {
		return nil, fmt.Errorf("no waiting execution found for correlation key: %s", correlationKey)
	}

	return executions[0], nil
}

// TimeoutWaitingExecutions checks for and times out executions that have been waiting too long
func (e *BaseExecutor) TimeoutWaitingExecutions(ctx context.Context) error {
	now := time.Now()

	for _, exec := range e.executions {
		if exec.Status != "PAUSED" {
			continue
		}

		// Check for timeout information in metadata
		if exec.Metadata != nil {
			if pausedAt, exists := exec.Metadata["paused_at"]; exists {
				if t, ok := pausedAt.(time.Time); ok {
					if timeoutSeconds, exists := exec.Metadata["timeout_seconds"]; exists {
						if timeout, ok := timeoutSeconds.(int); ok {
							if now.Sub(t) > time.Duration(timeout)*time.Second {
								// Timeout the execution
								exec.Status = "TIMEOUT"
								exec.EndTime = now
								exec.Error = fmt.Errorf("message timeout after %d seconds", timeout)

								// Remove from active executions
								delete(e.executions, exec.ID)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

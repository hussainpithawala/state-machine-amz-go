// internal/states/task.go

package states

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// TaskState represents an AWS Task state
// https://docs.aws.amazon.com/step-functions/latest/dg/state-task.html
type TaskState struct {
	BaseState
	Resource              string                 `json:"Resource"`
	Parameters            map[string]interface{} `json:"Parameters,omitempty"`
	TimeoutSeconds        *int                   `json:"TimeoutSeconds,omitempty"`
	HeartbeatSeconds      *int                   `json:"HeartbeatSeconds,omitempty"`
	Retry                 []RetryPolicy          `json:"Retry,omitempty"`
	Catch                 []CatchPolicy          `json:"Catch,omitempty"`
	ResultSelector        map[string]interface{} `json:"ResultSelector,omitempty"`
	TaskHandler           TaskHandler            `json:"-"` // Custom handler for executing tasks
	HeartbeatMonitor      bool                   `json:"-"` // Whether to monitor heartbeat
	ExecutionTimeoutTimer *time.Timer            `json:"-"` // Timer for execution timeout
}

// RetryPolicy defines retry behavior for a task
type RetryPolicy struct {
	ErrorEquals     []string `json:"ErrorEquals"`
	IntervalSeconds *int     `json:"IntervalSeconds,omitempty"`
	MaxAttempts     *int     `json:"MaxAttempts,omitempty"`
	BackoffRate     *float64 `json:"BackoffRate,omitempty"`
	MaxDelaySeconds *int     `json:"MaxDelaySeconds,omitempty"`
	JitterStrategy  string   `json:"JitterStrategy,omitempty"`
}

// CatchPolicy defines error catching behavior for a task
type CatchPolicy struct {
	ErrorEquals []string `json:"ErrorEquals"`
	ResultPath  *string  `json:"ResultPath,omitempty"`
	Next        string   `json:"Next"`
}

// TaskHandler is an interface for executing tasks
type TaskHandler interface {
	// Execute executes the task and returns the result
	Execute(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error)

	// ExecuteWithTimeout executes the task with a specific timeout context
	// This method should respect context cancellation and timeouts
	ExecuteWithTimeout(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}, timeoutSeconds *int) (interface{}, error)

	// CanHandle returns true if this handler can handle the resource
	CanHandle(resource string) bool
}

// Context keys for task execution
type contextKey string

const (
	// ExecutionContextKey is the key for storing execution context in context
	ExecutionContextKey contextKey = "execution_context"
)

// ExecutionContext provides access to execution-related functionality
type ExecutionContext interface {
	// GetTaskHandler retrieves a task handler for a resource
	GetTaskHandler(resource string) (func(context.Context, interface{}) (interface{}, error), bool)
}

// DefaultTaskHandler is an enhanced handler that delegates to execution context
type DefaultTaskHandler struct{}

// NewDefaultTaskHandler creates a new task handler
func NewDefaultTaskHandler() *DefaultTaskHandler {
	return &DefaultTaskHandler{}
}

// Execute tries to delegate to handler from execution context
func (h *DefaultTaskHandler) Execute(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
	// Try to get execution context
	if execCtx, ok := ctx.Value(ExecutionContextKey).(ExecutionContext); ok && execCtx != nil {
		// Get handler from execution context
		if handler, exists := execCtx.GetTaskHandler(resource); exists {
			// Apply parameters if provided
			taskInput := input
			if parameters != nil {
				processor := NewJSONPathProcessor()
				expandedInput, err := processor.expandValue(parameters, map[string]interface{}{
					"$": input,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to expand parameters: %w", err)
				}
				taskInput = expandedInput
			}

			// Execute the registered handler
			return handler(ctx, taskInput)
		}
	}

	// If no handler found, fall back to original behavior (return input as-is)
	return input, nil
}

// ExecuteWithTimeout executes the task with a specific timeout context
func (h *DefaultTaskHandler) ExecuteWithTimeout(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}, timeoutSeconds *int) (interface{}, error) {
	// If no timeout specified, execute directly
	if timeoutSeconds == nil || *timeoutSeconds <= 0 {
		return h.Execute(ctx, resource, input, parameters)
	}

	// Create a context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(*timeoutSeconds)*time.Second)
	defer cancel() // Properly cleanup resources when function returns

	// Create a channel to receive the result
	type result struct {
		value interface{}
		err   error
	}
	resultChan := make(chan result, 1)

	// Execute the task in a goroutine
	go func() {
		value, err := h.Execute(timeoutCtx, resource, input, parameters)
		resultChan <- result{value: value, err: err}
	}()

	// Wait for either the result or the timeout
	select {
	case res := <-resultChan:
		// Task completed before timeout
		return res.value, res.err
	case <-timeoutCtx.Done():
		// Timeout occurred
		return nil, timeoutCtx.Err()
	}
}

// CanHandle returns true if this handler can handle the resource
func (h *DefaultTaskHandler) CanHandle(resource string) bool {
	return true
}

// GetName returns the name of the state
func (t *TaskState) GetName() string {
	return t.Name
}

// GetType returns the type of the state
func (t *TaskState) GetType() string {
	return "Task"
}

// GetNext returns the next state name
func (t *TaskState) GetNext() *string {
	return t.Next
}

// IsEnd returns true if this is an end state
func (t *TaskState) IsEnd() bool {
	return t.End
}

// Validate validates the Task state configuration
func (t *TaskState) Validate() error {
	if t.Resource == "" {
		return fmt.Errorf("Task state '%s' Resource is required", t.Name)
	}

	if t.TimeoutSeconds != nil && *t.TimeoutSeconds <= 0 {
		return fmt.Errorf("Task state '%s' TimeoutSeconds must be positive", t.Name)
	}

	if t.HeartbeatSeconds != nil && *t.HeartbeatSeconds <= 0 {
		return fmt.Errorf("Task state '%s' HeartbeatSeconds must be positive", t.Name)
	}

	if t.HeartbeatSeconds != nil && t.TimeoutSeconds != nil {
		if *t.HeartbeatSeconds >= *t.TimeoutSeconds {
			return fmt.Errorf("Task state '%s' HeartbeatSeconds must be less than TimeoutSeconds", t.Name)
		}
	}

	// Validate retry policies
	for i, retry := range t.Retry {
		if len(retry.ErrorEquals) == 0 {
			return fmt.Errorf("Task state '%s' Retry policy %d: ErrorEquals is required", t.Name, i)
		}
		if retry.MaxAttempts != nil && *retry.MaxAttempts < 0 {
			return fmt.Errorf("Task state '%s' Retry policy %d: MaxAttempts must be non-negative", t.Name, i)
		}
		if retry.BackoffRate != nil && *retry.BackoffRate < 1.0 {
			return fmt.Errorf("Task state '%s' Retry policy %d: BackoffRate must be >= 1.0", t.Name, i)
		}
	}

	// Validate catch policies
	for i, catchPolicy := range t.Catch {
		if len(catchPolicy.ErrorEquals) == 0 {
			return fmt.Errorf("Task state '%s' Catch policy %d: ErrorEquals is required", t.Name, i)
		}
		if catchPolicy.Next == "" {
			return fmt.Errorf("Task state '%s' Catch policy %d: Next is required", t.Name, i)
		}
	}

	return nil
}

// Execute executes the Task state
func (t *TaskState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
	// Apply input path
	processor := NewJSONPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, t.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

	// Prepare parameters
	var taskInput interface{} = processedInput
	if t.Parameters != nil {
		taskInput, err = processor.expandValue(t.Parameters, map[string]interface{}{
			"$": processedInput,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to expand parameters: %w", err)
		}
	}

	// Get or use default task handler
	handler := t.TaskHandler
	if handler == nil {
		handler = NewDefaultTaskHandler()
	}

	// Execute task with retry logic
	var taskErr error
	var result interface{}
	maxAttempts := 1

	// Find max attempts from retry policies
	for _, retry := range t.Retry {
		if retry.MaxAttempts != nil && *retry.MaxAttempts > 0 {
			maxAttempts = *retry.MaxAttempts // + 1 // +1 for initial attempt
			break
		}
	}

	attempt := 0
	backoffDuration := time.Duration(1) * time.Second

	for attempt <= maxAttempts {
		attempt++

		// Execute the task with timeout - this now properly respects timeout context
		result, taskErr = handler.ExecuteWithTimeout(ctx, t.Resource, taskInput, t.Parameters, t.TimeoutSeconds)

		// If successful, break the loop
		if taskErr == nil {
			break
		}

		// Check if error matches any retry policy
		shouldRetry := false
		for _, retry := range t.Retry {
			if t.errorMatches(taskErr, retry.ErrorEquals) {
				shouldRetry = true
				if retry.BackoffRate != nil && *retry.BackoffRate > 1.0 {
					backoffDuration = time.Duration(float64(backoffDuration) * *retry.BackoffRate)
				}
				if retry.MaxDelaySeconds != nil {
					maxDuration := time.Duration(*retry.MaxDelaySeconds) * time.Second
					if backoffDuration > maxDuration {
						backoffDuration = maxDuration
					}
				}
				break
			}
		}

		if !shouldRetry || attempt > maxAttempts {
			break
		}

		// Wait before retrying - respect context cancellation
		select {
		case <-time.After(backoffDuration):
			// Continue to next attempt
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	// If task failed, check catch policies
	if taskErr != nil {
		for _, catchPolicy := range t.Catch {
			if t.errorMatches(taskErr, catchPolicy.ErrorEquals) {
				// Apply result path for caught error
				errorResult := map[string]interface{}{
					"Error": taskErr.Error(),
					"Cause": taskErr.Error(),
				}

				output, err := processor.ApplyResultPath(processedInput, errorResult, catchPolicy.ResultPath)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to apply result path in catch: %w", err)
				}

				return output, StringPtr(catchPolicy.Next), nil
			}
		}

		return nil, nil, taskErr
	}

	// Apply result selector if provided
	var output interface{} = result
	if t.ResultSelector != nil {
		output, err = processor.expandValue(t.ResultSelector, map[string]interface{}{
			"$": result,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply result selector: %w", err)
		}
	}

	// Apply result path
	output, err = processor.ApplyResultPath(processedInput, output, t.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path: %w", err)
	}

	// Apply output path
	output, err = processor.ApplyOutputPath(output, t.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply output path: %w", err)
	}

	return output, t.GetNext(), nil
}

// errorMatches checks if an error matches any of the error patterns
func (t *TaskState) errorMatches(err error, errorPatterns []string) bool {
	errorMsg := err.Error()
	for _, pattern := range errorPatterns {
		if pattern == "States.ALL" || pattern == errorMsg {
			return true
		}
	}
	return false
}

// MarshalJSON implements custom JSON marshaling
func (t *TaskState) MarshalJSON() ([]byte, error) {
	type Alias TaskState
	return json.Marshal(&struct {
		Type string `json:"Type"`
		*Alias
	}{
		Type:  "Task",
		Alias: (*Alias)(t),
	})
}

// GetNextStates returns all possible next states from Task transitions
func (t *TaskState) GetNextStates() []string {
	nextStates := make([]string, 0)

	// Add primary Next
	if t.Next != nil {
		nextStates = append(nextStates, *t.Next)
	}

	// Add all catch destinations
	for _, catchPolicy := range t.Catch {
		nextStates = append(nextStates, catchPolicy.Next)
	}

	return nextStates
}

// Helper functions for context management

// WithExecutionContext adds an execution context to the context
func WithExecutionContext(ctx context.Context, execCtx ExecutionContext) context.Context {
	return context.WithValue(ctx, ExecutionContextKey, execCtx)
}

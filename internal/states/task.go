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
func (h *DefaultTaskHandler) CanHandle(string) bool {
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
		return fmt.Errorf("task state '%s' Resource is required", t.Name)
	}

	if t.TimeoutSeconds != nil && *t.TimeoutSeconds <= 0 {
		return fmt.Errorf("task state '%s' TimeoutSeconds must be positive", t.Name)
	}

	if t.HeartbeatSeconds != nil && *t.HeartbeatSeconds <= 0 {
		return fmt.Errorf("task state '%s' HeartbeatSeconds must be positive", t.Name)
	}

	if t.HeartbeatSeconds != nil && t.TimeoutSeconds != nil {
		if *t.HeartbeatSeconds >= *t.TimeoutSeconds {
			return fmt.Errorf("task state '%s' HeartbeatSeconds must be less than TimeoutSeconds", t.Name)
		}
	}

	// Validate retry policies
	for i, retry := range t.Retry {
		if len(retry.ErrorEquals) == 0 {
			return fmt.Errorf("task state '%s' Retry policy %d: ErrorEquals is required", t.Name, i)
		}
		if retry.MaxAttempts != nil && *retry.MaxAttempts < 0 {
			return fmt.Errorf("task state '%s' Retry policy %d: MaxAttempts must be non-negative", t.Name, i)
		}
		if retry.BackoffRate != nil && *retry.BackoffRate < 1.0 {
			return fmt.Errorf("task state '%s' Retry policy %d: BackoffRate must be >= 1.0", t.Name, i)
		}
	}

	// Validate catch policies
	for i, catchPolicy := range t.Catch {
		if len(catchPolicy.ErrorEquals) == 0 {
			return fmt.Errorf("task state '%s' Catch policy %d: ErrorEquals is required", t.Name, i)
		}
		if catchPolicy.Next == "" {
			return fmt.Errorf("task state '%s' Catch policy %d: Next is required", t.Name, i)
		}
	}

	return nil
}

// Execute executes the Task state
func (t *TaskState) Execute(ctx context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	// Process input and parameters
	processor, taskInput, processedInput, err := t.prepareInput(input)
	if err != nil {
		return nil, nil, err
	}

	// Get task handler
	handler := t.getTaskHandler()

	// Execute task with retry logic
	result, taskErr, retryExhausted := t.executeWithRetry(ctx, handler, taskInput)
	if taskErr != nil && !retryExhausted {
		return nil, nil, taskErr
	}

	// Handle task result
	return t.handleTaskResult(ctx, processor, processedInput, result, taskErr)
}

// prepareInput processes input path and parameters
func (t *TaskState) prepareInput(input interface{}) (jsonPathProcessor *JSONPathProcessor, taskInput interface{}, processedInput interface{}, err error) {
	processor := NewJSONPathProcessor()

	// Apply input path
	processedInput, err = processor.ApplyInputPath(input, t.InputPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

	// Prepare parameters if provided
	taskInput = processedInput
	if t.Parameters != nil {
		taskInput, err = processor.expandValue(t.Parameters, map[string]interface{}{
			"$": processedInput,
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to expand parameters: %w", err)
		}
	}

	return processor, taskInput, processedInput, nil
}

// getTaskHandler returns the appropriate task handler
func (t *TaskState) getTaskHandler() TaskHandler {
	if t.TaskHandler != nil {
		return t.TaskHandler
	}
	return NewDefaultTaskHandler()
}

// executeWithRetry handles task execution with retry logic
func (t *TaskState) executeWithRetry(ctx context.Context, handler TaskHandler, taskInput interface{}) (interface{}, error, bool) {
	maxAttempts := t.calculateMaxAttempts()
	backoffDuration := time.Duration(1) * time.Second

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		// Execute task
		result, taskErr := handler.ExecuteWithTimeout(ctx, t.Resource, taskInput, t.Parameters, t.TimeoutSeconds)

		// If successful, return result
		if taskErr == nil {
			return result, nil, true
		}

		// Check if we should retry
		shouldRetry := t.shouldRetry(taskErr, attempt, maxAttempts)
		if !shouldRetry {
			return nil, taskErr, false
		}

		// Update backoff duration
		backoffDuration = t.calculateBackoffDuration(taskErr, backoffDuration)

		// Wait before retrying
		if err := t.waitForRetry(ctx, backoffDuration); err != nil {
			return nil, err, false
		}
	}

	return nil, fmt.Errorf("max retry attempts exceeded"), false
}

// calculateMaxAttempts determines the maximum number of retry attempts
func (t *TaskState) calculateMaxAttempts() int {
	maxAttempts := 1
	for _, retry := range t.Retry {
		if retry.MaxAttempts != nil && *retry.MaxAttempts > 0 {
			maxAttempts = *retry.MaxAttempts
			break
		}
	}
	return maxAttempts
}

// shouldRetry determines if a task should be retried
func (t *TaskState) shouldRetry(taskErr error, attempt, maxAttempts int) bool {
	if attempt >= maxAttempts {
		return false
	}

	// Check if error matches any retry policy
	for _, retry := range t.Retry {
		if t.errorMatches(taskErr, retry.ErrorEquals) {
			return true
		}
	}

	return false
}

// calculateBackoffDuration calculates the backoff duration for retries
func (t *TaskState) calculateBackoffDuration(taskErr error, currentDuration time.Duration) time.Duration {
	duration := currentDuration

	for _, retry := range t.Retry {
		if t.errorMatches(taskErr, retry.ErrorEquals) {
			// Apply backoff rate
			if retry.BackoffRate != nil && *retry.BackoffRate > 1.0 {
				duration = time.Duration(float64(duration) * *retry.BackoffRate)
			}

			// Apply max delay limit
			if retry.MaxDelaySeconds != nil {
				maxDuration := time.Duration(*retry.MaxDelaySeconds) * time.Second
				if duration > maxDuration {
					duration = maxDuration
				}
			}
			break
		}
	}

	return duration
}

// waitForRetry waits for the backoff duration or context cancellation
func (t *TaskState) waitForRetry(ctx context.Context, backoffDuration time.Duration) error {
	select {
	case <-time.After(backoffDuration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleTaskResult processes the task result and handles errors
func (t *TaskState) handleTaskResult(ctx context.Context, processor *JSONPathProcessor,
	processedInput interface{}, result interface{}, taskErr error) (output interface{}, nextState *string, err error) {
	// Handle task failure
	if taskErr != nil {
		return t.handleTaskFailure(ctx, processor, processedInput, taskErr)
	}

	// Process successful task result
	return t.processSuccessfulResult(processor, processedInput, result)
}

// handleTaskFailure processes task failures and catch policies
func (t *TaskState) handleTaskFailure(_ context.Context, processor *JSONPathProcessor,
	processedInput interface{}, taskErr error) (output interface{}, nextState *string, err error) {
	// Check catch policies
	for _, catchPolicy := range t.Catch {
		if t.errorMatches(taskErr, catchPolicy.ErrorEquals) {
			return t.handleCaughtError(processor, processedInput, taskErr, catchPolicy)
		}
	}

	// No catch policy matched, return error
	return nil, nil, taskErr
}

// handleCaughtError processes errors caught by catch policies
func (t *TaskState) handleCaughtError(processor *JSONPathProcessor, processedInput interface{},
	taskErr error, catchPolicy CatchPolicy) (output interface{}, nextState *string, err error) {
	// Create error result
	errorResult := map[string]interface{}{
		"Error": taskErr.Error(),
		"Cause": taskErr.Error(),
	}

	// Apply result path
	output, err = processor.ApplyResultPath(processedInput, errorResult, catchPolicy.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path in catch: %w", err)
	}

	return output, StringPtr(catchPolicy.Next), nil
}

// processSuccessfulResult processes successful task results
func (t *TaskState) processSuccessfulResult(processor *JSONPathProcessor,
	processedInput interface{}, result interface{}) (output interface{}, nextState *string, err error) {
	// Apply result selector if provided
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

// internal/states/task.go
package states

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
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
	Execute(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error)
	ExecuteWithTimeout(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}, timeoutSeconds *int) (interface{}, error)
	CanHandle(resource string) bool
}

// DefaultTaskHandler is an enhanced handler that delegates to execution context
type DefaultTaskHandler struct{}

func NewDefaultTaskHandler() *DefaultTaskHandler {
	return &DefaultTaskHandler{}
}

// Execute delegates to the handler registered in the execution context.
// NOTE: The 'input' parameter has ALREADY been fully expanded by TaskState.prepareInput.
func (h *DefaultTaskHandler) Execute(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
	execCtx, ok := ctx.Value(types.ExecutionContextKey).(types.ExecutionContext)
	if !ok || execCtx == nil {
		return nil, fmt.Errorf("task execution failed: no ExecutionContext found in context")
	}

	handler, exists := execCtx.GetTaskHandler(resource)
	if !exists || handler == nil {
		return nil, fmt.Errorf("task execution failed: no TaskHandler registered for resource '%s'", resource)
	}

	return handler(ctx, input)
}

func (h *DefaultTaskHandler) ExecuteWithTimeout(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}, timeoutSeconds *int) (interface{}, error) {
	if timeoutSeconds == nil || *timeoutSeconds <= 0 {
		return h.Execute(ctx, resource, input, parameters)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(*timeoutSeconds)*time.Second)
	defer cancel()

	type result struct {
		value interface{}
		err   error
	}
	resultChan := make(chan result, 1)

	go func() {
		value, err := h.Execute(timeoutCtx, resource, input, parameters)
		resultChan <- result{value: value, err: err}
	}()

	select {
	case res := <-resultChan:
		return res.value, res.err
	case <-timeoutCtx.Done():
		return nil, timeoutCtx.Err()
	}
}

func (h *DefaultTaskHandler) CanHandle(string) bool {
	return true
}

func (t *TaskState) GetName() string  { return t.Name }
func (t *TaskState) GetType() string  { return "Task" }
func (t *TaskState) GetNext() *string { return t.Next }
func (t *TaskState) IsEnd() bool      { return t.End }

// --- Validation ---

type ValidationRule interface {
	Validate(task *TaskState) error
}

type TaskStateValidator struct {
	rules []ValidationRule
}

func NewTaskStateValidator() *TaskStateValidator {
	return &TaskStateValidator{
		rules: []ValidationRule{
			&resourceRequiredRule{},
			&timeoutPositiveRule{},
			&heartbeatPositiveRule{},
			&heartbeatLessThanTimeoutRule{},
			&retryPolicyRule{},
			&catchPolicyRule{},
		},
	}
}

func (v *TaskStateValidator) Validate(task *TaskState) error {
	for _, rule := range v.rules {
		if err := rule.Validate(task); err != nil {
			return err
		}
	}
	return nil
}

type resourceRequiredRule struct{}

func (r *resourceRequiredRule) Validate(task *TaskState) error {
	if task.Resource == "" {
		return fmt.Errorf("task state '%s' Resource is required", task.Name)
	}
	return nil
}

type timeoutPositiveRule struct{}

func (r *timeoutPositiveRule) Validate(task *TaskState) error {
	if task.TimeoutSeconds != nil && *task.TimeoutSeconds <= 0 {
		return fmt.Errorf("task state '%s' TimeoutSeconds must be positive", task.Name)
	}
	return nil
}

type heartbeatPositiveRule struct{}

func (r *heartbeatPositiveRule) Validate(task *TaskState) error {
	if task.HeartbeatSeconds != nil && *task.HeartbeatSeconds <= 0 {
		return fmt.Errorf("task state '%s' HeartbeatSeconds must be positive", task.Name)
	}
	return nil
}

type heartbeatLessThanTimeoutRule struct{}

func (r *heartbeatLessThanTimeoutRule) Validate(task *TaskState) error {
	if task.HeartbeatSeconds != nil && task.TimeoutSeconds != nil {
		if *task.HeartbeatSeconds >= *task.TimeoutSeconds {
			return fmt.Errorf("task state '%s' HeartbeatSeconds must be less than TimeoutSeconds", task.Name)
		}
	}
	return nil
}

type retryPolicyRule struct{}

func (r *retryPolicyRule) Validate(task *TaskState) error {
	for i, retry := range task.Retry {
		if len(retry.ErrorEquals) == 0 {
			return fmt.Errorf("task state '%s' Retry policy %d: ErrorEquals is required", task.Name, i)
		}
		if retry.MaxAttempts != nil && *retry.MaxAttempts < 0 {
			return fmt.Errorf("task state '%s' Retry policy %d: MaxAttempts must be non-negative", task.Name, i)
		}
		if retry.BackoffRate != nil && *retry.BackoffRate < 1.0 {
			return fmt.Errorf("task state '%s' Retry policy %d: BackoffRate must be >= 1.0", task.Name, i)
		}
	}
	return nil
}

type catchPolicyRule struct{}

func (r *catchPolicyRule) Validate(task *TaskState) error {
	for i, catchPolicy := range task.Catch {
		if len(catchPolicy.ErrorEquals) == 0 {
			return fmt.Errorf("task state '%s' Catch policy %d: ErrorEquals is required", task.Name, i)
		}
		if catchPolicy.Next == "" {
			return fmt.Errorf("task state '%s' Catch policy %d: Next is required", task.Name, i)
		}
	}
	return nil
}

func (t *TaskState) Validate() error {
	return NewTaskStateValidator().Validate(t)
}

// --- Execution ---

func (t *TaskState) Execute(ctx context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	processor, taskInput, processedInput, err := t.prepareInput(input)
	if err != nil {
		return nil, nil, err
	}

	handler := t.getTaskHandler()
	result, retryExhausted, taskErr := t.executeWithRetry(ctx, handler, taskInput)

	if taskErr != nil && retryExhausted {
		return nil, nil, taskErr
	}

	return t.handleTaskResult(ctx, processor, processedInput, result, taskErr)
}

func (t *TaskState) prepareInput(input interface{}) (jsonPathProcessor *JSONPathProcessor, taskInput, processedInput interface{}, err error) {
	processor := NewJSONPathProcessor()

	processedInput, err = processor.ApplyInputPath(input, t.InputPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

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

func (t *TaskState) getTaskHandler() TaskHandler {
	if t.TaskHandler != nil {
		return t.TaskHandler
	}
	return NewDefaultTaskHandler()
}

// executeWithRetry handles task execution with retry logic, strictly scoping
// retries to the specific policy that matches the error (ASL Compliant).
func (t *TaskState) executeWithRetry(ctx context.Context, handler TaskHandler, taskInput interface{}) (result interface{}, ok bool, err error) {
	for attempt := 0; ; attempt++ {
		result, taskErr := handler.ExecuteWithTimeout(ctx, t.Resource, taskInput, t.Parameters, t.TimeoutSeconds)

		if taskErr == nil {
			return result, true, nil
		}

		// Find the matching retry policy for this specific error
		matchedPolicy := t.findMatchingRetryPolicy(taskErr)

		// If no policy matches, stop retrying immediately
		if matchedPolicy == nil {
			return nil, false, taskErr
		}

		// ASL Default for MaxAttempts is 3
		policyMaxAttempts := 3
		if matchedPolicy.MaxAttempts != nil {
			policyMaxAttempts = *matchedPolicy.MaxAttempts
		}

		// If we've exhausted the max attempts for this matched policy, stop retrying
		if attempt >= policyMaxAttempts {
			return nil, false, taskErr
		}

		// Calculate wait duration based on the matched policy and current attempt
		waitDuration := t.calculateWaitDuration(matchedPolicy, attempt)

		if err := t.waitForRetry(ctx, waitDuration); err != nil {
			return nil, false, err
		}
	}
}

// findMatchingRetryPolicy returns the first retry policy that matches the error
func (t *TaskState) findMatchingRetryPolicy(err error) *RetryPolicy {
	for i := range t.Retry {
		if t.errorMatches(err, t.Retry[i].ErrorEquals) {
			return &t.Retry[i]
		}
	}
	return nil
}

// calculateWaitDuration calculates the wait duration for a specific retry attempt
// Formula: IntervalSeconds * (BackoffRate ^ attempt)
func (t *TaskState) calculateWaitDuration(policy *RetryPolicy, attempt int) time.Duration {
	// ASL Default: 1 second
	intervalSeconds := 1
	if policy.IntervalSeconds != nil {
		intervalSeconds = *policy.IntervalSeconds
	}

	// ASL Default: 2.0
	backoffRate := 2.0
	if policy.BackoffRate != nil {
		backoffRate = *policy.BackoffRate
	}

	delaySeconds := float64(intervalSeconds) * math.Pow(backoffRate, float64(attempt))
	duration := time.Duration(delaySeconds * float64(time.Second))

	if policy.MaxDelaySeconds != nil {
		maxDuration := time.Duration(*policy.MaxDelaySeconds) * time.Second
		if duration > maxDuration {
			duration = maxDuration
		}
	}

	return duration
}

func (t *TaskState) waitForRetry(ctx context.Context, backoffDuration time.Duration) error {
	select {
	case <-time.After(backoffDuration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- Result Handling ---

func (t *TaskState) handleTaskResult(ctx context.Context, processor *JSONPathProcessor, processedInput, result interface{}, taskErr error) (output interface{}, nextState *string, err error) {
	if taskErr != nil {
		return t.handleTaskFailure(ctx, processor, processedInput, taskErr)
	}
	return t.processSuccessfulResult(processor, processedInput, result)
}

func (t *TaskState) handleTaskFailure(_ context.Context, processor *JSONPathProcessor, processedInput interface{}, taskErr error) (output interface{}, nextState *string, err error) {
	for _, catchPolicy := range t.Catch {
		if t.errorMatches(taskErr, catchPolicy.ErrorEquals) {
			return t.handleCaughtError(processor, processedInput, taskErr, catchPolicy)
		}
	}
	return nil, nil, taskErr
}

func (t *TaskState) handleCaughtError(processor *JSONPathProcessor, processedInput interface{}, taskErr error, catchPolicy CatchPolicy) (output interface{}, nextState *string, err error) {
	errorResult := map[string]interface{}{
		"Error": taskErr.Error(),
		"Cause": taskErr.Error(),
	}

	output, err = processor.ApplyResultPath(processedInput, errorResult, catchPolicy.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path in catch: %w", err)
	}
	return output, StringPtr(catchPolicy.Next), nil
}

func (t *TaskState) processSuccessfulResult(processor *JSONPathProcessor, processedInput, result interface{}) (output interface{}, next *string, err error) {
	output = result

	if t.ResultSelector != nil {
		output, err = processor.expandValue(t.ResultSelector, map[string]interface{}{
			"$": result,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply result selector: %w", err)
		}
	}

	output, err = processor.ApplyResultPath(processedInput, output, t.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path: %w", err)
	}

	output, err = processor.ApplyOutputPath(output, t.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply output path: %w", err)
	}

	return output, t.GetNext(), nil
}

// errorMatches checks if an error matches any of the error patterns (Bug #5 Fix).
func (t *TaskState) errorMatches(err error, errorPatterns []string) bool {
	if err == nil {
		return false
	}

	checkErr := func(e error) bool {
		for _, pattern := range errorPatterns {
			if pattern == "States.ALL" {
				return true
			}
			if named, ok := e.(interface{ ErrorName() string }); ok && named.ErrorName() == pattern {
				return true
			}
			if named, ok := e.(interface{ Name() string }); ok && named.Name() == pattern {
				return true
			}
			if e.Error() == pattern {
				return true
			}
			if strings.HasPrefix(e.Error(), pattern+":") || strings.HasPrefix(e.Error(), pattern+" ") {
				return true
			}
		}
		return false
	}

	if checkErr(err) {
		return true
	}

	curr := err
	for {
		curr = errors.Unwrap(curr)
		if curr == nil {
			break
		}
		if checkErr(curr) {
			return true
		}
	}
	return false
}

// --- Marshaling & Helpers ---

func (t *TaskState) MarshalJSON() ([]byte, error) {
	return MarshalStateWithBase(t.BaseState, struct {
		Resource         string                 `json:"Resource"`
		Parameters       map[string]interface{} `json:"Parameters,omitempty"`
		TimeoutSeconds   *int                   `json:"TimeoutSeconds,omitempty"`
		HeartbeatSeconds *int                   `json:"HeartbeatSeconds,omitempty"`
		Retry            []RetryPolicy          `json:"Retry,omitempty"`
		Catch            []CatchPolicy          `json:"Catch,omitempty"`
		ResultSelector   map[string]interface{} `json:"ResultSelector,omitempty"`
	}{
		Resource:         t.Resource,
		Parameters:       t.Parameters,
		TimeoutSeconds:   t.TimeoutSeconds,
		HeartbeatSeconds: t.HeartbeatSeconds,
		Retry:            t.Retry,
		Catch:            t.Catch,
		ResultSelector:   t.ResultSelector,
	})
}

func (t *TaskState) GetNextStates() []string {
	nextStates := make([]string, 0)
	if t.Next != nil {
		nextStates = append(nextStates, *t.Next)
	}
	for _, catchPolicy := range t.Catch {
		nextStates = append(nextStates, catchPolicy.Next)
	}
	return nextStates
}

func WithExecutionContext(ctx context.Context, execCtx types.ExecutionContext) context.Context {
	return context.WithValue(ctx, types.ExecutionContextKey, execCtx)
}

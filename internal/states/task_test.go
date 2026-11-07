package states

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testInput = "test"

// MockTaskHandler is a mock implementation of TaskHandler for testing
type MockTaskHandler struct {
	ExecuteFunc            func(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error)
	ExecuteWithTimeoutFunc func(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}, timeoutSeconds *int) (interface{}, error)
	CanHandleFunc          func(resource string) bool
}

func (m *MockTaskHandler) Execute(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, resource, input, parameters)
	}
	return input, nil
}

func (m *MockTaskHandler) ExecuteWithTimeout(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}, timeoutSeconds *int) (interface{}, error) {
	if m.ExecuteWithTimeoutFunc != nil {
		return m.ExecuteWithTimeoutFunc(ctx, resource, input, parameters, timeoutSeconds)
	}

	// Default implementation - if no timeout specified, execute directly
	if timeoutSeconds == nil || *timeoutSeconds <= 0 {
		return m.Execute(ctx, resource, input, parameters)
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
		value, err := m.Execute(timeoutCtx, resource, input, parameters)
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

func (m *MockTaskHandler) CanHandle(resource string) bool {
	if m.CanHandleFunc != nil {
		return m.CanHandleFunc(resource)
	}
	return true
}
func TestTaskState_Execute_Basic(t *testing.T) {
	ctx := context.Background()

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{
				"result": "success",
				"input":  input,
			}, nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			Next: StringPtr("NextState"),
		},
		Resource:    "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		TaskHandler: handler,
	}

	input := map[string]interface{}{
		"key": "value",
	}

	output, nextState, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, "NextState", *nextState)

	resultMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "success", resultMap["result"])
}

func TestTaskState_Execute_WithParameters(t *testing.T) {
	ctx := context.Background()

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			// Verify parameters were passed
			return map[string]interface{}{
				"params": parameters,
			}, nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		Parameters: map[string]interface{}{
			"param1": "value1",
			"param2": 42,
		},
		TaskHandler: handler,
	}

	input := "initial"

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.NotNil(t, output)
}

func TestTaskState_Execute_WithInputPath(t *testing.T) {
	ctx := context.Background()

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			return input, nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name:      "TaskState",
			Type:      "Task",
			End:       true,
			InputPath: StringPtr("$.data"),
		},
		Resource:    "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		TaskHandler: handler,
	}

	input := map[string]interface{}{
		"data": map[string]interface{}{
			"value": "test",
		},
		"other": "ignored",
	}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	outputMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test", outputMap["value"])
}

func TestTaskState_Execute_WithResultPath(t *testing.T) {
	ctx := context.Background()

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			return "task-result", nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name:       "TaskState",
			Type:       "Task",
			End:        true,
			ResultPath: StringPtr("$.taskResult"),
		},
		Resource:    "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		TaskHandler: handler,
	}

	input := map[string]interface{}{
		"original": "data",
	}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	outputMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "data", outputMap["original"])
	assert.Equal(t, "task-result", outputMap["taskResult"])
}

func TestTaskState_Execute_WithOutputPath(t *testing.T) {
	ctx := context.Background()

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{
				"result": "success",
				"extra":  "data",
			}, nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name:       "TaskState",
			Type:       "Task",
			End:        true,
			OutputPath: StringPtr("$.result"),
		},
		Resource:    "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		TaskHandler: handler,
	}

	input := "initial"

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestTaskState_Execute_WithTimeout(t *testing.T) {
	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			// Simulate a long-running task
			time.Sleep(3 * time.Second)
			return input, nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource:       "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		TimeoutSeconds: IntPtr(1),
		TaskHandler:    handler,
	}

	ctx := context.Background()
	input := testInput

	_, _, err := state.Execute(ctx, input)

	// Should timeout
	require.Error(t, err)
}

func TestTaskState_Execute_WithRetry_Success(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			callCount++
			// Fail on first attempt, succeed on second
			if callCount == 1 {
				return nil, fmt.Errorf("TemporaryError")
			}
			return "success", nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		Retry: []RetryPolicy{
			{
				ErrorEquals:     []string{"TemporaryError"},
				IntervalSeconds: IntPtr(0),
				MaxAttempts:     IntPtr(2),
				BackoffRate:     Float64Ptr(1.0),
			},
		},
		TaskHandler: handler,
	}

	input := testInput

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, 2, callCount)
}

func TestTaskState_Execute_WithRetry_ExhaustedAttempts(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			callCount++
			return nil, fmt.Errorf("PersistentError")
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		Retry: []RetryPolicy{
			{
				ErrorEquals:     []string{"PersistentError"},
				IntervalSeconds: IntPtr(0),
				MaxAttempts:     IntPtr(2),
				BackoffRate:     Float64Ptr(1.0),
			},
		},
		TaskHandler: handler,
	}

	input := testInput

	_, _, err := state.Execute(ctx, input)

	require.Error(t, err)
	assert.Equal(t, 3, callCount) // Initial + 2 retries
}

func TestTaskState_Execute_WithCatch(t *testing.T) {
	ctx := context.Background()

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("CustomError")
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		Catch: []CatchPolicy{
			{
				ErrorEquals: []string{"CustomError"},
				ResultPath:  StringPtr("$.error"),
				Next:        "ErrorHandler",
			},
		},
		TaskHandler: handler,
	}

	input := map[string]interface{}{
		"original": "data",
	}

	output, nextState, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "ErrorHandler", *nextState)

	outputMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "data", outputMap["original"])
	assert.NotNil(t, outputMap["error"])
}

func TestTaskState_Execute_WithResultSelector(t *testing.T) {
	ctx := context.Background()

	handler := &MockTaskHandler{
		ExecuteFunc: func(_ context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			return map[string]interface{}{
				"statusCode": 200,
				"body":       "success",
			}, nil
		},
	}

	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		ResultSelector: map[string]interface{}{
			"message": "$.body",
			"code":    "$.statusCode",
		},
		TaskHandler: handler,
	}

	input := testInput

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	outputMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "success", outputMap["message"])
	assert.Equal(t, int(200), outputMap["code"])
}

func TestTaskState_Validate(t *testing.T) {
	tests := []struct {
		name      string
		state     *TaskState
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid task state",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
			},
			shouldErr: false,
		},
		{
			name: "missing resource",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource: "",
			},
			shouldErr: true,
			errMsg:    "Resource is required",
		},
		{
			name: "invalid timeout",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource:       "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
				TimeoutSeconds: IntPtr(0),
			},
			shouldErr: true,
			errMsg:    "TimeoutSeconds must be positive",
		},
		{
			name: "invalid heartbeat",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource:         "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
				HeartbeatSeconds: IntPtr(-1),
			},
			shouldErr: true,
			errMsg:    "HeartbeatSeconds must be positive",
		},
		{
			name: "heartbeat >= timeout",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource:         "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
				TimeoutSeconds:   IntPtr(10),
				HeartbeatSeconds: IntPtr(10),
			},
			shouldErr: true,
			errMsg:    "HeartbeatSeconds must be less than TimeoutSeconds",
		},
		{
			name: "invalid retry - no error equals",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
				Retry: []RetryPolicy{
					{
						ErrorEquals: []string{},
					},
				},
			},
			shouldErr: true,
			errMsg:    "ErrorEquals is required",
		},
		{
			name: "invalid backoff rate",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
				Retry: []RetryPolicy{
					{
						ErrorEquals: []string{"Error"},
						BackoffRate: Float64Ptr(0.5),
					},
				},
			},
			shouldErr: true,
			errMsg:    "BackoffRate must be >= 1.0",
		},
		{
			name: "invalid catch - no next",
			state: &TaskState{
				BaseState: BaseState{
					Name: "TaskState",
					Type: "Task",
				},
				Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
				Catch: []CatchPolicy{
					{
						ErrorEquals: []string{"Error"},
						Next:        "",
					},
				},
			},
			shouldErr: true,
			errMsg:    "Next is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()

			if tt.shouldErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTaskState_GettersAndSetters(t *testing.T) {
	state := &TaskState{
		BaseState: BaseState{
			Name: "MyTaskState",
			Type: "Task",
			End:  true,
			Next: StringPtr("NextState"),
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
	}

	assert.Equal(t, "MyTaskState", state.GetName())
	assert.Equal(t, "Task", state.GetType())
	assert.True(t, state.IsEnd())
	assert.Equal(t, "NextState", *state.GetNext())
}

func TestTaskState_ErrorMatching(t *testing.T) {
	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
	}

	tests := []struct {
		name        string
		err         error
		patterns    []string
		shouldMatch bool
	}{
		{
			name:        "exact match",
			err:         fmt.Errorf("CustomError"),
			patterns:    []string{"CustomError"},
			shouldMatch: true,
		},
		{
			name:        "States.ALL wildcard",
			err:         fmt.Errorf("AnyError"),
			patterns:    []string{"States.ALL"},
			shouldMatch: true,
		},
		{
			name:        "no match",
			err:         fmt.Errorf("UnhandledError"),
			patterns:    []string{"CustomError"},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := state.errorMatches(tt.err, tt.patterns)
			assert.Equal(t, tt.shouldMatch, result)
		})
	}
}

// mockExecutionContext is a minimal mock for testing DefaultTaskHandler behavior
type mockExecutionContext struct {
	handlers map[string]func(ctx context.Context, input interface{}) (interface{}, error)
}

func (m *mockExecutionContext) GetTaskHandler(resource string) (func(ctx context.Context, input interface{}) (interface{}, error), bool) {
	h, ok := m.handlers[resource]
	return h, ok
}

func TestTaskState_DefaultTaskHandler_MissingContextFailsFast(t *testing.T) {
	// Do NOT set TaskHandler on the state, forcing it to use DefaultTaskHandler
	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		// TaskHandler is intentionally nil
	}

	// Empty context WITHOUT ExecutionContext injected
	emptyCtx := context.Background()
	input := map[string]interface{}{"data": "original"}

	output, nextState, err := state.Execute(emptyCtx, input)

	// ASSERTIONS: Must fail explicitly, NOT return input silently
	require.Error(t, err, "Execute must return an error when ExecutionContext is missing from context")
	assert.Contains(t, err.Error(), "no ExecutionContext found in context", "Error message must clearly indicate the missing context")
	assert.Nil(t, output, "Output should be nil when execution fails")
	assert.Nil(t, nextState, "NextState should be nil when execution fails")
}

func TestTaskState_DefaultTaskHandler_MissingHandlerFailsFast(t *testing.T) {
	// Do NOT set TaskHandler on the state, forcing it to use DefaultTaskHandler
	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		// TaskHandler is intentionally nil
	}

	// Context WITH ExecutionContext, but NO handler registered for the requested resource
	mockCtx := &mockExecutionContext{
		handlers: make(map[string]func(ctx context.Context, input interface{}) (interface{}, error)),
	}
	ctx := context.WithValue(context.Background(), types.ExecutionContextKey, mockCtx)
	input := map[string]interface{}{"data": "original"}

	output, nextState, err := state.Execute(ctx, input)

	// ASSERTIONS: Must fail explicitly, NOT return input silently
	require.Error(t, err, "Execute must return an error when no TaskHandler is registered for the resource")
	assert.Contains(t, err.Error(), "no TaskHandler registered for resource", "Error message must clearly indicate the missing handler")
	assert.Nil(t, output, "Output should be nil when execution fails")
	assert.Nil(t, nextState, "NextState should be nil when execution fails")
}

func TestTaskState_DefaultTaskHandler_SuccessWithValidContext(t *testing.T) {
	// Do NOT set TaskHandler on the state, forcing it to use DefaultTaskHandler
	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		// TaskHandler is intentionally nil
	}

	// Context WITH ExecutionContext AND a registered handler for the resource
	mockCtx := &mockExecutionContext{
		handlers: map[string]func(ctx context.Context, input interface{}) (interface{}, error){
			"arn:aws:lambda:us-east-1:123456789012:function:MyFunction": func(ctx context.Context, input interface{}) (interface{}, error) {
				return map[string]interface{}{"processed": true, "input": input}, nil
			},
		},
	}
	ctx := context.WithValue(context.Background(), types.ExecutionContextKey, mockCtx)
	input := map[string]interface{}{"data": "original"}

	output, nextState, err := state.Execute(ctx, input)

	// ASSERTIONS: Should succeed normally
	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.True(t, state.IsEnd()) // End state, so nextState should be nil
	assert.Nil(t, nextState)

	resultMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, resultMap["processed"])
}

func TestTaskState_Parameters_NestedJSONPath_NoDoubleExpansion(t *testing.T) {
	// Do NOT set TaskHandler on the state, forcing it to use DefaultTaskHandler
	state := &TaskState{
		BaseState: BaseState{
			Name: "TaskState",
			Type: "Task",
			End:  true,
		},
		Resource: "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		// Nested JSONPath lookup in Parameters
		Parameters: map[string]interface{}{
			"orderId":    "$.order.id",
			"customerId": "$.customer.details.id",
			"staticVal":  "literal",
		},
	}

	// Context WITH ExecutionContext and a handler that simply echoes the input it receives
	mockCtx := &mockExecutionContext{
		handlers: map[string]func(ctx context.Context, input interface{}) (interface{}, error){
			"arn:aws:lambda:us-east-1:123456789012:function:MyFunction": func(ctx context.Context, input interface{}) (interface{}, error) {
				// This handler should receive the ALREADY EXPANDED parameters.
				// If double-expansion occurs, it will receive an error or malformed data.
				return input, nil
			},
		},
	}
	ctx := context.WithValue(context.Background(), types.ExecutionContextKey, mockCtx)

	// Deeply nested input data
	input := map[string]interface{}{
		"order": map[string]interface{}{
			"id": "ORD-98765",
		},
		"customer": map[string]interface{}{
			"details": map[string]interface{}{
				"id": "CUST-12345",
			},
		},
		"ignored": "data",
	}

	output, nextState, err := state.Execute(ctx, input)

	// ASSERTIONS
	require.NoError(t, err, "Execution must not fail due to double JSONPath expansion")
	assert.Nil(t, nextState) // End state

	resultMap, ok := output.(map[string]interface{})
	require.True(t, ok, "Output should be the expanded parameters map")

	// Verify nested lookups resolved correctly
	assert.Equal(t, "ORD-98765", resultMap["orderId"])
	assert.Equal(t, "CUST-12345", resultMap["customerId"])
	assert.Equal(t, "literal", resultMap["staticVal"])

	// Verify non-referenced data was correctly excluded (Parameters acts as a filter/constructor)
	_, exists := resultMap["ignored"]
	assert.False(t, exists, "Non-referenced root data should not be in the expanded parameters")
}

// customNamedError is a package-level test error type that implements
// both the standard error interface and a custom ErrorName interface.
type customNamedError struct {
	name string
	msg  string
}

// Error implements the standard error interface (value receiver ensures both value and pointer work)
func (e customNamedError) Error() string { return e.msg }

// ErrorName implements our custom interface for typed error matching
func (e customNamedError) ErrorName() string { return e.name }

func TestTaskState_ErrorMatches_WrappedAndTypedErrors(t *testing.T) {
	state := &TaskState{
		BaseState: BaseState{Name: "TestTask", Type: "Task"},
		Resource:  "arn:aws:lambda:::test",
	}

	tests := []struct {
		name     string
		err      error
		patterns []string
		expected bool
	}{
		{
			name:     "exact string match (backwards compatibility)",
			err:      fmt.Errorf("SimpleError"),
			patterns: []string{"SimpleError"},
			expected: true,
		},
		{
			name:     "wrapped error with colon prefix",
			err:      fmt.Errorf("CustomError: something went wrong"),
			patterns: []string{"CustomError"},
			expected: true,
		},
		{
			name:     "wrapped error with space prefix",
			err:      fmt.Errorf("CustomError - something went wrong"),
			patterns: []string{"CustomError"},
			expected: true,
		},
		{
			name:     "typed error with ErrorName interface",
			err:      customNamedError{name: "CustomError", msg: "something went wrong"},
			patterns: []string{"CustomError"},
			expected: true,
		},
		{
			name:     "deeply wrapped error",
			err:      fmt.Errorf("outer wrapper: %w", fmt.Errorf("CustomError: inner details")),
			patterns: []string{"CustomError"},
			expected: true,
		},
		{
			name:     "States.ALL matches anything",
			err:      fmt.Errorf("RandomError: details"),
			patterns: []string{"States.ALL"},
			expected: true,
		},
		{
			name:     "no match for different error",
			err:      fmt.Errorf("CustomError: details"),
			patterns: []string{"DifferentError"},
			expected: false,
		},
		{
			name:     "no match for nil error",
			err:      nil,
			patterns: []string{"CustomError"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := state.errorMatches(tt.err, tt.patterns)
			assert.Equal(t, tt.expected, result, "errorMatches result mismatch for case: %s", tt.name)
		})
	}
}

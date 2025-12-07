// internal/states/task_with_executor_test.go
package states_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockExecutionContext implements states.ExecutionContext for testing
type MockExecutionContext struct {
	handlers map[string]func(context.Context, interface{}) (interface{}, error)
}

func NewMockExecutionContext() *MockExecutionContext {
	return &MockExecutionContext{
		handlers: make(map[string]func(context.Context, interface{}) (interface{}, error)),
	}
}

func (m *MockExecutionContext) RegisterHandler(resource string, handler func(context.Context, interface{}) (interface{}, error)) {
	m.handlers[resource] = handler
}

func (m *MockExecutionContext) GetTaskHandler(resource string) (func(context.Context, interface{}) (interface{}, error), bool) {
	handler, exists := m.handlers[resource]
	return handler, exists
}

// TestTaskExecutor demonstrates the new task execution flow
func TestTaskExecutor(t *testing.T) {
	ctx := context.Background()

	// Create a mock execution context with registered handlers
	mockExecCtx := NewMockExecutionContext()

	// Register a simple handler
	mockExecCtx.RegisterHandler("arn:aws:lambda:us-east-1:123456789012:function:HelloWorld", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Printf("Executing HelloWorld with input: %v\n", input)

		// Transform input
		if m, ok := input.(map[string]interface{}); ok {
			m["message"] = "Hello, World!"
			m["processed"] = true
			m["timestamp"] = time.Now().Format(time.RFC3339)
			return m, nil
		}
		return input, nil
	})

	// Register a handler with business logic
	mockExecCtx.RegisterHandler("arn:aws:states:::payment:process", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Printf("Processing payment with input: %v\n", input)

		payment, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid payment input")
		}

		// Validate required fields
		if amount, ok := payment["amount"].(float64); !ok || amount <= 0 {
			return nil, fmt.Errorf("invalid amount")
		}

		// Process payment
		payment["status"] = "COMPLETED"
		payment["transaction_id"] = fmt.Sprintf("TXN-%d", time.Now().UnixNano())
		payment["processed_at"] = time.Now().Format(time.RFC3339)

		return payment, nil
	})

	// Register a handler that returns errors for testing retry/catch
	mockExecCtx.RegisterHandler("arn:aws:lambda:::error:generator", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Printf("Generating error with input: %v\n", input)

		if m, ok := input.(map[string]interface{}); ok && m["should_fail"] == true {
			return nil, fmt.Errorf("States.TaskFailed: Task execution failed")
		}

		return map[string]interface{}{
			"status": "success",
		}, nil
	})

	// Register a timeout handler
	mockExecCtx.RegisterHandler("arn:aws:lambda:::slow:operation", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Printf("Starting slow operation\n")

		select {
		case <-time.After(2 * time.Second):
			fmt.Printf("Slow operation completed\n")
			return map[string]interface{}{
				"status": "slow_success",
			}, nil
		case <-ctx.Done():
			fmt.Printf("Slow operation cancelled: %v\n", ctx.Err())
			return nil, ctx.Err()
		}
	})

	// Add execution context to the test context
	ctx = states.WithExecutionContext(ctx, mockExecCtx)

	// Create a default task handler
	handler := states.NewDefaultTaskHandler()

	// Test 1: Basic task execution with registered handler
	t.Run("BasicTaskExecution", func(t *testing.T) {
		input := map[string]interface{}{
			"name": "Test User",
			"age":  30,
		}

		result, err := handler.Execute(ctx, "arn:aws:lambda:us-east-1:123456789012:function:HelloWorld", input, nil)
		require.NoError(t, err)
		require.NotNil(t, result)

		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok)

		assert.Equal(t, "Hello, World!", resultMap["message"])
		assert.Equal(t, true, resultMap["processed"])
		assert.Contains(t, resultMap, "timestamp")
	})

	// Test 2: Task execution with parameters
	t.Run("TaskWithParameters", func(t *testing.T) {
		input := map[string]interface{}{
			"payment": map[string]interface{}{
				"amount":   100.50,
				"currency": "USD",
			},
			"customer": map[string]interface{}{
				"id":   "cust_123",
				"name": "John Doe",
			},
		}

		parameters := map[string]interface{}{
			"amount":    "$.payment.amount",
			"currency":  "$.payment.currency",
			"customer":  "$.customer",
			"reference": "test-123",
		}

		result, err := handler.Execute(ctx, "arn:aws:states:::payment:process", input, parameters)
		require.NoError(t, err)

		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok)

		assert.Equal(t, 100.50, resultMap["amount"])
		assert.Equal(t, "USD", resultMap["currency"])
		assert.Equal(t, "COMPLETED", resultMap["status"])
		assert.Contains(t, resultMap, "transaction_id")
	})

	// Test 3: Task with timeout
	t.Run("TaskWithTimeout", func(t *testing.T) {
		timeout := 1 // 1 second timeout

		start := time.Now()
		result, err := handler.ExecuteWithTimeout(ctx, "arn:aws:lambda:::slow:operation", nil, nil, &timeout)
		elapsed := time.Since(start)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "context deadline exceeded")
		assert.Less(t, elapsed, 1500*time.Millisecond)   // Should timeout before 1.5s
		assert.Greater(t, elapsed, 900*time.Millisecond) // But after 0.9s
	})

	// Test 4: Fallback behavior (no handler registered)
	t.Run("FallbackToDefault", func(t *testing.T) {
		input := map[string]interface{}{
			"test": "data",
		}

		// This resource doesn't have a handler registered
		result, err := handler.Execute(ctx, "arn:aws:lambda:::unknown:function", input, nil)
		require.NoError(t, err)
		assert.Equal(t, input, result) // Should return input as-is
	})

	// Test 5: Error handling
	t.Run("ErrorHandling", func(t *testing.T) {
		input := map[string]interface{}{
			"should_fail": true,
		}

		result, err := handler.Execute(ctx, "arn:aws:lambda:::error:generator", input, nil)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "Task execution failed")
	})

	// Test 6: Success case for error generator
	t.Run("ErrorGeneratorSuccess", func(t *testing.T) {
		input := map[string]interface{}{
			"should_fail": false,
		}

		result, err := handler.Execute(ctx, "arn:aws:lambda:::error:generator", input, nil)
		require.NoError(t, err)

		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "success", resultMap["status"])
	})

	// Test 7: Without execution context in context
	t.Run("NoExecutionContext", func(t *testing.T) {
		emptyCtx := context.Background() // No execution context
		input := map[string]interface{}{
			"test": "data",
		}

		result, err := handler.Execute(emptyCtx, "arn:aws:lambda:::any:function", input, nil)
		require.NoError(t, err)
		assert.Equal(t, input, result) // Should fall back to returning input as-is
	})
}

// TestTaskStateIntegration tests the full TaskState integration
func TestTaskStateIntegration(t *testing.T) {
	ctx := context.Background()

	// Create mock execution context
	mockExecCtx := NewMockExecutionContext()
	mockExecCtx.RegisterHandler("arn:aws:lambda:function:TransformData", func(ctx context.Context, input interface{}) (interface{}, error) {
		data, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid input")
		}

		data["transformed"] = true
		data["processed_at"] = time.Now().Unix()
		return data, nil
	})

	// Create task state
	taskState := &states.TaskState{
		BaseState: states.BaseState{
			Name:       "ProcessData",
			Next:       states.StringPtr("NextState"),
			ResultPath: states.StringPtr("$.result"),
			OutputPath: states.StringPtr("$.result"),
		},
		Resource: "arn:aws:lambda:function:TransformData",
		Parameters: map[string]interface{}{
			"data": "$",
			"id":   "$.id",
		},
	}

	// Add execution context to context
	ctx = states.WithExecutionContext(ctx, mockExecCtx)

	// Test input
	input := map[string]interface{}{
		"id":   "test-123",
		"data": []interface{}{1, 2, 3},
	}

	// Execute task state
	result, nextState, err := taskState.Execute(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, nextState)
	assert.Equal(t, "NextState", *nextState)

	// Verify result
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, true, resultMap["transformed"])
	assert.Contains(t, resultMap, "processed_at")
	assert.Contains(t, resultMap, "id")
	assert.Contains(t, resultMap, "data")
}

// TestTaskRetryLogic demonstrates retry and catch functionality
func TestTaskRetryLogic(t *testing.T) {
	ctx := context.Background()

	// Create mock execution context
	mockExecCtx := NewMockExecutionContext()

	callCount := 0
	mockExecCtx.RegisterHandler("arn:aws:lambda:function:FlakyService", func(ctx context.Context, input interface{}) (interface{}, error) {
		callCount++
		fmt.Printf("FlakyService called %d times\n", callCount)

		if callCount < 3 {
			return nil, fmt.Errorf("States.TaskFailed")
		}

		return map[string]interface{}{
			"status":    "success",
			"attempts":  callCount,
			"finalized": true,
		}, nil
	})

	// Create task state with retry policy
	maxAttempts := 3
	intervalSeconds := 1
	backoffRate := 2.0

	taskState := &states.TaskState{
		BaseState: states.BaseState{
			Name: "FlakyTask",
		},
		Resource: "arn:aws:lambda:function:FlakyService",
		Retry: []states.RetryPolicy{
			{
				ErrorEquals:     []string{"States.TaskFailed"},
				IntervalSeconds: &intervalSeconds,
				MaxAttempts:     &maxAttempts,
				BackoffRate:     &backoffRate,
			},
		},
		Catch: []states.CatchPolicy{
			{
				ErrorEquals: []string{"States.TaskFailed"},
				Next:        "HandleFailure",
			},
		},
	}

	// Add execution context to context
	ctx = states.WithExecutionContext(ctx, mockExecCtx)

	// Execute task state
	start := time.Now()
	result, nextState, err := taskState.Execute(ctx, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, nextState) // Should succeed after retries

	// Should have retried 3 times
	assert.Equal(t, 3, callCount)

	// Should have taken around 1s + 2s = 3s for retries with backoff
	assert.Greater(t, elapsed, 2500*time.Millisecond)
	assert.Less(t, elapsed, 7000*time.Millisecond)

	// Verify result
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "success", resultMap["status"])
	assert.Equal(t, int(3), resultMap["attempts"]) // JSON numbers are float64
}

// TestTaskCatchLogic demonstrates catch functionality
func TestTaskCatchLogic(t *testing.T) {
	ctx := context.Background()

	// Create mock execution context
	mockExecCtx := NewMockExecutionContext()

	callCount := 0
	mockExecCtx.RegisterHandler("arn:aws:lambda:function:AlwaysFails", func(ctx context.Context, input interface{}) (interface{}, error) {
		callCount++
		return nil, fmt.Errorf("States.TaskFailed")
	})

	// Create task state with catch policy
	maxAttempts := 2
	taskState := &states.TaskState{
		BaseState: states.BaseState{
			Name: "FailingTask",
		},
		Resource: "arn:aws:lambda:function:AlwaysFails",
		Retry: []states.RetryPolicy{
			{
				ErrorEquals: []string{"States.TaskFailed"},
				MaxAttempts: &maxAttempts,
			},
		},
		Catch: []states.CatchPolicy{
			{
				ErrorEquals: []string{"States.TaskFailed"},
				ResultPath:  states.StringPtr("$.error"),
				Next:        "ErrorHandler",
			},
		},
	}

	// Add execution context to context
	ctx = states.WithExecutionContext(ctx, mockExecCtx)

	// Execute task state
	input := map[string]interface{}{
		"original": "data",
	}

	result, nextState, err := taskState.Execute(ctx, input)
	require.NoError(t, err) // No error because catch handled it
	require.NotNil(t, result)
	require.NotNil(t, nextState)
	assert.Equal(t, "ErrorHandler", *nextState)

	// Should have been called 3 times (initial + 2 retries)
	assert.Equal(t, 3, callCount)

	// Verify result contains error info
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)

	assert.Contains(t, resultMap, "original")
	assert.Contains(t, resultMap, "error")

	errorInfo, ok := resultMap["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, errorInfo, "Error")
	assert.Contains(t, errorInfo, "Cause")
}

// Demo function showing real-world usage
func TestExampleTaskExecutor(t *testing.T) {
	ctx := context.Background()

	// 1. Create execution context and register handlers
	mockExecCtx := NewMockExecutionContext()

	// Register a payment processor
	mockExecCtx.RegisterHandler("arn:aws:states:::payment:process", func(ctx context.Context, input interface{}) (interface{}, error) {
		payment := input.(map[string]interface{})

		// Business logic here
		payment["status"] = "processed"
		payment["transaction_id"] = fmt.Sprintf("TXN-%d", time.Now().UnixNano())

		return payment, nil
	})

	// Register an email sender
	mockExecCtx.RegisterHandler("arn:aws:states:::email:send", func(ctx context.Context, input interface{}) (interface{}, error) {
		email := input.(map[string]interface{})

		// Send email logic here
		fmt.Printf("Sending email to: %s\n", email["to"])
		email["sent"] = true
		email["sent_at"] = time.Now().Format(time.RFC3339)

		return email, nil
	})

	// 2. Add execution context to context
	ctx = states.WithExecutionContext(ctx, mockExecCtx)

	// 3. Create task handler
	handler := states.NewDefaultTaskHandler()

	// 4. Execute tasks
	paymentInput := map[string]interface{}{
		"amount":   99.99,
		"currency": "USD",
		"customer": "john@example.com",
	}

	result, err := handler.Execute(ctx, "arn:aws:states:::payment:process", paymentInput, nil)
	if err != nil {
		fmt.Printf("Payment failed: %v\n", err)
		return
	}

	fmt.Printf("Payment result: %v\n", result)

	// 5. Chain another task
	emailInput := map[string]interface{}{
		"to":      "john@example.com",
		"subject": "Payment Confirmation",
		"body":    "Your payment was successful!",
	}

	emailResult, err := handler.Execute(ctx, "arn:aws:states:::email:send", emailInput, nil)
	if err != nil {
		fmt.Printf("Email failed: %v\n", err)
		return
	}

	fmt.Printf("Email result: %v\n", emailResult)

	// Output would show the processed results
}

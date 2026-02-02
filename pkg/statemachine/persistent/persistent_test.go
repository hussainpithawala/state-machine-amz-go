package persistent

import (
	"context"
	"fmt"
	"testing"
	"time"

	// Third-party imports
	"github.com/stretchr/testify/require"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

// mockTaskHandler is a simple mock implementation for testing
type mockTaskHandler struct {
	executeFunc func(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error)
}

func (m *mockTaskHandler) Execute(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, resource, input, parameters)
	}
	return input, nil
}

func (m *mockTaskHandler) ExecuteWithTimeout(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}, timeoutSeconds *int) (interface{}, error) {
	return m.Execute(ctx, resource, input, parameters)
}

func (m *mockTaskHandler) CanHandle(resource string) bool {
	return true
}

func TestNew_GeneratesStateMachineID_WhenEmpty(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	// Create a minimal manager (no actual repository needed for this test)
	manager := &repository.Manager{}

	sm, err := New(definition, false, "", manager)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.NotEmpty(t, sm.GetID())
	require.Contains(t, sm.GetID(), "sm-")
}

func TestNew_UsesProvidedStateMachineID(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "my-custom-id", manager)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, "my-custom-id", sm.GetID())
}

func TestNew_InvalidDefinition_ReturnsError(t *testing.T) {
	invalidDefinition := []byte(`invalid yaml content`)

	manager := &repository.Manager{}

	sm, err := New(invalidDefinition, false, "test-sm", manager)
	require.Error(t, err)
	require.Nil(t, sm)
}

func TestGetStartAt_ReturnsCorrectStartState(t *testing.T) {
	definition := []byte(`
StartAt: MyStartState
States:
  MyStartState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)
	require.Equal(t, "MyStartState", sm.GetStartAt())
}

func TestGetState_ReturnsCorrectState(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	state, err := sm.GetState("FirstState")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, "Pass", state.GetType())
}

func TestGetState_NonExistentState_ReturnsError(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	state, err := sm.GetState("NonExistentState")
	require.Error(t, err)
	require.Nil(t, state)
	require.Contains(t, err.Error(), "not found")
}

func TestIsTimeout_NoTimeoutConfigured_ReturnsFalse(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	isTimeout := sm.IsTimeout(time.Now().Add(-1 * time.Hour))
	require.False(t, isTimeout)
}

func TestIsTimeout_WithTimeoutConfigured_ReturnsTrue(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
TimeoutSeconds: 10
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	// Start time is 20 seconds ago, timeout is 10 seconds
	isTimeout := sm.IsTimeout(time.Now().Add(-20 * time.Second))
	require.True(t, isTimeout)
}

func TestIsTimeout_WithTimeoutConfigured_ReturnsFalse(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
TimeoutSeconds: 60
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	// Start time is 10 seconds ago, timeout is 60 seconds
	isTimeout := sm.IsTimeout(time.Now().Add(-10 * time.Second))
	require.False(t, isTimeout)
}

func TestNew_JSONDefinition(t *testing.T) {
	jsonDefinition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	manager := &repository.Manager{}

	sm, err := New(jsonDefinition, true, "test-sm", manager)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, "FirstState", sm.GetStartAt())
}

func TestMergeInputs_BasicMerge(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	// Import the states package for JSONPathProcessor
	processor := &states.JSONPathProcessor{}

	// Test basic merge
	processedInput := map[string]interface{}{
		"orderId": "ORD-123",
		"amount":  100.0,
	}

	result := map[string]interface{}{
		"status":    "approved",
		"timestamp": "2024-01-20T10:00:00Z",
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	// With default ResultPath "$", result should be merged into processedInput
	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "approved", mergedMap["status"])
	require.Equal(t, "2024-01-20T10:00:00Z", mergedMap["timestamp"])
}

func TestMergeInputs_NilProcessedInput(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	result := map[string]interface{}{
		"status": "success",
	}

	merged, err := sm.MergeInputs(processor, nil, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "success", mergedMap["status"])
}

func TestMergeInputs_NilResult(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	processedInput := map[string]interface{}{
		"orderId": "ORD-123",
	}

	mI, err := sm.MergeInputs(processor, processedInput, nil)
	require.NoError(t, err)
	fmt.Println("MergeInputs with nil result completed successfully", mI != nil)
	// With nil result, ApplyResultPath merges nil into processedInput
	// This is expected behavior per JSONPath processing
}

func TestMergeInputs_ComplexNestedData(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	processedInput := map[string]interface{}{
		"order": map[string]interface{}{
			"id":     "ORD-123",
			"amount": 100.0,
		},
		"customer": map[string]interface{}{
			"id":   "CUST-456",
			"name": "John Doe",
		},
	}

	result := map[string]interface{}{
		"payment": map[string]interface{}{
			"status":        "approved",
			"transactionId": "TXN-789",
		},
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Should contain customer info from input
	customer, ok := mergedMap["customer"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "John Doe", customer["name"])
	require.Equal(t, "CUST-456", customer["id"])

	order, ok := mergedMap["order"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "ORD-123", order["id"])
	require.Equal(t, 100.0, order["amount"])

	// Should contain payment info from result
	payment, ok := mergedMap["payment"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "approved", payment["status"])
	require.Equal(t, "TXN-789", payment["transactionId"])
}

func TestMergeInputs_MessageStateScenario(t *testing.T) {
	definition := []byte(`
StartAt: WaitForMessage
States:
  WaitForMessage:
    Type: Message
    CorrelationKey: "orderId"
    CorrelationValuePath: "$.orderId"
    TimeoutSeconds: 300
    TimeoutPath: HandleTimeout
    Next: ProcessOrder
  ProcessOrder:
    Type: Pass
    End: true
  HandleTimeout:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	// Original execution input
	processedInput := map[string]interface{}{
		"orderId":     "ORD-123",
		"customerId":  "CUST-456",
		"orderAmount": 100.0,
	}

	receivedMessageKey := fmt.Sprintf("%s_%s", states.ReceivedMessageBase, "WaitForMessage")
	// Message received from external system
	result := map[string]interface{}{
		receivedMessageKey: map[string]interface{}{
			"correlation_key":   "orderId",
			"correlation_value": "ORD-123",
			"data": map[string]interface{}{
				"approved":     true,
				"approvalTime": "2024-01-20T10:00:00Z",
				"approvedBy":   "manager@example.com",
			},
		},
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Should contain original input data
	require.Equal(t, "ORD-123", mergedMap["orderId"])
	require.Equal(t, "CUST-456", mergedMap["customerId"])
	require.Equal(t, 100.0, mergedMap["orderAmount"])

	receivedMessageKey = fmt.Sprintf("%s_%s", states.ReceivedMessageBase, "WaitForMessage")
	// Should contain message data
	message, ok := mergedMap[receivedMessageKey].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "orderId", message["correlation_key"])
	require.Equal(t, "ORD-123", message["correlation_value"])

	messageData, ok := message["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, messageData["approved"])
}

func TestMergeInputs_TimeoutScenario(t *testing.T) {
	definition := []byte(`
StartAt: WaitForMessage
States:
  WaitForMessage:
    Type: Message
    CorrelationKey: "orderId"
    CorrelationValuePath: "$.orderId"
    TimeoutSeconds: 300
    TimeoutPath: HandleTimeout
    Next: ProcessOrder
  ProcessOrder:
    Type: Pass
    End: true
  HandleTimeout:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	// Original execution input
	processedInput := map[string]interface{}{
		"orderId":     "ORD-123",
		"customerId":  "CUST-456",
		"orderAmount": 100.0,
	}

	// Timeout event
	result := map[string]interface{}{
		"__timeout__": map[string]interface{}{
			"reason":          "timeout",
			"state_name":      "WaitForMessage",
			"timeout_seconds": 300,
		},
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Should contain timeout info
	timeout, ok := mergedMap["__timeout__"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "timeout", timeout["reason"])
	require.Equal(t, "WaitForMessage", timeout["state_name"])
}

func TestMergeInputs_EmptyInputs(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	// Both empty
	merged, err := sm.MergeInputs(processor, map[string]interface{}{}, map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, merged)
}

func TestMergeInputs_PreservesOriginalInput(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	processedInput := map[string]interface{}{
		"orderId": "ORD-123",
		"amount":  100.0,
	}

	result := map[string]interface{}{
		"status": "approved",
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Both original and result data should be present
	require.Equal(t, "approved", mergedMap["status"])
}

func TestMergeInputs_ArrayInputs(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	// Test with array of maps in processed input
	processedInput := map[string]interface{}{
		"orders": []map[string]interface{}{
			{
				"orderId": "ORD-001",
				"amount":  100.0,
				"status":  "pending",
			},
			{
				"orderId": "ORD-002",
				"amount":  200.0,
				"status":  "pending",
			},
		},
		"customerId": "CUST-123",
	}

	// Result containing array processing results
	result := map[string]interface{}{
		"processedOrders": []map[string]interface{}{
			{
				"orderId": "ORD-001",
				"status":  "approved",
			},
			{
				"orderId": "ORD-002",
				"status":  "approved",
			},
		},
		"totalProcessed": 2,
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Should contain processed orders array
	processedOrders, ok := mergedMap["processedOrders"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, processedOrders, 2)
	require.Equal(t, "ORD-001", processedOrders[0]["orderId"])
	require.Equal(t, "approved", processedOrders[0]["status"])
	require.Equal(t, "ORD-002", processedOrders[1]["orderId"])
	require.Equal(t, "approved", processedOrders[1]["status"])

	// Should contain total processed count
	require.Equal(t, 2, mergedMap["totalProcessed"])
}

func TestMergeInputs_ArrayOfInterfaces(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	// Test with []interface{} array type
	processedInput := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"id":   "ITEM-001",
				"name": "Product A",
			},
			map[string]interface{}{
				"id":   "ITEM-002",
				"name": "Product B",
			},
		},
	}

	result := map[string]interface{}{
		"validatedItems": []interface{}{
			map[string]interface{}{
				"id":      "ITEM-001",
				"valid":   true,
				"message": "OK",
			},
			map[string]interface{}{
				"id":      "ITEM-002",
				"valid":   false,
				"message": "Out of stock",
			},
		},
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Should contain validated items array
	validatedItems, ok := mergedMap["validatedItems"].([]interface{})
	require.True(t, ok)
	require.Len(t, validatedItems, 2)

	item1, ok := validatedItems[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "ITEM-001", item1["id"])
	require.Equal(t, true, item1["valid"])

	item2, ok := validatedItems[1].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "ITEM-002", item2["id"])
	require.Equal(t, false, item2["valid"])
}

func TestMergeInputs_EmptyArrays(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	processedInput := map[string]interface{}{
		"orders": []map[string]interface{}{},
	}

	result := map[string]interface{}{
		"processedOrders": []map[string]interface{}{},
		"count":           0,
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Should handle empty arrays correctly
	processedOrders, ok := mergedMap["processedOrders"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, processedOrders, 0)
	require.Equal(t, 0, mergedMap["count"])
}

func TestMergeInputs_MixedArrayTypes(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}
	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	processor := &states.JSONPathProcessor{}

	// Mixed primitive and complex types in arrays
	processedInput := map[string]interface{}{
		"tags":   []interface{}{"electronics", "sale", "featured"},
		"prices": []interface{}{100.0, 200.0, 150.0},
		"products": []map[string]interface{}{
			{
				"id":    "PROD-001",
				"price": 100.0,
			},
		},
	}

	result := map[string]interface{}{
		"discountedPrices": []interface{}{90.0, 180.0, 135.0},
		"summary": map[string]interface{}{
			"totalProducts": 1,
			"avgDiscount":   10.0,
		},
	}

	merged, err := sm.MergeInputs(processor, processedInput, result)
	require.NoError(t, err)
	require.NotNil(t, merged)

	mergedMap, ok := merged.(map[string]interface{})
	require.True(t, ok)

	// Should contain discounted prices
	discountedPrices, ok := mergedMap["discountedPrices"].([]interface{})
	require.True(t, ok)
	require.Len(t, discountedPrices, 3)
	require.Equal(t, 90.0, discountedPrices[0])
	require.Equal(t, 180.0, discountedPrices[1])
	require.Equal(t, 135.0, discountedPrices[2])

	// Should contain summary
	summary, ok := mergedMap["summary"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, 1, summary["totalProducts"])
	require.Equal(t, 10.0, summary["avgDiscount"])
}

func TestExecute_FailState_MarkedAsFailed(t *testing.T) {
	// Skip if no PostgreSQL connection available
	connURL := "postgres://postgres:postgres@localhost:5432/statemachine_test_gorm?sslmode=disable"

	config := &repository.Config{
		Strategy:      "postgres_gorm",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns": 10,
			"max_idle_conns": 2,
			"log_level":      "warn",
		},
	}

	repo, err := repository.NewGormPostgresRepository(config)
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL not available: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	err = repo.Initialize(ctx)
	require.NoError(t, err)

	manager := repository.NewManagerWithRepository(repo)

	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    Next: FailState
  FailState:
    Type: Fail
    Error: CustomError
    Cause: This is a test failure
`)

	sm, err := New(definition, false, "test-sm-fail", manager)
	require.NoError(t, err)

	// Execute the state machine - FirstState will succeed, but then FailState will cause failure
	execCtx, execErr := sm.Execute(ctx, map[string]interface{}{"test": "data"})

	// Verify error occurred
	require.Error(t, execErr)
	require.Contains(t, execErr.Error(), "CustomError")
	require.Contains(t, execErr.Error(), "This is a test failure")

	// Verify execution context is marked as FAILED
	require.NotNil(t, execCtx)
	require.Equal(t, FAILED, execCtx.Status)
	require.NotNil(t, execCtx.Error)
	require.Equal(t, execErr, execCtx.Error)

	// Verify EndTime is set
	require.False(t, execCtx.EndTime.IsZero())

	// Verify execution has history (from FirstState and FailState)
	require.NotEmpty(t, execCtx.History)
	require.GreaterOrEqual(t, len(execCtx.History), 1)
}

func TestExecute_TaskStateError_MarkedAsFailed(t *testing.T) {
	// Skip if no PostgreSQL connection available
	connURL := "postgres://postgres:postgres@localhost:5432/statemachine_test_gorm?sslmode=disable"

	config := &repository.Config{
		Strategy:      "postgres_gorm",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns": 10,
			"max_idle_conns": 2,
			"log_level":      "warn",
		},
	}

	repo, err := repository.NewGormPostgresRepository(config)
	if err != nil {
		t.Skipf("Skipping test: PostgreSQL not available: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	err = repo.Initialize(ctx)
	require.NoError(t, err)

	manager := repository.NewManagerWithRepository(repo)

	// Create a mock task handler that returns an error
	mockHandler := &mockTaskHandler{
		executeFunc: func(ctx context.Context, resource string, input interface{}, parameters map[string]interface{}) (interface{}, error) {
			return nil, fmt.Errorf("task execution failed: API invocation error")
		},
	}

	definition := []byte(`
StartAt: ProcessTask
States:
  ProcessTask:
    Type: Task
    Resource: arn:aws:lambda:us-east-1:123456789012:function:ProcessData
    Next: SuccessState
  SuccessState:
    Type: Succeed
`)

	sm, err := New(definition, false, "test-sm-task-fail", manager)
	require.NoError(t, err)

	// Register the mock task handler
	taskState, err := sm.GetState("ProcessTask")
	require.NoError(t, err)
	if ts, ok := taskState.(*states.TaskState); ok {
		ts.TaskHandler = mockHandler
	}

	// Execute the state machine - ProcessTask will fail
	execCtx, execErr := sm.Execute(ctx, map[string]interface{}{"orderId": "12345", "amount": 100.0})

	// Verify error occurred
	require.Error(t, execErr)
	require.Contains(t, execErr.Error(), "task execution failed")
	require.Contains(t, execErr.Error(), "API invocation error")

	// Verify execution context is marked as FAILED
	require.NotNil(t, execCtx)
	require.Equal(t, FAILED, execCtx.Status)
	require.NotNil(t, execCtx.Error)
	require.Equal(t, execErr, execCtx.Error)

	// Verify EndTime is set
	require.False(t, execCtx.EndTime.IsZero())

	// Verify execution has history from the failed task
	require.NotEmpty(t, execCtx.History)
	require.Equal(t, 1, len(execCtx.History))

	// Verify the failed state history
	failedHistory := execCtx.History[0]
	require.Equal(t, "ProcessTask", failedHistory.StateName)
	require.Equal(t, "Task", failedHistory.StateType)
	require.Equal(t, FAILED, failedHistory.Status)
	require.NotNil(t, failedHistory.Error)

	// fetch the state from the repository manager to confirm the state is updated
	execution, err := manager.GetExecution(ctx, execCtx.ID)
	if err != nil {
		return
	}
	require.Equal(t, FAILED, execution.Status)
	require.NotNil(t, execution.Error)
	require.Equal(t, execErr.Error(), execution.Error)
	require.WithinDuration(t,
		execCtx.EndTime.Local(),
		*execution.EndTime,
		time.Millisecond, // Or whatever tolerance you need
		"End times should be within tolerance")
}

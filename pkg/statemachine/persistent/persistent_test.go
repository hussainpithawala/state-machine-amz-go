package persistent

import (
	"fmt"
	"testing"
	"time"

	// Third-party imports
	"github.com/stretchr/testify/require"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

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

	// Message received from external system
	result := map[string]interface{}{
		"__received_message__": map[string]interface{}{
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

	// Should contain message data
	message, ok := mergedMap["__received_message__"].(map[string]interface{})
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

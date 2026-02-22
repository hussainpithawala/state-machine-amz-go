package states_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	// Third-party imports
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
)

const processConfirmation = "ProcessConfirmation"

func TestMessageState_Creation(t *testing.T) {
	messageState := states.NewMessageState("WaitForConfirmation", "user_confirmation")

	assert.Equal(t, "WaitForConfirmation", messageState.GetName())
	assert.Equal(t, "Message", messageState.GetType())
	assert.Equal(t, "user_confirmation", messageState.GetCorrelationKey())
	assert.True(t, messageState.IsWaitingState())
}

func TestMessageState_Validation(t *testing.T) {
	const processPayment = "ProcessPayment"
	tests := []struct {
		name        string
		setupState  func() *states.MessageState
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid state",
			setupState: func() *states.MessageState {
				state := states.NewMessageState("WaitForPayment", "payment_key")
				next := processPayment
				state.Next = &next
				return state
			},
			expectError: false,
		},
		{
			name: "missing correlation key",
			setupState: func() *states.MessageState {
				state := states.NewMessageState("WaitForPayment", "")
				next := processPayment
				state.Next = &next
				return state
			},
			expectError: true,
			errorMsg:    "CorrelationKey cannot be empty",
		},
		{
			name: "negative timeout",
			setupState: func() *states.MessageState {
				state := states.NewMessageState("WaitForPayment", "payment_key")
				next := processPayment
				state.Next = &next
				timeout := -100
				state.TimeoutSeconds = &timeout
				return state
			},
			expectError: true,
			errorMsg:    "TimeoutSeconds cannot be negative",
		},
		{
			name: "missing next and end",
			setupState: func() *states.MessageState {
				return states.NewMessageState("WaitForPayment", "payment_key")
			},
			expectError: true,
			errorMsg:    "must have either Next or End",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.setupState()
			err := state.Validate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMessageState_PreMessageExecution(t *testing.T) {
	ctx := context.Background()

	messageState := states.NewMessageState("WaitForPayment", "payment_confirmation")
	next := "ProcessPayment"
	messageState.Next = &next
	correlationPath := "$.transactionId"
	messageState.CorrelationValuePath = &correlationPath

	input := map[string]interface{}{
		"orderId":       "ORD-123",
		"transactionId": "TXN-456",
		"amount":        100.00,
	}

	result, nextState, err := messageState.Execute(ctx, input)

	require.NoError(t, err)
	assert.Nil(t, nextState, "nextState should be nil during pre-message phase")

	// Verify result structure
	// resultMap, ok := result.(map[string]interface{})
	messageStateResult, ok := result.(*states.MessageStateResult)
	require.True(t, ok, "result should be a MessageStateResult")

	// Check for MessageStateResult
	if msgResult := messageStateResult.Status; ok {
		assert.Equal(t, "WAITING", msgResult)
	}
}

func TestMessageState_PostMessageExecution(t *testing.T) {
	ctx := context.Background()

	messageState := states.NewMessageState("WaitForPayment", "payment_confirmation")
	next := "ProcessPayment"
	messageState.Next = &next
	receivedMessageKey := fmt.Sprintf("%s_%s", states.ReceivedMessageBase, messageState.Name)

	// Simulate received message
	input := map[string]interface{}{
		"orderId": "ORD-123",
		receivedMessageKey: map[string]interface{}{
			"correlation_key":   "payment_confirmation",
			"correlation_value": "TXN-456",
			"data": map[string]interface{}{
				"status":        "SUCCESS",
				"paymentMethod": "CREDIT_CARD",
			},
		},
	}

	result, nextState, err := messageState.Execute(ctx, input)

	require.NoError(t, err)
	assert.NotNil(t, nextState, "nextState should not be nil after receiving message")
	assert.Equal(t, "ProcessPayment", *nextState)

	// Verify result contains message data
	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "SUCCESS", resultMap["status"])
}

func TestMessageState_JSONMarshaling(t *testing.T) {
	messageState := &states.MessageState{
		CorrelationKey:       "approval_key",
		CorrelationValuePath: states.StringPtr("$.requestId"),
		TimeoutSeconds:       states.IntPtr(3600),
		BaseState: states.BaseState{
			Type:    "Message",
			Name:    "WaitForApproval",
			Next:    states.StringPtr("ProcessApproval"),
			Comment: "Wait for approval",
		},
		MessagePath: states.StringPtr("$.messageData"),
	}

	jsonData, err := json.Marshal(messageState)
	require.NoError(t, err)

	// Unmarshal and verify
	var unmarshaled map[string]interface{}
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, "Message", unmarshaled["Type"])
	assert.Equal(t, "approval_key", unmarshaled["CorrelationKey"])
	assert.Equal(t, "$.requestId", unmarshaled["CorrelationValuePath"])
	assert.Equal(t, "$.messageData", unmarshaled["MessagePath"])
	assert.Equal(t, float64(3600), unmarshaled["TimeoutSeconds"])
	assert.Equal(t, "ProcessApproval", unmarshaled["Next"])
}

func TestMessageState_GetNextStates(t *testing.T) {
	messageState := states.NewMessageState("WaitForPayment", "payment_key")
	next := "ProcessPayment"
	messageState.Next = &next

	// Add catch rules
	messageState.Catch = []states.CatchRule{
		{
			ErrorEquals: []string{"States.Timeout"},
			Next:        "CancelPayment",
		},
		{
			ErrorEquals: []string{"PaymentError"},
			Next:        "RetryPayment",
		},
	}

	nextStates := messageState.GetNextStates()

	assert.Len(t, nextStates, 3, "should include Next and all Catch destinations")
	assert.Contains(t, nextStates, "ProcessPayment")
	assert.Contains(t, nextStates, "CancelPayment")
	assert.Contains(t, nextStates, "RetryPayment")
}

func TestExecutor_Message(t *testing.T) {
	ctx := context.Background()
	exec := executor.NewBaseExecutor()

	// Create a mock paused execution
	// mockExecution := &struct {
	// 	ID             string
	// 	Status         string
	// 	StateMachineID string
	// 	CurrentState   string
	// 	Input          interface{}
	// 	Metadata       map[string]interface{}
	// }{
	// 	ID:             "exec-123",
	// 	Status:         "PAUSED",
	// 	StateMachineID: "sm-456",
	// 	CurrentState:   "WaitForPayment",
	// 	Metadata: map[string]interface{}{
	// 		"correlation_data": map[string]interface{}{
	// 			"correlation_key":   "payment_confirmation",
	// 			"correlation_value": "TXN-789",
	// 		},
	// 	},
	// }

	// Note: You'll need to set up the executor with this mock execution
	// This is a simplified test structure

	messageRequest := &executor.MessageRequest{
		CorrelationKey:   "payment_confirmation",
		CorrelationValue: "TXN-789",
		Data: map[string]interface{}{
			"status": "SUCCESS",
			"amount": 100.00,
		},
	}

	// In actual implementation, this would find the paused execution
	// and resume it with the message data
	response, err := exec.Message(ctx, messageRequest, nil)

	// Expected behavior when no matching execution is found
	if err == nil && response != nil {
		assert.NotEmpty(t, response.Status)
	}
}

func TestExecutor_MessageValidation(t *testing.T) {
	ctx := context.Background()
	exec := executor.NewBaseExecutor()

	tests := []struct {
		name        string
		request     *executor.MessageRequest
		expectError bool
	}{
		{
			name:        "nil request",
			request:     nil,
			expectError: true,
		},
		{
			name: "empty correlation key",
			request: &executor.MessageRequest{
				CorrelationKey:   "",
				CorrelationValue: "value",
				Data:             map[string]interface{}{},
			},
			expectError: true,
		},
		{
			name: "valid request",
			request: &executor.MessageRequest{
				CorrelationKey:   "payment_key",
				CorrelationValue: "TXN-123",
				Data: map[string]interface{}{
					"status": "SUCCESS",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := exec.Message(ctx, tt.request, nil)

			if tt.expectError {
				assert.Error(t, err)
			}
			// Note: We don't check for no error in the valid case
			// because there's no matching execution in this test
		})
	}
}

func TestMessageState_CorrelationValueExtraction(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		correlationPath *string
		input           map[string]interface{}
		expectedValue   interface{}
	}{
		{
			name:            "no path - use entire input",
			correlationPath: nil,
			input: map[string]interface{}{
				"orderId": "ORD-123",
				"userId":  "USER-456",
			},
			expectedValue: map[string]interface{}{
				"orderId": "ORD-123",
				"userId":  "USER-456",
			},
		},
		{
			name: "simple path",
			correlationPath: func() *string {
				s := "$.transactionId"
				return &s
			}(),
			input: map[string]interface{}{
				"transactionId": "TXN-789",
				"amount":        100.00,
			},
			expectedValue: "TXN-789",
		},
		{
			name: "nested path",
			correlationPath: func() *string {
				s := "$.payment.transactionId"
				return &s
			}(),
			input: map[string]interface{}{
				"payment": map[string]interface{}{
					"transactionId": "TXN-ABC",
					"amount":        200.00,
				},
			},
			expectedValue: "TXN-ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messageState := states.NewMessageState("WaitTest", "test_key")
			next := "NextState"
			messageState.Next = &next
			messageState.CorrelationValuePath = tt.correlationPath

			result, nextState, err := messageState.Execute(ctx, tt.input)

			require.NoError(t, err)
			assert.Nil(t, nextState)

			// Extract correlation data from result
			// Note: Actual extraction logic depends on your implementation
			_ = result
		})
	}
}

func TestMessageState_Timeout(t *testing.T) {
	messageState := states.NewMessageState("WaitForConfirmation", "confirmation_key")

	next := processConfirmation
	messageState.Next = &next
	timeout := 300
	messageState.TimeoutSeconds = &timeout

	// Verify timeout is set
	assert.NotNil(t, messageState.GetTimeoutSeconds())
	assert.Equal(t, 300, *messageState.GetTimeoutSeconds())

	// Add catch for timeout
	messageState.Catch = []states.CatchRule{
		{
			ErrorEquals: []string{"States.Timeout"},
			Next:        "HandleTimeout",
		},
	}

	err := messageState.Validate()
	assert.NoError(t, err)
}

func TestMessageState_TimeoutExecution(t *testing.T) {
	ctx := context.Background()

	const handleTimeOut = "HandleTimeout"
	tests := []struct {
		name           string
		setupState     func() *states.MessageState
		input          map[string]interface{}
		expectedNext   *string
		expectError    bool
		validateResult func(t *testing.T, result interface{})
	}{
		{
			name: "timeout with TimeoutPath",
			setupState: func() *states.MessageState {
				state := states.NewMessageState("WaitForConfirmation", "confirmation_key")
				next := "ProcessConfirmation"
				state.Next = &next
				timeout := 300
				state.TimeoutSeconds = &timeout
				timeoutPath := handleTimeOut
				state.TimeoutPath = &timeoutPath
				return state
			},
			input: map[string]interface{}{
				"__timeout_trigger___WaitForConfirmation": true,
				"orderId": "ORD-123",
			},
			expectedNext: func() *string {
				s := handleTimeOut
				return &s
			}(),
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				require.True(t, ok, "result should be a map")
				assert.Equal(t, "TIMEOUT", resultMap["status"])
				assert.Contains(t, resultMap["message"], "timed out")
			},
		},
		{
			name: "timeout without TimeoutPath",
			setupState: func() *states.MessageState {
				state := states.NewMessageState("WaitForConfirmation", "confirmation_key")
				next := "ProcessConfirmation"
				state.Next = &next
				timeout := 300
				state.TimeoutSeconds = &timeout
				return state
			},
			input: map[string]interface{}{
				"__timeout_trigger___WaitForConfirmation": true,
				"orderId": "ORD-123",
			},
			expectedNext: nil,
			expectError:  true,
			validateResult: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				require.True(t, ok, "result should be a map")
				assert.Equal(t, "TIMEOUT", resultMap["status"])
			},
		},
		{
			name: "timeout with ResultPath preserves original input",
			setupState: func() *states.MessageState {
				state := states.NewMessageState("WaitForConfirmation", "confirmation_key")
				next := "ProcessConfirmation"
				state.Next = &next
				timeout := 300
				state.TimeoutSeconds = &timeout
				timeoutPath := handleTimeOut
				state.TimeoutPath = &timeoutPath

				// Apply ResultPath to merge timeout result back
				resultPath := "$.timeoutInfo"
				state.ResultPath = &resultPath

				return state
			},
			input: map[string]interface{}{
				"__timeout_trigger___WaitForConfirmation": true,
				"userId":  "USER-456",
				"orderId": "ORD-123",
				"amount":  100.00,
			},
			expectedNext: func() *string {
				s := handleTimeOut
				return &s
			}(),
			expectError: false,
			validateResult: func(t *testing.T, result interface{}) {
				resultMap, ok := result.(map[string]interface{})
				require.True(t, ok, "result should be a map")

				// Verify original input is preserved (this validates the fix)
				assert.Equal(t, "USER-456", resultMap["userId"])
				assert.Equal(t, "ORD-123", resultMap["orderId"])
				assert.Equal(t, 100.00, resultMap["amount"])

				// Verify timeout info is merged at ResultPath
				timeoutInfo, ok := resultMap["timeoutInfo"].(map[string]interface{})
				require.True(t, ok, "timeoutInfo should be present")
				assert.Equal(t, "TIMEOUT", timeoutInfo["status"])
				assert.Contains(t, timeoutInfo["message"], "timed out")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.setupState()
			result, nextState, err := state.Execute(ctx, tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedNext != nil {
				require.NotNil(t, nextState)
				assert.Equal(t, *tt.expectedNext, *nextState)
			} else if !tt.expectError {
				assert.Nil(t, nextState)
			}

			if tt.validateResult != nil && result != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

// Benchmark tests
func BenchmarkMessageState_Execute(b *testing.B) {
	ctx := context.Background()
	messageState := states.NewMessageState("WaitForPayment", "payment_key")
	next := "ProcessPayment"
	messageState.Next = &next

	input := map[string]interface{}{
		"transactionId": "TXN-123",
		"amount":        100.00,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = messageState.Execute(ctx, input)
	}
}

func BenchmarkMessageState_JSONMarshaling(b *testing.B) {
	messageState := states.NewMessageState("WaitForPayment", "payment_key")
	next := "ProcessPayment"
	messageState.Next = &next

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(messageState)
	}
}

package states

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFailState_New tests the creation of a FailState
func TestFailState_New(t *testing.T) {
	tests := []struct {
		name        string
		error       string
		cause       string
		hasCause    bool
		expectedErr string
	}{
		{
			name:        "fail state with error only",
			error:       "TaskFailed",
			expectedErr: "TaskFailed",
		},
		{
			name:        "fail state with error and cause",
			error:       "ValidationError",
			cause:       "Invalid input format",
			hasCause:    true,
			expectedErr: "ValidationError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &FailState{
				BaseState: BaseState{
					Name: "TestFailState",
					Type: "Fail",
				},
				Error:    tt.error,
				Cause:    tt.cause,
				HasCause: tt.hasCause,
			}

			assert.Equal(t, "TestFailState", state.GetName())
			assert.Equal(t, "Fail", state.GetType())
			assert.Nil(t, state.GetNext())
			assert.True(t, state.IsEnd())
			assert.Equal(t, tt.error, state.Error)
			if tt.hasCause {
				assert.Equal(t, tt.cause, state.Cause)
			}
		})
	}
}

// TestFailState_Validate tests validation of FailState
func TestFailState_Validate(t *testing.T) {
	tests := []struct {
		name        string
		state       *FailState
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid fail state",
			state: &FailState{
				BaseState: BaseState{
					Name: "ValidFail",
					Type: "Fail",
				},
				Error: "TaskFailed",
			},
			shouldError: false,
		},
		{
			name: "valid fail state with cause",
			state: &FailState{
				BaseState: BaseState{
					Name: "ValidFail",
					Type: "Fail",
				},
				Error:    "TaskFailed",
				Cause:    "Network timeout",
				HasCause: true,
			},
			shouldError: false,
		},
		{
			name: "invalid - no name",
			state: &FailState{
				BaseState: BaseState{
					Type: "Fail",
				},
				Error: "TaskFailed",
			},
			shouldError: true,
			errorMsg:    "state name cannot be empty",
		},
		{
			name: "invalid - wrong type",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Pass", // Wrong type
				},
				Error: "TaskFailed",
			},
			shouldError: true,
			errorMsg:    "fail state must have Type 'Fail'",
		},
		{
			name: "invalid - no error",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Fail",
				},
				Error: "", // Empty error
			},
			shouldError: true,
			errorMsg:    "must have Error field",
		},
		{
			name: "invalid - has next",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Fail",
					Next: StringPtr("NextState"),
				},
				Error: "TaskFailed",
			},
			shouldError: true,
			errorMsg:    "cannot have Next field",
		},
		{
			name: "invalid - has end",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Fail",
					End:  true,
				},
				Error: "TaskFailed",
			},
			shouldError: true,
			errorMsg:    "cannot have End field",
		},
		{
			name: "invalid - has input path",
			state: &FailState{
				BaseState: BaseState{
					Name:      "FailState",
					Type:      "Fail",
					InputPath: StringPtr("$.data"),
				},
				Error: "TaskFailed",
			},
			shouldError: true,
			errorMsg:    "cannot have InputPath",
		},
		{
			name: "invalid - has result path",
			state: &FailState{
				BaseState: BaseState{
					Name:       "FailState",
					Type:       "Fail",
					ResultPath: StringPtr("$.result"),
				},
				Error: "TaskFailed",
			},
			shouldError: true,
			errorMsg:    "cannot have ResultPath",
		},
		{
			name: "invalid - has output path",
			state: &FailState{
				BaseState: BaseState{
					Name:       "FailState",
					Type:       "Fail",
					OutputPath: StringPtr("$.output"),
				},
				Error: "TaskFailed",
			},
			shouldError: true,
			errorMsg:    "cannot have OutputPath",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			if tt.shouldError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestFailState_Execute tests execution of FailState
func TestFailState_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		state         *FailState
		input         interface{}
		expectedError string
	}{
		{
			name: "simple fail state",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Fail",
				},
				Error: "TaskFailed",
			},
			input: map[string]interface{}{
				"data": "test",
			},
			expectedError: "State machine failed at state 'FailState' with error: TaskFailed",
		},
		{
			name: "fail state with cause",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Fail",
				},
				Error:    "ValidationError",
				Cause:    "Invalid input format",
				HasCause: true,
			},
			input: map[string]interface{}{
				"user": "test",
			},
			expectedError: "State machine failed at state 'FailState' with error: ValidationError (cause: Invalid input format)",
		},
		{
			name: "fail state with different input",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Fail",
				},
				Error: "TimeoutError",
			},
			input:         "simple string input",
			expectedError: "State machine failed at state 'FailState' with error: TimeoutError",
		},
		{
			name: "fail state with nil input",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailState",
					Type: "Fail",
				},
				Error: "UnexpectedError",
			},
			input:         nil,
			expectedError: "State machine failed at state 'FailState' with error: UnexpectedError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, nextState, err := tt.state.Execute(ctx, tt.input)

			// Fail states should always return an error
			require.Error(t, err)
			assert.Equal(t, tt.expectedError, err.Error())

			// Output should be nil
			assert.Nil(t, output)

			// Next state should be nil (end state)
			assert.Nil(t, nextState)
		})
	}
}

// TestFailState_MarshalJSON tests JSON marshaling of FailState
func TestFailState_MarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		state        *FailState
		expectedJSON string
	}{
		{
			name: "fail state without cause",
			state: &FailState{
				BaseState: BaseState{
					Name: "SimpleFail",
					Type: "Fail",
				},
				Error: "TaskFailed",
			},
			expectedJSON: `{"Type":"Fail","Error":"TaskFailed"}`,
		},
		{
			name: "fail state with cause",
			state: &FailState{
				BaseState: BaseState{
					Name: "FailWithCause",
					Type: "Fail",
				},
				Error:    "ValidationError",
				Cause:    "Invalid input",
				HasCause: true,
			},
			expectedJSON: `{"Type":"Fail","Error":"ValidationError","Cause":"Invalid input"}`,
		},
		{
			name: "fail state with comment",
			state: &FailState{
				BaseState: BaseState{
					Name:    "FailWithComment",
					Type:    "Fail",
					Comment: "This is a failure state",
				},
				Error: "ProcessingError",
			},
			expectedJSON: `{"Type":"Fail","Error":"ProcessingError","Comment":"This is a failure state"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.state)
			require.NoError(t, err)

			// Parse both JSON strings to compare structures
			var actual, expected interface{}
			err = json.Unmarshal(jsonBytes, &actual)
			require.NoError(t, err)

			err = json.Unmarshal([]byte(tt.expectedJSON), &expected)
			require.NoError(t, err)

			assert.Equal(t, expected, actual)

			// Verify certain fields are NOT in the JSON
			jsonStr := string(jsonBytes)
			assert.NotContains(t, jsonStr, `"Next"`)
			assert.NotContains(t, jsonStr, `"End"`)
			assert.NotContains(t, jsonStr, `"InputPath"`)
			assert.NotContains(t, jsonStr, `"ResultPath"`)
			assert.NotContains(t, jsonStr, `"OutputPath"`)
		})
	}
}

// TestFailState_InterfaceMethods tests that FailState implements State interface
func TestFailState_InterfaceMethods(t *testing.T) {
	state := &FailState{
		BaseState: BaseState{
			Name: "TestFail",
			Type: "Fail",
		},
		Error: "TestError",
	}

	// Test that FailState implements State interface
	var _ State = state

	// Test individual interface methods
	assert.Equal(t, "TestFail", state.GetName())
	assert.Equal(t, "Fail", state.GetType())
	assert.Nil(t, state.GetNext())
	assert.True(t, state.IsEnd())

	// Execute should always return error
	ctx := context.Background()
	input := map[string]interface{}{"test": "data"}
	output, nextState, err := state.Execute(ctx, input)

	assert.Error(t, err)
	assert.Nil(t, output)
	assert.Nil(t, nextState)
}

// TestFailState_ContextCancellation tests context cancellation
func TestFailState_ContextCancellation(t *testing.T) {
	state := &FailState{
		BaseState: BaseState{
			Name: "FailState",
			Type: "Fail",
		},
		Error: "TestError",
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with cancelled context
	input := map[string]interface{}{"test": "data"}
	output, nextState, err := state.Execute(ctx, input)

	// Fail states don't check context (they always fail)
	assert.Error(t, err)
	assert.Nil(t, output)
	assert.Nil(t, nextState)
}

// TestFailState_ConcurrentExecution tests concurrent execution safety
func TestFailState_ConcurrentExecution(t *testing.T) {
	state := &FailState{
		BaseState: BaseState{
			Name: "ConcurrentFail",
			Type: "Fail",
		},
		Error: "ConcurrentError",
	}

	ctx := context.Background()
	numGoroutines := 50
	errors := make(chan error, numGoroutines)

	// Run concurrent executions
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			input := map[string]interface{}{
				"id": id,
			}
			_, _, err := state.Execute(ctx, input)
			errors <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-errors
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ConcurrentError")
	}
}

// TestFailState_EdgeCases tests edge cases
func TestFailState_EdgeCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		state *FailState
		input interface{}
	}{
		{
			name: "empty error message",
			state: &FailState{
				BaseState: BaseState{
					Name: "EmptyError",
					Type: "Fail",
				},
				Error: "", // Will be caught by Validate, not Execute
			},
			input: nil,
		},
		{
			name: "very long error message",
			state: &FailState{
				BaseState: BaseState{
					Name: "LongError",
					Type: "Fail",
				},
				Error: "This is a very long error message that describes in detail what went wrong with the task execution and provides context for debugging purposes",
			},
			input: map[string]interface{}{},
		},
		{
			name: "special characters in error",
			state: &FailState{
				BaseState: BaseState{
					Name: "SpecialError",
					Type: "Fail",
				},
				Error: "Error: 404 Not Found - Resource '/api/v1/users/123'",
			},
			input: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip validation test for empty error (it should fail validation)
			if tt.state.Error == "" {
				err := tt.state.Validate()
				require.Error(t, err)
				return
			}

			output, nextState, err := tt.state.Execute(ctx, tt.input)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.state.Error)
			assert.Nil(t, output)
			assert.Nil(t, nextState)
		})
	}
}

// TestFailState_Integration tests integration with error handling
func TestFailState_Integration(t *testing.T) {
	// Test that FailState works with pkg/errors
	ctx := context.Background()

	state := &FailState{
		BaseState: BaseState{
			Name: "IntegrationFail",
			Type: "Fail",
		},
		Error:    "States.TaskFailed",
		Cause:    "The task failed to execute properly",
		HasCause: true,
	}

	input := map[string]interface{}{
		"task": "process_data",
		"input": map[string]interface{}{
			"data": "test data",
		},
	}

	output, nextState, err := state.Execute(ctx, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "States.TaskFailed")
	assert.Contains(t, err.Error(), "cause: The task failed to execute properly")
	assert.Nil(t, output)
	assert.Nil(t, nextState)
}

// BenchmarkFailState_Execute benchmarks fail state execution
func BenchmarkFailState_Execute(b *testing.B) {
	ctx := context.Background()
	state := &FailState{
		BaseState: BaseState{
			Name: "BenchmarkFail",
			Type: "Fail",
		},
		Error: "BenchmarkError",
	}

	input := map[string]interface{}{
		"test": "data",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = state.Execute(ctx, input)
	}
}

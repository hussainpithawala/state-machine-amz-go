package states

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSucceedState_New tests the creation of a SucceedState
func TestSucceedState_New(t *testing.T) {
	state := &SucceedState{
		BaseState: BaseState{
			Name: "TestSucceedState",
			Type: "Succeed",
		},
	}

	assert.Equal(t, "TestSucceedState", state.GetName())
	assert.Equal(t, "Succeed", state.GetType())
	assert.Nil(t, state.GetNext())
	assert.True(t, state.IsEnd())
}

// TestSucceedState_Validate tests validation of SucceedState
func TestSucceedState_Validate(t *testing.T) {
	tests := []struct {
		name        string
		state       *SucceedState
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid succeed state",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "ValidSucceed",
					Type: "Succeed",
				},
			},
			shouldError: false,
		},
		{
			name: "valid succeed state with input path",
			state: &SucceedState{
				BaseState: BaseState{
					Name:      "ValidSucceed",
					Type:      "Succeed",
					InputPath: StringPtr("$.data"),
				},
			},
			shouldError: false,
		},
		{
			name: "valid succeed state with output path",
			state: &SucceedState{
				BaseState: BaseState{
					Name:       "ValidSucceed",
					Type:       "Succeed",
					OutputPath: StringPtr("$.result"),
				},
			},
			shouldError: false,
		},
		{
			name: "valid succeed state with both paths",
			state: &SucceedState{
				BaseState: BaseState{
					Name:       "ValidSucceed",
					Type:       "Succeed",
					InputPath:  StringPtr("$.input"),
					OutputPath: StringPtr("$.output"),
				},
			},
			shouldError: false,
		},
		{
			name: "valid succeed state with comment",
			state: &SucceedState{
				BaseState: BaseState{
					Name:    "ValidSucceed",
					Type:    "Succeed",
					Comment: "Successful completion",
				},
			},
			shouldError: false,
		},
		{
			name: "invalid - no name",
			state: &SucceedState{
				BaseState: BaseState{
					Type: "Succeed",
				},
			},
			shouldError: true,
			errorMsg:    "state name cannot be empty",
		},
		{
			name: "invalid - wrong type",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Pass", // Wrong type
				},
			},
			shouldError: true,
			errorMsg:    "succeed state must have Type 'Succeed'",
		},
		{
			name: "invalid - has next",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
					Next: StringPtr("NextState"),
				},
			},
			shouldError: true,
			errorMsg:    "cannot have Next field",
		},
		{
			name: "invalid - has end",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
					End:  true,
				},
			},
			shouldError: true,
			errorMsg:    "cannot have End field",
		},
		{
			name: "invalid - has result path",
			state: &SucceedState{
				BaseState: BaseState{
					Name:       "SucceedState",
					Type:       "Succeed",
					ResultPath: StringPtr("$.result"),
				},
			},
			shouldError: true,
			errorMsg:    "cannot have ResultPath",
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

// TestSucceedState_Execute tests execution of SucceedState
func TestSucceedState_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		state          *SucceedState
		input          interface{}
		expectedOutput interface{}
	}{
		{
			name: "simple succeed state",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input: map[string]interface{}{
				"key": "value",
			},
			expectedOutput: map[string]interface{}{
				"key": "value",
			},
		},
		{
			name: "with input path",
			state: &SucceedState{
				BaseState: BaseState{
					Name:      "SucceedState",
					Type:      "Succeed",
					InputPath: StringPtr("$.data"),
				},
			},
			input: map[string]interface{}{
				"data": "input data",
				"meta": "metadata",
			},
			expectedOutput: "input data",
		},
		{
			name: "with output path wrapping",
			state: &SucceedState{
				BaseState: BaseState{
					Name:       "SucceedState",
					Type:       "Succeed",
					OutputPath: StringPtr("$.result"),
				},
			},
			input: map[string]interface{}{
				"processed": true,
			},
			expectedOutput: map[string]interface{}{
				"result": map[string]interface{}{
					"processed": true,
				},
			},
		},
		{
			name: "with both input and output paths",
			state: &SucceedState{
				BaseState: BaseState{
					Name:       "SucceedState",
					Type:       "Succeed",
					InputPath:  StringPtr("$.user.name"),
					OutputPath: StringPtr("$.username"),
				},
			},
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "John Doe",
					"age":  30,
				},
			},
			expectedOutput: map[string]interface{}{
				"username": "John Doe",
			},
		},
		{
			name: "nil input",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input:          nil,
			expectedOutput: nil,
		},
		{
			name: "empty input",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input:          map[string]interface{}{},
			expectedOutput: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, nextState, err := tt.state.Execute(ctx, tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, output)
			assert.Nil(t, nextState) // Succeed states always return nil for next
		})
	}
}

// TestSucceedState_MarshalJSON tests JSON marshaling of SucceedState
func TestSucceedState_MarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		state        *SucceedState
		expectedJSON string
	}{
		{
			name: "simple succeed state",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SimpleSucceed",
					Type: "Succeed",
				},
			},
			expectedJSON: `{"Type":"Succeed"}`,
		},
		{
			name: "succeed state with input path",
			state: &SucceedState{
				BaseState: BaseState{
					Name:      "SucceedWithInputPath",
					Type:      "Succeed",
					InputPath: StringPtr("$.data"),
				},
			},
			expectedJSON: `{"Type":"Succeed","InputPath":"$.data"}`,
		},
		{
			name: "succeed state with output path",
			state: &SucceedState{
				BaseState: BaseState{
					Name:       "SucceedWithOutputPath",
					Type:       "Succeed",
					OutputPath: StringPtr("$.result"),
				},
			},
			expectedJSON: `{"Type":"Succeed","OutputPath":"$.result"}`,
		},
		{
			name: "succeed state with comment",
			state: &SucceedState{
				BaseState: BaseState{
					Name:    "SucceedWithComment",
					Type:    "Succeed",
					Comment: "Successful completion state",
				},
			},
			expectedJSON: `{"Type":"Succeed","Comment":"Successful completion state"}`,
		},
		{
			name: "succeed state with all optional fields",
			state: &SucceedState{
				BaseState: BaseState{
					Name:       "CompleteSucceed",
					Type:       "Succeed",
					InputPath:  StringPtr("$.input"),
					OutputPath: StringPtr("$.output"),
					Comment:    "Complete succeed state",
				},
			},
			expectedJSON: `{"Type":"Succeed","InputPath":"$.input","OutputPath":"$.output","Comment":"Complete succeed state"}`,
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
			assert.NotContains(t, jsonStr, `"ResultPath"`)
		})
	}
}

// TestSucceedState_InterfaceMethods tests that SucceedState implements State interface
func TestSucceedState_InterfaceMethods(t *testing.T) {
	state := &SucceedState{
		BaseState: BaseState{
			Name: "TestSucceed",
			Type: "Succeed",
		},
	}

	// Test that SucceedState implements State interface
	var _ State = state

	// Test individual interface methods
	assert.Equal(t, "TestSucceed", state.GetName())
	assert.Equal(t, "Succeed", state.GetType())
	assert.Nil(t, state.GetNext())
	assert.True(t, state.IsEnd())

	// Execute should succeed
	ctx := context.Background()
	input := map[string]interface{}{"test": "data"}
	output, nextState, err := state.Execute(ctx, input)

	assert.NoError(t, err)
	assert.Equal(t, input, output)
	assert.Nil(t, nextState)
}

// TestSucceedState_ContextCancellation tests context cancellation
func TestSucceedState_ContextCancellation(t *testing.T) {
	state := &SucceedState{
		BaseState: BaseState{
			Name: "SucceedState",
			Type: "Succeed",
		},
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with cancelled context
	input := map[string]interface{}{"test": "data"}
	output, nextState, err := state.Execute(ctx, input)

	// Succeed states don't check context
	assert.NoError(t, err)
	assert.Equal(t, input, output)
	assert.Nil(t, nextState)
}

// TestSucceedState_ConcurrentExecution tests concurrent execution safety
func TestSucceedState_ConcurrentExecution(t *testing.T) {
	state := &SucceedState{
		BaseState: BaseState{
			Name: "ConcurrentSucceed",
			Type: "Succeed",
		},
	}

	ctx := context.Background()
	numGoroutines := 100
	results := make(chan interface{}, numGoroutines)

	// Run concurrent executions
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			input := map[string]interface{}{
				"id": id,
			}
			output, _, _ := state.Execute(ctx, input)
			results <- output
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		result := <-results
		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, resultMap, "id")
	}
}

// TestSucceedState_EdgeCases tests edge cases
func TestSucceedState_EdgeCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		state       *SucceedState
		input       interface{}
		description string
	}{
		{
			name: "string input",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input:       "plain string",
			description: "should handle string input",
		},
		{
			name: "number input",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input:       42,
			description: "should handle number input",
		},
		{
			name: "boolean input",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input:       true,
			description: "should handle boolean input",
		},
		{
			name: "array input",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input:       []interface{}{1, 2, 3},
			description: "should handle array input",
		},
		{
			name: "complex nested input",
			state: &SucceedState{
				BaseState: BaseState{
					Name: "SucceedState",
					Type: "Succeed",
				},
			},
			input: map[string]interface{}{
				"users": []interface{}{
					map[string]interface{}{"name": "Alice", "age": 30},
					map[string]interface{}{"name": "Bob", "age": 25},
				},
				"metadata": map[string]interface{}{
					"count":  2,
					"source": "database",
				},
			},
			description: "should handle complex nested input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, nextState, err := tt.state.Execute(ctx, tt.input)

			assert.NoError(t, err, tt.description)
			assert.Equal(t, tt.input, output, tt.description)
			assert.Nil(t, nextState, tt.description)
		})
	}
}

// TestSucceedState_Integration tests integration with JSONPath processor
func TestSucceedState_Integration(t *testing.T) {
	ctx := context.Background()

	// Test complex data transformation
	input := map[string]interface{}{
		"transaction": map[string]interface{}{
			"id":        "txn_12345",
			"amount":    100.50,
			"currency":  "USD",
			"status":    "completed",
			"timestamp": "2024-01-15T10:30:00Z",
		},
		"user": map[string]interface{}{
			"id":    "user_67890",
			"email": "test@example.com",
		},
		"metadata": map[string]interface{}{
			"processed": true,
			"version":   "1.0",
		},
	}

	// Test 1: Extract transaction summary
	state1 := &SucceedState{
		BaseState: BaseState{
			Name:       "ExtractTransaction",
			Type:       "Succeed",
			InputPath:  StringPtr("$.transaction"),
			OutputPath: StringPtr("$.summary"),
		},
	}

	output1, next1, err1 := state1.Execute(ctx, input)
	require.NoError(t, err1)
	assert.Nil(t, next1)

	// Should have transaction data wrapped in "summary"
	outputMap1, ok := output1.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, outputMap1, "summary")
	summary := outputMap1["summary"].(map[string]interface{})
	assert.Equal(t, "txn_12345", summary["id"])
	assert.Equal(t, 100.50, summary["amount"])

	// Test 2: Create success response
	state2 := &SucceedState{
		BaseState: BaseState{
			Name:      "CreateResponse",
			Type:      "Succeed",
			InputPath: StringPtr("$"),
		},
	}

	output2, next2, err2 := state2.Execute(ctx, input)
	require.NoError(t, err2)
	assert.Nil(t, next2)
	assert.Equal(t, input, output2)
}

// BenchmarkSucceedState_Execute benchmarks succeed state execution
func BenchmarkSucceedState_Execute(b *testing.B) {
	ctx := context.Background()
	state := &SucceedState{
		BaseState: BaseState{
			Name: "BenchmarkSucceed",
			Type: "Succeed",
		},
	}

	input := map[string]interface{}{
		"test": "data",
		"nested": map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": "value",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = state.Execute(ctx, input)
	}
}

// TestSucceedState_ComparisonWithFailState tests differences between Succeed and Fail states
func TestSucceedState_ComparisonWithFailState(t *testing.T) {
	ctx := context.Background()

	succeedState := &SucceedState{
		BaseState: BaseState{
			Name: "Success",
			Type: "Succeed",
		},
	}

	failState := &FailState{
		BaseState: BaseState{
			Name: "Failure",
			Type: "Fail",
		},
		Error: "TestError",
	}

	input := map[string]interface{}{"data": "test"}

	// Test SucceedState
	succeedOutput, succeedNext, succeedErr := succeedState.Execute(ctx, input)
	assert.NoError(t, succeedErr)
	assert.Equal(t, input, succeedOutput)
	assert.Nil(t, succeedNext)

	// Test FailState
	failOutput, failNext, failErr := failState.Execute(ctx, input)
	assert.Error(t, failErr)
	assert.Nil(t, failOutput)
	assert.Nil(t, failNext)

	// Verify they're both end states
	assert.True(t, succeedState.IsEnd())
	assert.True(t, failState.IsEnd())

	// Verify they both return nil for next state
	assert.Nil(t, succeedState.GetNext())
	assert.Nil(t, failState.GetNext())
}

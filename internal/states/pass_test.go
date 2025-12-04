package states

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPassState_Execute(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		state          *PassState
		input          interface{}
		expectedOutput interface{}
		expectedNext   *string
		shouldError    bool
	}{
		{
			name: "passthrough - no result or parameters",
			state: &PassState{
				BaseState: BaseState{
					Name: "PassState",
					Type: "Pass",
					Next: StringPtr("NextState"),
				},
			},
			input: map[string]interface{}{
				"key": "value",
			},
			expectedOutput: map[string]interface{}{
				"key": "value",
			},
			expectedNext: StringPtr("NextState"),
			shouldError:  false,
		},
		{
			name: "with static result",
			state: &PassState{
				BaseState: BaseState{
					Name: "PassState",
					Type: "Pass",
					Next: StringPtr("NextState"),
				},
				Result: map[string]interface{}{
					"message": "Hello World",
					"status":  "success",
				},
			},
			input: map[string]interface{}{
				"original": "data",
			},
			expectedOutput: map[string]interface{}{
				"message": "Hello World",
				"status":  "success",
			},
			expectedNext: StringPtr("NextState"),
			shouldError:  false,
		},
		{
			name: "with parameters",
			state: &PassState{
				BaseState: BaseState{
					Name: "PassState",
					Type: "Pass",
					End:  true,
				},
				Parameters: map[string]interface{}{
					"processed": true,
					"timestamp": "2024-01-01",
				},
			},
			input: map[string]interface{}{
				"original": "data",
			},
			expectedOutput: map[string]interface{}{
				"processed": true,
				"timestamp": "2024-01-01",
			},
			expectedNext: nil,
			shouldError:  false,
		},
		{
			name: "with input path extraction",
			state: &PassState{
				BaseState: BaseState{
					Name:      "PassState",
					Type:      "Pass",
					Next:      StringPtr("NextState"),
					InputPath: StringPtr("$.data"),
				},
			},
			input: map[string]interface{}{
				"data": "input data",
				"meta": "metadata",
			},
			expectedOutput: "input data",
			expectedNext:   StringPtr("NextState"),
			shouldError:    false,
		},
		// In the failing test case, make sure we're setting ResultPath in BaseState:
		{
			name: "with result path merging",
			state: &PassState{
				BaseState: BaseState{
					Name:       "PassState",
					Type:       "Pass",
					Next:       StringPtr("NextState"),
					ResultPath: StringPtr("$.output"), // This is in BaseState
				},
				Result: "new value",
			},
			input: map[string]interface{}{
				"input": "original",
			},
			expectedOutput: map[string]interface{}{
				"input":  "original",
				"output": "new value",
			},
			expectedNext: StringPtr("NextState"),
			shouldError:  false,
		},
		{
			name: "with output path wrapping",
			state: &PassState{
				BaseState: BaseState{
					Name:       "PassState",
					Type:       "Pass",
					Next:       StringPtr("NextState"),
					OutputPath: StringPtr("$.result"),
				},
				Result: "processed",
			},
			input: map[string]interface{}{
				"any": "data",
			},
			expectedOutput: map[string]interface{}{
				"result": "processed",
			},
			expectedNext: StringPtr("NextState"),
			shouldError:  false,
		},
		{
			name: "end state execution",
			state: &PassState{
				BaseState: BaseState{
					Name: "PassState",
					Type: "Pass",
					End:  true,
				},
				Result: "final result",
			},
			input: map[string]interface{}{
				"initial": "data",
			},
			expectedOutput: "final result",
			expectedNext:   nil,
			shouldError:    false,
		},
		{
			name: "complex path scenario",
			state: &PassState{
				BaseState: BaseState{
					Name:       "PassState",
					Type:       "Pass",
					Next:       StringPtr("NextState"),
					InputPath:  StringPtr("$.user.data"),
					ResultPath: StringPtr("$.processed"),
					OutputPath: StringPtr("$.output"),
				},
				Result: "success",
			},
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"data": "user data",
					"meta": "user meta",
				},
				"system": "info",
			},
			expectedOutput: map[string]interface{}{
				"output": map[string]interface{}{
					"processed": "success",
				},
			},
			expectedNext: StringPtr("NextState"),
			shouldError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute the state
			output, nextState, err := tt.state.Execute(ctx, tt.input)

			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify output
				assert.Equal(t, tt.expectedOutput, output)

				// Verify next state
				if tt.expectedNext == nil {
					assert.Nil(t, nextState)
				} else {
					require.NotNil(t, nextState)
					assert.Equal(t, *tt.expectedNext, *nextState)
				}
			}
		})
	}
}

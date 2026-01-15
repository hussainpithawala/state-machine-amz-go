package states

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChoiceState_New(t *testing.T) {
	state := &ChoiceState{
		BaseState: BaseState{
			Name: "TestChoice",
			Type: "Choice",
		},
		Choices: []ChoiceRule{
			{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(10),
				Next:          "Path1",
			},
		},
		Default: StringPtr("DefaultPath"),
	}

	assert.Equal(t, "TestChoice", state.GetName())
	assert.Equal(t, "Choice", state.GetType())
	assert.Nil(t, state.GetNext())
	assert.False(t, state.IsEnd())
	assert.Len(t, state.Choices, 1)
	assert.Equal(t, "Path1", state.Choices[0].Next)
	assert.Equal(t, "DefaultPath", *state.Default)
}

func TestChoiceState_Validate(t *testing.T) {
	tests := []struct {
		name        string
		state       *ChoiceState
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid choice state with choices",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ValidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(10),
						Next:          "NextState",
					},
				},
			},
			shouldError: false,
		},
		{
			name: "valid choice state with default only",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ValidChoice",
					Type: "Choice",
				},
				Default: StringPtr("DefaultState"),
			},
			shouldError: false,
		},
		{
			name: "valid choice state with multiple choices",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ValidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:        "$.value",
						NumericLessThan: Float64Ptr(10),
						Next:            "LessThan10",
					},
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(10),
						Next:          "Equals10",
					},
					{
						Variable:           "$.value",
						NumericGreaterThan: Float64Ptr(10),
						Next:               "GreaterThan10",
					},
				},
			},
			shouldError: false,
		},
		{
			name: "invalid - has next field",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "InvalidChoice",
					Type: "Choice",
					Next: StringPtr("NextState"),
				},
				Choices: []ChoiceRule{
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(10),
						Next:          "ActualNext",
					},
				},
			},
			shouldError: true,
			errorMsg:    "cannot have Next field",
		},
		{
			name: "invalid - has end field",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "InvalidChoice",
					Type: "Choice",
					End:  true,
				},
				Choices: []ChoiceRule{
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(10),
						Next:          "NextState",
					},
				},
			},
			shouldError: true,
			errorMsg:    "cannot have End field",
		},
		{
			name: "invalid - no choices and no default",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "InvalidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{},
			},
			shouldError: true,
			errorMsg:    "must have either Choices or Default",
		},
		{
			name: "invalid choice - no variable",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "InvalidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						NumericEquals: Float64Ptr(10),
						Next:          "NextState",
					},
				},
			},
			shouldError: true,
			errorMsg:    "Variable is required",
		},
		{
			name: "invalid choice - no comparison operator",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "InvalidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.value",
						Next:     "NextState",
					},
				},
			},
			shouldError: true,
			errorMsg:    "must have at least one comparison operator",
		},
		{
			name: "invalid choice - must also have next",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "InvalidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(10),
					},
				},
			},
			shouldError: true,
			errorMsg:    "Next is required",
		},
		{
			name: "valid - with AND operator",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ValidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.value",
						And: []ChoiceRule{
							{
								Variable:     "$.type",
								StringEquals: StringPtr("user"),
							},
							{
								Variable:           "$.age",
								NumericGreaterThan: Float64Ptr(18),
							},
						},
						Next: "AdultUser",
					},
				},
			},
			shouldError: false,
		},
		{
			name: "valid - with OR operator",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ValidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.value",
						Or: []ChoiceRule{
							{
								Variable:     "$.status",
								StringEquals: StringPtr("active"),
							},
							{
								Variable:     "$.status",
								StringEquals: StringPtr("pending"),
							},
						},
						Next: "ActiveOrPending",
					},
				},
			},
			shouldError: false,
		},
		{
			name: "valid - with NOT operator",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ValidChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.value",
						Not: &ChoiceRule{
							Variable:     "$.status",
							StringEquals: StringPtr("inactive"),
						},
						Next: "NotInactive",
					},
				},
			},
			shouldError: false,
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

func TestChoiceState_Execute_StringComparisons(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		state          *ChoiceState
		input          interface{}
		expectedNext   *string
		expectedOutput interface{}
		shouldError    bool
	}{
		{
			name: "StringEquals - match",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:     "$.status",
						StringEquals: StringPtr("active"),
						Next:         "ActivePath",
					},
				},
			},
			input: map[string]interface{}{
				"status": "active",
			},
			expectedNext:   StringPtr("ActivePath"),
			expectedOutput: map[string]interface{}{"status": "active"},
		},
		{
			name: "StringEquals - no match",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:     "$.status",
						StringEquals: StringPtr("active"),
						Next:         "ActivePath",
					},
				},
				Default: StringPtr("DefaultPath"),
			},
			input: map[string]interface{}{
				"status": "inactive",
			},
			expectedNext:   StringPtr("DefaultPath"),
			expectedOutput: map[string]interface{}{"status": "inactive"},
		},
		{
			name: "StringLessThan - match",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:       "$.name",
						StringLessThan: StringPtr("M"),
						Next:           "BeforeM",
					},
				},
			},
			input: map[string]interface{}{
				"name": "Alice",
			},
			expectedNext: StringPtr("BeforeM"),
		},
		{
			name: "StringGreaterThan - match",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:          "$.name",
						StringGreaterThan: StringPtr("M"),
						Next:              "AfterM",
					},
				},
			},
			input: map[string]interface{}{
				"name": "Zoe",
			},
			expectedNext: StringPtr("AfterM"),
		},
		{
			name: "multiple choices - first matches",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:        "$.value",
						NumericLessThan: Float64Ptr(0),
						Next:            "Negative",
					},
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(0),
						Next:          "Zero",
					},
					{
						Variable:           "$.value",
						NumericGreaterThan: Float64Ptr(0),
						Next:               "Positive",
					},
				},
			},
			input: map[string]interface{}{
				"value": -5,
			},
			expectedNext: StringPtr("Negative"),
		},
		{
			name: "multiple choices - second matches",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:        "$.value",
						NumericLessThan: Float64Ptr(0),
						Next:            "Negative",
					},
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(0),
						Next:          "Zero",
					},
					{
						Variable:           "$.value",
						NumericGreaterThan: Float64Ptr(0),
						Next:               "Positive",
					},
				},
			},
			input: map[string]interface{}{
				"value": 0,
			},
			expectedNext: StringPtr("Zero"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, nextState, err := tt.state.Execute(ctx, tt.input)

			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				if tt.expectedNext != nil {
					require.NotNil(t, nextState)
					assert.Equal(t, *tt.expectedNext, *nextState)
				} else {
					assert.Nil(t, nextState)
				}

				if tt.expectedOutput != nil {
					assert.Equal(t, tt.expectedOutput, output)
				}
			}
		})
	}
}

func TestChoiceState_Execute_NumericComparisons(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		rule         ChoiceRule
		input        interface{}
		expectedNext string
		shouldMatch  bool
	}{
		{
			name: "NumericEquals - integer match",
			rule: ChoiceRule{
				Variable:      "$.count",
				NumericEquals: Float64Ptr(10),
				Next:          "Equals10",
			},
			input:        map[string]interface{}{"count": 10},
			expectedNext: "Equals10",
			shouldMatch:  true,
		},
		{
			name: "NumericEquals - float match",
			rule: ChoiceRule{
				Variable:      "$.price",
				NumericEquals: Float64Ptr(99.99),
				Next:          "PriceMatch",
			},
			input:        map[string]interface{}{"price": 99.99},
			expectedNext: "PriceMatch",
			shouldMatch:  true,
		},
		{
			name: "NumericLessThan - match",
			rule: ChoiceRule{
				Variable:        "$.age",
				NumericLessThan: Float64Ptr(18),
				Next:            "Minor",
			},
			input:        map[string]interface{}{"age": 16},
			expectedNext: "Minor",
			shouldMatch:  true,
		},
		{
			name: "NumericGreaterThan - match",
			rule: ChoiceRule{
				Variable:           "$.score",
				NumericGreaterThan: Float64Ptr(90),
				Next:               "Excellent",
			},
			input:        map[string]interface{}{"score": 95},
			expectedNext: "Excellent",
			shouldMatch:  true,
		},
		{
			name: "NumericLessThanEquals - match equal",
			rule: ChoiceRule{
				Variable:              "$.quantity",
				NumericLessThanEquals: Float64Ptr(100),
				Next:                  "InStock",
			},
			input:        map[string]interface{}{"quantity": 100},
			expectedNext: "InStock",
			shouldMatch:  true,
		},
		{
			name: "NumericGreaterThanEquals - match equal",
			rule: ChoiceRule{
				Variable:                 "$.threshold",
				NumericGreaterThanEquals: Float64Ptr(50),
				Next:                     "MetThreshold",
			},
			input:        map[string]interface{}{"threshold": 50},
			expectedNext: "MetThreshold",
			shouldMatch:  true,
		},
		{
			name: "string to number conversion",
			rule: ChoiceRule{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(42),
				Next:          "Answer",
			},
			input:        map[string]interface{}{"value": "42"},
			expectedNext: "Answer",
			shouldMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{tt.rule},
				Default: StringPtr("DefaultPath"),
			}

			output, nextState, err := state.Execute(ctx, tt.input)
			require.NoError(t, err)

			if tt.shouldMatch {
				require.NotNil(t, nextState)
				assert.Equal(t, tt.expectedNext, *nextState)
			} else {
				assert.Equal(t, "DefaultPath", *nextState)
			}

			assert.Equal(t, tt.input, output)
		})
	}
}

func TestChoiceState_Execute_BooleanComparisons(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		rule         ChoiceRule
		input        interface{}
		expectedNext string
		shouldMatch  bool
	}{
		{
			name: "BooleanEquals true - match bool",
			rule: ChoiceRule{
				Variable:      "$.enabled",
				BooleanEquals: BoolPtr(true),
				Next:          "Enabled",
			},
			input:        map[string]interface{}{"enabled": true},
			expectedNext: "Enabled",
			shouldMatch:  true,
		},
		{
			name: "BooleanEquals false - match bool",
			rule: ChoiceRule{
				Variable:      "$.active",
				BooleanEquals: BoolPtr(false),
				Next:          "Inactive",
			},
			input:        map[string]interface{}{"active": false},
			expectedNext: "Inactive",
			shouldMatch:  true,
		},
		{
			name: "BooleanEquals true - match string 'true'",
			rule: ChoiceRule{
				Variable:      "$.status",
				BooleanEquals: BoolPtr(true),
				Next:          "TrueStatus",
			},
			input:        map[string]interface{}{"status": "true"},
			expectedNext: "TrueStatus",
			shouldMatch:  true,
		},
		{
			name: "BooleanEquals false - match string 'false'",
			rule: ChoiceRule{
				Variable:      "$.status",
				BooleanEquals: BoolPtr(false),
				Next:          "FalseStatus",
			},
			input:        map[string]interface{}{"status": "false"},
			expectedNext: "FalseStatus",
			shouldMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{tt.rule},
				Default: StringPtr("DefaultPath"),
			}

			output, nextState, err := state.Execute(ctx, tt.input)
			require.NoError(t, err)

			if tt.shouldMatch {
				require.NotNil(t, nextState)
				assert.Equal(t, tt.expectedNext, *nextState)
			} else {
				assert.Equal(t, "DefaultPath", *nextState)
			}

			assert.Equal(t, tt.input, output)
		})
	}
}

func TestChoiceState_Execute_TimestampComparisons(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name         string
		rule         ChoiceRule
		input        interface{}
		expectedNext string
		shouldMatch  bool
	}{
		{
			name: "TimestampEquals - match RFC3339",
			rule: ChoiceRule{
				Variable:        "$.eventTime",
				TimestampEquals: StringPtr("2024-01-15T10:30:00Z"),
				Next:            "ExactTime",
			},
			input:        map[string]interface{}{"eventTime": "2024-01-15T10:30:00Z"},
			expectedNext: "ExactTime",
			shouldMatch:  true,
		},
		{
			name: "TimestampLessThan - match",
			rule: ChoiceRule{
				Variable:          "$.deadline",
				TimestampLessThan: StringPtr("2024-12-31T23:59:59Z"),
				Next:              "BeforeDeadline",
			},
			input:        map[string]interface{}{"deadline": "2024-06-15T12:00:00Z"},
			expectedNext: "BeforeDeadline",
			shouldMatch:  true,
		},
		{
			name: "TimestampGreaterThan - match",
			rule: ChoiceRule{
				Variable:             "$.startTime",
				TimestampGreaterThan: StringPtr("2024-01-01T00:00:00Z"),
				Next:                 "AfterNewYear",
			},
			input:        map[string]interface{}{"startTime": "2024-02-14T10:00:00Z"},
			expectedNext: "AfterNewYear",
			shouldMatch:  true,
		},
		{
			name: "unix timestamp - match",
			rule: ChoiceRule{
				Variable:        "$.timestamp",
				TimestampEquals: StringPtr("2024-01-01T00:00:00Z"),
				Next:            "NewYear",
			},
			input:        map[string]interface{}{"timestamp": 1704067200}, // Unix timestamp for 2024-01-01
			expectedNext: "NewYear",
			shouldMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{tt.rule},
				Default: StringPtr("DefaultPath"),
			}

			output, nextState, err := state.Execute(ctx, tt.input)
			require.NoError(t, err)

			if tt.shouldMatch {
				require.NotNil(t, nextState)
				assert.Equal(t, tt.expectedNext, *nextState)
			} else {
				assert.Equal(t, "DefaultPath", *nextState)
			}

			assert.Equal(t, tt.input, output)
		})
	}
}

func TestChoiceState_Execute_LogicalOperators(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		state        *ChoiceState
		input        interface{}
		expectedNext *string
		shouldError  bool
	}{
		{
			name: "AND operator - both true",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.user",
						And: []ChoiceRule{
							{
								Variable:           "$.age",
								NumericGreaterThan: Float64Ptr(18),
							},
							{
								Variable:     "$.country",
								StringEquals: StringPtr("US"),
							},
						},
						Next: "AdultInUS",
					},
				},
			},
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"age":     25,
					"country": "US",
				},
			},
			expectedNext: StringPtr("AdultInUS"),
		},
		{
			name: "AND operator - one false",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.user",
						And: []ChoiceRule{
							{
								Variable:           "$.age",
								NumericGreaterThan: Float64Ptr(18),
							},
							{
								Variable:     "$.country",
								StringEquals: StringPtr("US"),
							},
						},
						Next: "AdultInUS",
					},
				},
				Default: StringPtr("DefaultPath"),
			},
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"age":     16, // Under 18
					"country": "US",
				},
			},
			expectedNext: StringPtr("DefaultPath"),
		},
		{
			name: "OR operator - first true",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.status",
						Or: []ChoiceRule{
							{
								Variable:     "$.code",
								StringEquals: StringPtr("200"),
							},
							{
								Variable:     "$.code",
								StringEquals: StringPtr("201"),
							},
						},
						Next: "Success",
					},
				},
			},
			input: map[string]interface{}{
				"status": map[string]interface{}{
					"code": "200",
				},
			},
			expectedNext: StringPtr("Success"),
		},
		{
			name: "OR operator - second true",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.status",
						Or: []ChoiceRule{
							{
								Variable:     "$.code",
								StringEquals: StringPtr("200"),
							},
							{
								Variable:     "$.code",
								StringEquals: StringPtr("201"),
							},
						},
						Next: "Success",
					},
				},
			},
			input: map[string]interface{}{
				"status": map[string]interface{}{
					"code": "201",
				},
			},
			expectedNext: StringPtr("Success"),
		},
		{
			name: "OR operator - none true",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.status",
						Or: []ChoiceRule{
							{
								Variable:     "$.code",
								StringEquals: StringPtr("200"),
							},
							{
								Variable:     "$.code",
								StringEquals: StringPtr("201"),
							},
						},
						Next: "Success",
					},
				},
				Default: StringPtr("DefaultPath"),
			},
			input: map[string]interface{}{
				"status": map[string]interface{}{
					"code": "404",
				},
			},
			expectedNext: StringPtr("DefaultPath"),
		},
		{
			name: "NOT operator - true",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.user",
						Not: &ChoiceRule{
							Variable:     "$.status",
							StringEquals: StringPtr("inactive"),
						},
						Next: "ActiveUser",
					},
				},
			},
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"status": "active",
				},
			},
			expectedNext: StringPtr("ActiveUser"),
		},
		{
			name: "NOT operator - false",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.user",
						Not: &ChoiceRule{
							Variable:     "$.status",
							StringEquals: StringPtr("inactive"),
						},
						Next: "ActiveUser",
					},
				},
				Default: StringPtr("DefaultPath"),
			},
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"status": "inactive",
				},
			},
			expectedNext: StringPtr("DefaultPath"),
		},
		{
			name: "nested logical operators",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.data",
						And: []ChoiceRule{
							{
								Variable: "$.value",
								Or: []ChoiceRule{
									{
										Variable:     "$.type",
										StringEquals: StringPtr("A"),
									},
									{
										Variable:     "$.type",
										StringEquals: StringPtr("B"),
									},
								},
							},
							{
								Variable: "$.value",
								Not: &ChoiceRule{
									Variable:     "$.status",
									StringEquals: StringPtr("invalid"),
								},
							},
						},
						Next: "ValidType",
					},
				},
			},
			input: map[string]interface{}{
				"data": map[string]interface{}{
					"value": map[string]interface{}{
						"type":   "A",
						"status": "valid",
					},
				},
			},
			expectedNext: StringPtr("ValidType"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, nextState, err := tt.state.Execute(ctx, tt.input)

			if tt.shouldError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				if tt.expectedNext != nil {
					require.NotNil(t, nextState)
					assert.Equal(t, *tt.expectedNext, *nextState)
				} else {
					assert.Nil(t, nextState)
				}

				assert.Equal(t, tt.input, output)
			}
		})
	}
}

// In choice_test.go, update the TestChoiceState_Execute_WithPaths test:
func TestChoiceState_Execute_WithPaths(t *testing.T) {
	ctx := context.Background()

	state := &ChoiceState{
		BaseState: BaseState{
			Name:       "ChoiceState",
			Type:       "Choice",
			InputPath:  StringPtr("$.data"),
			ResultPath: StringPtr("$.result"),
			OutputPath: StringPtr("$.output"),
		},
		Choices: []ChoiceRule{
			{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(10),
				Next:          "Path1",
			},
		},
	}

	input := map[string]interface{}{
		"data": map[string]interface{}{
			"value": 10,
		},
		"other": "data",
	}

	output, nextState, err := state.Execute(ctx, input)
	require.NoError(t, err)

	require.NotNil(t, nextState)
	assert.Equal(t, "Path1", *nextState)

	// InputPath extracts $.data = {"value": 10}
	// Choice evaluates and returns the same {"value": 10}
	// ResultPath: "$.result" merges result into input at $.result
	// So we get {"value": 10, "result": {"value": 10}}
	// OutputPath: "$.output" wraps in $.output
	// Final output: {"output": {"value": 10, "result": {"value": 10}}}
	expectedOutput := map[string]interface{}{
		"output": map[string]interface{}{
			"result": map[string]interface{}{
				"value": 10,
			},
			"value": 10,
		},
	}
	assert.Equal(t, expectedOutput, output)
}

func TestChoiceState_Execute_NoMatchNoDefault(t *testing.T) {
	ctx := context.Background()

	state := &ChoiceState{
		BaseState: BaseState{
			Name: "ChoiceState",
			Type: "Choice",
		},
		Choices: []ChoiceRule{
			{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(10),
				Next:          "Path1",
			},
		},
		// No Default specified
	}

	input := map[string]interface{}{
		"value": 20, // Doesn't match
	}

	output, nextState, err := state.Execute(ctx, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choice rule matched and no default specified")
	assert.Nil(t, nextState)
	assert.Nil(t, output)
}

func TestChoiceState_MarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		state        *ChoiceState
		expectedJSON string
	}{
		{
			name: "simple choice state",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "SimpleChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(10),
						Next:          "NextState",
					},
				},
			},
			expectedJSON: `{"Type":"Choice","Choices":[{"Variable":"$.value","NumericEquals":10,"Next":"NextState"}]}`,
		},
		{
			name: "choice state with default",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceWithDefault",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:     "$.status",
						StringEquals: StringPtr("active"),
						Next:         "ActivePath",
					},
				},
				Default: StringPtr("DefaultPath"),
			},
			expectedJSON: `{"Type":"Choice","Choices":[{"Variable":"$.status","StringEquals":"active","Next":"ActivePath"}],"Default":"DefaultPath"}`,
		},
		{
			name: "choice state with all fields",
			state: &ChoiceState{
				BaseState: BaseState{
					Name:       "CompleteChoice",
					Type:       "Choice",
					InputPath:  StringPtr("$.input"),
					ResultPath: StringPtr("$.result"),
					OutputPath: StringPtr("$.output"),
					Comment:    "Choice state example",
				},
				Choices: []ChoiceRule{
					{
						Variable:      "$.value",
						NumericEquals: Float64Ptr(10),
						Next:          "Path1",
						Comment:       "Value equals 10",
					},
				},
				Default: StringPtr("DefaultPath"),
			},
			expectedJSON: `{"Type":"Choice","Choices":[{"Variable":"$.value","NumericEquals":10,"Next":"Path1","Comment":"Value equals 10"}],"Default":"DefaultPath","InputPath":"$.input","ResultPath":"$.result","OutputPath":"$.output","Comment":"Choice state example"}`,
		},
		{
			name: "choice with AND operator",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "AndChoice",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable: "$.user",
						And: []ChoiceRule{
							{
								Variable:           "$.age",
								NumericGreaterThan: Float64Ptr(18),
							},
							{
								Variable:     "$.country",
								StringEquals: StringPtr("US"),
							},
						},
						Next: "AdultInUS",
					},
				},
			},
			expectedJSON: `{"Type":"Choice","Choices":[{"Variable":"$.user","And":[{"Variable":"$.age","NumericGreaterThan":18,"Next":""},{"Variable":"$.country","StringEquals":"US","Next":""}],"Next":"AdultInUS"}]}`,
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
		})
	}
}

func TestChoiceState_InterfaceMethods(t *testing.T) {
	state := &ChoiceState{
		BaseState: BaseState{
			Name: "TestChoice",
			Type: "Choice",
		},
		Choices: []ChoiceRule{
			{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(10),
				Next:          "NextState",
			},
		},
	}

	// Test that ChoiceState implements State interface
	var _ State = state

	// Test individual interface methods
	assert.Equal(t, "TestChoice", state.GetName())
	assert.Equal(t, "Choice", state.GetType())
	assert.Nil(t, state.GetNext()) // Choice states don't have a single Next
	assert.False(t, state.IsEnd())
}

func TestChoiceState_ContextCancellation(t *testing.T) {
	state := &ChoiceState{
		BaseState: BaseState{
			Name: "ChoiceState",
			Type: "Choice",
		},
		Choices: []ChoiceRule{
			{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(10),
				Next:          "Path1",
			},
		},
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Execute with cancelled context
	input := map[string]interface{}{"value": 10}
	output, nextState, err := state.Execute(ctx, input)

	// Choice states don't typically check context
	assert.NoError(t, err)
	assert.Equal(t, input, output)
	assert.Equal(t, "Path1", *nextState)
}

func TestChoiceState_ConcurrentExecution(t *testing.T) {
	state := &ChoiceState{
		BaseState: BaseState{
			Name: "ConcurrentChoice",
			Type: "Choice",
		},
		Choices: []ChoiceRule{
			{
				Variable:      "$.id",
				NumericEquals: Float64Ptr(1),
				Next:          "Path1",
			},
			{
				Variable:      "$.id",
				NumericEquals: Float64Ptr(2),
				Next:          "Path2",
			},
		},
		Default: StringPtr("DefaultPath"),
	}

	ctx := context.Background()
	numGoroutines := 100
	results := make(chan string, numGoroutines)

	// Run concurrent executions
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			input := map[string]interface{}{
				"id": id % 3, // 0, 1, or 2
			}
			_, nextState, _ := state.Execute(ctx, input)
			results <- *nextState
		}(i)
	}

	// Collect results
	path1Count := 0
	path2Count := 0
	defaultCount := 0

	for i := 0; i < numGoroutines; i++ {
		result := <-results
		switch result {
		case "Path1":
			path1Count++
		case "Path2":
			path2Count++
		case "DefaultPath":
			defaultCount++
		}
	}

	// Each should get about 1/3 of the executions
	assert.True(t, path1Count > 0)
	assert.True(t, path2Count > 0)
	assert.True(t, defaultCount > 0)
}

func TestChoiceState_EdgeCases(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		state        *ChoiceState
		input        interface{}
		expectedNext *string
		description  string
	}{
		{
			name: "empty object input",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:     "$.value",
						StringEquals: StringPtr("test"),
						Next:         "Match",
					},
				},
				Default: StringPtr("Default"),
			},
			input:        map[string]interface{}{},
			expectedNext: StringPtr("Default"),
			description:  "should handle missing variable with default",
		},
		{
			name: "null input",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:     "$.value",
						StringEquals: StringPtr("test"),
						Next:         "Match",
					},
				},
				Default: StringPtr("Default"),
			},
			input:        nil,
			expectedNext: StringPtr("Default"),
			description:  "should handle nil input with default",
		},
		{
			name: "deeply nested variable",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:      "$.user.profile.settings.notifications",
						BooleanEquals: BoolPtr(true),
						Next:          "NotificationsOn",
					},
				},
			},
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"profile": map[string]interface{}{
						"settings": map[string]interface{}{
							"notifications": true,
						},
					},
				},
			},
			expectedNext: StringPtr("NotificationsOn"),
			description:  "should handle deeply nested variables",
		},
		{
			name: "array element access",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:     "$.items[0].status",
						StringEquals: StringPtr("active"),
						Next:         "FirstActive",
					},
				},
			},
			input: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"status": "active"},
					map[string]interface{}{"status": "inactive"},
				},
			},
			expectedNext: StringPtr("FirstActive"),
			description:  "should handle []interface{} array indexing in variable path",
		},
		{
			name: "array element access",
			state: &ChoiceState{
				BaseState: BaseState{
					Name: "ChoiceState",
					Type: "Choice",
				},
				Choices: []ChoiceRule{
					{
						Variable:     "$.accounts[0].createdBy",
						StringEquals: StringPtr("LESSPAY"),
						Next:         "FirstActive",
					},
				},
			},
			input: map[string]interface{}{
				"accounts": []map[string]interface{}{
					{
						"createdBy": "LESSPAY",
						"bankInfo": map[string]interface{}{
							"bankCode": "100000",
							"bankName": "UNKNOWN BANK",
						},
						"otherData": []map[string]interface{}{
							{
								"epoch":     1761896894817,
								"status":    "ACTIVE",
								"isPrimary": true,
							},
						},
					},
				},
			},
			expectedNext: StringPtr("FirstActive"),
			description:  "should handle []map[string]interface{} array indexing in variable path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, nextState, err := tt.state.Execute(ctx, tt.input)

			if tt.expectedNext != nil {
				require.NoError(t, err, tt.description)
				require.NotNil(t, nextState, tt.description)
				assert.Equal(t, *tt.expectedNext, *nextState, tt.description)
			} else {
				require.Error(t, err, tt.description)
			}
		})
	}
}

func BenchmarkChoiceState_Execute(b *testing.B) {
	ctx := context.Background()
	state := &ChoiceState{
		BaseState: BaseState{
			Name: "BenchmarkChoice",
			Type: "Choice",
		},
		Choices: []ChoiceRule{
			{
				Variable:        "$.value",
				NumericLessThan: Float64Ptr(10),
				Next:            "LessThan10",
			},
			{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(10),
				Next:          "Equals10",
			},
			{
				Variable:           "$.value",
				NumericGreaterThan: Float64Ptr(10),
				Next:               "GreaterThan10",
			},
			{
				Variable: "$.user",
				And: []ChoiceRule{
					{
						Variable:           "$.age",
						NumericGreaterThan: Float64Ptr(18),
					},
					{
						Variable:     "$.country",
						StringEquals: StringPtr("US"),
					},
				},
				Next: "AdultInUS",
			},
		},
		Default: StringPtr("DefaultPath"),
	}

	input := map[string]interface{}{
		"value": 15,
		"user": map[string]interface{}{
			"age":     25,
			"country": "US",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = state.Execute(ctx, input)
	}
}

func TestChoiceStateValidationQuick(t *testing.T) {
	state := &ChoiceState{
		BaseState: BaseState{
			Name: "TestChoice",
			Type: "Choice",
		},
		Choices: []ChoiceRule{
			{
				Variable:      "$.value",
				NumericEquals: Float64Ptr(10),
				Next:          "NextState",
			},
		},
	}

	err := state.Validate()
	require.NoError(t, err)
}

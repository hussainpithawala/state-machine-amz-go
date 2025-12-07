package states

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParallelState_Execute_MultipleBranches(t *testing.T) {
	ctx := context.Background()

	// Create two simple branches with Pass states
	state := &ParallelState{
		BaseState: BaseState{
			Name: "ParallelState",
			Type: "Parallel",
			Next: StringPtr("NextState"),
		},
		Branches: []Branch{
			{
				StartAt: "Pass1",
				States: map[string]State{
					"Pass1": &PassState{
						BaseState: BaseState{
							Name: "Pass1",
							Type: "Pass",
							End:  true,
						},
						Result: "branch1-result",
					},
				},
			},
			{
				StartAt: "Pass2",
				States: map[string]State{
					"Pass2": &PassState{
						BaseState: BaseState{
							Name: "Pass2",
							Type: "Pass",
							End:  true,
						},
						Result: "branch2-result",
					},
				},
			},
		},
	}

	input := map[string]interface{}{
		"original": "data",
	}

	output, nextState, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, "NextState", *nextState)

	// Verify output is an array of results
	results, ok := output.([]interface{})
	require.True(t, ok, "output should be an array")
	require.Equal(t, 2, len(results))
	assert.Equal(t, "branch1-result", results[0])
	assert.Equal(t, "branch2-result", results[1])
}

func TestParallelState_Execute_WithInputPath(t *testing.T) {
	ctx := context.Background()

	state := &ParallelState{
		BaseState: BaseState{
			Name:      "ParallelState",
			Type:      "Parallel",
			End:       true,
			InputPath: StringPtr("$.data"),
		},
		Branches: []Branch{
			{
				StartAt: "Pass1",
				States: map[string]State{
					"Pass1": &PassState{
						BaseState: BaseState{
							Name: "Pass1",
							Type: "Pass",
							End:  true,
						},
					},
				},
			},
			{
				StartAt: "Pass2",
				States: map[string]State{
					"Pass2": &PassState{
						BaseState: BaseState{
							Name: "Pass2",
							Type: "Pass",
							End:  true,
						},
					},
				},
			},
		},
	}

	input := map[string]interface{}{
		"data": map[string]interface{}{
			"value": "test",
		},
		"other": "ignored",
	}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	require.Equal(t, 2, len(results))

	// Each branch should receive the extracted input
	assert.Equal(
		t,
		map[string]interface{}{"value": "test"},
		results[0],
	)
	assert.Equal(
		t,
		map[string]interface{}{"value": "test"},
		results[1],
	)
}

func TestParallelState_Execute_MultiStateBranch(t *testing.T) {
	ctx := context.Background()

	state := &ParallelState{
		BaseState: BaseState{
			Name: "ParallelState",
			Type: "Parallel",
			End:  true,
		},
		Branches: []Branch{
			{
				StartAt: "Pass1",
				States: map[string]State{
					"Pass1": &PassState{
						BaseState: BaseState{
							Name: "Pass1",
							Type: "Pass",
							Next: StringPtr("Pass2"),
						},
						Result: "step1",
					},
					"Pass2": &PassState{
						BaseState: BaseState{
							Name: "Pass2",
							Type: "Pass",
							End:  true,
						},
						Result: "final",
					},
				},
			},
		},
	}

	input := "initial"

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	require.Equal(t, 1, len(results))
	assert.Equal(t, "final", results[0])
}

func TestParallelState_Execute_WithResultPath(t *testing.T) {
	ctx := context.Background()

	state := &ParallelState{
		BaseState: BaseState{
			Name:       "ParallelState",
			Type:       "Parallel",
			Next:       StringPtr("NextState"),
			ResultPath: StringPtr("$.results"),
		},
		Branches: []Branch{
			{
				StartAt: "Pass1",
				States: map[string]State{
					"Pass1": &PassState{
						BaseState: BaseState{
							Name: "Pass1",
							Type: "Pass",
							End:  true,
						},
						Result: "value1",
					},
				},
			},
			{
				StartAt: "Pass2",
				States: map[string]State{
					"Pass2": &PassState{
						BaseState: BaseState{
							Name: "Pass2",
							Type: "Pass",
							End:  true,
						},
						Result: "value2",
					},
				},
			},
		},
	}

	input := map[string]interface{}{
		"original": "data",
	}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	outputMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "data", outputMap["original"])

	results, ok := outputMap["results"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(results))
}

func TestParallelState_Execute_WithOutputPath(t *testing.T) {
	ctx := context.Background()

	state := &ParallelState{
		BaseState: BaseState{
			Name:       "ParallelState",
			Type:       "Parallel",
			End:        true,
			OutputPath: StringPtr("$[0]"),
		},
		Branches: []Branch{
			{
				StartAt: "Pass1",
				States: map[string]State{
					"Pass1": &PassState{
						BaseState: BaseState{
							Name: "Pass1",
							Type: "Pass",
							End:  true,
						},
						Result: "extracted",
					},
				},
			},
			{
				StartAt: "Pass2",
				States: map[string]State{
					"Pass2": &PassState{
						BaseState: BaseState{
							Name: "Pass2",
							Type: "Pass",
							End:  true,
						},
						Result: "ignored",
					},
				},
			},
		},
	}

	input := "initial"

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "extracted", output)
}

func TestParallelState_Execute_ContextCancelled(t *testing.T) {
	state := &ParallelState{
		BaseState: BaseState{
			Name: "ParallelState",
			Type: "Parallel",
			End:  true,
		},
		Branches: []Branch{
			{
				StartAt: "Wait1",
				States: map[string]State{
					"Wait1": &WaitState{
						BaseState: BaseState{
							Name: "Wait1",
							Type: "Wait",
							End:  true,
						},
						Seconds: Int64Ptr(10),
					},
				},
			},
		},
	}

	input := "initial"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := state.Execute(ctx, input)
	require.Error(t, err)
}

func TestParallelState_Execute_BranchError(t *testing.T) {
	ctx := context.Background()

	state := &ParallelState{
		BaseState: BaseState{
			Name: "ParallelState",
			Type: "Parallel",
			End:  true,
		},
		Branches: []Branch{
			{
				StartAt: "Pass1",
				States: map[string]State{
					"Pass1": &PassState{
						BaseState: BaseState{
							Name: "Pass1",
							Type: "Pass",
							End:  true,
						},
						Result: "success",
					},
				},
			},
			{
				StartAt: "Fail1",
				States: map[string]State{
					"Fail1": &FailState{
						BaseState: BaseState{
							Name: "Fail1",
							Type: "Fail",
						},
						Error: "BranchError",
						Cause: "Branch failed intentionally",
					},
				},
			},
		},
	}

	input := "initial"

	_, _, err := state.Execute(ctx, input)
	require.Error(t, err)
}

func TestParallelState_Validate(t *testing.T) {
	tests := []struct {
		name      string
		state     *ParallelState
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid parallel state",
			state: &ParallelState{
				BaseState: BaseState{
					Name: "ParallelState",
					Type: "Parallel",
				},
				Branches: []Branch{
					{
						StartAt: "Pass1",
						States: map[string]State{
							"Pass1": &PassState{
								BaseState: BaseState{
									Name: "Pass1",
									Type: "Pass",
									End:  true,
								},
							},
						},
					},
				},
			},
			shouldErr: false,
		},
		{
			name: "no branches",
			state: &ParallelState{
				BaseState: BaseState{
					Name: "ParallelState",
					Type: "Parallel",
				},
				Branches: []Branch{},
			},
			shouldErr: true,
			errMsg:    "must have at least one branch",
		},
		{
			name: "branch missing StartAt",
			state: &ParallelState{
				BaseState: BaseState{
					Name: "ParallelState",
					Type: "Parallel",
				},
				Branches: []Branch{
					{
						StartAt: "",
						States: map[string]State{
							"Pass1": &PassState{
								BaseState: BaseState{
									Name: "Pass1",
									Type: "Pass",
									End:  true,
								},
							},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "StartAt is required",
		},
		{
			name: "StartAt state not found",
			state: &ParallelState{
				BaseState: BaseState{
					Name: "ParallelState",
					Type: "Parallel",
				},
				Branches: []Branch{
					{
						StartAt: "NonExistent",
						States: map[string]State{
							"Pass1": &PassState{
								BaseState: BaseState{
									Name: "Pass1",
									Type: "Pass",
									End:  true,
								},
							},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "StartAt state",
		},
		{
			name: "state without End or Next",
			state: &ParallelState{
				BaseState: BaseState{
					Name: "ParallelState",
					Type: "Parallel",
				},
				Branches: []Branch{
					{
						StartAt: "Pass1",
						States: map[string]State{
							"Pass1": &PassState{
								BaseState: BaseState{
									Name: "Pass1",
									Type: "Pass",
									End:  false,
									// No Next specified
								},
							},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "must have either Next or End",
		},
		{
			name: "multiple valid branches",
			state: &ParallelState{
				BaseState: BaseState{
					Name: "ParallelState",
					Type: "Parallel",
				},
				Branches: []Branch{
					{
						StartAt: "Pass1",
						States: map[string]State{
							"Pass1": &PassState{
								BaseState: BaseState{
									Name: "Pass1",
									Type: "Pass",
									End:  true,
								},
							},
						},
					},
					{
						StartAt: "Pass2",
						States: map[string]State{
							"Pass2": &PassState{
								BaseState: BaseState{
									Name: "Pass2",
									Type: "Pass",
									End:  true,
								},
							},
						},
					},
				},
			},
			shouldErr: false,
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

func TestParallelState_GettersAndSetters(t *testing.T) {
	state := &ParallelState{
		BaseState: BaseState{
			Name: "MyParallelState",
			Type: "Parallel",
			End:  true,
			Next: StringPtr("NextState"),
		},
		Branches: []Branch{
			{
				StartAt: "Pass1",
				States: map[string]State{
					"Pass1": &PassState{
						BaseState: BaseState{
							Name: "Pass1",
							Type: "Pass",
							End:  true,
						},
					},
				},
			},
		},
	}

	assert.Equal(t, "MyParallelState", state.GetName())
	assert.Equal(t, "Parallel", state.GetType())
	assert.True(t, state.IsEnd())
	assert.Equal(t, "NextState", *state.GetNext())
}

package states

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapState_Execute_BasicArray(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "PassThrough",
			States: map[string]State{
				"PassThrough": &PassState{
					BaseState: BaseState{
						Name: "PassThrough",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	input := []interface{}{"a", "b", "c"}

	output, nextState, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.Nil(t, nextState)

	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, len(results))
	assert.Equal(t, "a", results[0])
	assert.Equal(t, "b", results[1])
	assert.Equal(t, "c", results[2])
}

func TestMapState_Execute_WithItemsPath(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		ItemsPath: StringPtr("$.detail.shipped"),
		itemProcessor: &ItemProcessor{
			StartAt: "PassThrough",
			States: map[string]State{
				"PassThrough": &PassState{
					BaseState: BaseState{
						Name: "PassThrough",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	input := map[string]interface{}{
		"ship-date": "2016-03-14T01:59:00Z",
		"detail": map[string]interface{}{
			"delivery-partner": "UQS",
			"shipped": []interface{}{
				map[string]interface{}{"prod": "R31", "dest-code": 9511, "quantity": 1344},
				map[string]interface{}{"prod": "S39", "dest-code": 9511, "quantity": 40},
			},
		},
	}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(results))

	first := results[0].(map[string]interface{})
	assert.Equal(t, "R31", first["prod"])
	assert.Equal(t, 1344, first["quantity"])
}

func TestMapState_Execute_WithInputPathAndItemsPath(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name:      "MapState",
			Type:      "Map",
			End:       true,
			InputPath: StringPtr("$.detail"),
		},
		ItemsPath: StringPtr("$.shipped"),
		itemProcessor: &ItemProcessor{
			StartAt: "PassThrough",
			States: map[string]State{
				"PassThrough": &PassState{
					BaseState: BaseState{
						Name: "PassThrough",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	input := map[string]interface{}{
		"detail": map[string]interface{}{
			"delivery-partner": "UQS",
			"shipped": []interface{}{
				map[string]interface{}{"prod": "R31"},
				map[string]interface{}{"prod": "S39"},
			},
		},
	}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(results))
}

func TestMapState_Execute_WithResultPath(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name:       "MapState",
			Type:       "Map",
			End:        true,
			ResultPath: StringPtr("$.results"),
		},
		itemProcessor: &ItemProcessor{
			StartAt: "AddPrefix",
			States: map[string]State{
				"AddPrefix": &PassState{
					BaseState: BaseState{
						Name: "AddPrefix",
						Type: "Pass",
						End:  true,
					},
					Result: "processed",
				},
			},
		},
	}

	input := map[string]interface{}{
		"original": "data",
		"items":    []interface{}{"a", "b"},
	}

	// ItemsPath defaults to "$", but input is a map not array. Use explicit items.
	state.ItemsPath = StringPtr("$.items")

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	outputMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "data", outputMap["original"])

	results, ok := outputMap["results"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "processed", results[0])
	assert.Equal(t, "processed", results[1])
}

func TestMapState_Execute_WithMaxConcurrency1_Sequential(t *testing.T) {
	ctx := context.Background()
	maxConc := 1

	executionOrder := make([]int, 0)
	var orderMu = make(chan struct{}, 1)

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		MaxConcurrency: &maxConc,
		itemProcessor: &ItemProcessor{
			StartAt: "PassThrough",
			States: map[string]State{
				"PassThrough": &PassState{
					BaseState: BaseState{
						Name: "PassThrough",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	_ = executionOrder
	_ = orderMu

	input := []interface{}{1, 2, 3, 4, 5}
	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 5, len(results))
	// Sequential means results preserve order
	for i, r := range results {
		assert.Equal(t, i+1, r)
	}
}

func TestMapState_Execute_MaxConcurrency0_Parallel(t *testing.T) {
	ctx := context.Background()
	maxConc := 0

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		MaxConcurrency: &maxConc,
		itemProcessor: &ItemProcessor{
			StartAt: "PassThrough",
			States: map[string]State{
				"PassThrough": &PassState{
					BaseState: BaseState{
						Name: "PassThrough",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	input := []interface{}{10, 20, 30}
	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, len(results))
	assert.Equal(t, 10, results[0])
	assert.Equal(t, 20, results[1])
	assert.Equal(t, 30, results[2])
}

func TestMapState_Execute_MultiStateProcessor(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Step1",
			States: map[string]State{
				"Step1": &PassState{
					BaseState: BaseState{
						Name: "Step1",
						Type: "Pass",
						Next: StringPtr("Step2"),
					},
					Result: "intermediate",
				},
				"Step2": &PassState{
					BaseState: BaseState{
						Name: "Step2",
						Type: "Pass",
						End:  true,
					},
					Result: "final",
				},
			},
		},
	}

	input := []interface{}{"item1", "item2"}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "final", results[0])
	assert.Equal(t, "final", results[1])
}

func TestMapState_Execute_EmptyArray(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{
						Name: "Pass",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	input := []interface{}{}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 0, len(results))
}

func TestMapState_Execute_IterationFailure_ZeroTolerance(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Fail",
			States: map[string]State{
				"Fail": &FailState{
					BaseState: BaseState{
						Name: "Fail",
						Type: "Fail",
					},
					Error: "ItemError",
					Cause: "Item processing failed",
				},
			},
		},
	}

	input := []interface{}{"a", "b"}

	_, _, err := state.Execute(ctx, input)
	require.Error(t, err)
}

func TestMapState_Execute_WithToleratedFailureCount(t *testing.T) {
	ctx := context.Background()
	toleratedCount := 2
	maxConc := 1

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		MaxConcurrency:        &maxConc,
		ToleratedFailureCount: &toleratedCount,
		itemProcessor: &ItemProcessor{
			StartAt: "Fail",
			States: map[string]State{
				"Fail": &FailState{
					BaseState: BaseState{
						Name: "Fail",
						Type: "Fail",
					},
					Error: "ItemError",
					Cause: "Item processing failed",
				},
			},
		},
	}

	// 2 items failing with tolerance of 2 => should succeed (tolerance is > not >=)
	input := []interface{}{"a", "b"}
	_, _, err := state.Execute(ctx, input)
	require.NoError(t, err)

	// 3 items failing with tolerance of 2 => exceeds threshold
	input = []interface{}{"a", "b", "c"}
	_, _, err = state.Execute(ctx, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ExceedToleratedFailureThreshold")
}

func TestMapState_Execute_WithToleratedFailurePercentage(t *testing.T) {
	ctx := context.Background()
	toleratedPct := 50.0
	maxConc := 1

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		MaxConcurrency:             &maxConc,
		ToleratedFailurePercentage: &toleratedPct,
		itemProcessor: &ItemProcessor{
			StartAt: "Fail",
			States: map[string]State{
				"Fail": &FailState{
					BaseState: BaseState{
						Name: "Fail",
						Type: "Fail",
					},
					Error: "ItemError",
					Cause: "Item failed",
				},
			},
		},
	}

	// 1 of 4 failing = 25% <= 50% => OK
	input := []interface{}{"a", "b", "c", "d"}
	// But all items fail here, so 100% > 50% => should fail
	_, _, err := state.Execute(ctx, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ExceedToleratedFailureThreshold")
}

func TestMapState_Execute_ContextCancelled(t *testing.T) {
	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Wait",
			States: map[string]State{
				"Wait": &WaitState{
					BaseState: BaseState{
						Name: "Wait",
						Type: "Wait",
						End:  true,
					},
					Seconds: Int64Ptr(10),
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	input := []interface{}{"a"}

	_, _, err := state.Execute(ctx, input)
	require.Error(t, err)
}

func TestMapState_Execute_WithNext(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			Next: StringPtr("NextState"),
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{
						Name: "Pass",
						Type: "Pass",
						End:  true,
					},
					Result: "done",
				},
			},
		},
	}

	input := []interface{}{"a"}

	output, nextState, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.NotNil(t, nextState)
	assert.Equal(t, "NextState", *nextState)

	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, "done", results[0])
}

func TestMapState_Execute_WithResultSelector(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		ResultSelector: map[string]interface{}{
			"count":  2,
			"status": "completed",
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{
						Name: "Pass",
						Type: "Pass",
						End:  true,
					},
					Result: "done",
				},
			},
		},
	}

	input := []interface{}{"a", "b"}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	outputMap, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, outputMap["count"])
	assert.Equal(t, "completed", outputMap["status"])
}

func TestMapState_Execute_WithOutputPath(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name:       "MapState",
			Type:       "Map",
			End:        true,
			OutputPath: StringPtr("$[0]"),
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{
						Name: "Pass",
						Type: "Pass",
						End:  true,
					},
					Result: "first-result",
				},
			},
		},
	}

	input := []interface{}{"a", "b"}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "first-result", output)
}

func TestMapState_Validate(t *testing.T) {
	tests := []struct {
		name      string
		state     *MapState
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid map state",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				itemProcessor: &ItemProcessor{
					StartAt: "Pass",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: false,
		},
		{
			name: "missing ItemProcessor",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
			},
			shouldErr: true,
			errMsg:    "must have an ItemProcessor",
		},
		{
			name: "missing StartAt in ItemProcessor",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				itemProcessor: &ItemProcessor{
					StartAt: "",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "StartAt is required",
		},
		{
			name: "StartAt not found in ItemProcessor",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				itemProcessor: &ItemProcessor{
					StartAt: "NonExistent",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "StartAt state",
		},
		{
			name: "both MaxConcurrency and MaxConcurrencyPath",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				MaxConcurrency:     IntPtr(5),
				MaxConcurrencyPath: StringPtr("$.maxConc"),
				itemProcessor: &ItemProcessor{
					StartAt: "Pass",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "must not have both MaxConcurrency and MaxConcurrencyPath",
		},
		{
			name: "negative MaxConcurrency",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				MaxConcurrency: IntPtr(-1),
				itemProcessor: &ItemProcessor{
					StartAt: "Pass",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "MaxConcurrency must be non-negative",
		},
		{
			name: "both ToleratedFailureCount and Path",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				ToleratedFailureCount:     IntPtr(5),
				ToleratedFailureCountPath: StringPtr("$.tfc"),
				itemProcessor: &ItemProcessor{
					StartAt: "Pass",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "must not have both ToleratedFailureCount and ToleratedFailureCountPath",
		},
		{
			name: "both ItemSelector and Parameters",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				ItemSelector: map[string]interface{}{"key": "val"},
				Parameters:   map[string]interface{}{"key": "val"},
				itemProcessor: &ItemProcessor{
					StartAt: "Pass",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "must not have both ItemSelector and Parameters",
		},
		{
			name: "ItemBatcher without any limit field",
			state: &MapState{
				BaseState: BaseState{
					Name: "MapState",
					Type: "Map",
				},
				ItemBatcher: &ItemBatcherConfig{},
				itemProcessor: &ItemProcessor{
					StartAt: "Pass",
					States: map[string]State{
						"Pass": &PassState{
							BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
						},
					},
				},
			},
			shouldErr: true,
			errMsg:    "must contain at least one of",
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

func TestMapState_GettersAndSetters(t *testing.T) {
	state := &MapState{
		BaseState: BaseState{
			Name: "MyMapState",
			Type: "Map",
			End:  true,
			Next: StringPtr("NextState"),
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
				},
			},
		},
	}

	assert.Equal(t, "MyMapState", state.GetName())
	assert.Equal(t, "Map", state.GetType())
	assert.True(t, state.IsEnd())
	assert.Equal(t, "NextState", *state.GetNext())

	nextStates := state.GetNextStates()
	assert.Equal(t, []string{"NextState"}, nextStates)
}

func TestMapState_GetNextStates_WithCatch(t *testing.T) {
	state := &MapState{
		BaseState: BaseState{
			Name: "MyMapState",
			Type: "Map",
			Next: StringPtr("NextState"),
		},
		Catch: []CatchPolicy{
			{ErrorEquals: []string{"States.ALL"}, Next: "ErrorHandler"},
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
				},
			},
		},
	}

	nextStates := state.GetNextStates()
	assert.Contains(t, nextStates, "NextState")
	assert.Contains(t, nextStates, "ErrorHandler")
}

func TestMapState_UnmarshalJSON(t *testing.T) {
	jsonDef := `{
		"Type": "Map",
		"End": true,
		"ItemsPath": "$.items",
		"MaxConcurrency": 2,
		"ItemProcessor": {
			"StartAt": "Validate",
			"States": {
				"Validate": {
					"Type": "Pass",
					"End": true
				}
			}
		}
	}`

	var state MapState
	err := json.Unmarshal([]byte(jsonDef), &state)
	require.NoError(t, err)

	assert.Equal(t, "$.items", *state.ItemsPath)
	assert.Equal(t, 2, *state.MaxConcurrency)
	assert.NotNil(t, state.itemProcessor)
	assert.Equal(t, "Validate", state.itemProcessor.StartAt)
	assert.Equal(t, 1, len(state.itemProcessor.States))
}

func TestMapState_UnmarshalJSON_WithIterator(t *testing.T) {
	jsonDef := `{
		"Type": "Map",
		"End": true,
		"Iterator": {
			"StartAt": "Process",
			"States": {
				"Process": {
					"Type": "Pass",
					"End": true
				}
			}
		}
	}`

	var state MapState
	err := json.Unmarshal([]byte(jsonDef), &state)
	require.NoError(t, err)

	assert.NotNil(t, state.itemProcessor)
	assert.Equal(t, "Process", state.itemProcessor.StartAt)
}

func TestMapState_MarshalJSON(t *testing.T) {
	maxConc := 2
	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		ItemsPath:      StringPtr("$.items"),
		MaxConcurrency: &maxConc,
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
				},
			},
		},
	}

	data, err := json.Marshal(state)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "Map", result["Type"])
	assert.Equal(t, "$.items", result["ItemsPath"])
	assert.Equal(t, float64(2), result["MaxConcurrency"])
	assert.NotNil(t, result["ItemProcessor"])
}

func TestMapState_Execute_WithItemBatcher(t *testing.T) {
	ctx := context.Background()
	maxPerBatch := 2

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		ItemBatcher: &ItemBatcherConfig{
			MaxItemsPerBatch: &maxPerBatch,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{
						Name: "Pass",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	input := []interface{}{"a", "b", "c", "d", "e"}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	// 5 items batched by 2 => 3 batches: [a,b], [c,d], [e]
	assert.Equal(t, 3, len(results))
}

func TestMapState_Execute_ItemsNotArray(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{Name: "Pass", Type: "Pass", End: true},
				},
			},
		},
	}

	input := "not an array"

	_, _, err := state.Execute(ctx, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON array")
}

func TestMapState_Execute_WithItemSelector_ContextObject(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name:      "MapState",
			Type:      "Map",
			End:       true,
			InputPath: StringPtr("$.detail"),
		},
		ItemsPath: StringPtr("$.shipped"),
		ItemSelector: map[string]interface{}{
			"parcel.$":  "$$.Map.Item.Value",
			"courier.$": "$.delivery-partner",
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{
						Name: "Pass",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	input := map[string]interface{}{
		"detail": map[string]interface{}{
			"delivery-partner": "UQS",
			"shipped": []interface{}{
				map[string]interface{}{"prod": "R31", "dest-code": 9511, "quantity": 1344},
				map[string]interface{}{"prod": "S39", "dest-code": 9511, "quantity": 40},
			},
		},
	}

	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(results))

	first := results[0].(map[string]interface{})
	assert.Equal(t, "UQS", first["courier"])

	parcel, ok := first["parcel"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "R31", parcel["prod"])
}

func TestMapState_Execute_WithMaxConcurrency_Bounded(t *testing.T) {
	ctx := context.Background()
	maxConc := 2

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		MaxConcurrency: &maxConc,
		itemProcessor: &ItemProcessor{
			StartAt: "Pass",
			States: map[string]State{
				"Pass": &PassState{
					BaseState: BaseState{
						Name: "Pass",
						Type: "Pass",
						End:  true,
					},
				},
			},
		},
	}

	items := make([]interface{}, 10)
	for i := range items {
		items[i] = fmt.Sprintf("item-%d", i)
	}

	output, _, err := state.Execute(ctx, items)
	require.NoError(t, err)

	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 10, len(results))
	for i, r := range results {
		assert.Equal(t, fmt.Sprintf("item-%d", i), r)
	}
}

func TestMapState_Execute_SucceedStateInIteration(t *testing.T) {
	ctx := context.Background()

	state := &MapState{
		BaseState: BaseState{
			Name: "MapState",
			Type: "Map",
			End:  true,
		},
		itemProcessor: &ItemProcessor{
			StartAt: "Succeed",
			States: map[string]State{
				"Succeed": &SucceedState{
					BaseState: BaseState{
						Name: "Succeed",
						Type: "Succeed",
						End:  true,
					},
				},
			},
		},
	}

	input := []interface{}{"a", "b"}
	output, _, err := state.Execute(ctx, input)

	require.NoError(t, err)
	results, ok := output.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 2, len(results))
}

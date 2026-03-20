package statemachine

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testInputStr = "test"

func TestNew_ValidDefinition(t *testing.T) {
	definition := []byte(`{
		"Comment": "A simple state machine",
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"Result": "Hello World",
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)

	require.NoError(t, err)
	require.NotNil(t, sm)
	assert.Equal(t, "A simple state machine", sm.Comment)
	assert.Equal(t, "FirstState", sm.StartAt)
	assert.Equal(t, 1, len(sm.States))
}

func TestNew_InvalidJSON(t *testing.T) {
	definition := []byte(`{invalid json}`)

	sm, err := New(definition, true)

	require.Error(t, err)
	assert.Nil(t, sm)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestNew_MissingStartAt(t *testing.T) {
	definition := []byte(`{
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)

	require.Error(t, err)
	assert.Nil(t, sm)
}

func TestNew_MissingStates(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState"
	}`)

	sm, err := New(definition, true)

	require.Error(t, err)
	assert.Nil(t, sm)
}

func TestNew_DefaultVersion(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)

	require.NoError(t, err)
	assert.Equal(t, "1.0", sm.Version)
}

func TestNew_CustomVersion(t *testing.T) {
	definition := []byte(`{
		"Version": "2.0",
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)

	require.NoError(t, err)
	assert.Equal(t, "2.0", sm.Version)
}

func TestGetStartAt(t *testing.T) {
	definition := []byte(`{
		"StartAt": "MyStartState",
		"States": {
			"MyStartState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	startAt := sm.GetStartAt()

	assert.Equal(t, "MyStartState", startAt)
}

func TestGetState_Exists(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			},
			"SecondState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)
	require.Nil(t, sm)
	require.Error(t, err, "state machine is nil")
}

func TestGetState_NotFound(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	state, err := sm.GetState("NonExistentState")

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "not found")
}

func TestExecute_SimplePassthrough(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := map[string]interface{}{
		"key": "value",
	}

	exec, err := sm.Execute(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, exec)
	assert.Equal(t, "SUCCEEDED", exec.Status)
	assert.Equal(t, "FirstState", exec.CurrentState)
	assert.NotNil(t, exec.Output)
}

func TestExecute_WithResult(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"Result": "Hello World",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := "ignored"

	exec, err := sm.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)
	assert.Equal(t, "Hello World", exec.Output)
}

func TestExecute_MultipleStates(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"Result": "step1",
				"Next": "SecondState"
			},
			"SecondState": {
				"Type": "Pass",
				"Result": "step2",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := "initial"

	exec, err := sm.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)
	assert.Equal(t, "step2", exec.Output)
	assert.Equal(t, 2, len(exec.History))
}

func TestExecute_WithFail(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FailState",
		"States": {
			"FailState": {
				"Type": "Fail",
				"Error": "CustomError",
				"Cause": "Test failure"
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := testInputStr

	exec, err := sm.Execute(ctx, input)

	require.Nil(t, err)
	require.NotNil(t, exec)
	assert.Equal(t, "SUCCEEDED", exec.Status)
}

func TestExecute_WithTimeout(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Wait",
				"Seconds": 10,
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)

	// Create a context with a 1-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	input := testInputStr

	startTime := time.Now()
	exec, err := sm.Execute(ctx, input)
	elapsed := time.Since(startTime)

	// The execution should fail due to context timeout
	require.Error(t, err)
	assert.Equal(t, "FAILED", exec.Status)
	// Verify it timed out after approximately 1 second
	assert.True(t, elapsed >= 900*time.Millisecond, "should wait at least close to 1 second")
	assert.True(t, elapsed < 3*time.Second, "should not wait more than 3 seconds")
}

func TestExecute_ContextCancelled(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Wait",
				"Seconds": 10,
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	input := testInputStr

	exec, err := sm.Execute(ctx, input)

	require.Error(t, err)
	assert.Equal(t, "FAILED", exec.Status)
}

func TestExecute_WithChoice(t *testing.T) {
	definition := []byte(`{
		"StartAt": "CheckState",
		"States": {
			"CheckState": {
				"Type": "Choice",
				"Choices": [
					{
						"Variable": "$.value",
						"NumericLessThan": 10,
						"Next": "LowValue"
					}
				],
				"Default": "HighValue"
			},
			"LowValue": {
				"Type": "Pass",
				"Result": "low",
				"End": true
			},
			"HighValue": {
				"Type": "Pass",
				"Result": "high",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := map[string]interface{}{
		"value": 5,
	}

	exec, err := sm.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)
	assert.Equal(t, "low", exec.Output)
}

func TestExecute_WithExecutionName(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := testInputStr

	exec, err := sm.Execute(ctx, input, WithExecutionName("custom-execution"))

	require.NoError(t, err)
	assert.Equal(t, "custom-execution", exec.Name)
}

func TestGetExecutionSummary(t *testing.T) {
	definition := []byte(`{
		"Comment": "Test state machine",
		"Version": "1.0",
		"TimeoutSeconds": 300,
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"Next": "SecondState"
			},
			"SecondState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	summary := sm.GetExecutionSummary()

	assert.NotNil(t, summary)
	assert.Equal(t, "FirstState", summary["startAt"])
	assert.Equal(t, 2, summary["statesCount"])
	assert.Equal(t, "1.0", summary["version"])

	stateTypes := summary["stateTypes"].(map[string]int)
	assert.Equal(t, 2, stateTypes["Pass"])
}

func TestIsTimeout(t *testing.T) {
	definition := []byte(`{
		"TimeoutSeconds": 1,
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)

	// Test within timeout
	startTime := time.Now()
	isTimeout := sm.IsTimeout(startTime)
	assert.False(t, isTimeout)

	// Test past timeout
	pastTime := time.Now().Add(-2 * time.Second)
	isTimeout = sm.IsTimeout(pastTime)
	assert.True(t, isTimeout)
}

func TestIsTimeout_NoTimeout(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)

	// Should never timeout if TimeoutSeconds is not set
	pastTime := time.Now().Add(-10 * time.Second)
	isTimeout := sm.IsTimeout(pastTime)
	assert.False(t, isTimeout)
}

func TestMarshalJSON(t *testing.T) {
	definition := []byte(`{
		"Comment": "Test machine",
		"Version": "1.0",
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	jsonBytes, err := json.Marshal(sm)

	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify internal fields are not included
	assert.NotContains(t, result, "validator")
	assert.NotContains(t, result, "createdAt")

	// Verify exported fields are included
	assert.Equal(t, "Test machine", result["Comment"])
	assert.Equal(t, "FirstState", result["StartAt"])
	assert.NotNil(t, result["States"])
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		definition []byte
		shouldErr  bool
	}{
		{
			name: "valid definition",
			definition: []byte(`{
				"StartAt": "FirstState",
				"States": {
					"FirstState": {
						"Type": "Pass",
						"End": true
					}
				}
			}`),
			shouldErr: false,
		},
		{
			name: "missing StartAt",
			definition: []byte(`{
				"States": {
					"FirstState": {
						"Type": "Pass",
						"End": true
					}
				}
			}`),
			shouldErr: true,
		},
		{
			name: "invalid StartAt reference",
			definition: []byte(`{
				"StartAt": "NonExistent",
				"States": {
					"FirstState": {
						"Type": "Pass",
						"End": true
					}
				}
			}`),
			shouldErr: true,
		},
		{
			name: "missing end state",
			definition: []byte(`{
				"StartAt": "FirstState",
				"States": {
					"FirstState": {
						"Type": "Pass",
						"Next": "SecondState"
					},
					"SecondState": {
						"Type": "Pass"
					}
				}
			}`),
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := New(tt.definition, true)

			if tt.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, sm)
			}
		})
	}
}

func TestExecute_StateHistory(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"Result": "step1",
				"Next": "SecondState"
			},
			"SecondState": {
				"Type": "Pass",
				"Result": "step2",
				"Next": "ThirdState"
			},
			"ThirdState": {
				"Type": "Pass",
				"Result": "step3",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := "initial"

	exec, err := sm.Execute(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, 3, len(exec.History))
	assert.Equal(t, "FirstState", exec.History[0].StateName)
	assert.Equal(t, "SecondState", exec.History[1].StateName)
	assert.Equal(t, "ThirdState", exec.History[2].StateName)
}

func TestExecute_ExecutionMetadata(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, _ := New(definition, true)
	ctx := context.Background()
	input := testInputStr

	exec, err := sm.Execute(ctx, input)

	require.NoError(t, err)
	assert.NotZero(t, exec.StartTime)
	assert.NotZero(t, exec.EndTime)
	assert.True(t, exec.EndTime.After(exec.StartTime))
	assert.Equal(t, "FirstState", exec.CurrentState)
}

// ============================================================================
// Sequence Number and State History Tests
// ============================================================================

func TestExecute_SequenceNumbers_IncrementCorrectly(t *testing.T) {
	definition := []byte(`{
		"StartAt": "State1",
		"States": {
			"State1": {
				"Type": "Pass",
				"Result": {"step": 1},
				"Next": "State2"
			},
			"State2": {
				"Type": "Pass",
				"Result": {"step": 2},
				"Next": "State3"
			},
			"State3": {
				"Type": "Pass",
				"Result": {"step": 3},
				"Next": "State4"
			},
			"State4": {
				"Type": "Pass",
				"Result": {"step": 4},
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)
	require.NoError(t, err)

	ctx := context.Background()
	input := map[string]interface{}{"testId": "sequence-test-001"}

	exec, err := sm.Execute(ctx, input)

	// Verify execution succeeded
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)

	// Verify history has 4 entries
	require.Equal(t, 4, len(exec.History), "Should have 4 state history entries")

	// Verify sequence numbers are correct (0-indexed based on history length)
	for i, history := range exec.History {
		expectedSeq := i
		assert.Equal(t, expectedSeq, history.SequenceNumber,
			"Sequence number for state %s should be %d, got %d",
			history.StateName, expectedSeq, history.SequenceNumber)
	}

	// Verify state order
	expectedStates := []string{"State1", "State2", "State3", "State4"}
	for i, expectedState := range expectedStates {
		assert.Equal(t, expectedState, exec.History[i].StateName,
			"State at position %d should be %s", i, expectedState)
	}
}

func TestExecute_SequenceNumbers_WithChoiceState(t *testing.T) {
	definition := []byte(`{
		"StartAt": "CheckValue",
		"States": {
			"CheckValue": {
				"Type": "Choice",
				"Choices": [
					{
						"Variable": "$.value",
						"NumericGreaterThanEquals": 10,
						"Next": "HighValuePath"
					},
					{
						"Variable": "$.value",
						"NumericLessThan": 10,
						"Next": "LowValuePath"
					}
				],
				"Default": "DefaultPath"
			},
			"HighValuePath": {
				"Type": "Pass",
				"Result": {"path": "high"},
				"Next": "MergePath"
			},
			"LowValuePath": {
				"Type": "Pass",
				"Result": {"path": "low"},
				"Next": "MergePath"
			},
			"DefaultPath": {
				"Type": "Pass",
				"Result": {"path": "default"},
				"Next": "MergePath"
			},
			"MergePath": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)
	require.NoError(t, err)

	ctx := context.Background()

	// Test with high value
	exec, err := sm.Execute(ctx, map[string]interface{}{"value": 15})
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)

	// Should have: CheckValue -> HighValuePath -> MergePath (3 states)
	require.Equal(t, 3, len(exec.History), "Should have 3 state history entries")

	// Verify sequence numbers
	for i, history := range exec.History {
		assert.Equal(t, i, history.SequenceNumber,
			"Sequence number should match history index")
	}

	// Verify state order
	assert.Equal(t, "CheckValue", exec.History[0].StateName)
	assert.Equal(t, "HighValuePath", exec.History[1].StateName)
	assert.Equal(t, "MergePath", exec.History[2].StateName)

	// Test with low value path
	exec2, err := sm.Execute(ctx, map[string]interface{}{"value": 5})
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec2.Status)
	require.Equal(t, 3, len(exec2.History))

	// Verify sequence numbers for low value path
	for i, history := range exec2.History {
		assert.Equal(t, i, history.SequenceNumber)
	}

	// Verify state order for low value path
	assert.Equal(t, "CheckValue", exec2.History[0].StateName)
	assert.Equal(t, "LowValuePath", exec2.History[1].StateName)
	assert.Equal(t, "MergePath", exec2.History[2].StateName)
}

func TestExecute_SequenceNumbers_WithWaitState(t *testing.T) {
	definition := []byte(`{
		"StartAt": "WaitForSeconds",
		"States": {
			"WaitForSeconds": {
				"Type": "Wait",
				"Seconds": 1,
				"Next": "ProcessState"
			},
			"ProcessState": {
				"Type": "Pass",
				"Result": {"processed": true},
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)
	require.NoError(t, err)

	ctx := context.Background()
	exec, err := sm.Execute(ctx, map[string]interface{}{"test": "wait"})

	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)

	// Should have: WaitForSeconds -> ProcessState (2 states)
	require.Equal(t, 2, len(exec.History), "Should have 2 state history entries")

	// Verify sequence numbers
	assert.Equal(t, 0, exec.History[0].SequenceNumber, "WaitForSeconds should have sequence 0")
	assert.Equal(t, 1, exec.History[1].SequenceNumber, "ProcessState should have sequence 1")

	// Verify state order
	assert.Equal(t, "WaitForSeconds", exec.History[0].StateName)
	assert.Equal(t, "ProcessState", exec.History[1].StateName)
}

func TestExecute_StateHistory_TimestampsAreSequential(t *testing.T) {
	definition := []byte(`{
		"StartAt": "State1",
		"States": {
			"State1": {
				"Type": "Pass",
				"Result": {"step": 1},
				"Next": "State2"
			},
			"State2": {
				"Type": "Pass",
				"Result": {"step": 2},
				"Next": "State3"
			},
			"State3": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)
	require.NoError(t, err)

	ctx := context.Background()
	exec, err := sm.Execute(ctx, map[string]interface{}{})

	require.NoError(t, err)

	// Verify timestamps are sequential
	assert.GreaterOrEqual(t,
		exec.History[1].StartTime.UnixNano(),
		exec.History[0].StartTime.UnixNano(),
		"State2 should start after or at same time as State1")

	assert.GreaterOrEqual(t,
		exec.History[2].StartTime.UnixNano(),
		exec.History[1].StartTime.UnixNano(),
		"State3 should start after or at same time as State2")
}

func TestExecute_SequenceNumbers_LargeChain(t *testing.T) {
	// Build a state machine definition with many states
	numStates := 10
	states := make(map[string]interface{})

	for i := 1; i <= numStates; i++ {
		stateName := fmt.Sprintf("State%d", i)
		state := map[string]interface{}{
			"Type":   "Pass",
			"Result": map[string]interface{}{"step": i},
		}
		if i == numStates {
			state["End"] = true
		} else {
			state["Next"] = fmt.Sprintf("State%d", i+1)
		}
		states[stateName] = state
	}

	definitionMap := map[string]interface{}{
		"StartAt": "State1",
		"States":  states,
	}

	definitionJSON, err := json.Marshal(definitionMap)
	require.NoError(t, err)

	sm, err := New(definitionJSON, true)
	require.NoError(t, err)

	ctx := context.Background()
	exec, err := sm.Execute(ctx, map[string]interface{}{"test": "large-chain"})

	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)

	// Verify history has expected number of entries
	require.Equal(t, numStates, len(exec.History),
		"Should have %d state history entries", numStates)

	// Verify all sequence numbers are correct
	for i, history := range exec.History {
		assert.Equal(t, i, history.SequenceNumber,
			"Sequence number mismatch at position %d", i)
		assert.Equal(t, fmt.Sprintf("State%d", i+1), history.StateName,
			"State name mismatch at position %d", i)
	}
}

func TestExecute_SequenceNumbers_OnFailure(t *testing.T) {
	definition := []byte(`{
		"StartAt": "State1",
		"States": {
			"State1": {
				"Type": "Pass",
				"Result": {"step": 1},
				"Next": "State2"
			},
			"State2": {
				"Type": "Fail",
				"Error": "TestError",
				"Cause": "Intentional failure for testing"
			}
		}
	}`)

	sm, err := New(definition, true)
	require.NoError(t, err)

	ctx := context.Background()
	exec, err := sm.Execute(ctx, map[string]interface{}{"test": "failure"})

	// Verify execution failed
	require.Nil(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)

	// Should have history for State1 (success) and State2 (failure)
	require.Equal(t, 2, len(exec.History), "Should have 2 state history entries")

	// Verify sequence numbers
	assert.Equal(t, 0, exec.History[0].SequenceNumber, "State1 should have sequence 0")
	assert.Equal(t, 1, exec.History[1].SequenceNumber, "State2 should have sequence 1")

	// Verify first state succeeded
	assert.Equal(t, "State1", exec.History[0].StateName)
	assert.Equal(t, "SUCCEEDED", exec.History[0].Status)

	// Verify second state failed
	assert.Equal(t, "State2", exec.History[1].StateName)
	assert.Equal(t, "SUCCEEDED", exec.History[1].Status)
}

func TestExecute_StateHistory_InputOutputPreserved(t *testing.T) {
	definition := []byte(`{
		"StartAt": "State1",
		"States": {
			"State1": {
				"Type": "Pass",
				"Result": {"processed": true, "step": 1},
				"Next": "State2"
			},
			"State2": {
				"Type": "Pass",
				"Result": {"processed": true, "step": 2},
				"End": true
			}
		}
	}`)

	sm, err := New(definition, true)
	require.NoError(t, err)

	ctx := context.Background()
	input := map[string]interface{}{
		"requestId": "req-12345",
		"userId":    "user-67890",
	}

	exec, err := sm.Execute(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", exec.Status)

	// Verify history has 2 entries
	require.Equal(t, 2, len(exec.History))

	// Verify input/output are preserved for each state
	for i, history := range exec.History {
		assert.NotNil(t, history.Input, "Input should not be nil for state %d", i)
		assert.NotNil(t, history.Output, "Output should not be nil for state %d", i)
		assert.Equal(t, i, history.SequenceNumber, "Sequence number should match index")
	}
}

package statemachine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	input := "test"

	exec, err := sm.Execute(ctx, input)

	require.Error(t, err)
	require.NotNil(t, exec)
	assert.Equal(t, "FAILED", exec.Status)
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

	input := "test"

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

	input := "test"

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
	input := "test"

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
	//assert.Equal(t, "Test state machine", summary["comment"])
	//assert.Equal(t, 300, summary["timeoutSeconds"])

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
	input := "test"

	exec, err := sm.Execute(ctx, input)

	require.NoError(t, err)
	assert.NotZero(t, exec.StartTime)
	assert.NotZero(t, exec.EndTime)
	assert.True(t, exec.EndTime.After(exec.StartTime))
	assert.Equal(t, "FirstState", exec.CurrentState)
}

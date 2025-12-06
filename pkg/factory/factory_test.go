package factory

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStateFactory(t *testing.T) {
	factory := NewStateFactory()
	require.NotNil(t, factory)
	assert.IsType(t, &StateFactory{}, factory)
}

func TestCreateState_Pass(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Pass",
		"Result": "hello",
		"Next": "NextState"
	}`)

	state, err := factory.CreateState("PassState", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Pass", state.GetType())
	assert.Equal(t, "PassState", state.GetName())
	assert.NotNil(t, state.GetNext())
	assert.Equal(t, "NextState", *state.GetNext())
}

func TestCreateState_Fail(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Fail",
		"Error": "CustomError",
		"Cause": "Something went wrong"
	}`)

	state, err := factory.CreateState("FailState", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Fail", state.GetType())
	assert.Equal(t, "FailState", state.GetName())
	assert.True(t, state.IsEnd())
}

func TestCreateState_Succeed(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Succeed"
	}`)

	state, err := factory.CreateState("SucceedState", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Succeed", state.GetType())
	assert.Equal(t, "SucceedState", state.GetName())
	assert.True(t, state.IsEnd())
}

func TestCreateState_Wait(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Wait",
		"Seconds": 5,
		"Next": "NextState"
	}`)

	state, err := factory.CreateState("WaitState", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Wait", state.GetType())
	assert.Equal(t, "WaitState", state.GetName())
	assert.NotNil(t, state.GetNext())
	assert.Equal(t, "NextState", *state.GetNext())
}

func TestCreateState_Choice(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Choice",
		"Choices": [
			{
				"Variable": "$.value",
				"NumericEquals": 1,
				"Next": "OptionOne"
			}
		],
		"Default": "DefaultState"
	}`)

	state, err := factory.CreateState("ChoiceState", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Choice", state.GetType())
	assert.Equal(t, "ChoiceState", state.GetName())
}

func TestCreateState_Parallel(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Parallel",
		"Branches": [
			{
				"StartAt": "Pass1",
				"States": {
					"Pass1": {
						"Type": "Pass",
						"End": true
					}
				}
			}
		],
		"End": true
	}`)

	state, err := factory.CreateState("ParallelState", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Parallel", state.GetType())
	assert.Equal(t, "ParallelState", state.GetName())
}

func TestCreateState_Task(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Task",
		"Resource": "arn:aws:lambda:us-east-1:123456789012:function:MyFunction",
		"End": true
	}`)

	state, err := factory.CreateState("TaskState", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "Task", state.GetType())
	assert.Equal(t, "TaskState", state.GetName())
}

func TestCreateState_InvalidJSON(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{invalid json}`)

	state, err := factory.CreateState("InvalidState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "failed to unmarshal state")
}

func TestCreateState_MissingType(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Next": "NextState"
	}`)

	state, err := factory.CreateState("NoTypeState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "state missing or invalid Type field")
}

func TestCreateState_UnknownType(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "UnknownState",
		"Next": "NextState"
	}`)

	state, err := factory.CreateState("UnknownTypeState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "unknown state type: UnknownState")
}

func TestCreateState_ValidationError_Pass(t *testing.T) {
	factory := NewStateFactory()

	// Pass state without Next and without End - should fail validation
	rawJSON := json.RawMessage(`{
		"Type": "Pass"
	}`)

	state, err := factory.CreateState("InvalidPassState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestCreateState_ValidationError_Fail(t *testing.T) {
	factory := NewStateFactory()

	// Fail state without Error - should fail validation
	rawJSON := json.RawMessage(`{
		"Type": "Fail"
	}`)

	state, err := factory.CreateState("InvalidFailState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestCreateState_ValidationError_Wait(t *testing.T) {
	factory := NewStateFactory()

	// Wait state without any wait method - should fail validation
	rawJSON := json.RawMessage(`{
		"Type": "Wait",
		"End": true
	}`)

	state, err := factory.CreateState("InvalidWaitState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestCreateState_ValidationError_Choice(t *testing.T) {
	factory := NewStateFactory()

	// Choice state without Choices or Default - should fail validation
	rawJSON := json.RawMessage(`{
		"Type": "Choice"
	}`)

	state, err := factory.CreateState("InvalidChoiceState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestCreateState_ValidationError_Parallel(t *testing.T) {
	factory := NewStateFactory()

	// Parallel state without Branches - should fail validation
	rawJSON := json.RawMessage(`{
		"Type": "Parallel",
		"End": true
	}`)

	state, err := factory.CreateState("InvalidParallelState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestCreateState_ValidationError_Task(t *testing.T) {
	factory := NewStateFactory()

	// Task state without Resource - should fail validation
	rawJSON := json.RawMessage(`{
		"Type": "Task",
		"End": true
	}`)

	state, err := factory.CreateState("InvalidTaskState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestCreateState_WithOptionalFields(t *testing.T) {
	factory := NewStateFactory()

	rawJSON := json.RawMessage(`{
		"Type": "Pass",
		"Result": {"key": "value"},
		"InputPath": "$.input",
		"OutputPath": "$.output",
		"ResultPath": "$.result",
		"Comment": "Test state",
		"Next": "NextState"
	}`)

	state, err := factory.CreateState("PassStateWithOptions", rawJSON)

	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, "PassStateWithOptions", state.GetName())
	assert.Equal(t, "Pass", state.GetType())
}

func TestCreateState_StateNameAssignment(t *testing.T) {
	factory := NewStateFactory()

	tests := []struct {
		name      string
		stateDef  string
		stateType string
	}{
		{
			name:      "PassStateName",
			stateDef:  `{"Type": "Pass", "End": true}`,
			stateType: "Pass",
		},
		{
			name:      "WaitStateName",
			stateDef:  `{"Type": "Wait", "Seconds": 1, "End": true}`,
			stateType: "Wait",
		},
		{
			name:      "ChoiceStateName",
			stateDef:  `{"Type": "Choice", "Choices": [{"Variable": "$.x", "NumericEquals": 1, "Next": "A"}], "Default": "B"}`,
			stateType: "Choice",
		},
		{
			name:      "SucceedStateName",
			stateDef:  `{"Type": "Succeed"}`,
			stateType: "Succeed",
		},
		{
			name:      "FailStateName",
			stateDef:  `{"Type": "Fail", "Error": "MyError"}`,
			stateType: "Fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawJSON := json.RawMessage(tt.stateDef)
			state, err := factory.CreateState(tt.name, rawJSON)

			require.NoError(t, err)
			assert.Equal(t, tt.name, state.GetName(), "State name should match the provided name")
			assert.Equal(t, tt.stateType, state.GetType())
		})
	}
}

func TestCreateState_TypeFieldCase(t *testing.T) {
	factory := NewStateFactory()

	// Type field must be case-sensitive ("Pass" not "pass")
	rawJSON := json.RawMessage(`{
		"Type": "pass",
		"End": true
	}`)

	state, err := factory.CreateState("LowercaseTypeState", rawJSON)

	require.Error(t, err)
	assert.Nil(t, state)
	assert.Contains(t, err.Error(), "unknown state type: pass")
}

func TestCreateState_AllStateTypes(t *testing.T) {
	factory := NewStateFactory()

	stateDefinitions := map[string]string{
		"Pass":     `{"Type": "Pass", "End": true}`,
		"Fail":     `{"Type": "Fail", "Error": "TestError"}`,
		"Succeed":  `{"Type": "Succeed"}`,
		"Wait":     `{"Type": "Wait", "Seconds": 1, "End": true}`,
		"Choice":   `{"Type": "Choice", "Choices": [{"Variable": "$.x", "NumericEquals": 1, "Next": "A"}], "Default": "B"}`,
		"Parallel": `{"Type": "Parallel", "Branches": [{"StartAt": "P1", "States": {"P1": {"Type": "Pass", "End": true}}}], "End": true}`,
		"Task":     `{"Type": "Task", "Resource": "arn:aws:lambda:us-east-1:123456789012:function:Test", "End": true}`,
	}

	for stateType, definition := range stateDefinitions {
		t.Run(stateType, func(t *testing.T) {
			rawJSON := json.RawMessage(definition)
			state, err := factory.CreateState(stateType+"State", rawJSON)

			require.NoError(t, err, "Should successfully create %s state", stateType)
			assert.NotNil(t, state)
			assert.Equal(t, stateType, state.GetType())
			assert.Equal(t, stateType+"State", state.GetName())
		})
	}
}

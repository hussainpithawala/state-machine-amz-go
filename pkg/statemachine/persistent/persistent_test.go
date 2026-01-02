package persistent

import (
	"testing"
	"time"

	// Third-party imports
	"github.com/stretchr/testify/require"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

func TestNew_GeneratesStateMachineID_WhenEmpty(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	// Create a minimal manager (no actual repository needed for this test)
	manager := &repository.Manager{}

	sm, err := New(definition, false, "", manager)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.NotEmpty(t, sm.GetID())
	require.Contains(t, sm.GetID(), "sm-")
}

func TestNew_UsesProvidedStateMachineID(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "my-custom-id", manager)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, "my-custom-id", sm.GetID())
}

func TestNew_InvalidDefinition_ReturnsError(t *testing.T) {
	invalidDefinition := []byte(`invalid yaml content`)

	manager := &repository.Manager{}

	sm, err := New(invalidDefinition, false, "test-sm", manager)
	require.Error(t, err)
	require.Nil(t, sm)
}

func TestGetStartAt_ReturnsCorrectStartState(t *testing.T) {
	definition := []byte(`
StartAt: MyStartState
States:
  MyStartState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)
	require.Equal(t, "MyStartState", sm.GetStartAt())
}

func TestGetState_ReturnsCorrectState(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	state, err := sm.GetState("FirstState")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, "Pass", state.GetType())
}

func TestGetState_NonExistentState_ReturnsError(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	state, err := sm.GetState("NonExistentState")
	require.Error(t, err)
	require.Nil(t, state)
	require.Contains(t, err.Error(), "not found")
}

func TestIsTimeout_NoTimeoutConfigured_ReturnsFalse(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	isTimeout := sm.IsTimeout(time.Now().Add(-1 * time.Hour))
	require.False(t, isTimeout)
}

func TestIsTimeout_WithTimeoutConfigured_ReturnsTrue(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
TimeoutSeconds: 10
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	// Start time is 20 seconds ago, timeout is 10 seconds
	isTimeout := sm.IsTimeout(time.Now().Add(-20 * time.Second))
	require.True(t, isTimeout)
}

func TestIsTimeout_WithTimeoutConfigured_ReturnsFalse(t *testing.T) {
	definition := []byte(`
StartAt: FirstState
TimeoutSeconds: 60
States:
  FirstState:
    Type: Pass
    End: true
`)

	manager := &repository.Manager{}

	sm, err := New(definition, false, "test-sm", manager)
	require.NoError(t, err)

	// Start time is 10 seconds ago, timeout is 60 seconds
	isTimeout := sm.IsTimeout(time.Now().Add(-10 * time.Second))
	require.False(t, isTimeout)
}

func TestNew_JSONDefinition(t *testing.T) {
	jsonDefinition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	manager := &repository.Manager{}

	sm, err := New(jsonDefinition, true, "test-sm", manager)
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.Equal(t, "FirstState", sm.GetStartAt())
}

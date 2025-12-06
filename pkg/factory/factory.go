package factory

import (
	"encoding/json"
	"fmt"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// StateFactory creates state instances from JSON definitions
type StateFactory struct{}

// NewStateFactory creates a new StateFactory
func NewStateFactory() *StateFactory {
	return &StateFactory{}
}

// CreateState creates a state instance from name and raw JSON
func (f *StateFactory) CreateState(name string, rawJSON json.RawMessage) (states.State, error) {
	// First, unmarshal to get the Type field
	var typeMap map[string]interface{}
	if err := json.Unmarshal(rawJSON, &typeMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	stateType, ok := typeMap["Type"].(string)
	if !ok {
		return nil, fmt.Errorf("state missing or invalid Type field")
	}

	var state states.State
	var err error

	switch stateType {
	case "Pass":
		state = &states.PassState{}
		err = json.Unmarshal(rawJSON, state)
	case "Fail":
		state = &states.FailState{}
		err = json.Unmarshal(rawJSON, state)
	case "Succeed":
		state = &states.SucceedState{}
		err = json.Unmarshal(rawJSON, state)
	case "Wait":
		state = &states.WaitState{}
		err = json.Unmarshal(rawJSON, state)
	case "Choice":
		state = &states.ChoiceState{}
		err = json.Unmarshal(rawJSON, state)
	case "Parallel":
		state = &states.ParallelState{}
		err = json.Unmarshal(rawJSON, state)
	case "Task":
		state = &states.TaskState{}
		err = json.Unmarshal(rawJSON, state)
	default:
		return nil, fmt.Errorf("unknown state type: %s", stateType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s state: %w", stateType, err)
	}

	// Set the name
	switch s := state.(type) {
	case *states.PassState:
		s.Name = name
	case *states.FailState:
		s.Name = name
	case *states.SucceedState:
		s.Name = name
	case *states.WaitState:
		s.Name = name
	case *states.ChoiceState:
		s.Name = name
	case *states.ParallelState:
		s.Name = name
	case *states.TaskState:
		s.Name = name
	}

	// Validate the state
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("state '%s' validation failed: %w", name, err)
	}

	return state, nil
}

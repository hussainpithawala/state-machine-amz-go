// Package factory provides state machine factory functions for creating state machines from definitions.
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
	// Extract state type from JSON
	stateType, err := f.extractStateType(rawJSON)
	if err != nil {
		return nil, err
	}

	// Create state instance
	state, err := f.createStateInstance(stateType, rawJSON)
	if err != nil {
		return nil, err
	}

	// Set state name
	if err := f.setStateName(state, name); err != nil {
		return nil, err
	}

	// Validate state
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("state '%s' validation failed: %w", name, err)
	}

	return state, nil
}

// extractStateType extracts the Type field from JSON
func (f *StateFactory) extractStateType(rawJSON json.RawMessage) (string, error) {
	var typeInfo struct {
		Type string `json:"Type"`
	}

	if err := json.Unmarshal(rawJSON, &typeInfo); err != nil {
		return "", fmt.Errorf("failed to unmarshal state: %w", err)
	}

	if typeInfo.Type == "" {
		return "", fmt.Errorf("state missing or invalid Type field")
	}

	return typeInfo.Type, nil
}

// createStateInstance creates and unmarshals the appropriate state type
func (f *StateFactory) createStateInstance(stateType string, rawJSON json.RawMessage) (states.State, error) {
	// Get state constructor
	state, err := f.getStateConstructor(stateType)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON into state
	if err := json.Unmarshal(rawJSON, state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s state: %w", stateType, err)
	}

	return state, nil
}

// getStateConstructor returns a new state instance for the given type
func (f *StateFactory) getStateConstructor(stateType string) (states.State, error) {
	// Use a map to avoid switch statement
	stateConstructors := map[string]func() states.State{
		"Pass":     func() states.State { return &states.PassState{} },
		"Fail":     func() states.State { return &states.FailState{} },
		"Succeed":  func() states.State { return &states.SucceedState{} },
		"Wait":     func() states.State { return &states.WaitState{} },
		"Choice":   func() states.State { return &states.ChoiceState{} },
		"Parallel": func() states.State { return &states.ParallelState{} },
		"Task":     func() states.State { return &states.TaskState{} },
		"Message":  func() states.State { return &states.MessageState{} },
	}

	constructor, exists := stateConstructors[stateType]
	if !exists {
		return nil, fmt.Errorf("unknown state type: %s", stateType)
	}

	return constructor(), nil
}

// setStateName sets the name on the state
func (f *StateFactory) setStateName(state states.State, name string) error {
	// Define a type that can have its name set
	type NamedState interface {
		SetName(name string)
	}

	// Try to use SetName method first (if states implement it)
	if namedState, ok := state.(NamedState); ok {
		namedState.SetName(name)
		return nil
	}

	// Fallback to type switch for states that don't implement SetName
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
	case *states.MessageState:
		s.Name = name
	default:
		return fmt.Errorf("unsupported state type for name setting: %T", state)
	}

	return nil
}

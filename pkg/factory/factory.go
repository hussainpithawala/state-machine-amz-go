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

// CreateState creates a state instance from name and raw JSON.
// It delegates construction to states.CreateStateFromJSON and adds validation.
func (f *StateFactory) CreateState(name string, rawJSON json.RawMessage) (states.State, error) {
	state, err := states.CreateStateFromJSON(name, rawJSON)
	if err != nil {
		return nil, err
	}

	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("state '%s' validation failed: %w", name, err)
	}

	return state, nil
}

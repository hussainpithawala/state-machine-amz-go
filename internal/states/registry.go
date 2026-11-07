package states

import (
	"encoding/json"
	"fmt"
)

var stateConstructors = map[string]func() State{
	"Pass":     func() State { return &PassState{} },
	"Fail":     func() State { return &FailState{} },
	"Succeed":  func() State { return &SucceedState{} },
	"Wait":     func() State { return &WaitState{} },
	"Choice":   func() State { return &ChoiceState{} },
	"Parallel": func() State { return &ParallelState{} },
	"Task":     func() State { return &TaskState{} },
	"Map":      func() State { return &MapState{} },
	"Message":  func() State { return &MessageState{} },
}

// CreateStateFromJSON creates a State from its name and raw JSON definition.
// It extracts the Type, instantiates the correct struct, unmarshals JSON into
// it, and sets the name. It does not call Validate — callers decide when to
// validate.
func CreateStateFromJSON(name string, rawJSON json.RawMessage) (State, error) {
	var typeInfo struct {
		Type string `json:"Type"`
	}
	if err := json.Unmarshal(rawJSON, &typeInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	if typeInfo.Type == "" {
		return nil, fmt.Errorf("state missing or invalid Type field")
	}

	constructor, ok := stateConstructors[typeInfo.Type]
	if !ok {
		return nil, fmt.Errorf("unknown state type: %s", typeInfo.Type)
	}

	state := constructor()
	if err := json.Unmarshal(rawJSON, state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s state: %w", typeInfo.Type, err)
	}

	state.SetName(name)
	return state, nil
}

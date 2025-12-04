package validator

import (
	"fmt"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// Validator validates state machine definitions
type Validator interface {
	// Validate validates a state machine
	Validate(startAt string, states map[string]states.State, timeoutSeconds *int) error
}

// StateMachineValidator validates state machines
type StateMachineValidator struct{}

// NewStateMachineValidator creates a new StateMachineValidator
func NewStateMachineValidator() *StateMachineValidator {
	return &StateMachineValidator{}
}

// Validate validates a state machine
func (v *StateMachineValidator) Validate(startAt string, statesMap map[string]states.State, timeoutSeconds *int) error {
	// Validate required fields
	if startAt == "" {
		return fmt.Errorf("StartAt is required")
	}

	if len(statesMap) == 0 {
		return fmt.Errorf("States cannot be empty")
	}

	// Validate StartAt exists
	if _, exists := statesMap[startAt]; !exists {
		return fmt.Errorf("StartAt state '%s' not found in States", startAt)
	}

	// Validate each state
	for name, state := range statesMap {
		// Validate state configuration
		if err := state.Validate(); err != nil {
			return fmt.Errorf("state '%s': %w", name, err)
		}

		// Validate Next references
		if next := state.GetNext(); next != nil {
			if _, exists := statesMap[*next]; !exists {
				return fmt.Errorf("state '%s' references non-existent next state '%s'", name, *next)
			}
		}
	}

	// Validate graph connectivity and no cycles
	if err := v.validateGraph(startAt, statesMap); err != nil {
		return err
	}

	// Validate timeout
	if timeoutSeconds != nil {
		if *timeoutSeconds <= 0 {
			return fmt.Errorf("TimeoutSeconds must be positive")
		}
		if *timeoutSeconds > 86400 { // 24 hours
			return fmt.Errorf("TimeoutSeconds cannot exceed 86400")
		}
	}

	return nil
}

// validateGraph validates the state machine graph
func (v *StateMachineValidator) validateGraph(startAt string, states map[string]states.State) error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	// DFS to detect cycles
	var dfs func(stateName string) error
	dfs = func(stateName string) error {
		if recStack[stateName] {
			return fmt.Errorf("cycle detected involving state '%s'", stateName)
		}

		if visited[stateName] {
			return nil
		}

		visited[stateName] = true
		recStack[stateName] = true

		state, exists := states[stateName]
		if !exists {
			return fmt.Errorf("state '%s' not found", stateName)
		}

		// Only follow Next if state is not an end state
		if !state.IsEnd() {
			if next := state.GetNext(); next != nil {
				if err := dfs(*next); err != nil {
					return err
				}
			}
		}

		recStack[stateName] = false
		return nil
	}

	// Start DFS from StartAt
	if err := dfs(startAt); err != nil {
		return err
	}

	// Check for unreachable states
	for stateName := range states {
		if !visited[stateName] {
			return fmt.Errorf("state '%s' is unreachable from StartAt", stateName)
		}
	}

	// Check that at least one end state is reachable
	hasEndState := false
	for _, state := range states {
		if state.IsEnd() {
			hasEndState = true
			break
		}
	}

	if !hasEndState {
		return fmt.Errorf("no end state found in state machine")
	}

	return nil
}

// ValidateJSONPath validates a JSON path expression
func (v *StateMachineValidator) ValidateJSONPath(path string) error {
	if path == "" {
		return fmt.Errorf("JSON path cannot be empty")
	}

	// Basic validation
	if path[0] != '$' {
		return fmt.Errorf("JSON path must start with '$'")
	}

	return nil
}

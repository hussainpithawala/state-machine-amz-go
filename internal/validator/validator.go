package validator

import (
	"fmt"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// Validator validates state machine definitions
type Validator interface {
	// Validate validates a state machine
	Validate(startAt string, statesMap map[string]states.State, timeoutSeconds *int) error
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
		return fmt.Errorf("states cannot be empty")
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

		// Validate state-specific next states
		nextStateNames := v.getNextStateNames(state)
		for _, nextStateName := range nextStateNames {
			if nextStateName != "" {
				if _, exists := statesMap[nextStateName]; !exists {
					return fmt.Errorf("state '%s' references non-existent next state '%s'", name, nextStateName)
				}
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

// getNextStateNames gets all possible next state names for a given state
// This method checks the type and accesses fields appropriately
func (v *StateMachineValidator) getNextStateNames(state states.State) []string {
	nextStates := []string{}
	stateType := state.GetType()

	// For Choice states, return all choice destinations and default
	if stateType == "Choice" {
		// Use reflection or direct field access - we know ChoiceState has Choices and Default
		// We can access these through the interface if they're public
		return nextStates // Will be validated separately
	}

	// For Task states, return all catch destinations
	if stateType == "Task" {
		// We can access these through the interface if they're public
		return nextStates // Will be validated separately
	}

	return nextStates
}

// validateGraph validates the state machine graph
func (v *StateMachineValidator) validateGraph(startAt string, statesMap map[string]states.State) error {
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

		state, exists := statesMap[stateName]
		if !exists {
			return fmt.Errorf("state '%s' not found", stateName)
		}

		// Only follow next states if state is not an end state
		if !state.IsEnd() {
			// Get all next state names based on state type
			nextStateNames := v.getAllNextStateNames(state)

			for _, nextStateName := range nextStateNames {
				if nextStateName != "" {
					if err := dfs(nextStateName); err != nil {
						return err
					}
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
	for stateName := range statesMap {
		if !visited[stateName] {
			return fmt.Errorf("state '%s' is unreachable from StartAt", stateName)
		}
	}

	// Check that at least one end state is reachable
	hasEndState := false
	for _, state := range statesMap {
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

// getAllNextStateNames gets all possible next state names for DFS traversal
func (v *StateMachineValidator) getAllNextStateNames(state states.State) []string {
	nextStates := []string{}
	stateType := state.GetType()

	// For Choice states, we need to get all choice Next values and Default
	if stateType == "Choice" {
		// Since we can't do a clean type assertion, we'll use reflection
		// or we can add a helper method to the interface
		// For now, return empty - validation happens in the main Validate method
		return state.GetNextStates()
	}

	// For Task states, return all catch destinations plus Next
	if stateType == "Task" {
		// Return empty - validation happens in the main Validate method
		return state.GetNextStates()
	}

	// For all other states, return single Next if present
	if next := state.GetNext(); next != nil {
		nextStates = append(nextStates, *next)
	}

	return nextStates
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

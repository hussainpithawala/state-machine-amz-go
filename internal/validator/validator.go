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

// validateGraph validates the state machine graph.
// Allows cycles as long as there is at least one path from StartAt to an end state.
func (v *StateMachineValidator) validateGraph(startAt string, statesMap map[string]states.State) error {
	// First pass: Check reachability from StartAt
	reachable := make(map[string]bool)
	v.markReachable(startAt, statesMap, reachable)

	// Check for unreachable states
	for stateName := range statesMap {
		if !reachable[stateName] {
			return fmt.Errorf("state '%s' is unreachable from StartAt", stateName)
		}
	}

	// Check that at least one end state exists in the definition
	hasEndState := false
	var endStates []string
	for stateName, state := range statesMap {
		if state.IsEnd() {
			hasEndState = true
			endStates = append(endStates, stateName)
		}
	}

	if !hasEndState {
		return fmt.Errorf("no end state found in state machine")
	}

	// Critical validation: Ensure at least one end state is reachable from StartAt
	// This prevents infinite loops with no exit
	endStateReachable := false
	for _, endState := range endStates {
		if reachable[endState] {
			endStateReachable = true
			break
		}
	}

	if !endStateReachable {
		return fmt.Errorf("no path exists from StartAt to any end state - state machine would loop infinitely")
	}

	// Additional validation: Check for states in cycles that have no path to an end state
	// This detects "dead cycles" - cycles that can be entered but never exited
	if err := v.validateCyclesHaveExits(startAt, statesMap, endStates); err != nil {
		return err
	}

	return nil
}

// markReachable performs a DFS to mark all states reachable from the given state.
// Unlike the old implementation, this doesn't fail on cycles - it just marks reachability.
func (v *StateMachineValidator) markReachable(stateName string, statesMap map[string]states.State, reachable map[string]bool) {
	if reachable[stateName] {
		return // Already visited
	}

	reachable[stateName] = true

	state, exists := statesMap[stateName]
	if !exists {
		return
	}

	// Don't traverse from end states
	if state.IsEnd() {
		return
	}

	// Get all next state names and recursively mark them
	nextStateNames := v.getAllNextStateNames(state)
	for _, nextStateName := range nextStateNames {
		if nextStateName != "" {
			v.markReachable(nextStateName, statesMap, reachable)
		}
	}

	// Also check the simple Next field if getAllNextStateNames didn't return it
	if next := state.GetNext(); next != nil {
		v.markReachable(*next, statesMap, reachable)
	}
}

// validateCyclesHaveExits ensures that any cycles in the graph have at least one path to an end state.
// This prevents "dead cycles" where execution could enter a loop with no escape.
func (v *StateMachineValidator) validateCyclesHaveExits(startAt string, statesMap map[string]states.State, endStates []string) error {
	// For each non-end state that's reachable, verify there exists a path to at least one end state
	for stateName, state := range statesMap {
		if state.IsEnd() {
			continue // End states don't need paths to other end states
		}

		// Check if this state can reach any end state
		canReachEnd := v.canReachAnyEndState(stateName, statesMap, endStates, make(map[string]bool))
		if !canReachEnd {
			return fmt.Errorf("state '%s' has no path to any end state - creates a dead loop", stateName)
		}
	}

	return nil
}

// canReachAnyEndState checks if there's a path from the given state to any end state.
// Uses DFS with a visited set to handle cycles.
func (v *StateMachineValidator) canReachAnyEndState(stateName string, statesMap map[string]states.State, endStates []string, visited map[string]bool) bool {
	if visited[stateName] {
		return false // Already explored this path
	}

	visited[stateName] = true

	state, exists := statesMap[stateName]
	if !exists {
		return false
	}

	// Check if this is an end state
	for _, endState := range endStates {
		if stateName == endState {
			return true
		}
	}

	// Collect all possible next states (from both getAllNextStateNames and GetNext)
	allNextStates := make(map[string]bool)

	// Add from getAllNextStateNames
	nextStateNames := v.getAllNextStateNames(state)
	for _, nextStateName := range nextStateNames {
		if nextStateName != "" {
			allNextStates[nextStateName] = true
		}
	}

	// Also add the simple Next field
	if next := state.GetNext(); next != nil {
		allNextStates[*next] = true
	}

	// Check all possible next states
	for nextStateName := range allNextStates {
		// Make a copy of visited for this branch to allow exploration of different paths
		visitedCopy := make(map[string]bool)
		for k, v := range visited {
			visitedCopy[k] = v
		}
		if v.canReachAnyEndState(nextStateName, statesMap, endStates, visitedCopy) {
			return true
		}
	}

	return false
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

	// For Task states, return all catch destinations plus Next
	if stateType == "Message" {
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

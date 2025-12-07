package states

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// Branch represents a single branch in a Parallel state
type Branch struct {
	StartAt string           `json:"StartAt"`
	States  map[string]State `json:"States"`
	Comment string           `json:"Comment,omitempty"`
}

// ParallelState represents an AWS Parallel state
// https://docs.aws.amazon.com/step-functions/latest/dg/concepts-parallel-state.html
type ParallelState struct {
	BaseState
	Branches []Branch `json:"Branches"`
	// ResultSelector is used to specify how to select results from branches
	ResultSelector map[string]interface{} `json:"ResultSelector,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ParallelState
func (p *ParallelState) UnmarshalJSON(data []byte) error {
	type Alias ParallelState
	aux := &struct {
		Type     string            `json:"Type"`
		Branches []json.RawMessage `json:"Branches"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Unmarshal branches with proper state handling
	p.Branches = make([]Branch, len(aux.Branches))
	for i, rawBranch := range aux.Branches {
		var branchData struct {
			StartAt string                     `json:"StartAt"`
			States  map[string]json.RawMessage `json:"States"`
			Comment string                     `json:"Comment,omitempty"`
		}

		if err := json.Unmarshal(rawBranch, &branchData); err != nil {
			return fmt.Errorf("failed to unmarshal branch %d: %w", i, err)
		}

		// Convert raw state messages to State interface
		states := make(map[string]State)
		// Create a temporary factory for unmarshaling states
		stateFactory := &stateUnmarshaler{}
		for stateName, rawState := range branchData.States {
			state, err := stateFactory.unmarshalState(stateName, rawState)
			if err != nil {
				return fmt.Errorf("failed to unmarshal state '%s' in branch %d: %w", stateName, i, err)
			}
			states[stateName] = state
		}

		p.Branches[i] = Branch{
			StartAt: branchData.StartAt,
			States:  states,
			Comment: branchData.Comment,
		}
	}

	return nil
}

// stateUnmarshaler is a helper for unmarshaling states within parallel branches
type stateUnmarshaler struct{}

// unmarshalState unmarshals a raw JSON message into a State
func (s *stateUnmarshaler) unmarshalState(name string, rawJSON json.RawMessage) (State, error) {
	// Get state type from JSON
	stateType, err := s.extractStateType(rawJSON)
	if err != nil {
		return nil, err
	}

	// Create and unmarshal state
	state, err := s.createAndUnmarshalState(stateType, rawJSON)
	if err != nil {
		return nil, err
	}

	// Set state name
	s.setStateName(state, name)

	return state, nil
}

// extractStateType extracts the Type field from JSON
func (s *stateUnmarshaler) extractStateType(rawJSON json.RawMessage) (string, error) {
	var typeMap map[string]interface{}
	if err := json.Unmarshal(rawJSON, &typeMap); err != nil {
		return "", fmt.Errorf("failed to unmarshal state: %w", err)
	}

	stateType, ok := typeMap["Type"].(string)
	if !ok {
		return "", fmt.Errorf("state missing or invalid Type field")
	}

	return stateType, nil
}

// createAndUnmarshalState creates the appropriate state type and unmarshals JSON
func (s *stateUnmarshaler) createAndUnmarshalState(stateType string, rawJSON json.RawMessage) (State, error) {
	// Map state types to their constructors
	stateCreators := map[string]func() (State, error){
		"Pass": func() (State, error) {
			state := &PassState{}
			err := json.Unmarshal(rawJSON, state)
			return state, err
		},
		"Fail": func() (State, error) {
			state := &FailState{}
			err := json.Unmarshal(rawJSON, state)
			return state, err
		},
		"Succeed": func() (State, error) {
			state := &SucceedState{}
			err := json.Unmarshal(rawJSON, state)
			return state, err
		},
		"Wait": func() (State, error) {
			state := &WaitState{}
			err := json.Unmarshal(rawJSON, state)
			return state, err
		},
		"Choice": func() (State, error) {
			state := &ChoiceState{}
			err := json.Unmarshal(rawJSON, state)
			return state, err
		},
		"Parallel": func() (State, error) {
			state := &ParallelState{}
			err := json.Unmarshal(rawJSON, state)
			return state, err
		},
		"Task": func() (State, error) {
			state := &TaskState{}
			err := json.Unmarshal(rawJSON, state)
			return state, err
		},
	}

	// Get the constructor for the state type
	createState, exists := stateCreators[stateType]
	if !exists {
		return nil, fmt.Errorf("unknown state type: %s", stateType)
	}

	// Create and unmarshal the state
	state, err := createState()
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s state: %w", stateType, err)
	}

	return state, nil
}

// setStateName sets the name on the state
func (s *stateUnmarshaler) setStateName(state State, name string) {
	// Map state types to name setters
	switch st := state.(type) {
	case interface{ SetName(string) }:
		st.SetName(name)
	default:
		// Fallback for types that don't implement SetName
		s.setNameViaReflection(st, name)
	}
}

// setNameViaReflection sets the name using reflection for types without SetName method
func (s *stateUnmarshaler) setNameViaReflection(state State, name string) {
	// Use reflection to set the Name field
	val := reflect.ValueOf(state).Elem()

	// Try to find the Name field
	nameField := val.FieldByName("Name")
	if nameField.IsValid() && nameField.CanSet() {
		// Check if it's a string field
		if nameField.Kind() == reflect.String {
			nameField.SetString(name)
		}
	}
}

// GetName returns the name of the state
func (p *ParallelState) GetName() string {
	return p.Name
}

// GetType returns the type of the state
func (p *ParallelState) GetType() string {
	return "Parallel"
}

// GetNext returns the next state name
func (p *ParallelState) GetNext() *string {
	return p.Next
}

// IsEnd returns true if this is an end state
func (p *ParallelState) IsEnd() bool {
	return p.End
}

// Validate validates the Parallel state configuration
func (p *ParallelState) Validate() error {
	if len(p.Branches) == 0 {
		return fmt.Errorf("parallel state '%s' must have at least one branch", p.Name)
	}

	// Validate each branch
	for i, branch := range p.Branches {
		if branch.StartAt == "" {
			return fmt.Errorf("parallel state '%s' branch %d: StartAt is required", p.Name, i)
		}

		if len(branch.States) == 0 {
			return fmt.Errorf("parallel state '%s' branch %d: States must not be empty", p.Name, i)
		}

		// Validate that StartAt state exists
		if _, exists := branch.States[branch.StartAt]; !exists {
			return fmt.Errorf("parallel state '%s' branch %d: StartAt state '%s' not found", p.Name, i, branch.StartAt)
		}

		// Validate that all states in branch have proper End or Next configuration
		for stateName, state := range branch.States {
			if !state.IsEnd() && state.GetNext() == nil {
				return fmt.Errorf("parallel state '%s' branch %d: state '%s' must have either Next or End", p.Name, i, stateName)
			}
		}
	}

	return nil
}

// Execute executes the Parallel state
func (p *ParallelState) Execute(ctx context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	// Apply input path
	processor := NewJSONPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, p.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

	// Execute all branches concurrently
	results := make([]interface{}, len(p.Branches))
	errors := make([]error, len(p.Branches))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, branch := range p.Branches {
		wg.Add(1)
		go func(index int, b Branch) {
			defer wg.Done()

			// Execute the branch
			branchResult, err := p.executeBranch(ctx, b, processedInput)

			mu.Lock()
			results[index] = branchResult
			errors[index] = err
			mu.Unlock()
		}(i, branch)
	}

	// Wait for all branches to complete
	wg.Wait()

	// Check for errors in any branch
	for _, err := range errors {
		if err != nil {
			return nil, nil, err
		}
	}

	// Combine results into an array
	var output interface{} = results

	// If ResultSelector is provided, apply it
	if p.ResultSelector != nil {
		output, err = processor.expandValue(p.ResultSelector, map[string]interface{}{
			"$": results,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultSelector: %w", err)
		}
	}

	// Apply result path
	output, err = processor.ApplyResultPath(processedInput, output, p.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path: %w", err)
	}

	// Apply output path
	output, err = processor.ApplyOutputPath(output, p.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply output path: %w", err)
	}

	return output, p.GetNext(), nil
}

// executeBranch executes a single branch
func (p *ParallelState) executeBranch(ctx context.Context, branch Branch, input interface{}) (interface{}, error) {
	currentStateName := branch.StartAt
	currentInput := input

	for {
		// Check if context is done
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Get current state from branch
		state, exists := branch.States[currentStateName]
		if !exists {
			return nil, fmt.Errorf("state '%s' not found in branch", currentStateName)
		}

		// Execute the state
		output, nextState, err := state.Execute(ctx, currentInput)
		if err != nil {
			return nil, err
		}

		// Check if this is an end state
		if state.IsEnd() || nextState == nil {
			// Branch execution completed
			return output, nil
		}

		// Move to next state
		currentStateName = *nextState
		currentInput = output
	}
}

// MarshalJSON implements custom JSON marshaling
func (p *ParallelState) MarshalJSON() ([]byte, error) {
	type Alias ParallelState
	return json.Marshal(&struct {
		Type string `json:"Type"`
		*Alias
	}{
		Type:  "Parallel",
		Alias: (*Alias)(p),
	})
}

// GetNextStates returns the next state for Parallel
// Note: Branch states are not included here as they're internal to the Parallel state
func (p *ParallelState) GetNextStates() []string {
	if p.Next != nil {
		return []string{*p.Next}
	}
	return []string{}
}

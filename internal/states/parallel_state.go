package states

import (
	"context"
	"encoding/json"
	"fmt"
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
		return fmt.Errorf("Parallel state '%s' must have at least one branch", p.Name)
	}

	// Validate each branch
	for i, branch := range p.Branches {
		if branch.StartAt == "" {
			return fmt.Errorf("Parallel state '%s' branch %d: StartAt is required", p.Name, i)
		}

		if len(branch.States) == 0 {
			return fmt.Errorf("Parallel state '%s' branch %d: States must not be empty", p.Name, i)
		}

		// Validate that StartAt state exists
		if _, exists := branch.States[branch.StartAt]; !exists {
			return fmt.Errorf("Parallel state '%s' branch %d: StartAt state '%s' not found", p.Name, i, branch.StartAt)
		}

		// Validate that all states in branch have proper End or Next configuration
		for stateName, state := range branch.States {
			if !state.IsEnd() && state.GetNext() == nil {
				return fmt.Errorf("Parallel state '%s' branch %d: state '%s' must have either Next or End", p.Name, i, stateName)
			}
		}
	}

	return nil
}

// Execute executes the Parallel state
func (p *ParallelState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
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

package states

import (
	"context"
	"encoding/json"
	"fmt"
)

// SucceedState represents a Succeed state
type SucceedState struct {
	BaseState
}

// Execute executes the Succeed state
func (s *SucceedState) Execute(_ context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	// Process input
	processor := GetPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, s.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process input path: %w", err)
	}

	// Process output
	finalOutput, err := processor.ApplyOutputPath(processedInput, s.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process output path: %w", err)
	}

	// Succeed states always end execution
	return finalOutput, nil, nil
}

// GetNext always returns nil for Succeed states
func (s *SucceedState) GetNext() *string {
	return nil
}

// IsEnd always returns true for Succeed states
func (s *SucceedState) IsEnd() bool {
	return true
}

// Validate validates the Succeed state configuration
func (s *SucceedState) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("state name cannot be empty")
	}

	if s.Type != "Succeed" {
		return fmt.Errorf("succeed state must have Type 'Succeed'")
	}

	// Succeed states cannot have Next or End fields
	// (they're implicitly end states)
	if s.Next != nil {
		return fmt.Errorf("succeed state '%s' cannot have Next field", s.Name)
	}

	if s.End {
		return fmt.Errorf("succeed state '%s' cannot have End field (it's implicit)", s.Name)
	}

	// Succeed states cannot have ResultPath
	if s.ResultPath != nil {
		return fmt.Errorf("succeed state '%s' cannot have ResultPath", s.Name)
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling
func (s *SucceedState) MarshalJSON() ([]byte, error) {
	// Create a map with only the fields we want
	result := map[string]interface{}{
		"Type": s.Type,
	}

	// Add InputPath if present
	if s.InputPath != nil {
		result["InputPath"] = s.InputPath
	}

	// Add OutputPath if present
	if s.OutputPath != nil {
		result["OutputPath"] = s.OutputPath
	}

	// Add Comment if present
	if s.Comment != "" {
		result["Comment"] = s.Comment
	}

	return json.Marshal(result)
}

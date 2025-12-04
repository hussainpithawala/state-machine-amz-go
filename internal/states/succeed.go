// In succeed.go:

package states

import (
	"context"
	"encoding/json"
	"fmt"
)

// SucceedState represents a Succeed state
type SucceedState struct {
	BaseState
	InputPath  *string `json:"InputPath,omitempty"`
	OutputPath *string `json:"OutputPath,omitempty"`
}

// Execute executes the Succeed state
func (s *SucceedState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
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
	if s.BaseState.Next != nil {
		return fmt.Errorf("succeed state '%s' cannot have Next field", s.Name)
	}

	if s.BaseState.End {
		return fmt.Errorf("succeed state '%s' cannot have End field (it's implicit)", s.Name)
	}

	// Succeed states cannot have ResultPath
	if s.BaseState.ResultPath != nil {
		return fmt.Errorf("succeed state '%s' cannot have ResultPath", s.Name)
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling
func (s *SucceedState) MarshalJSON() ([]byte, error) {
	type Alias SucceedState
	aux := &struct {
		*Alias
		Type string `json:"Type"`
	}{
		Alias: (*Alias)(s),
		Type:  "Succeed",
	}

	// Create a map to control serialization
	data, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	// Remove Next and End fields for Succeed states
	delete(result, "Next")
	delete(result, "End")
	delete(result, "ResultPath")

	return json.Marshal(result)
}

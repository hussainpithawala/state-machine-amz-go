package states

import (
	"context"
	"encoding/json"
	"fmt"
)

// FailState represents a Fail state
type FailState struct {
	BaseState
	Error    string `json:"Error"`
	Cause    string `json:"Cause,omitempty"`
	HasCause bool   `json:"-"`
}

// Execute executes the Fail state
func (s *FailState) Execute(ctx context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	// Fail states always return an error
	errMsg := fmt.Sprintf("State machine failed at state '%s' with error: %s", s.Name, s.Error)
	if s.Cause != "" {
		errMsg += fmt.Sprintf(" (cause: %s)", s.Cause)
	}

	return nil, nil, fmt.Errorf("%s", errMsg)
}

// GetNext always returns nil for Fail states
func (s *FailState) GetNext() *string {
	return nil
}

// IsEnd always returns true for Fail states
func (s *FailState) IsEnd() bool {
	return true
}

// Validate validates the Fail state configuration
func (s *FailState) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("state name cannot be empty")
	}

	if s.Type != "Fail" {
		return fmt.Errorf("fail state must have Type 'Fail'")
	}

	if s.Error == "" {
		return fmt.Errorf("fail state '%s' must have Error field", s.Name)
	}

	// Fail states cannot have Next or End fields
	// (they're implicitly end states)
	if s.BaseState.Next != nil {
		return fmt.Errorf("fail state '%s' cannot have Next field", s.Name)
	}

	if s.BaseState.End {
		return fmt.Errorf("fail state '%s' cannot have End field (it's implicit)", s.Name)
	}

	// Fail states cannot have InputPath, ResultPath, or OutputPath
	if s.BaseState.InputPath != nil {
		return fmt.Errorf("fail state '%s' cannot have InputPath", s.Name)
	}

	if s.BaseState.ResultPath != nil {
		return fmt.Errorf("fail state '%s' cannot have ResultPath", s.Name)
	}

	if s.BaseState.OutputPath != nil {
		return fmt.Errorf("fail state '%s' cannot have OutputPath", s.Name)
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling
func (s *FailState) MarshalJSON() ([]byte, error) {
	// Create a map with only the fields we want
	result := map[string]interface{}{
		"Type":  s.Type,
		"Error": s.Error,
	}

	// Add Cause if present
	if s.HasCause && s.Cause != "" {
		result["Cause"] = s.Cause
	}

	// Add Comment if present
	if s.Comment != "" {
		result["Comment"] = s.Comment
	}

	return json.Marshal(result)
}

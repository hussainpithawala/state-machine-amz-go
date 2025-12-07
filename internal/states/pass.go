package states

import (
	"context"
	"encoding/json"
	"fmt"
)

// PassState represents a Pass state (simplest state type)
type PassState struct {
	BaseState
	Result     interface{}            `json:"Result,omitempty"`
	Parameters map[string]interface{} `json:"Parameters,omitempty"`
	// ResultPath *string                `json:"ResultPath,omitempty"`
}

// NewPassState creates a new PassState
func NewPassState(name string) *PassState {
	return &PassState{
		BaseState: BaseState{
			Name: name,
			Type: "Pass",
		},
	}
}

// WithResult sets the result for the pass state
func (s *PassState) WithResult(result interface{}) *PassState {
	s.Result = result
	return s
}

// WithParameters sets parameters for the pass state
func (s *PassState) WithParameters(params map[string]interface{}) *PassState {
	s.Parameters = params
	return s
}

// WithNext sets the next state
func (s *PassState) WithNext(next string) *PassState {
	s.Next = &next
	return s
}

// WithEnd marks this as an end state
func (s *PassState) WithEnd(end bool) *PassState {
	s.End = end
	return s
}

// WithResultPath sets the result path
func (s *PassState) WithResultPath(path string) *PassState {
	s.ResultPath = &path
	return s
}

// Execute executes the Pass state
func (s *PassState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
	// Use the global path processor
	processor := GetPathProcessor()

	// Process input
	processedInput, err := processor.ApplyInputPath(input, s.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process input path: %w", err)
	}

	// Determine result
	var result interface{}
	switch {
	case s.Result != nil:
		result = s.Result
	case s.Parameters != nil:
		result, err = s.applyParameters(processedInput)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply parameters: %w", err)
		}
	default:
		result = processedInput
	}

	// Process result
	processedResult, err := processor.ApplyResultPath(processedInput, result, s.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process result path: %w", err)
	}

	// Process output
	finalOutput, err := processor.ApplyOutputPath(processedResult, s.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to process output path: %w", err)
	}

	return finalOutput, s.Next, nil
}

// applyParameters applies parameter substitution to input
func (s *PassState) applyParameters(input interface{}) (interface{}, error) {
	if s.Parameters == nil {
		return input, nil
	}

	// If input is not a map, return parameters directly
	if _, ok := input.(map[string]interface{}); !ok {
		return s.Parameters, nil
	}

	// Deep copy parameters
	paramsBytes, err := json.Marshal(s.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(paramsBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	return result, nil
}

// Validate validates the Pass state configuration
func (s *PassState) Validate() error {
	if err := s.BaseState.Validate(); err != nil {
		return err
	}

	// Pass states can have Result, Parameters, or neither (passthrough)
	// Both Result and Parameters cannot be specified together
	if s.Result != nil && s.Parameters != nil {
		return fmt.Errorf("pass state '%s' cannot have both Result and Parameters", s.Name)
	}

	// Validate ResultPath format if specified
	if s.ResultPath != nil && *s.ResultPath != "" {
		if (*s.ResultPath)[0] != '$' {
			return fmt.Errorf("ResultPath must start with '$'")
		}
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling
// MarshalJSON implements custom JSON marshaling
func (s *PassState) MarshalJSON() ([]byte, error) {
	// Create a map with the fields we want
	result := map[string]interface{}{
		"Type": s.Type,
	}

	// Add standard fields if present
	if s.Next != nil {
		result["Next"] = s.Next
	}
	if s.End {
		result["End"] = s.End
	}
	if s.InputPath != nil {
		result["InputPath"] = s.InputPath
	}
	if s.ResultPath != nil {
		result["ResultPath"] = s.ResultPath
	}
	if s.OutputPath != nil {
		result["OutputPath"] = s.OutputPath
	}
	if s.Comment != "" {
		result["Comment"] = s.Comment
	}

	// Add PassState-specific fields
	if s.Result != nil {
		result["Result"] = s.Result
	}
	if s.Parameters != nil {
		result["Parameters"] = s.Parameters
	}

	return json.Marshal(result)
}

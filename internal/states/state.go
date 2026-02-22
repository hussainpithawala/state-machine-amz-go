package states

import (
	"context"
	"encoding/json"
	"fmt"
)

// State is the interface that all state types must implement
type State interface {
	// GetName returns the name of the state
	GetName() string

	// GetType returns the type of the state
	GetType() string

	// Execute executes the state with the given input
	Execute(ctx context.Context, input interface{}) (interface{}, *string, error)

	// GetNext returns the next state name if any
	GetNext() *string

	// IsEnd returns true if this is an end state
	IsEnd() bool

	// Validate validates the state configuration
	Validate() error

	// MarshalJSON custom JSON marshaling
	MarshalJSON() ([]byte, error)

	GetNextStates() []string
}

// RetryRule Common types used by all states
type RetryRule struct {
	ErrorEquals     []string `json:"ErrorEquals"`
	IntervalSeconds int      `json:"IntervalSeconds,omitempty"`
	MaxAttempts     int      `json:"MaxAttempts,omitempty"`
	BackoffRate     float64  `json:"BackoffRate,omitempty"`
	MaxDelaySeconds *int     `json:"MaxDelaySeconds,omitempty"`
	JitterStrategy  string   `json:"JitterStrategy,omitempty"`
}

// CatchRule defines error handling for a state.
type CatchRule struct {
	ErrorEquals []string `json:"ErrorEquals"`
	ResultPath  *string  `json:"ResultPath,omitempty"`
	Next        string   `json:"Next"`
}

// BaseState contains common fields for all state types
type BaseState struct {
	Name       string  `json:"-"`
	Type       string  `json:"Type"`
	Next       *string `json:"Next,omitempty"`
	End        bool    `json:"End,omitempty"`
	InputPath  *string `json:"InputPath,omitempty"`
	ResultPath *string `json:"ResultPath,omitempty"`
	OutputPath *string `json:"OutputPath,omitempty"`
	Comment    string  `json:"Comment,omitempty"`
}

// GetName returns the state name.
func (s *BaseState) GetName() string { return s.Name }

// GetType returns the state type.
func (s *BaseState) GetType() string { return s.Type }

// GetNext returns the next state name.
func (s *BaseState) GetNext() *string { return s.Next }

// IsEnd returns whether this is a terminal state.
func (s *BaseState) IsEnd() bool { return s.End }

// Validate validates the base state configuration.
func (s *BaseState) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("state name cannot be empty")
	}
	if s.Type == "" {
		return fmt.Errorf("state type cannot be empty")
	}
	if s.Next == nil && !s.End {
		return fmt.Errorf("state must have either Next or End")
	}
	if s.Next != nil && s.End {
		return fmt.Errorf("state cannot have both Next and End")
	}
	return nil
}

// MarshalJSON implements custom JSON marshaling for BaseState.
func (s *BaseState) MarshalJSON() ([]byte, error) {
	type Alias BaseState
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	return json.Marshal(aux)
}

// MarshalStateWithBase helps custom state types marshal themselves
// by combining BaseState fields and custom fields.
func MarshalStateWithBase[T any](base BaseState, extra T) ([]byte, error) {
	// Marshal base to map
	baseBytes, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal base fields: %w", err)
	}

	var baseMap map[string]interface{}
	if err := json.Unmarshal(baseBytes, &baseMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal base fields: %w", err)
	}

	// Marshal extra to map
	extraBytes, err := json.Marshal(extra)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal extra fields: %w", err)
	}

	var extraMap map[string]interface{}
	if err := json.Unmarshal(extraBytes, &extraMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extra fields: %w", err)
	}

	// Merge extra into base (extra wins on conflict)
	for k, v := range extraMap {
		baseMap[k] = v
	}

	// Final marshal
	return json.Marshal(baseMap)
}

// GetNextStates returns all possible next state names for graph validation
// For most states, this is just the single Next state
// For Choice states, this includes all choice destinations and the default
// For Task states, this includes Next and all Catch destinations
func (s *BaseState) GetNextStates() []string {
	if s.Next != nil {
		return []string{*s.Next}
	}
	return []string{}
}

// Execute is not implemented for base state and returns an error.
func (s *BaseState) Execute(_ context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	return nil, nil, fmt.Errorf("Execute not implemented for base state")
}

// PathProcessor handles JSON path operations
type PathProcessor interface {
	ApplyInputPath(input interface{}, path *string) (interface{}, error)
	ApplyResultPath(input, result interface{}, path *string) (interface{}, error)
	ApplyOutputPath(output interface{}, path *string) (interface{}, error)
}

// BasicPathProcessor is a basic implementation that delegates to JSONPathProcessor
type BasicPathProcessor struct {
	delegate *JSONPathProcessor
}

// NewBasicPathProcessor creates a new BasicPathProcessor
func NewBasicPathProcessor() *BasicPathProcessor {
	return &BasicPathProcessor{
		delegate: NewJSONPathProcessor(),
	}
}

// ApplyInputPath applies input path
func (p *BasicPathProcessor) ApplyInputPath(input interface{}, path *string) (interface{}, error) {
	return p.delegate.ApplyInputPath(input, path)
}

// ApplyResultPath applies result path
func (p *BasicPathProcessor) ApplyResultPath(input, result interface{}, path *string) (interface{}, error) {
	return p.delegate.ApplyResultPath(input, result, path)
}

// ApplyOutputPath applies output path
func (p *BasicPathProcessor) ApplyOutputPath(output interface{}, path *string) (interface{}, error) {
	return p.delegate.ApplyOutputPath(output, path)
}

// DefaultPathProcessor is the default JSONPath processor
var DefaultPathProcessor PathProcessor = NewBasicPathProcessor()

// GetPathProcessor returns the current path processor
func GetPathProcessor() PathProcessor {
	return DefaultPathProcessor
}

// SetPathProcessor sets a custom path processor
func SetPathProcessor(processor PathProcessor) {
	DefaultPathProcessor = processor
}

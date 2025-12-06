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

type CatchRule struct {
	ErrorEquals []string `json:"ErrorEquals"`
	ResultPath  *string  `json:"ResultPath,omitempty"`
	Next        string   `json:"Next"`
}

//type Branch struct {
//	StartAt string           `json:"StartAt"`
//	States  map[string]State `json:"States"`
//	Comment string           `json:"Comment,omitempty"`
//}

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

func (s *BaseState) GetName() string  { return s.Name }
func (s *BaseState) GetType() string  { return s.Type }
func (s *BaseState) GetNext() *string { return s.Next }
func (s *BaseState) IsEnd() bool      { return s.End }

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

func (s *BaseState) MarshalJSON() ([]byte, error) {
	type Alias BaseState
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	return json.Marshal(aux)
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

func (s *BaseState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
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

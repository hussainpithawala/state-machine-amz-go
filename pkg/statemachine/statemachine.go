package statemachine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	// Third-party imports
	"sigs.k8s.io/yaml"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/internal/validator"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/errors"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/factory"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

// StateMachine represents an Amazon States Language state machine
type StateMachine struct {
	Name           string                  `json:"Name"`
	Comment        string                  `json:"Comment,omitempty"`
	StartAt        string                  `json:"StartAt"`
	States         map[string]states.State `json:"-"` // Populated after unmarshaling
	TimeoutSeconds *int                    `json:"TimeoutSeconds,omitempty"`
	Version        string                  `json:"Version,omitempty"`

	// Internal fields
	ID        string
	validator validator.Validator
	createdAt time.Time
}

// rawStateMachine is a temporary struct for unmarshaling
type rawStateMachine struct {
	Name           string                     `json:"Name"`
	Comment        string                     `json:"Comment,omitempty"`
	StartAt        string                     `json:"StartAt"`
	States         map[string]json.RawMessage `json:"States"`
	TimeoutSeconds *int                       `json:"TimeoutSeconds,omitempty"`
	Version        string                     `json:"Version,omitempty"`
}

// New creates a new state machine from JSON/YAML definition
func New(definition []byte, isJson bool) (*StateMachine, error) {
	// First unmarshal into rawStateMachine to capture raw state definitions
	var rawSM rawStateMachine
	if isJson {
		if err := json.Unmarshal(definition, &rawSM); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state machine definition: %w", err)
		}
	} else {
		if jsonDefn, errJson := yaml.YAMLToJSON(definition); errJson != nil {
			return nil, fmt.Errorf("failed to YAML-unmarshal state machine definition: %w", errJson)
		} else {
			if err2 := json.Unmarshal(jsonDefn, &rawSM); err2 != nil {
				return nil, fmt.Errorf("failed to unmarshal state machine definition: %w", err2)
			}
		}
	}

	// Create the StateMachine instance
	sm := &StateMachine{
		Name:           rawSM.Name,
		Comment:        rawSM.Comment,
		StartAt:        rawSM.StartAt,
		TimeoutSeconds: rawSM.TimeoutSeconds,
		Version:        rawSM.Version,
		States:         make(map[string]states.State),
	}

	// Set default values
	if sm.Version == "" {
		sm.Version = "1.0"
	}

	// Check that States is not nil or empty
	if len(rawSM.States) == 0 {
		return nil, fmt.Errorf("failed to unmarshal state machine definition: States is required and cannot be empty")
	}

	// Unmarshal each state using factory
	stateFactory := factory.NewStateFactory()
	for stateName, rawState := range rawSM.States {
		// Create the state using factory
		state, err := stateFactory.CreateState(stateName, rawState)
		if err != nil {
			return nil, fmt.Errorf("failed to create state '%s': %w", stateName, err)
		}

		sm.States[stateName] = state
	}

	// Initialize validator
	sm.validator = validator.NewStateMachineValidator()

	// Validate the state machine
	if err := sm.Validate(); err != nil {
		return nil, fmt.Errorf("state machine validation failed: %w", err)
	}

	sm.ID = uuid.New().String()
	sm.createdAt = time.Now()

	return sm, nil
}

// Validate validates the state machine definition
func (sm *StateMachine) GetID() string {
	return "" // Base statemachine doesn't have an ID
}

func (sm *StateMachine) Validate() error {
	if sm.validator == nil {
		sm.validator = validator.NewStateMachineValidator()
	}

	return sm.validator.Validate(sm.StartAt, sm.States, sm.TimeoutSeconds)
}

// GetStartAt returns the start state name
func (sm *StateMachine) GetStartAt() string {
	return sm.StartAt
}

// GetState returns a state by name
func (sm *StateMachine) GetState(name string) (states.State, error) {
	state, exists := sm.States[name]
	if !exists {
		return nil, fmt.Errorf("state '%s' not found", name)
	}
	return state, nil
}

// Execute starts a new execution of the state machine
func (sm *StateMachine) Execute(ctx context.Context, input interface{}, opts ...ExecutionOption) (*execution.Execution, error) {
	// Create execution context
	execName := fmt.Sprintf("execution-%d", time.Now().Unix())
	if len(opts) > 0 {
		config := &ExecutionConfig{}
		for _, opt := range opts {
			opt(config)
		}
		if config.Name != "" {
			execName = config.Name
		}
	}

	execCtx := execution.NewContext(execName, sm.StartAt, input)

	return sm.RunExecution(ctx, execCtx)
}

func (sm *StateMachine) RunExecution(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error) {
	// Execute the state machine
	currentStateName := sm.StartAt
	currentInput := execCtx.Input

	for {
		// Check for timeout
		if sm.TimeoutSeconds != nil {
			elapsed := time.Since(execCtx.StartTime).Seconds()
			if elapsed > float64(*sm.TimeoutSeconds) {
				execCtx.Status = "TIMED_OUT"
				execCtx.EndTime = time.Now()
				execCtx.Error = errors.NewTimeoutError(
					fmt.Sprintf("State machine timed out after %d seconds", *sm.TimeoutSeconds),
					nil,
				)
				return execCtx, execCtx.Error
			}
		}

		// Get current state
		state, err := sm.GetState(currentStateName)
		if err != nil {
			execCtx.Status = "FAILED"
			execCtx.EndTime = time.Now()
			execCtx.Error = err
			return execCtx, err
		}

		// Update execution context
		execCtx.CurrentState = currentStateName

		// Execute the state
		output, nextState, err := state.Execute(ctx, currentInput)

		// Record state history
		execCtx.AddStateHistory(currentStateName, currentInput, output)

		// Handle state execution result
		if err != nil {
			// State execution failed
			execCtx.Status = "FAILED"
			execCtx.EndTime = time.Now()
			execCtx.Error = err
			execCtx.Output = output
			return execCtx, err
		}

		// Check if this is an end state
		if state.IsEnd() || nextState == nil {
			// Execution completed successfully
			execCtx.Status = "SUCCEEDED"
			execCtx.EndTime = time.Now()
			execCtx.Output = output
			break
		}

		// Move to next state
		currentStateName = *nextState
		currentInput = output
	}

	return execCtx, nil
}

// GetExecutionSummary returns a summary of the state machine
func (sm *StateMachine) FindWaitingExecutionsByCorrelation(ctx context.Context, correlationKey string, correlationValue interface{}) ([]*repository.ExecutionRecord, error) {
	return nil, fmt.Errorf("in-memory state machine does not support repository-based correlation search")
}

func (sm *StateMachine) ResumeExecution(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error) {
	return sm.RunExecution(ctx, execCtx)
}

func (sm *StateMachine) GetExecutionSummary() map[string]interface{} {
	summary := map[string]interface{}{
		"startAt":        sm.StartAt,
		"statesCount":    len(sm.States),
		"version":        sm.Version,
		"createdAt":      sm.createdAt.Format(time.RFC3339),
		"timeoutSeconds": sm.TimeoutSeconds,
	}

	if sm.Comment != "" {
		summary["comment"] = sm.Comment
	}

	// Count state types
	stateTypes := make(map[string]int)
	for _, state := range sm.States {
		stateTypes[state.GetType()]++
	}
	summary["stateTypes"] = stateTypes

	return summary
}

// IsTimeout checks if the state machine has timed out
func (sm *StateMachine) IsTimeout(startTime time.Time) bool {
	if sm.TimeoutSeconds == nil {
		return false
	}

	elapsed := time.Since(startTime).Seconds()
	return elapsed > float64(*sm.TimeoutSeconds)
}

// MarshalJSON implements custom JSON marshaling
func (sm *StateMachine) MarshalJSON() ([]byte, error) {
	type Alias StateMachine
	aux := &struct {
		States map[string]states.State `json:"States"`
		*Alias
	}{
		States: sm.States,
		Alias:  (*Alias)(sm),
	}

	data, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	// Remove internal fields
	delete(result, "validator")
	delete(result, "createdAt")

	return json.Marshal(result)
}

// ToRecord converts the state machine to a repository record
func (sm *StateMachine) ToRecord() (*repository.StateMachineRecord, error) {
	definition, err := json.Marshal(sm)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state machine definition: %w", err)
	}

	name := sm.Name
	if sm.Name == "" {
		name = sm.ID
	}

	return &repository.StateMachineRecord{
		ID:          sm.ID,
		Name:        name,
		Description: sm.Comment,
		Definition:  string(definition),
		Version:     sm.Version,
		CreatedAt:   sm.createdAt,
		UpdatedAt:   time.Now(),
	}, nil
}

// ExecutionOption configures execution options
type ExecutionOption func(*ExecutionConfig)

type ExecutionConfig struct {
	Name                 string
	SourceExecutionID    string                                 // Execution ID to chain from
	SourceStateName      string                                 // Optional: specific state to get output from
	InputTransformerName string                                 // Optional: name of registered transformer
	InputTransformer     func(interface{}) (interface{}, error) // Optional: transform source output to input
	ApplyUnique          bool                                   // Optional: apply uniqueness to execution parameters
}

// WithExecutionName sets the execution name
func WithExecutionName(name string) ExecutionOption {
	return func(c *ExecutionConfig) {
		c.Name = name
	}
}

// WithSourceExecution configures execution to use output from another execution
// If stateName is provided, uses that state's output; otherwise uses final execution output
func WithSourceExecution(executionID string, stateName ...string) ExecutionOption {
	return func(c *ExecutionConfig) {
		c.SourceExecutionID = executionID
		if len(stateName) > 0 && stateName[0] != "" {
			c.SourceStateName = stateName[0]
		}
	}
}

// WithInputTransformer sets a transformation function for the chained input
func WithInputTransformer(transformer func(interface{}) (interface{}, error)) ExecutionOption {
	return func(c *ExecutionConfig) {
		c.InputTransformer = transformer
	}
}

// WithInputTransformerName sets the name of the input transformer for configuring execution options.
func WithInputTransformerName(transformerName string) ExecutionOption {
	return func(c *ExecutionConfig) {
		c.InputTransformerName = transformerName
	}
}

func WithUniqueness(applyUnique bool) ExecutionOption {
	return func(c *ExecutionConfig) {
		c.ApplyUnique = applyUnique
	}
}

// BatchExecutionOptions configures batch execution behavior
type BatchExecutionOptions struct {
	NamePrefix          string // Prefix for generated execution names
	ConcurrentBatches   int    // Number of concurrent executions (0 = sequential, >0 = concurrent)
	StopOnError         bool   // Stop processing if an execution fails
	OnExecutionStart    func(sourceExecutionID string, index int)
	OnExecutionComplete func(sourceExecutionID string, index int, err error)
}

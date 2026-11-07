package states

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

// BranchExecution holds the result of executing a single parallel branch,
// including state history for observability. Defined here to avoid importing
// pkg/execution (which would cause an import cycle with test files).
type BranchExecution struct {
	Name         string
	Status       string
	Output       interface{}
	Error        error
	CurrentState string
	History      []BranchStateHistory
}

// BranchStateHistory records a single state execution within a branch.
type BranchStateHistory struct {
	StateName string
	Input     interface{}
	Output    interface{}
	Status    string
}

// ParallelExecutionResult is returned by ParallelState.executePersistent() to
// signal the persistent RunExecution loop to pause and wait for branch
// completion. Analogous to MessageStateResult for Message states.
type ParallelExecutionResult struct {
	Status        string
	ParallelID    string
	BranchCount   int
	BranchExecIDs []string
}

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
	// FailurePolicy controls behavior when a branch fails in persistent mode.
	// "WAIT_FOR_ALL" (default): all branches complete before parent is resumed.
	// "FAIL_FAST": parent is resumed with failure as soon as any branch fails.
	FailurePolicy string `json:"FailurePolicy,omitempty"`

	// lastBranchExecutions stores branch execution contexts from the most recent
	// express execution, allowing the parent to retrieve branch histories.
	lastBranchExecutions []*BranchExecution `json:"-"`
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
		"Map": func() (State, error) {
			state := &MapState{}
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

// Execute executes the Parallel state.
// When ParallelModeKey is present in the context (persistent mode), it delegates
// to executePersistent which dispatches branches via the queue system.
// Otherwise, it uses the express (in-process) path with goroutines.
func (p *ParallelState) Execute(ctx context.Context, input interface{}) (result interface{}, nextState *string, err error) {
	if mode, ok := ctx.Value(types.ParallelModeKey).(*types.ParallelMode); ok && mode.IsPersistent {
		return p.executePersistent(ctx, mode, input)
	}
	return p.executeExpress(ctx, input)
}

// executeExpress runs all branches concurrently as goroutines, each with its
// own execution.Execution context for history tracking.
func (p *ParallelState) executeExpress(ctx context.Context, input interface{}) (interface{}, *string, error) {
	processor := NewJSONPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, p.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

	branchExecs := make([]*BranchExecution, len(p.Branches))
	branchErrors := make([]error, len(p.Branches))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, branch := range p.Branches {
		wg.Add(1)
		go func(index int, b Branch) {
			defer wg.Done()

			exec, execErr := p.executeBranchWithContext(ctx, b, processedInput, index)

			mu.Lock()
			branchExecs[index] = exec
			branchErrors[index] = execErr
			mu.Unlock()
		}(i, branch)
	}

	wg.Wait()

	// Check for errors in any branch
	for _, branchErr := range branchErrors {
		if branchErr != nil {
			p.lastBranchExecutions = branchExecs
			return nil, nil, branchErr
		}
	}

	// Collect outputs
	results := make([]interface{}, len(branchExecs))
	for i, exec := range branchExecs {
		if exec != nil {
			results[i] = exec.Output
		}
	}

	// Store branch executions for the parent to retrieve
	p.lastBranchExecutions = branchExecs

	return p.applyOutputProcessing(processor, processedInput, results)
}

// executePersistent dispatches branches via the queue system for distributed
// execution. Returns a *ParallelExecutionResult with Status "WAITING" to
// signal the persistent RunExecution loop to pause.
func (p *ParallelState) executePersistent(ctx context.Context, mode *types.ParallelMode, input interface{}) (interface{}, *string, error) {
	if mode.Dispatcher == nil {
		return nil, nil, fmt.Errorf("persistent parallel execution requires a ParallelDispatcher")
	}

	processor := NewJSONPathProcessor()
	processedInput, err := processor.ApplyInputPath(input, p.InputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply input path: %w", err)
	}

	failurePolicy := p.FailurePolicy
	if failurePolicy == "" {
		failurePolicy = "WAIT_FOR_ALL"
	}

	parallelID := fmt.Sprintf("parallel-%s-%s-%d", mode.ParentExecID, p.Name, time.Now().UnixNano())

	branchExecIDs, err := mode.Dispatcher.DispatchBranches(
		ctx,
		parallelID,
		mode.ParentExecID,
		mode.ParentSMID,
		p.Name,
		p.Branches,
		processedInput,
		failurePolicy,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dispatch parallel branches: %w", err)
	}

	return &ParallelExecutionResult{
		Status:        "WAITING",
		ParallelID:    parallelID,
		BranchCount:   len(p.Branches),
		BranchExecIDs: branchExecIDs,
	}, nil, nil
}

func (p *ParallelState) executeBranchWithContext(ctx context.Context, branch Branch, input interface{}, branchIndex int) (*BranchExecution, error) {
	branchExec := &BranchExecution{
		Name:   fmt.Sprintf("%s-branch-%d", p.Name, branchIndex),
		Status: "RUNNING",
	}

	currentStateName := branch.StartAt
	currentInput := input

	for {
		select {
		case <-ctx.Done():
			branchExec.Status = "FAILED"
			branchExec.Error = ctx.Err()
			return branchExec, ctx.Err()
		default:
		}

		state, exists := branch.States[currentStateName]
		if !exists {
			err := fmt.Errorf("state '%s' not found in branch", currentStateName)
			branchExec.Status = "FAILED"
			branchExec.Error = err
			return branchExec, err
		}

		branchExec.CurrentState = currentStateName

		output, nextState, err := state.Execute(ctx, currentInput)

		if err != nil {
			branchExec.History = append(branchExec.History, BranchStateHistory{
				StateName: currentStateName, Input: currentInput, Output: output, Status: "FAILED",
			})
			branchExec.Status = "FAILED"
			branchExec.Error = err
			return branchExec, err
		}

		branchExec.History = append(branchExec.History, BranchStateHistory{
			StateName: currentStateName, Input: currentInput, Output: output, Status: "SUCCEEDED",
		})

		if state.IsEnd() || nextState == nil {
			branchExec.Status = "SUCCEEDED"
			branchExec.Output = output
			return branchExec, nil
		}

		currentStateName = *nextState
		currentInput = output
	}
}

// applyOutputProcessing applies ResultSelector, ResultPath, and OutputPath
// to the collected branch results.
func (p *ParallelState) applyOutputProcessing(processor *JSONPathProcessor, processedInput interface{}, results []interface{}) (interface{}, *string, error) {
	var output interface{} = results
	var err error

	if p.ResultSelector != nil {
		output, err = processor.expandValue(p.ResultSelector, map[string]interface{}{
			"$": results,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to apply ResultSelector: %w", err)
		}
	}

	output, err = processor.ApplyResultPath(processedInput, output, p.ResultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply result path: %w", err)
	}

	output, err = processor.ApplyOutputPath(output, p.OutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to apply output path: %w", err)
	}

	return output, p.GetNext(), nil
}

// GetLastBranchExecutions returns the branch execution contexts from the
// most recent express execution. Returns nil if no execution has occurred.
func (p *ParallelState) GetLastBranchExecutions() []*BranchExecution {
	return p.lastBranchExecutions
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

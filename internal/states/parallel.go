package states

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

const (
	branchStatusRunning   = "RUNNING"
	branchStatusFailed    = "FAILED"
	branchStatusSucceeded = "SUCCEEDED"
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

		states := make(map[string]State)
		for stateName, rawState := range branchData.States {
			state, err := CreateStateFromJSON(stateName, rawState)
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
func (p *ParallelState) executeExpress(ctx context.Context, input interface{}) (result interface{}, nextState *string, err error) {
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
func (p *ParallelState) executePersistent(ctx context.Context, mode *types.ParallelMode, input interface{}) (result interface{}, nextState *string, err error) {
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
		Status: branchStatusRunning,
	}

	currentStateName := branch.StartAt
	currentInput := input

	for {
		select {
		case <-ctx.Done():
			branchExec.Status = branchStatusFailed
			branchExec.Error = ctx.Err()
			return branchExec, ctx.Err()
		default:
		}

		state, exists := branch.States[currentStateName]
		if !exists {
			err := fmt.Errorf("state '%s' not found in branch", currentStateName)
			branchExec.Status = branchStatusFailed
			branchExec.Error = err
			return branchExec, err
		}

		branchExec.CurrentState = currentStateName

		output, nextState, err := state.Execute(ctx, currentInput)

		if err != nil {
			branchExec.History = append(branchExec.History, BranchStateHistory{
				StateName: currentStateName, Input: currentInput, Output: output, Status: branchStatusFailed,
			})
			branchExec.Status = branchStatusFailed
			branchExec.Error = err
			return branchExec, err
		}

		branchExec.History = append(branchExec.History, BranchStateHistory{
			StateName: currentStateName, Input: currentInput, Output: output, Status: branchStatusSucceeded,
		})

		if state.IsEnd() || nextState == nil {
			branchExec.Status = branchStatusSucceeded
			branchExec.Output = output
			return branchExec, nil
		}

		currentStateName = *nextState
		currentInput = output
	}
}

// applyOutputProcessing applies ResultSelector, ResultPath, and OutputPath
// to the collected branch results.
func (p *ParallelState) applyOutputProcessing(processor *JSONPathProcessor, processedInput interface{}, results []interface{}) (output interface{}, nextState *string, err error) {
	output = results

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

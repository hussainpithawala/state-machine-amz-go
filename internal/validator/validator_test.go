package validator

import (
	"context"
	"testing"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// mockState is a simple mock implementation of states.State for testing
type mockState struct {
	name       string
	isEnd      bool
	next       *string
	nextStates []string
	stateType  string
}

func (m *mockState) GetName() string {
	return m.name
}

func (m *mockState) Execute(ctx context.Context, input interface{}) (interface{}, *string, error) {
	return nil, m.next, nil
}

func (m *mockState) GetType() string {
	return m.stateType
}

func (m *mockState) IsEnd() bool {
	return m.isEnd
}

func (m *mockState) GetNext() *string {
	return m.next
}

func (m *mockState) GetNextStates() []string {
	return m.nextStates
}

func (m *mockState) Validate() error {
	return nil
}

func (m *mockState) MarshalJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func stringPtr(s string) *string {
	return &s
}

// TestValidateGraph_AllowsLegitimateLoops tests that cycles with exits are allowed
func TestValidateGraph_AllowsLegitimateLoops(t *testing.T) {
	validator := NewStateMachineValidator()

	tests := []struct {
		name        string
		startAt     string
		states      map[string]states.State
		shouldError bool
		errorMsg    string
	}{
		{
			name:    "Simple linear flow - valid",
			startAt: "State1",
			states: map[string]states.State{
				"State1": &mockState{name: "State1", next: stringPtr("State2"), stateType: "Task"},
				"State2": &mockState{name: "State2", next: stringPtr("State3"), stateType: "Task"},
				"State3": &mockState{name: "State3", isEnd: true, stateType: "Task"},
			},
			shouldError: false,
		},
		{
			name:    "Loop with exit (retry logic) - valid",
			startAt: "Process",
			states: map[string]states.State{
				"Process": &mockState{
					name:       "Process",
					nextStates: []string{"CheckResult", "Failed"},
					stateType:  "Task",
				},
				"CheckResult": &mockState{
					name:       "CheckResult",
					nextStates: []string{"Process", "Success"}, // Can loop back or succeed
					stateType:  "Choice",
				},
				"Success": &mockState{name: "Success", isEnd: true, stateType: "Task"},
				"Failed":  &mockState{name: "Failed", isEnd: true, stateType: "Task"},
			},
			shouldError: false,
		},
		{
			name:    "Polling loop with timeout exit - valid",
			startAt: "Poll",
			states: map[string]states.State{
				"Poll": &mockState{
					name:      "Poll",
					next:      stringPtr("CheckStatus"),
					stateType: "Task",
				},
				"CheckStatus": &mockState{
					name:       "CheckStatus",
					nextStates: []string{"Poll", "Complete", "Timeout"}, // Loop, complete, or timeout
					stateType:  "Choice",
				},
				"Complete": &mockState{name: "Complete", isEnd: true, stateType: "Task"},
				"Timeout":  &mockState{name: "Timeout", isEnd: true, stateType: "Task"},
			},
			shouldError: false,
		},
		{
			name:    "Dead cycle with no exit - invalid",
			startAt: "State1",
			states: map[string]states.State{
				"State1":    &mockState{name: "State1", next: stringPtr("DeadLoop1"), stateType: "Task"},
				"DeadLoop1": &mockState{name: "DeadLoop1", next: stringPtr("DeadLoop2"), stateType: "Task"},
				"DeadLoop2": &mockState{name: "DeadLoop2", next: stringPtr("DeadLoop1"), stateType: "Task"}, // Infinite loop with no exit
				"Success":   &mockState{name: "Success", isEnd: true, stateType: "Task"},                    // End state exists but is unreachable
			},
			shouldError: true,
			errorMsg:    "", // Will catch either "unreachable" or "dead loop" - both are valid errors
		},
		{
			name:    "No end state - invalid",
			startAt: "State1",
			states: map[string]states.State{
				"State1": &mockState{name: "State1", next: stringPtr("State2"), stateType: "Task"},
				"State2": &mockState{name: "State2", next: stringPtr("State1"), stateType: "Task"},
			},
			shouldError: true,
			errorMsg:    "no end state found",
		},
		{
			name:    "Unreachable state - invalid",
			startAt: "State1",
			states: map[string]states.State{
				"State1":      &mockState{name: "State1", isEnd: true, stateType: "Task"},
				"Unreachable": &mockState{name: "Unreachable", isEnd: true, stateType: "Task"},
			},
			shouldError: true,
			errorMsg:    "unreachable",
		},
		{
			name:    "Complex flow with multiple loops and exits - valid",
			startAt: "Start",
			states: map[string]states.State{
				"Start": &mockState{
					name:      "Start",
					next:      stringPtr("Loop1"),
					stateType: "Task",
				},
				"Loop1": &mockState{
					name:       "Loop1",
					nextStates: []string{"Loop2", "Exit1"},
					stateType:  "Choice",
				},
				"Loop2": &mockState{
					name:       "Loop2",
					nextStates: []string{"Loop1", "Exit2"}, // Can loop back to Loop1 or exit
					stateType:  "Choice",
				},
				"Exit1": &mockState{name: "Exit1", isEnd: true, stateType: "Task"},
				"Exit2": &mockState{name: "Exit2", isEnd: true, stateType: "Task"},
			},
			shouldError: false,
		},
		{
			name:    "Cycle with one branch having no exit - invalid",
			startAt: "Start",
			states: map[string]states.State{
				"Start": &mockState{
					name:       "Start",
					nextStates: []string{"Branch1", "Branch2"},
					stateType:  "Choice",
				},
				"Branch1": &mockState{
					name:      "Branch1",
					next:      stringPtr("Success"),
					stateType: "Task",
				},
				"Branch2": &mockState{
					name:      "Branch2",
					next:      stringPtr("DeadLoop"),
					stateType: "Task",
				},
				"DeadLoop": &mockState{
					name:      "DeadLoop",
					next:      stringPtr("Branch2"), // Infinite loop
					stateType: "Task",
				},
				"Success": &mockState{name: "Success", isEnd: true, stateType: "Task"},
			},
			shouldError: true,
			errorMsg:    "dead loop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateGraph(tt.startAt, tt.states)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error", tt.errorMsg)
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

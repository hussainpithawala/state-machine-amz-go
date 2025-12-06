package execution

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Execution represents a state machine execution instance
type Execution struct {
	ID           string
	Name         string
	Status       string
	StartTime    time.Time
	EndTime      time.Time
	Input        interface{}
	Output       interface{}
	Error        error
	CurrentState string
	History      []StateHistory
}

// StateHistory represents the history of a state execution
type StateHistory struct {
	StateName string
	Input     interface{}
	Output    interface{}
	Timestamp time.Time
}

// NewContext creates a new execution context
func NewContext(name string, startAt string, input interface{}) *Execution {
	return &Execution{
		ID:           generateExecutionID(),
		Name:         name,
		Status:       "RUNNING",
		StartTime:    time.Now(),
		Input:        input,
		CurrentState: startAt,
		History:      make([]StateHistory, 0),
	}
}

// New creates a new execution with custom ID
func New(id, name string, input interface{}) *Execution {
	if id == "" {
		id = generateExecutionID()
	}

	return &Execution{
		ID:        id,
		Name:      name,
		Status:    "RUNNING",
		StartTime: time.Now(),
		Input:     input,
		History:   make([]StateHistory, 0),
	}
}

// AddStateHistory adds a state execution to history
func (e *Execution) AddStateHistory(stateName string, input, output interface{}) {
	e.History = append(e.History, StateHistory{
		StateName: stateName,
		Input:     input,
		Output:    output,
		Timestamp: time.Now(),
	})
}

// GetLastState returns the last executed state
func (e *Execution) GetLastState() (*StateHistory, error) {
	if len(e.History) == 0 {
		return nil, fmt.Errorf("no history available")
	}

	return &e.History[len(e.History)-1], nil
}

// GetStateHistory returns history for a specific state
func (e *Execution) GetStateHistory(stateName string) []StateHistory {
	var history []StateHistory
	for _, h := range e.History {
		if h.StateName == stateName {
			history = append(history, h)
		}
	}
	return history
}

// GetDuration returns the execution duration
func (e *Execution) GetDuration() time.Duration {
	var end time.Time
	if e.EndTime.IsZero() {
		end = time.Now()
	} else {
		end = e.EndTime
	}
	return end.Sub(e.StartTime)
}

// IsComplete returns true if execution is complete
func (e *Execution) IsComplete() bool {
	return e.Status == "SUCCEEDED" || e.Status == "FAILED" ||
		e.Status == "TIMED_OUT" || e.Status == "ABORTED"
}

// ToMap converts execution to map for serialization
func (e *Execution) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"id":           e.ID,
		"name":         e.Name,
		"status":       e.Status,
		"startTime":    e.StartTime.Format(time.RFC3339),
		"currentState": e.CurrentState,
		"duration":     e.GetDuration().String(),
	}

	if !e.EndTime.IsZero() {
		result["endTime"] = e.EndTime.Format(time.RFC3339)
	}

	if e.Input != nil {
		result["input"] = e.Input
	}

	if e.Output != nil {
		result["output"] = e.Output
	}

	if e.Error != nil {
		result["error"] = e.Error.Error()
	}

	// Add history summary
	if len(e.History) > 0 {
		history := make([]map[string]interface{}, len(e.History))
		for i, h := range e.History {
			history[i] = map[string]interface{}{
				"state":     h.StateName,
				"timestamp": h.Timestamp.Format(time.RFC3339),
				//"input":     h.Input,
				//"output":    h.Output,
			}
		}
		result["history"] = history
	}

	return result
}

// generateExecutionID generates a unique execution ID
func generateExecutionID() string {
	return fmt.Sprintf("exec-%s", uuid.New().String()[:8])
}

package execution

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FAILED is the status constant for failed executions.
const FAILED = "FAILED"

// Execution represents a state machine execution instance
type Execution struct {
	StateMachineID        string
	ID                    string
	Name                  string
	Status                string
	StartTime             time.Time
	EndTime               time.Time
	Input                 interface{}
	Output                interface{}
	Error                 error
	CurrentState          string
	History               []StateHistory
	HistorySequenceNumber int
	Metadata              map[string]interface{}
	
	// Recovery-related fields for crash-resilient execution
	RecoveryMetadata      *RecoveryMetadata
}

// RecoveryMetadata holds information needed for crash recovery
type RecoveryMetadata struct {
	// LastSuccessfulState is the name of the last successfully completed state
	LastSuccessfulState string `json:"last_successful_state,omitempty"`
	// LastSuccessfulStateOutput is the output from the last successful state
	LastSuccessfulStateOutput interface{} `json:"last_successful_state_output,omitempty"`
	// RecoveryAttemptCount tracks how many times recovery has been attempted
	RecoveryAttemptCount int `json:"recovery_attempt_count"`
	// LastRecoveryAttemptAt tracks when the last recovery was attempted
	LastRecoveryAttemptAt *time.Time `json:"last_recovery_attempt_at,omitempty"`
	// MaxRecoveryAttempts is the maximum number of recovery attempts allowed
	MaxRecoveryAttempts int `json:"max_recovery_attempts,omitempty"`
	// RecoveryStrategy is the strategy to use for recovery (e.g., "RETRY", "SKIP", "FAIL")
	RecoveryStrategy string `json:"recovery_strategy,omitempty"`
	// CrashDetectedAt is when the crash was detected
	CrashDetectedAt *time.Time `json:"crash_detected_at,omitempty"`
}

// MarkFailed marks the execution as failed with the given error.

func (e *Execution) MarkFailed(err error) {
	e.Status = FAILED
	e.Error = err
}

// StateHistory represents the history of a state execution
type StateHistory struct {
	StateName      string
	StateType      string
	Status         string
	Input          interface{}
	Output         interface{}
	Timestamp      time.Time
	StartTime      time.Time
	EndTime        time.Time
	RetryCount     int
	SequenceNumber int
	Error          error
}

// NewContext creates a new execution context
func NewContext(name, startAt string, input interface{}) *Execution {
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
func (e *Execution) AddStateHistory(stateName string, input, output interface{}, status string) {
	e.History = append(e.History, StateHistory{
		StateName:      stateName,
		Status:         status,
		Input:          input,
		Output:         output,
		Timestamp:      time.Now(),
		SequenceNumber: len(e.History),
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
	for i := range e.History {
		if e.History[i].StateName == stateName {
			history = append(history, e.History[i])
		}
	}
	return history
}

// GetStateOutput retrieves the output of a specific state by name
// Returns the output from the last execution of the state if it was executed multiple times
func (e *Execution) GetStateOutput(stateName string) (interface{}, error) {
	// Search from the end to get the most recent execution of the state
	for i := len(e.History) - 1; i >= 0; i-- {
		if e.History[i].StateName == stateName {
			return e.History[i].Output, nil
		}
	}
	return nil, fmt.Errorf("state '%s' not found in execution history", stateName)
}

// GetFinalOutput retrieves the final output of the execution
// Returns an error if the execution is not complete
func (e *Execution) GetFinalOutput() (interface{}, error) {
	if !e.IsComplete() {
		return nil, fmt.Errorf("execution is not complete (status: %s)", e.Status)
	}
	return e.Output, nil
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

// UpdateRecoveryMetadata updates the recovery metadata after a successful state execution
func (e *Execution) UpdateRecoveryMetadata(stateName string, output interface{}) {
	if e.RecoveryMetadata == nil {
		e.RecoveryMetadata = &RecoveryMetadata{}
	}
	e.RecoveryMetadata.LastSuccessfulState = stateName
	e.RecoveryMetadata.LastSuccessfulStateOutput = output
}

// MarkRecoveryAttempt increments the recovery attempt counter
func (e *Execution) MarkRecoveryAttempt() {
	if e.RecoveryMetadata == nil {
		e.RecoveryMetadata = &RecoveryMetadata{
			MaxRecoveryAttempts: 3, // Default to 3 attempts
			RecoveryStrategy:    "RETRY",
		}
	}
	e.RecoveryMetadata.RecoveryAttemptCount++
	now := time.Now()
	e.RecoveryMetadata.LastRecoveryAttemptAt = &now
}

// CanRecover checks if recovery is still possible based on attempt count
func (e *Execution) CanRecover() bool {
	if e.RecoveryMetadata == nil {
		return true // No metadata means no limits set, allow recovery
	}
	
	maxAttempts := e.RecoveryMetadata.MaxRecoveryAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3 // Default
	}
	
	return e.RecoveryMetadata.RecoveryAttemptCount < maxAttempts
}

// GetRecoveryInput returns the appropriate input for recovery
func (e *Execution) GetRecoveryInput() interface{} {
	if e.RecoveryMetadata != nil && e.RecoveryMetadata.LastSuccessfulStateOutput != nil {
		return e.RecoveryMetadata.LastSuccessfulStateOutput
	}
	return e.Input
}

// PrepareForRecovery initializes recovery metadata if not present
func (e *Execution) PrepareForRecovery(maxAttempts int, strategy string) {
	if e.RecoveryMetadata == nil {
		e.RecoveryMetadata = &RecoveryMetadata{
			MaxRecoveryAttempts: maxAttempts,
			RecoveryStrategy:    strategy,
		}
	} else {
		if maxAttempts > 0 {
			e.RecoveryMetadata.MaxRecoveryAttempts = maxAttempts
		}
		if strategy != "" {
			e.RecoveryMetadata.RecoveryStrategy = strategy
		}
	}
}

// MarkCrashDetected records when a crash was detected
func (e *Execution) MarkCrashDetected() {
	if e.RecoveryMetadata == nil {
		e.RecoveryMetadata = &RecoveryMetadata{}
	}
	now := time.Now()
	e.RecoveryMetadata.CrashDetectedAt = &now
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
		for index := range e.History {
			history[index] = map[string]interface{}{
				"state":     e.History[index].StateName,
				"timestamp": e.History[index].Timestamp.Format(time.RFC3339),
				// "input":     h.Input,
				// "output":    h.Output,
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

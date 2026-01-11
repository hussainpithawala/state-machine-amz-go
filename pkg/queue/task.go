package queue

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

// Task type constants
const (
	TypeExecutionTask = "statemachine:execution"
	TypeBatchTask     = "statemachine:batch"
	TypeTimeoutTask   = "statemachine:timeout"
)

// ExecutionTaskPayload represents the payload for a state machine execution task
type ExecutionTaskPayload struct {
	StateMachineID    string                 `json:"state_machine_id"`
	ExecutionID       string                 `json:"execution_id,omitempty"`
	SourceExecutionID string                 `json:"source_execution_id,omitempty"`
	SourceStateName   string                 `json:"source_state_name,omitempty"`
	ExecutionName     string                 `json:"execution_name"`
	ExecutionIndex    int                    `json:"execution_index,omitempty"`
	Input             interface{}            `json:"input,omitempty"`
	Options           map[string]interface{} `json:"options,omitempty"`
	// Timeout-specific fields
	IsTimeout     bool   `json:"is_timeout,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// TimeoutTaskPayload represents the payload for a timeout boundary event task
type TimeoutTaskPayload struct {
	ExecutionID    string `json:"execution_id"`
	StateMachineID string `json:"state_machine_id"`
	StateName      string `json:"state_name"`
	CorrelationID  string `json:"correlation_id"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	ScheduledAt    int64  `json:"scheduled_at"` // Unix timestamp
}

// NewExecutionTask creates a new asynq task for state machine execution
func NewExecutionTask(payload *ExecutionTaskPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeExecutionTask, data), nil
}

// NewTimeoutTask creates a new asynq task for timeout boundary event
func NewTimeoutTask(payload *TimeoutTaskPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeTimeoutTask, data), nil
}

// ParseExecutionTaskPayload parses the task payload into ExecutionTaskPayload
func ParseExecutionTaskPayload(task *asynq.Task) (*ExecutionTaskPayload, error) {
	var payload ExecutionTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// ParseTimeoutTaskPayload parses the task payload into TimeoutTaskPayload
func ParseTimeoutTaskPayload(task *asynq.Task) (*TimeoutTaskPayload, error) {
	var payload TimeoutTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

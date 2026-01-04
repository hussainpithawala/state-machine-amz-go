package queue

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

// Task type constants
const (
	TypeExecutionTask = "statemachine:execution"
	TypeBatchTask     = "statemachine:batch"
)

// ExecutionTaskPayload represents the payload for a state machine execution task
type ExecutionTaskPayload struct {
	StateMachineID    string                 `json:"state_machine_id"`
	SourceExecutionID string                 `json:"source_execution_id"`
	SourceStateName   string                 `json:"source_state_name"`
	ExecutionName     string                 `json:"execution_name"`
	ExecutionIndex    int                    `json:"execution_index"`
	Input             interface{}            `json:"input,omitempty"`
	Options           map[string]interface{} `json:"options,omitempty"`
}

// NewExecutionTask creates a new asynq task for state machine execution
func NewExecutionTask(payload *ExecutionTaskPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeExecutionTask, data), nil
}

// ParseExecutionTaskPayload parses the task payload into ExecutionTaskPayload
func ParseExecutionTaskPayload(task *asynq.Task) (*ExecutionTaskPayload, error) {
	var payload ExecutionTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

package queue

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExecutionTask_Success(t *testing.T) {
	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm",
		ExecutionName:  "test-execution",
		Input:          map[string]interface{}{"key": "value"},
	}

	task, err := NewExecutionTask(payload)

	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, TypeExecutionTask, task.Type())

	// Verify payload can be unmarshaled
	var unmarshaled ExecutionTaskPayload
	err = json.Unmarshal(task.Payload(), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, "test-sm", unmarshaled.StateMachineID)
	assert.Equal(t, "test-execution", unmarshaled.ExecutionName)
}

func TestNewExecutionTask_WithTimeoutFields(t *testing.T) {
	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm",
		ExecutionID:    "exec-123",
		ExecutionName:  "test-execution",
		IsTimeout:      true,
		CorrelationID:  "corr-456",
	}

	task, err := NewExecutionTask(payload)

	require.NoError(t, err)
	assert.NotNil(t, task)

	var unmarshaled ExecutionTaskPayload
	err = json.Unmarshal(task.Payload(), &unmarshaled)
	require.NoError(t, err)
	assert.True(t, unmarshaled.IsTimeout)
	assert.Equal(t, "corr-456", unmarshaled.CorrelationID)
	assert.Equal(t, "exec-123", unmarshaled.ExecutionID)
}

func TestNewExecutionTask_WithChainedExecution(t *testing.T) {
	payload := &ExecutionTaskPayload{
		StateMachineID:    "test-sm",
		ExecutionName:     "chained-execution",
		SourceExecutionID: "source-exec-123",
		SourceStateName:   "SourceState",
	}

	task, err := NewExecutionTask(payload)

	require.NoError(t, err)
	assert.NotNil(t, task)

	var unmarshaled ExecutionTaskPayload
	err = json.Unmarshal(task.Payload(), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, "source-exec-123", unmarshaled.SourceExecutionID)
	assert.Equal(t, "SourceState", unmarshaled.SourceStateName)
}

func TestNewExecutionTask_WithExecutionIndex(t *testing.T) {
	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm",
		ExecutionName:  "batch-execution",
		ExecutionIndex: 5,
	}

	task, err := NewExecutionTask(payload)

	require.NoError(t, err)
	assert.NotNil(t, task)

	var unmarshaled ExecutionTaskPayload
	err = json.Unmarshal(task.Payload(), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, 5, unmarshaled.ExecutionIndex)
}

func TestNewExecutionTask_WithOptions(t *testing.T) {
	options := map[string]interface{}{
		"priority":  "high",
		"traceId":   "trace-123",
		"timestamp": 1234567890,
	}

	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm",
		ExecutionName:  "test-execution",
		Options:        options,
	}

	task, err := NewExecutionTask(payload)

	require.NoError(t, err)
	assert.NotNil(t, task)

	var unmarshaled ExecutionTaskPayload
	err = json.Unmarshal(task.Payload(), &unmarshaled)
	require.NoError(t, err)

	// JSON unmarshaling converts numbers to float64
	assert.Equal(t, "high", unmarshaled.Options["priority"])
	assert.Equal(t, "trace-123", unmarshaled.Options["traceId"])
	assert.Equal(t, float64(1234567890), unmarshaled.Options["timestamp"])
}

func TestNewTimeoutTask_Success(t *testing.T) {
	now := time.Now().Unix()
	payload := &TimeoutTaskPayload{
		ExecutionID:    "exec-123",
		StateMachineID: "test-sm",
		StateName:      "MessageState",
		CorrelationID:  "corr-456",
		TimeoutSeconds: 60,
		ScheduledAt:    now,
	}

	task, err := NewTimeoutTask(payload)

	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, TypeTimeoutTask, task.Type())

	// Verify payload can be unmarshaled
	var unmarshaled TimeoutTaskPayload
	err = json.Unmarshal(task.Payload(), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, "exec-123", unmarshaled.ExecutionID)
	assert.Equal(t, "test-sm", unmarshaled.StateMachineID)
	assert.Equal(t, "MessageState", unmarshaled.StateName)
	assert.Equal(t, "corr-456", unmarshaled.CorrelationID)
	assert.Equal(t, 60, unmarshaled.TimeoutSeconds)
	assert.Equal(t, now, unmarshaled.ScheduledAt)
}

func TestParseExecutionTaskPayload_Success(t *testing.T) {
	originalPayload := &ExecutionTaskPayload{
		StateMachineID: "test-sm",
		ExecutionID:    "exec-789",
		ExecutionName:  "parse-test",
		IsTimeout:      true,
		CorrelationID:  "corr-999",
	}

	// Create task
	task, err := NewExecutionTask(originalPayload)
	require.NoError(t, err)

	// Parse it back
	parsedPayload, err := ParseExecutionTaskPayload(task)
	require.NoError(t, err)
	assert.Equal(t, "test-sm", parsedPayload.StateMachineID)
	assert.Equal(t, "exec-789", parsedPayload.ExecutionID)
	assert.Equal(t, "parse-test", parsedPayload.ExecutionName)
	assert.True(t, parsedPayload.IsTimeout)
	assert.Equal(t, "corr-999", parsedPayload.CorrelationID)
}

func TestParseExecutionTaskPayload_InvalidJSON(t *testing.T) {
	// Create task with invalid JSON
	task := asynq.NewTask(TypeExecutionTask, []byte("invalid json"))

	_, err := ParseExecutionTaskPayload(task)
	assert.Error(t, err)
}

func TestParseTimeoutTaskPayload_Success(t *testing.T) {
	now := time.Now().Unix()
	originalPayload := &TimeoutTaskPayload{
		ExecutionID:    "exec-111",
		StateMachineID: "test-sm",
		StateName:      "WaitState",
		CorrelationID:  "corr-222",
		TimeoutSeconds: 120,
		ScheduledAt:    now,
	}

	// Create task
	task, err := NewTimeoutTask(originalPayload)
	require.NoError(t, err)

	// Parse it back
	parsedPayload, err := ParseTimeoutTaskPayload(task)
	require.NoError(t, err)
	assert.Equal(t, "exec-111", parsedPayload.ExecutionID)
	assert.Equal(t, "test-sm", parsedPayload.StateMachineID)
	assert.Equal(t, "WaitState", parsedPayload.StateName)
	assert.Equal(t, "corr-222", parsedPayload.CorrelationID)
	assert.Equal(t, 120, parsedPayload.TimeoutSeconds)
	assert.Equal(t, now, parsedPayload.ScheduledAt)
}

func TestParseTimeoutTaskPayload_InvalidJSON(t *testing.T) {
	// Create task with invalid JSON
	task := asynq.NewTask(TypeTimeoutTask, []byte("not valid json"))

	_, err := ParseTimeoutTaskPayload(task)
	assert.Error(t, err)
}

func TestTaskTypeConstants(t *testing.T) {
	assert.Equal(t, "statemachine:execution", TypeExecutionTask)
	assert.Equal(t, "statemachine:batch", TypeBatchTask)
	assert.Equal(t, "statemachine:timeout", TypeTimeoutTask)
}

func TestExecutionTaskPayload_EmptyOptionalFields(t *testing.T) {
	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm",
		ExecutionName:  "minimal-execution",
	}

	task, err := NewExecutionTask(payload)
	require.NoError(t, err)

	parsedPayload, err := ParseExecutionTaskPayload(task)
	require.NoError(t, err)
	assert.Equal(t, "", parsedPayload.ExecutionID)
	assert.Equal(t, "", parsedPayload.SourceExecutionID)
	assert.Equal(t, "", parsedPayload.SourceStateName)
	assert.Equal(t, 0, parsedPayload.ExecutionIndex)
	assert.Nil(t, parsedPayload.Input)
	assert.Nil(t, parsedPayload.Options)
	assert.False(t, parsedPayload.IsTimeout)
	assert.Equal(t, "", parsedPayload.CorrelationID)
}

func TestTimeoutTaskPayload_AllFields(t *testing.T) {
	now := time.Now().Unix()
	payload := &TimeoutTaskPayload{
		ExecutionID:    "full-exec-id",
		StateMachineID: "full-sm-id",
		StateName:      "FullStateName",
		CorrelationID:  "full-corr-id",
		TimeoutSeconds: 300,
		ScheduledAt:    now,
	}

	task, err := NewTimeoutTask(payload)
	require.NoError(t, err)

	parsedPayload, err := ParseTimeoutTaskPayload(task)
	require.NoError(t, err)

	assert.Equal(t, payload.ExecutionID, parsedPayload.ExecutionID)
	assert.Equal(t, payload.StateMachineID, parsedPayload.StateMachineID)
	assert.Equal(t, payload.StateName, parsedPayload.StateName)
	assert.Equal(t, payload.CorrelationID, parsedPayload.CorrelationID)
	assert.Equal(t, payload.TimeoutSeconds, parsedPayload.TimeoutSeconds)
	assert.Equal(t, payload.ScheduledAt, parsedPayload.ScheduledAt)
}

func TestExecutionTaskPayload_ComplexInput(t *testing.T) {
	complexInput := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{"id": 1, "name": "Alice"},
			map[string]interface{}{"id": 2, "name": "Bob"},
		},
		"metadata": map[string]interface{}{
			"timestamp": 1234567890,
			"version":   "1.0",
		},
	}

	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm",
		ExecutionName:  "complex-input-test",
		Input:          complexInput,
	}

	task, err := NewExecutionTask(payload)
	require.NoError(t, err)

	parsedPayload, err := ParseExecutionTaskPayload(task)
	require.NoError(t, err)

	// JSON unmarshaling converts to generic types
	parsedInput, ok := parsedPayload.Input.(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, parsedInput, "users")
	assert.Contains(t, parsedInput, "metadata")
}

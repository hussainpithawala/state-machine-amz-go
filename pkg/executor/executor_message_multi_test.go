package executor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepository struct {
	repository.Repository
	repository.MessageRepository
	smRecord *repository.StateMachineRecord
}

func (m *mockRepository) GetStateMachine(ctx context.Context, id string) (*repository.StateMachineRecord, error) {
	if m.smRecord != nil && m.smRecord.ID == id {
		return m.smRecord, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockRepository) FindWaitingCorrelations(ctx context.Context, filter *repository.MessageCorrelationFilter) ([]*repository.MessageCorrelationRecord, error) {
	return []*repository.MessageCorrelationRecord{
		{
			ExecutionID:    "exec-repo-1",
			StateMachineID: "sm-repo-1",
			CorrelationKey: "orderId",
			Status:         "WAITING",
		},
	}, nil
}

func (m *mockRepository) GetExecution(ctx context.Context, id string) (*repository.ExecutionRecord, error) {
	return &repository.ExecutionRecord{
		ExecutionID:    id,
		StateMachineID: "sm-repo-1",
		Status:         executor.PAUSED,
		CurrentState:   "State1",
	}, nil
}

func (m *mockRepository) SaveExecution(ctx context.Context, record *repository.ExecutionRecord) error {
	return nil
}
func (m *mockRepository) SaveStateHistory(ctx context.Context, record *repository.StateHistoryRecord) error {
	return nil
}
func (m *mockRepository) UpdateCorrelationStatus(ctx context.Context, id string, status string) error {
	return nil
}

func (m *mockRepository) SaveMessageCorrelation(ctx context.Context, record *repository.MessageCorrelationRecord) error {
	return nil
}
func (m *mockRepository) GetMessageCorrelation(ctx context.Context, id string) (*repository.MessageCorrelationRecord, error) {
	return nil, nil
}
func (m *mockRepository) DeleteMessageCorrelation(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListTimedOutCorrelations(ctx context.Context, currentTimestamp int64) ([]*repository.MessageCorrelationRecord, error) {
	return nil, nil
}

type mockStateMachine struct {
	id string
}

func (m *mockStateMachine) GetStartAt() string                         { return "" }
func (m *mockStateMachine) GetState(name string) (states.State, error) { return nil, nil }
func (m *mockStateMachine) IsTimeout(startTime time.Time) bool         { return false }
func (m *mockStateMachine) RunExecution(ctx context.Context, input interface{}, execCtx *execution.Execution) (*execution.Execution, error) {
	return execCtx, nil
}
func (m *mockStateMachine) ResumeExecution(ctx context.Context, execCtx *execution.Execution) (*execution.Execution, error) {
	execCtx.Status = "SUCCEEDED"
	return execCtx, nil
}
func (m *mockStateMachine) FindWaitingExecutionsByCorrelation(ctx context.Context, correlationKey string, correlationValue interface{}) ([]*repository.ExecutionRecord, error) {
	return nil, nil
}
func (m *mockStateMachine) GetID() string { return m.id }

func TestMessage_MultipleExecutions(t *testing.T) {
	ctx := context.Background()
	e := executor.NewBaseExecutor()

	// Create two paused executions in memory
	exec1 := &execution.Execution{
		ID:             "exec-1",
		StateMachineID: "sm-1",
		Status:         executor.PAUSED,
		Metadata: map[string]interface{}{
			"correlation_data": map[string]interface{}{
				"correlation_key":   "orderId",
				"correlation_value": "ORD-123",
			},
		},
	}
	exec2 := &execution.Execution{
		ID:             "exec-2",
		StateMachineID: "sm-1",
		Status:         executor.PAUSED,
		Metadata: map[string]interface{}{
			"correlation_data": map[string]interface{}{
				"correlation_key":   "orderId",
				"correlation_value": "ORD-123",
			},
		},
	}

	// Manually add to executor's internal map (since PauseExecution does this)
	err := e.PauseExecution(ctx, exec1, map[string]interface{}{
		"correlation_key":   "orderId",
		"correlation_value": "ORD-123",
	})
	require.NoError(t, err)
	err = e.PauseExecution(ctx, exec2, map[string]interface{}{
		"correlation_key":   "orderId",
		"correlation_value": "ORD-123",
	})
	require.NoError(t, err)

	e.AddStateMachine(&mockStateMachine{id: "sm-1"})

	request := &executor.MessageRequest{
		CorrelationKey:   "orderId",
		CorrelationValue: "ORD-123",
		Data:             map[string]interface{}{"approved": true},
	}

	resp, err := e.Message(ctx, request, nil)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "RESUMED", resp.Status)

	// Verify both were resumed (status changed to SUCCEEDED in our mock)
	s1, _ := e.GetStatus("exec-1")
	assert.Equal(t, "SUCCEEDED", s1.Status)
	s2, _ := e.GetStatus("exec-2")
	assert.Equal(t, "SUCCEEDED", s2.Status)
}

// TestMessage_DynamicSMLoading verifies that the executor can load the correct state machine
// when the provided one doesn't match the execution's state machine ID.
func TestMessage_DynamicSMLoading(t *testing.T) {
	ctx := context.Background()
	e := executor.NewBaseExecutor()

	// 2. Setup mock repository and manager
	mockRepo := &mockRepository{
		smRecord: &repository.StateMachineRecord{
			ID:         "sm-repo-1",
			Definition: `{"StartAt": "State1", "States": {"State1": {"Type": "Pass", "End": true}}}`,
		},
	}
	manager := repository.NewManagerWithRepository(mockRepo)

	// 3. Execution in repository has StateMachineID "sm-repo-1"
	// (Our mockRepository.FindWaitingCorrelations returns one such execution)

	e.SetRepositoryManager(manager)

	request := &executor.MessageRequest{
		CorrelationKey:   "orderId",
		CorrelationValue: "ORD-REPO-123",
		Data:             map[string]interface{}{"val": 1},
	}

	// 4. Call Message.
	// It should find "exec-repo-1" (from repository), notice it needs "sm-repo-1",
	// and load it from the manager.
	resp, err := e.Message(ctx, request, nil)
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "RESUMED", resp.Status)
	assert.Equal(t, "exec-repo-1", resp.ExecutionID)
	assert.Equal(t, "sm-repo-1", resp.StateMachineID)
}

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	// Third-party imports
	"github.com/stretchr/testify/require"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
)

type fakeStrategy struct {
	initializeCalls int
	closeCalls      int

	saveExecutionCalls    int
	saveStateHistoryCalls int
	saveStateMachineCalls int

	lastSavedExecution    *ExecutionRecord
	lastSavedStateHistory *StateHistoryRecord
	lastSavedStateMachine *StateMachineRecord

	getExecutionID    string
	getHistoryID      string
	getStateMachineID string

	//	listFilters map[string]interface{}
	listLimit  int
	listOffset int
}

func (f *fakeStrategy) Initialize(_ context.Context) error  { f.initializeCalls++; return nil }
func (f *fakeStrategy) Close() error                        { f.closeCalls++; return nil }
func (f *fakeStrategy) HealthCheck(_ context.Context) error { return nil }
func (f *fakeStrategy) DeleteExecution(_ context.Context, _ string) error {
	return nil
}

func (f *fakeStrategy) SaveExecution(_ context.Context, record *ExecutionRecord) error {
	f.saveExecutionCalls++
	f.lastSavedExecution = record
	return nil
}

func (f *fakeStrategy) GetExecution(_ context.Context, executionID string) (*ExecutionRecord, error) {
	f.getExecutionID = executionID
	return &ExecutionRecord{ExecutionID: executionID}, nil
}

func (f *fakeStrategy) SaveStateHistory(_ context.Context, record *StateHistoryRecord) error {
	f.saveStateHistoryCalls++
	f.lastSavedStateHistory = record
	return nil
}

func (f *fakeStrategy) GetStateHistory(_ context.Context, executionID string) ([]*StateHistoryRecord, error) {
	f.getHistoryID = executionID
	return []*StateHistoryRecord{{ExecutionID: executionID, StateName: "Any"}}, nil
}

func (f *fakeStrategy) ListExecutions(_ context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error) {
	f.listLimit = filter.Limit
	f.listOffset = filter.Offset
	return []*ExecutionRecord{{ExecutionID: "exec-1"}}, nil
}

func (f *fakeStrategy) CountExecutions(_ context.Context, _ *ExecutionFilter) (int64, error) {
	return 1, nil
}

func (f *fakeStrategy) SaveStateMachine(_ context.Context, record *StateMachineRecord) error {
	f.saveStateMachineCalls++
	f.lastSavedStateMachine = record
	return nil
}

func (f *fakeStrategy) GetStateMachine(_ context.Context, stateMachineID string) (*StateMachineRecord, error) {
	f.getStateMachineID = stateMachineID
	return &StateMachineRecord{ID: stateMachineID}, nil
}

func TestNewPersistenceManager_UnsupportedStrategy(t *testing.T) {
	pm, err := NewPersistenceManager(&Config{Strategy: "nope"})
	require.Error(t, err)
	require.Nil(t, pm)
	require.Contains(t, err.Error(), "unsupported persistence repository")
}

func TestNewPersistenceManager_NotImplementedStrategies(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		wantMsg  string
	}{
		{name: "dynamodb", strategy: "dynamodb", wantMsg: "not yet implemented"},
		{name: "redis", strategy: "redis", wantMsg: "not yet implemented"},
		{name: "memory", strategy: "memory", wantMsg: "not yet implemented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm, err := NewPersistenceManager(&Config{Strategy: tt.strategy})
			require.Error(t, err)
			require.Nil(t, pm)
			require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.wantMsg))
		})
	}
}

func TestManager_InitializeAndClose_DelegatesToStrategy(t *testing.T) {
	fs := &fakeStrategy{}
	pm := &Manager{repository: fs, config: &Config{Strategy: "fake"}}

	require.NoError(t, pm.Initialize(context.Background()))
	require.NoError(t, pm.Close())

	require.Equal(t, 1, fs.initializeCalls)
	require.Equal(t, 1, fs.closeCalls)
}

func TestManager_SaveExecution_MapsFields_EndTimeAndError(t *testing.T) {
	fs := &fakeStrategy{}
	pm := &Manager{repository: fs, config: &Config{Strategy: "fake"}}

	exec := &execution.Execution{
		ID:           "exec-123",
		Name:         "my-exec",
		Input:        map[string]any{"k": "v"},
		Output:       "out",
		Status:       "FAILED",
		StartTime:    time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		EndTime:      time.Date(2025, 1, 2, 3, 5, 0, 0, time.UTC),
		CurrentState: "SomeState",
		Error:        errors.New("boom"),
	}

	err := pm.SaveExecution(context.Background(), exec)
	require.NoError(t, err)

	require.Equal(t, 1, fs.saveExecutionCalls)
	require.NotNil(t, fs.lastSavedExecution)

	rec := fs.lastSavedExecution
	require.Equal(t, "exec-123", rec.ExecutionID)
	require.Equal(t, "my-exec", rec.Name)
	require.Equal(t, exec.Input, rec.Input)
	require.Equal(t, exec.Output, rec.Output)
	require.Equal(t, "FAILED", rec.Status)
	require.Equal(t, exec.StartTime, *rec.StartTime)
	require.Equal(t, exec.EndTime, *rec.EndTime)
	require.Equal(t, "SomeState", rec.CurrentState)

	require.NotNil(t, rec.EndTime)
	require.Equal(t, exec.EndTime, *rec.EndTime)

	require.Equal(t, "boom", rec.Error)
}

func TestManager_SaveExecution_DoesNotSetEndTimeOrError_WhenMissing(t *testing.T) {
	fs := &fakeStrategy{}
	pm := &Manager{repository: fs, config: &Config{Strategy: "fake"}}

	exec := &execution.Execution{
		ID:           "exec-1",
		Name:         "n",
		Status:       "RUNNING",
		StartTime:    time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC),
		CurrentState: "S1",
		// EndTime is zero, Error is nil
	}

	require.NoError(t, pm.SaveExecution(context.Background(), exec))
	require.NotNil(t, fs.lastSavedExecution)

	require.Nil(t, fs.lastSavedExecution.EndTime)
	require.Equal(t, "", fs.lastSavedExecution.Error)
}

func TestManager_SaveStateHistory_MapsFields_EndTimeAndError(t *testing.T) {
	fs := &fakeStrategy{}
	pm := &Manager{repository: fs, config: &Config{Strategy: "fake"}}

	exec := &execution.Execution{
		ID:        "exec-9",
		StartTime: time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC),
	}

	h := &execution.StateHistory{
		StateName:      "A",
		StateType:      "Pass",
		Status:         "FAILED",
		Input:          "in",
		Output:         "out",
		StartTime:      time.Date(2025, 3, 4, 5, 6, 8, 0, time.UTC),
		EndTime:        time.Date(2025, 3, 4, 5, 6, 9, 0, time.UTC),
		RetryCount:     2,
		SequenceNumber: 7,
		Error:          errors.New("state failed"),
	}

	require.NoError(t, pm.SaveStateHistory(context.Background(), exec, h))

	require.Equal(t, 1, fs.saveStateHistoryCalls)
	require.NotNil(t, fs.lastSavedStateHistory)

	rec := fs.lastSavedStateHistory
	require.Equal(t, "exec-9", rec.ExecutionID)
	require.Equal(t, exec.StartTime, *rec.ExecutionStartTime)
	require.Equal(t, "A", rec.StateName)
	require.Equal(t, "Pass", rec.StateType)
	require.Equal(t, "in", rec.Input)
	require.Equal(t, "out", rec.Output)
	require.Equal(t, "FAILED", rec.Status)
	require.Equal(t, h.StartTime, *rec.StartTime)
	require.Equal(t, h.EndTime, *rec.EndTime)
	require.Equal(t, 2, rec.RetryCount)
	require.Equal(t, 7, rec.SequenceNumber)

	require.NotNil(t, rec.EndTime)
	require.Equal(t, h.EndTime, *rec.EndTime)

	require.Equal(t, "state failed", rec.Error)

	// ID is time-based; just validate shape/prefix.
	require.NotEmpty(t, rec.ID)
	require.True(t, strings.HasPrefix(rec.ID, "exec-9-A-"))
}

func TestManager_GetExecution_GetStateHistory_ListExecutions_Delegates(t *testing.T) {
	fs := &fakeStrategy{}
	pm := &Manager{repository: fs, config: &Config{Strategy: "fake"}}

	_, err := pm.GetExecution(context.Background(), "exec-abc")
	require.NoError(t, err)
	require.Equal(t, "exec-abc", fs.getExecutionID)

	_, err = pm.GetStateHistory(context.Background(), "exec-hist")
	require.NoError(t, err)
	require.Equal(t, "exec-hist", fs.getHistoryID)

	_, err = pm.ListExecutions(context.Background(), &ExecutionFilter{
		Offset: 20,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, 10, fs.listLimit)
	require.Equal(t, 20, fs.listOffset)
}

func TestManager_SaveStateMachine_Delegates(t *testing.T) {
	fs := &fakeStrategy{}
	pm := &Manager{repository: fs, config: &Config{Strategy: "fake"}}

	sm := &StateMachineRecord{
		ID:         "sm-123",
		Name:       "test-sm",
		Definition: "{}",
		Version:    "1.0",
	}

	err := pm.SaveStateMachine(context.Background(), sm)
	require.NoError(t, err)

	require.Equal(t, 1, fs.saveStateMachineCalls)
	require.NotNil(t, fs.lastSavedStateMachine)
	require.Equal(t, "sm-123", fs.lastSavedStateMachine.ID)
	require.False(t, fs.lastSavedStateMachine.CreatedAt.IsZero())
	require.False(t, fs.lastSavedStateMachine.UpdatedAt.IsZero())
}

func TestManager_GetStateMachine_Delegates(t *testing.T) {
	fs := &fakeStrategy{}
	pm := &Manager{repository: fs, config: &Config{Strategy: "fake"}}

	_, err := pm.GetStateMachine(context.Background(), "sm-123")
	require.NoError(t, err)
	require.Equal(t, "sm-123", fs.getStateMachineID)
}

func TestGenerateHistoryID_UniqueForDifferentTimestamps(t *testing.T) {
	t1 := time.Unix(100, 1)
	t2 := time.Unix(100, 2)

	id1 := generateHistoryID("exec-x", "StateY", t1)
	id2 := generateHistoryID("exec-x", "StateY", t2)

	require.NotEqual(t, id1, id2)
	require.True(t, strings.HasPrefix(id1, "exec-x-StateY-"))
	require.True(t, strings.HasPrefix(id2, "exec-x-StateY-"))
}

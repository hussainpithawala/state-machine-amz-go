package persistent

import (
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	// Third-party imports
	"github.com/stretchr/testify/require"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

type fakeRepository struct {
	saveExecutionCalls          int
	saveStateHistoryCalls       int
	saveStateMachineCalls       int
	saveMessageCorrelationCalls int

	lastExecutionID           string
	lastStateMachineID        string
	lastCorrelationRecord     *repository.MessageCorrelationRecord
	lastUpdateCorrelationID   string
	lastUpdateCorrelationStat string

	//	lastListFilters map[string]interface{}
	lastListLimit  int
	lastListOffset int

	getExecutionID string
	getHistoryID   string

	saveExecutionErr    error
	saveStateHistoryErr error
}

func (f *fakeRepository) SaveMessageCorrelation(ctx context.Context, record *repository.MessageCorrelationRecord) error {
	f.saveMessageCorrelationCalls++
	f.lastCorrelationRecord = record
	return nil
}

func (f *fakeRepository) GetMessageCorrelation(ctx context.Context, id string) (*repository.MessageCorrelationRecord, error) {
	return nil, nil
}

func (f *fakeRepository) FindWaitingCorrelations(ctx context.Context, filter *repository.MessageCorrelationFilter) ([]*repository.MessageCorrelationRecord, error) {
	return nil, nil
}

func (f *fakeRepository) UpdateCorrelationStatus(ctx context.Context, id, status string) error {
	f.lastUpdateCorrelationID = id
	f.lastUpdateCorrelationStat = status
	return nil
}

func (f *fakeRepository) DeleteMessageCorrelation(ctx context.Context, id string) error {
	return nil
}

func (f *fakeRepository) ListTimedOutCorrelations(ctx context.Context, currentTimestamp int64) ([]*repository.MessageCorrelationRecord, error) {
	return nil, nil
}

func (f *fakeRepository) Initialize(_ context.Context) error  { return nil }
func (f *fakeRepository) Close() error                        { return nil }
func (f *fakeRepository) HealthCheck(_ context.Context) error { return nil }
func (f *fakeRepository) DeleteExecution(_ context.Context, _ string) error {
	return nil
}

func (f *fakeRepository) SaveExecution(_ context.Context, record *repository.ExecutionRecord) error {
	f.saveExecutionCalls++
	if record != nil {
		f.lastExecutionID = record.ExecutionID
	}
	return f.saveExecutionErr
}

func (f *fakeRepository) GetExecution(_ context.Context, executionID string) (*repository.ExecutionRecord, error) {
	f.getExecutionID = executionID
	return &repository.ExecutionRecord{ExecutionID: executionID}, nil
}

func (f *fakeRepository) SaveStateHistory(_ context.Context, _ *repository.StateHistoryRecord) error {
	f.saveStateHistoryCalls++
	return f.saveStateHistoryErr
}

func (f *fakeRepository) GetStateHistory(_ context.Context, executionID string) ([]*repository.StateHistoryRecord, error) {
	f.getHistoryID = executionID
	return []*repository.StateHistoryRecord{
		{ExecutionID: executionID, StateName: "SomeState"},
	}, nil
}

func (f *fakeRepository) ListExecutions(_ context.Context, filter *repository.ExecutionFilter) ([]*repository.ExecutionRecord, error) {
	// store a copy so caller can't mutate after the call
	f.lastListLimit = filter.Limit
	f.lastListOffset = filter.Offset

	return []*repository.ExecutionRecord{
		{ExecutionID: "exec-1"},
	}, nil
}

func (f *fakeRepository) CountExecutions(_ context.Context, _ *repository.ExecutionFilter) (int64, error) {
	return 1, nil
}

func (f *fakeRepository) SaveStateMachine(_ context.Context, record *repository.StateMachineRecord) error {
	f.saveStateMachineCalls++
	if record != nil {
		f.lastStateMachineID = record.ID
	}
	return nil
}

func (f *fakeRepository) GetStateMachine(_ context.Context, stateMachineID string) (*repository.StateMachineRecord, error) {
	return &repository.StateMachineRecord{
		ID:         stateMachineID,
		Definition: `{"StartAt": "FirstState", "States": {"FirstState": {"Type": "Pass", "End": true}}}`,
	}, nil
}

// setUnexportedField sets an unexported struct field via unsafe reflection (test-only helper).
func setUnexportedField(t *testing.T, obj any, fieldName string, value any) {
	t.Helper()

	rv := reflect.ValueOf(obj)
	require.True(t, rv.Kind() == reflect.Ptr && !rv.IsNil(), "obj must be a non-nil pointer")

	elem := rv.Elem()
	require.True(t, elem.Kind() == reflect.Struct, "obj must point to a struct")

	field := elem.FieldByName(fieldName)
	require.True(t, field.IsValid(), "field %q not found", fieldName)

	// Create a writable view onto the unexported field.
	ptr := unsafe.Pointer(field.UnsafeAddr())
	writable := reflect.NewAt(field.Type(), ptr).Elem()

	writable.Set(reflect.ValueOf(value))
}

func newTestRepoManager(t *testing.T, testRepository repository.Repository) *repository.Manager {
	t.Helper()
	m := &repository.Manager{}
	setUnexportedField(t, m, "repository", testRepository)
	setUnexportedField(t, m, "config", &repository.Config{Strategy: "fake"})
	return m
}

func TestNew_GeneratesStateMachineID_WhenEmpty(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	pm, err := New(definition, true, "", newTestRepoManager(t, &fakeRepository{}))
	require.NoError(t, err)
	require.NotNil(t, pm)
	require.NotEmpty(t, pm.stateMachineID)
	require.Contains(t, pm.stateMachineID, "sm-")
}

func TestNew_UsesProvidedStateMachineID(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	pm, err := New(definition, true, "my-sm-id", newTestRepoManager(t, &fakeRepository{}))
	require.NoError(t, err)
	require.Equal(t, "my-sm-id", pm.stateMachineID)
}

func TestExecute_PersistsExecutionAndHistory_Success(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "Result": "ok", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	manager := newTestRepoManager(t, testStrategy)

	pm, err := New(definition, true, "sm-fixed", manager)
	require.NoError(t, err)

	exec, err := pm.Execute(context.Background(), map[string]any{"in": "x"}, statemachine.WithExecutionName("custom-exec"))
	require.NoError(t, err)
	require.NotNil(t, exec)

	require.Equal(t, "SUCCEEDED", exec.Status)
	require.Equal(t, "custom-exec", exec.Name)
	require.Equal(t, "ok", exec.Output)

	// The implementation sets execCtx.ID = pm.stateMachineID before persisting.
	require.Equal(t, "sm-fixed", testStrategy.lastExecutionID)

	// At least:
	// 1) initial persist in Execute
	// 2) persist after state execution in RunExecution loop
	// 3) final persist when end state reached
	require.GreaterOrEqual(t, testStrategy.saveExecutionCalls, 3)

	// One state executed => one history persist attempt.
	require.Equal(t, 1, testStrategy.saveStateHistoryCalls)
}

func TestRunExecution_ContextCancelled_PersistsCancelledAndReturnsError(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-cancel", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	// Prepare an execution, then cancel the context before running.
	execCtx := executionContextForTest(t, pm, "exec-cancelled", "FirstState", "input")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec, runErr := pm.RunExecution(ctx, execCtx.Input, execCtx)
	require.Error(t, runErr)
	require.NotNil(t, exec)

	require.Equal(t, "CANCELLED", exec.Status)
	require.NotNil(t, exec.Error)
	require.False(t, exec.EndTime.IsZero())

	// Should persist at least once on cancellation path.
	require.GreaterOrEqual(t, testStrategy.saveExecutionCalls, 1)
}

func TestRunExecution_StateNotFound_PersistsFailedAndReturnsError(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-missing", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	execCtx := executionContextForTest(t, pm, "exec-1", "DOES_NOT_EXIST", "input")
	exec, runErr := pm.RunExecution(context.Background(), execCtx.Input, execCtx)

	require.Error(t, runErr)
	require.NotNil(t, exec)
	require.Equal(t, FAILED, exec.Status)
	require.NotNil(t, exec.Error)
	require.False(t, exec.EndTime.IsZero())
	require.GreaterOrEqual(t, testStrategy.saveExecutionCalls, 1)
}

func TestListExecutions_AddsStateMachineIDFilter(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-filter", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	_, err = pm.ListExecutions(context.Background(), &repository.ExecutionFilter{Limit: 10, Offset: 20})
	require.NoError(t, err)

	require.Equal(t, 10, testStrategy.lastListLimit)
	require.Equal(t, 20, testStrategy.lastListOffset)
}

func TestGetExecution_DelegatesToRepositoryManager(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-x", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	rec, err := pm.GetExecution(context.Background(), "exec-123")
	require.NoError(t, err)
	require.NotNil(t, rec)
	require.Equal(t, "exec-123", testStrategy.getExecutionID)
}

func TestGetExecutionHistory_DelegatesToRepositoryManager(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-x", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	h, err := pm.GetExecutionHistory(context.Background(), "exec-777")
	require.NoError(t, err)
	require.NotEmpty(t, h)
	require.Equal(t, "exec-777", testStrategy.getHistoryID)
}

func TestSaveDefinition_DelegatesToRepositoryManager(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-save-def", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	err = pm.SaveDefinition(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, testStrategy.saveStateMachineCalls)
	require.Equal(t, "sm-save-def", testStrategy.lastStateMachineID)
}

func TestGetDefinition_DelegatesToRepositoryManager(t *testing.T) {
	definition := []byte(`{
		"StartAt": "FirstState",
		"States": {
			"FirstState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-x", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	rec, err := pm.GetDefinition(context.Background(), "sm-123")
	require.NoError(t, err)
	require.NotNil(t, rec)
	require.Equal(t, "sm-123", rec.ID)
}

func TestExecute_WithExistingContext(t *testing.T) {
	definition := []byte(`{
		"StartAt": "PassState",
		"States": {
			"PassState": { "Type": "Pass", "End": true }
		}
	}`)

	testStrategy := &fakeRepository{}
	pm, err := New(definition, true, "sm-existing", newTestRepoManager(t, testStrategy))
	require.NoError(t, err)

	inputData := map[string]interface{}{"key": "value"}
	execCtx := &execution.Execution{
		ID:    "custom-id",
		Input: inputData,
	}

	exec, err := pm.Execute(context.Background(), execCtx)
	require.NoError(t, err)
	require.Equal(t, "custom-id", exec.ID)
	require.Equal(t, inputData, exec.Input)
	require.Equal(t, "SUCCEEDED", exec.Status)
}

func TestNewFromDefnId_Success(t *testing.T) {
	testStrategy := &fakeRepository{}
	manager := newTestRepoManager(t, testStrategy)

	ctx := context.Background()
	smId := "test-sm-id"

	pm, err := NewFromDefnId(ctx, smId, manager)
	require.NoError(t, err)
	require.NotNil(t, pm)
	require.Equal(t, smId, pm.stateMachineID)
	require.NotNil(t, pm.statemachine)
	require.Equal(t, manager, pm.repositoryManager)
}

func TestRunExecution_MessageState_PausesAndResumes(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepository{}
	manager := newTestRepoManager(t, repo)

	// Define a state machine with a Message state
	definition := `{
		"StartAt": "WaitForMessage",
		"States": {
			"WaitForMessage": {
				"Type": "Message",
				"CorrelationKey": "orderId",
				"Next": "FinalState"
			},
			"FinalState": {
				"Type": "Pass",
				"End": true
			}
		}
	}`

	pm, err := New([]byte(definition), true, "test-sm", manager)
	require.NoError(t, err)

	input := map[string]interface{}{"orderId": "123"}
	execCtx := execution.NewContext("test-exec", "WaitForMessage", input)
	execCtx.ID = "test-exec-id"

	// 1. Run Execution - should pause at WaitForMessage
	result, err := pm.RunExecution(ctx, execCtx.Input, execCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, PAUSED, result.Status)
	require.Equal(t, "WaitForMessage", result.CurrentState)

	// Verify correlation was saved
	require.Equal(t, 1, repo.saveMessageCorrelationCalls)
	require.NotNil(t, repo.lastCorrelationRecord)
	require.Equal(t, "orderId", repo.lastCorrelationRecord.CorrelationKey)
	// correlationValue should be equal to the input map because CorrelationValuePath is nil
	require.Equal(t, input, repo.lastCorrelationRecord.CorrelationValue)
	require.Equal(t, "WAITING", repo.lastCorrelationRecord.Status)

	// Verify history has WAITING status
	require.Len(t, result.History, 1)
	require.Equal(t, "WAITING", result.History[0].Status)

	// 2. Resume Execution with message data
	resumeInput := map[string]interface{}{
		"orderId": "123",
		"__received_message__": map[string]interface{}{
			"correlation_key":   "orderId",
			"correlation_value": "123",
			"data":              map[string]interface{}{"status": "shipped"},
		},
	}
	execCtx.Input = resumeInput

	result, err = pm.ResumeExecution(ctx, execCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "SUCCEEDED", result.Status)
	require.Equal(t, "FinalState", result.CurrentState)

	// Verify history has SUCCEEDED status for WaitForMessage after resumption
	// It should have 2 history entries for WaitForMessage (one WAITING, one SUCCEEDED)
	// plus one for FinalState
	require.Len(t, result.History, 3)
	require.Equal(t, "WaitForMessage", result.History[0].StateName)
	require.Equal(t, "WAITING", result.History[0].Status)
	require.Equal(t, "WaitForMessage", result.History[1].StateName)
	require.Equal(t, "SUCCEEDED", result.History[1].Status)
	require.Equal(t, "FinalState", result.History[2].StateName)
	require.Equal(t, "SUCCEEDED", result.History[2].Status)
}

// executionContextForTest creates an execution context by calling pm.Execute-equivalent logic
// without depending on time-based defaults. (Keeps tests deterministic.)
func executionContextForTest(t *testing.T, pm *StateMachine, name, startState string, input any) *execution.Execution {
	t.Helper()

	// Reuse the real execution constructor from pkg/execution through the public API shape we know:
	// We'll run Execute-like setup: name, start state, input, and ensure IDs are present.
	exec := execution.NewContext(name, startState, input)
	exec.ID = pm.stateMachineID
	exec.StartTime = time.Now()
	exec.CurrentState = startState
	return exec
}

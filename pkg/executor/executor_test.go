package executor

import (
	"context"
	"testing"
	"time"

	// Third-party imports
	"github.com/stretchr/testify/require"

	// Project-specific/Internal imports
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
)

func TestStateRegistry_RegisterAndGetTaskHandler(t *testing.T) {
	r := NewStateRegistry()

	called := false
	handler := func(ctx context.Context, input interface{}) (interface{}, error) {
		called = true
		return input, nil
	}

	r.RegisterTaskHandler("resource://x", handler)

	got, ok := r.GetTaskHandler("resource://x")
	require.True(t, ok)
	require.NotNil(t, got)

	out, err := got(context.Background(), "in")
	require.NoError(t, err)
	require.Equal(t, "in", out)
	require.True(t, called)

	_, ok = r.GetTaskHandler("resource://missing")
	require.False(t, ok)
}

func TestNewBaseExecutor_InitializesMapsAndRegistry(t *testing.T) {
	e := NewBaseExecutor()
	require.NotNil(t, e)
	require.NotNil(t, e.executions)
	require.NotNil(t, e.registry)
	require.NotNil(t, e.registry.taskHandlers)
}

func TestBaseExecutor_GetStatus_NotFound(t *testing.T) {
	e := NewBaseExecutor()

	got, err := e.GetStatus("does-not-exist")
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "not found")
}

func TestBaseExecutor_GetStatus_Found(t *testing.T) {
	e := NewBaseExecutor()

	exec := &execution.Execution{
		ID:        "exec-1",
		Name:      "n1",
		Status:    "RUNNING",
		StartTime: time.Now(),
	}
	e.executions[exec.ID] = exec

	got, err := e.GetStatus("exec-1")
	require.NoError(t, err)
	require.Same(t, exec, got)
}

func TestBaseExecutor_Stop_NilExecution(t *testing.T) {
	e := NewBaseExecutor()

	err := e.Stop(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be nil")
}

func TestBaseExecutor_Stop_SetsAborted_EndTime_AndRemovesFromActive(t *testing.T) {
	e := NewBaseExecutor()

	exec := &execution.Execution{
		ID:        "exec-1",
		Status:    "RUNNING",
		StartTime: time.Now(),
	}
	e.executions[exec.ID] = exec

	before := time.Now()
	err := e.Stop(context.Background(), exec)
	after := time.Now()

	require.NoError(t, err)
	require.Equal(t, "ABORTED", exec.Status)
	require.False(t, exec.EndTime.IsZero())
	require.True(t, !exec.EndTime.Before(before) && !exec.EndTime.After(after), "EndTime should be set to 'now'")

	_, exists := e.executions["exec-1"]
	require.False(t, exists, "execution should be removed from active executions map")
}

func TestBaseExecutor_ListExecutions(t *testing.T) {
	e := NewBaseExecutor()

	require.Empty(t, e.ListExecutions())

	exec1 := &execution.Execution{ID: "exec-1", Status: "RUNNING"}
	exec2 := &execution.Execution{ID: "exec-2", Status: "SUCCEEDED"}
	e.executions[exec1.ID] = exec1
	e.executions[exec2.ID] = exec2

	list := e.ListExecutions()
	require.Len(t, list, 2)

	// Order is not guaranteed (map iteration), so check membership.
	ids := map[string]bool{list[0].ID: true, list[1].ID: true}
	require.True(t, ids["exec-1"])
	require.True(t, ids["exec-2"])
}

func TestBaseExecutor_RegisterGoFunction_RegistersHandlerWithExpectedARN(t *testing.T) {
	e := NewBaseExecutor()

	handler := func(ctx context.Context, input interface{}) (interface{}, error) {
		return "ok", nil
	}

	arn := "arn:aws:states:::lambda:function:MyFn"
	e.RegisterGoFunction(arn, handler)

	got, ok := e.registry.GetTaskHandler(arn)
	require.True(t, ok)
	require.NotNil(t, got)

	out, err := got(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "ok", out)
}

func TestBaseExecutor_ExecuteGoTask_ReturnsInputPlaceholder(t *testing.T) {
	e := NewBaseExecutor()

	out, err := e.ExecuteGoTask(context.Background(), nil, map[string]any{"k": "v"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"k": "v"}, out)
}

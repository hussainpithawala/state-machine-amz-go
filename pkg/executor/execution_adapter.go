// pkg/executor/execution_adapter.go
package executor

import (
	"context"

	_ "github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// ExecutionContextAdapter adapts the BaseExecutor to the states.ExecutionContext interface
type ExecutionContextAdapter struct {
	executor *BaseExecutor
}

// NewExecutionContextAdapter creates a new adapter
func NewExecutionContextAdapter(executor *BaseExecutor) *ExecutionContextAdapter {
	return &ExecutionContextAdapter{
		executor: executor,
	}
}

// GetTaskHandler implements states.ExecutionContext interface
func (a *ExecutionContextAdapter) GetTaskHandler(resource string) (func(context.Context, interface{}) (interface{}, error), bool) {
	if a.executor == nil || a.executor.registry == nil {
		return nil, false
	}
	return a.executor.registry.GetTaskHandler(resource)
}

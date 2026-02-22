// pkg/repository/repository.go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
)

// ExecutionRecord represents the execution data to be persisted
type ExecutionRecord struct {
	ExecutionID    string                 `json:"execution_id"`
	StateMachineID string                 `json:"state_machine_id"`
	Name           string                 `json:"name"`
	Input          interface{}            `json:"input"`
	Output         interface{}            `json:"output,omitempty"`
	Status         string                 `json:"status"`
	StartTime      *time.Time             `json:"start_time"`
	EndTime        *time.Time             `json:"end_time,omitempty"`
	CurrentState   string                 `json:"current_state"`
	Error          string                 `json:"error,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// StateHistoryRecord represents a single state execution in history
type StateHistoryRecord struct {
	ID                 string                 `json:"id"`
	ExecutionID        string                 `json:"execution_id"`
	ExecutionStartTime *time.Time             `json:"execution_start_time"`
	StateName          string                 `json:"state_name"`
	StateType          string                 `json:"state_type"`
	Input              interface{}            `json:"input"`
	Output             interface{}            `json:"output,omitempty"`
	Status             string                 `json:"status"`
	StartTime          *time.Time             `json:"start_time"`
	EndTime            *time.Time             `json:"end_time,omitempty"`
	Error              string                 `json:"error,omitempty"`
	RetryCount         int                    `json:"retry_count"`
	SequenceNumber     int                    `json:"sequence_number"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// StateMachineRecord represents a state machine definition to be persisted
type StateMachineRecord struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Definition  string                 `json:"definition"` // JSON representation of the state machine
	Type        string                 `json:"type,omitempty"`
	Version     string                 `json:"version"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// LinkedExecutionRecord represents a linkage between a source execution and a targeted execution
type LinkedExecutionRecord struct {
	ID                     string    `json:"id"`
	SourceStateMachineID   string    `json:"source_state_machine_id"`
	SourceExecutionID      string    `json:"source_execution_id"`
	SourceStateName        string    `json:"source_state_name,omitempty"`
	InputTransformerName   string    `json:"input_transformer_name,omitempty"`
	TargetStateMachineName string    `json:"target_state_machine_name"`
	TargetExecutionID      string    `json:"target_execution_id"`
	CreatedAt              time.Time `json:"created_at"`
}

// Manager manages the persistence repository
type Manager struct {
	repository Repository
	config     *Config
}

// NewPersistenceManager creates a new persistence manager
func NewPersistenceManager(config *Config) (*Manager, error) {
	var repository Repository
	var err error

	switch config.Strategy {
	case "postgres":
		repository, err = NewPostgresRepository(config)
	case "gorm-postgres":
		repository, err = NewGormPostgresRepository(config)
	case "dynamodb":
		return nil, fmt.Errorf("DynamoDB repository not yet implemented")
	case "redis":
		return nil, fmt.Errorf("redis repository not yet implemented")
	case "memory":
		return nil, fmt.Errorf("InMemory repository not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported persistence repository: %s", config.Strategy)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create persistence repository: %w", err)
	}

	return &Manager{
		repository: repository,
		config:     config,
	}, nil
}

// NewManagerWithRepository creates a new persistence manager with a specific repository (mainly for testing)
func NewManagerWithRepository(repo Repository) *Manager {
	return &Manager{
		repository: repo,
	}
}

// Initialize initializes the persistence layer
func (pm *Manager) Initialize(ctx context.Context) error {
	return pm.repository.Initialize(ctx)
}

// Close closes the persistence layer
func (pm *Manager) Close() error {
	return pm.repository.Close()
}

// GetRepository returns the underlying repository (for testing)
func (pm *Manager) GetRepository() Repository {
	return pm.repository
}

// SaveExecution saves an execution record
func (pm *Manager) SaveExecution(ctx context.Context, exec *execution.Execution) error {
	record := &ExecutionRecord{
		ExecutionID:    exec.ID,
		StateMachineID: exec.StateMachineID,
		Name:           exec.Name,
		Input:          exec.Input,
		Output:         exec.Output,
		Status:         exec.Status,
		StartTime:      &exec.StartTime,
		CurrentState:   exec.CurrentState,
	}

	if !exec.EndTime.IsZero() {
		record.EndTime = &exec.EndTime
	}

	if exec.Error != nil {
		record.Error = exec.Error.Error()
	}

	return pm.repository.SaveExecution(ctx, record)
}

// SaveStateMachine saves a state machine definition
func (pm *Manager) SaveStateMachine(ctx context.Context, sm *StateMachineRecord) error {
	if sm.CreatedAt.IsZero() {
		sm.CreatedAt = time.Now()
	}
	if sm.UpdatedAt.IsZero() {
		sm.UpdatedAt = time.Now()
	}
	return pm.repository.SaveStateMachine(ctx, sm)
}

// GetStateMachine retrieves a state machine by ID
func (pm *Manager) GetStateMachine(ctx context.Context, stateMachineID string) (*StateMachineRecord, error) {
	return pm.repository.GetStateMachine(ctx, stateMachineID)
}

// ListStateMachines lists all state machines with filtering
func (pm *Manager) ListStateMachines(ctx context.Context, filter *DefinitionFilter) ([]*StateMachineRecord, error) {
	return pm.repository.ListStateMachines(ctx, filter)
}

// SaveStateHistory saves a state history entry
func (pm *Manager) SaveStateHistory(ctx context.Context, executionInstance *execution.Execution, history *execution.StateHistory) error {
	record := &StateHistoryRecord{
		ID:                 generateHistoryID(executionInstance.ID, history.StateName, time.Now()),
		ExecutionID:        executionInstance.ID,
		ExecutionStartTime: &executionInstance.StartTime,
		StateName:          history.StateName,
		StateType:          history.StateType,
		Input:              history.Input,
		Output:             history.Output,
		Status:             history.Status,
		StartTime:          &history.StartTime,
		RetryCount:         history.RetryCount,
		SequenceNumber:     history.SequenceNumber,
	}

	if !history.EndTime.IsZero() {
		record.EndTime = &history.EndTime
	}

	if history.Error != nil {
		record.Error = history.Error.Error()
	}

	return pm.repository.SaveStateHistory(ctx, record)
}

// GetExecution retrieves an execution
func (pm *Manager) GetExecution(ctx context.Context, executionID string) (*ExecutionRecord, error) {
	return pm.repository.GetExecution(ctx, executionID)
}

// GetStateHistory retrieves state history
func (pm *Manager) GetStateHistory(ctx context.Context, executionID string) ([]*StateHistoryRecord, error) {
	return pm.repository.GetStateHistory(ctx, executionID)
}

// ListExecutions lists executions
func (pm *Manager) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error) {
	return pm.repository.ListExecutions(ctx, filter)
}

// ListNonLinkedExecutions lists executions that have no linked executions matching the filter criteria
// This allows finding executions that don't have specific types of linked executions
// For example: executions with no SUCCEEDED linked executions from a specific state
func (pm *Manager) ListNonLinkedExecutions(ctx context.Context, executionFilter *ExecutionFilter, linkedExecutionFilter *LinkedExecutionFilter) ([]*ExecutionRecord, error) {
	return pm.repository.ListNonLinkedExecutions(ctx, executionFilter, linkedExecutionFilter)
}

func (pm *Manager) CountExecutions(ctx context.Context, filter *ExecutionFilter) (int64, error) {
	return pm.repository.CountExecutions(ctx, filter)
}

// GetExecutionOutput retrieves output from an execution (final or specific state)
func (pm *Manager) GetExecutionOutput(ctx context.Context, executionID, stateName string) (interface{}, error) {
	return pm.repository.GetExecutionOutput(ctx, executionID, stateName)
}

// ListExecutionIDs returns only execution IDs matching the filter
func (pm *Manager) ListExecutionIDs(ctx context.Context, filter *ExecutionFilter) ([]string, error) {
	return pm.repository.ListExecutionIDs(ctx, filter)
}

// SaveLinkedExecution saves a linked execution record
func (pm *Manager) SaveLinkedExecution(ctx context.Context, linkedExec *LinkedExecutionRecord) error {
	return pm.repository.SaveLinkedExecution(ctx, linkedExec)
}

// ListLinkedExecutions lists linked executions with filtering
func (pm *Manager) ListLinkedExecutions(ctx context.Context, filter *LinkedExecutionFilter) ([]*LinkedExecutionRecord, error) {
	return pm.repository.ListLinkedExecutions(ctx, filter)
}

// CountLinkedExecutions returns the count of linked executions matching the filter
func (pm *Manager) CountLinkedExecutions(ctx context.Context, filter *LinkedExecutionFilter) (int64, error) {
	return pm.repository.CountLinkedExecutions(ctx, filter)
}

// Helper function to generate unique history IDs
func generateHistoryID(executionID, stateName string, timestamp time.Time) string {
	return fmt.Sprintf("%s-%s-%d", executionID, stateName, timestamp.UnixNano())
}

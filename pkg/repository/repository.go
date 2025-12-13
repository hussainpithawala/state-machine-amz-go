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

// Strategy defines the interface for different persistence backends
type Strategy interface {
	// Initialize the persistence layer
	Initialize(ctx context.Context) error

	// Close the persistence connection
	Close() error

	// SaveExecution saves or updates an execution record
	SaveExecution(ctx context.Context, record *ExecutionRecord) error

	// GetExecution retrieves an execution by ID
	GetExecution(ctx context.Context, executionID string) (*ExecutionRecord, error)

	// SaveStateHistory saves a state execution to history
	SaveStateHistory(ctx context.Context, record *StateHistoryRecord) error

	// GetStateHistory retrieves all state history for an execution
	GetStateHistory(ctx context.Context, executionID string) ([]*StateHistoryRecord, error)

	// ListExecutions lists executions with optional filters
	ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error)

	// DeleteExecution removes an execution and its history
	DeleteExecution(ctx context.Context, executionID string) error

	// HealthCheck verifies the persistence backend is available
	HealthCheck(ctx context.Context) error
}

// Config holds configuration for the repository layer

// Manager manages the persistence strategy
type Manager struct {
	strategy Strategy
	config   *Config
}

// NewPersistenceManager creates a new persistence manager
func NewPersistenceManager(config *Config) (*Manager, error) {
	var strategy Strategy
	var err error

	switch config.Strategy {
	case "postgres":
		strategy, err = NewPostgresStrategy(config)
	case "gorm-postgres":
		strategy, err = NewGormStrategy(config)
	case "dynamodb":
		return nil, fmt.Errorf("DynamoDB strategy not yet implemented")
	case "redis":
		return nil, fmt.Errorf("redis strategy not yet implemented")
	case "memory":
		return nil, fmt.Errorf("InMemory strategy not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported persistence strategy: %s", config.Strategy)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create persistence strategy: %w", err)
	}

	return &Manager{
		strategy: strategy,
		config:   config,
	}, nil
}

// Initialize initializes the persistence layer
func (pm *Manager) Initialize(ctx context.Context) error {
	return pm.strategy.Initialize(ctx)
}

// Close closes the persistence layer
func (pm *Manager) Close() error {
	return pm.strategy.Close()
}

// SaveExecution saves an execution record
func (pm *Manager) SaveExecution(ctx context.Context, exec *execution.Execution) error {
	record := &ExecutionRecord{
		ExecutionID:  exec.ID,
		Name:         exec.Name,
		Input:        exec.Input,
		Output:       exec.Output,
		Status:       exec.Status,
		StartTime:    &exec.StartTime,
		CurrentState: exec.CurrentState,
	}

	if !exec.EndTime.IsZero() {
		record.EndTime = &exec.EndTime
	}

	if exec.Error != nil {
		record.Error = exec.Error.Error()
	}

	return pm.strategy.SaveExecution(ctx, record)
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

	return pm.strategy.SaveStateHistory(ctx, record)
}

// GetExecution retrieves an execution
func (pm *Manager) GetExecution(ctx context.Context, executionID string) (*ExecutionRecord, error) {
	return pm.strategy.GetExecution(ctx, executionID)
}

// GetStateHistory retrieves state history
func (pm *Manager) GetStateHistory(ctx context.Context, executionID string) ([]*StateHistoryRecord, error) {
	return pm.strategy.GetStateHistory(ctx, executionID)
}

// ListExecutions lists executions
func (pm *Manager) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error) {
	return pm.strategy.ListExecutions(ctx, filter)
}

// Helper function to generate unique history IDs
func generateHistoryID(executionID, stateName string, timestamp time.Time) string {
	return fmt.Sprintf("%s-%s-%d", executionID, stateName, timestamp.UnixNano())
}

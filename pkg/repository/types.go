// pkg/repository/types.go
package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

// Execution represents a state machine execution
type Execution struct {
	ID             string
	StateMachineID string
	Name           string
	Input          map[string]interface{}
	Output         map[string]interface{}
	Status         string // RUNNING, SUCCEEDED, FAILED, CANCELLED, TIMED_OUT, ABORTED
	StartTime      time.Time
	EndTime        time.Time
	CurrentState   string
	Error          error
	Metadata       map[string]interface{}
}

// StateHistory represents a single state execution in the workflow
type StateHistory struct {
	ID             string
	ExecutionID    string
	StateName      string
	StateType      string
	Input          map[string]interface{}
	Output         map[string]interface{}
	Status         string // SUCCEEDED, FAILED, RUNNING, CANCELLED, TIMED_OUT, RETRYING
	StartTime      time.Time
	EndTime        time.Time
	Error          error
	RetryCount     int
	SequenceNumber int
	Metadata       map[string]interface{}
}

// ExecutionFilter defines filters for querying executions
type ExecutionFilter struct {
	Status         string    // Filter by status
	StateMachineID string    // Filter by state machine ID
	Name           string    // Filter by name (supports partial match)
	StartAfter     time.Time // Filter executions started after this time
	StartBefore    time.Time // Filter executions started before this time
	Limit          int       // Maximum number of results
	Offset         int       // Offset for pagination
}

// Statistics represents aggregated execution statistics
type Statistics struct {
	StateMachineID string
	ByStatus       map[string]*StatusStatistics
	TotalCount     int64
	UpdatedAt      time.Time
}

// StatusStatistics represents statistics for a specific status
type StatusStatistics struct {
	Count              int64
	AvgDurationSeconds float64
	MinDurationSeconds float64
	MaxDurationSeconds float64
	P50Duration        float64 // Median
	P95Duration        float64 // 95th percentile
	P99Duration        float64 // 99th percentile
	FirstExecution     time.Time
	LastExecution      time.Time
}

// Repository defines the interface for execution persistence
type Repository interface {
	// Initialize creates necessary database schema
	Initialize(ctx context.Context) error

	// Close closes the repository connection
	Close() error

	// SaveExecution saves or updates an execution
	SaveExecution(ctx context.Context, execution *ExecutionRecord) error

	// GetExecution retrieves an execution by ID
	GetExecution(ctx context.Context, executionID string) (*ExecutionRecord, error)

	// SaveStateHistory saves a state history entry
	SaveStateHistory(ctx context.Context, history *StateHistoryRecord) error

	// GetStateHistory retrieves all state history for an execution
	GetStateHistory(ctx context.Context, executionID string) ([]*StateHistoryRecord, error)

	// ListExecutions lists executions with filtering and pagination
	ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error)

	// CountExecutions returns the count of executions matching the filter
	CountExecutions(ctx context.Context, filter *ExecutionFilter) (int64, error)

	// DeleteExecution removes an execution and its history
	DeleteExecution(ctx context.Context, executionID string) error

	// HealthCheck verifies the repository is accessible
	HealthCheck(ctx context.Context) error

	// SaveStateMachine saves a state machine definition
	SaveStateMachine(ctx context.Context, stateMachine *StateMachineRecord) error

	// GetStateMachine retrieves a state machine by ID
	GetStateMachine(ctx context.Context, stateMachineID string) (*StateMachineRecord, error)
}

// ExtendedRepository defines additional repository capabilities
type ExtendedRepository interface {
	Repository
	MessageRepository
	// GetExecutionWithHistory retrieves an execution with its full state history
	GetExecutionWithHistory(ctx context.Context, executionID string) (*ExecutionRecord, []*StateHistoryRecord, error)

	// GetStatistics returns aggregated statistics for a state machine
	GetStatistics(ctx context.Context, stateMachineID string) (*Statistics, error)

	// UpdateStatistics refreshes the execution statistics
	UpdateStatistics(ctx context.Context) error
}

// Config holds repository configuration
type Config struct {
	Strategy      string                 // Repository repository: "gorm", "postgres", "memory"
	ConnectionURL string                 // Database connection URL
	Options       map[string]interface{} // Strategy-specific options
}

// NewRepository creates a new repository based on the repository
func NewRepository(config *Config) (Repository, error) {
	switch config.Strategy {
	case "gorm":
		return NewGormPostgresRepository(config)
	case "postgres":
		return NewPostgresRepository(config)
	case "memory":
		return nil, fmt.Errorf("in-memory repository is not supported: %s", config.Strategy)
	default:
		return nil, fmt.Errorf("unsupported repository repository: %s", config.Strategy)
	}
}

// ExecutionStatus constants
const (
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
	StatusCancelled = "CANCELLED"
	StatusTimedOut  = "TIMED_OUT"
	StatusAborted   = "ABORTED"
)

// StateHistoryStatus constants
const (
	StateStatusSucceeded = "SUCCEEDED"
	StateStatusFailed    = "FAILED"
	StateStatusRunning   = "RUNNING"
	StateStatusCancelled = "CANCELLED"
	StateStatusTimedOut  = "TIMED_OUT"
	StateStatusRetrying  = "RETRYING"
)

// IsTerminal returns true if the execution status is terminal
func (e *Execution) IsTerminal() bool {
	switch e.Status {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusTimedOut, StatusAborted:
		return true
	default:
		return false
	}
}

// Duration returns the execution duration
func (e *Execution) Duration() time.Duration {
	if e.EndTime.IsZero() {
		return time.Since(e.StartTime)
	}
	return e.EndTime.Sub(e.StartTime)
}

// IsTerminal returns true if the state history status is terminal
func (sh *StateHistory) IsTerminal() bool {
	switch sh.Status {
	case StateStatusSucceeded, StateStatusFailed, StateStatusCancelled, StateStatusTimedOut:
		return true
	default:
		return false
	}
}

// Duration returns the state execution duration
func (sh *StateHistory) Duration() time.Duration {
	if sh.EndTime.IsZero() {
		return time.Since(sh.StartTime)
	}
	return sh.EndTime.Sub(sh.StartTime)
}

// NewExecution creates a new execution with default values
func NewExecution(stateMachineID, name string, input map[string]interface{}) *Execution {
	return &Execution{
		ID:             generateExecutionID(),
		StateMachineID: stateMachineID,
		Name:           name,
		Input:          input,
		Status:         StatusRunning,
		StartTime:      time.Now().UTC(),
		Metadata:       make(map[string]interface{}),
	}
}

// NewStateHistory creates a new state history entry
func NewStateHistory(executionID, stateName, stateType string, input map[string]interface{}, sequenceNumber int) *StateHistory {
	return &StateHistory{
		ID:             generateStateHistoryID(executionID, stateName, sequenceNumber),
		ExecutionID:    executionID,
		StateName:      stateName,
		StateType:      stateType,
		Input:          input,
		Status:         StateStatusRunning,
		StartTime:      time.Now().UTC(),
		SequenceNumber: sequenceNumber,
		RetryCount:     0,
		Metadata:       make(map[string]interface{}),
	}
}

// MarkSucceeded marks the execution as succeeded
func (e *Execution) MarkSucceeded(output map[string]interface{}) {
	e.Status = StatusSucceeded
	e.Output = output
	e.EndTime = time.Now().UTC()
}

// MarkFailed marks the execution as failed
func (e *Execution) MarkFailed(err error) {
	e.Status = StatusFailed
	e.Error = err
	e.EndTime = time.Now().UTC()
}

// MarkCancelled marks the execution as cancelled
func (e *Execution) MarkCancelled() {
	e.Status = StatusCancelled
	e.EndTime = time.Now().UTC()
}

// MarkTimedOut marks the execution as timed out
func (e *Execution) MarkTimedOut() {
	e.Status = StatusTimedOut
	e.EndTime = time.Now().UTC()
}

// MarkSucceeded marks the state history as succeeded
func (sh *StateHistory) MarkSucceeded(output map[string]interface{}) {
	sh.Status = StateStatusSucceeded
	sh.Output = output
	sh.EndTime = time.Now().UTC()
}

// MarkFailed marks the state history as failed
func (sh *StateHistory) MarkFailed(err error) {
	sh.Status = StateStatusFailed
	sh.Error = err
	sh.EndTime = time.Now().UTC()
}

// IncrementRetry increments the retry count
func (sh *StateHistory) IncrementRetry() {
	sh.RetryCount++
	sh.Status = StateStatusRetrying
}

// Helper functions

// generateExecutionID generates a unique execution ID
func generateExecutionID() string {
	b := make([]byte, 16)
	// Check the error return value
	_, err := rand.Read(b)

	if err != nil {
		// Log the error and panic/fatal exit, as cryptographic security has failed
		log.Fatalf("Fatal error reading from crypto/rand: %v", err)
	}
	return fmt.Sprintf("[generateExecutionID] exec-%s", hex.EncodeToString(b))
}

// generateStateHistoryID generates a unique state history ID
func generateStateHistoryID(executionID, stateName string, sequenceNumber int) string {
	b := make([]byte, 8)
	_, err := rand.Read(b)

	if err != nil {
		// Log the error and panic/fatal exit, as cryptographic security has failed
		log.Fatalf("[generateStateHistoryID] Fatal error reading from crypto/rand: %v", err)
	}

	return fmt.Sprintf("%s-%s-%d-%s", executionID, stateName, sequenceNumber, hex.EncodeToString(b)[:8])
}

// Clone creates a deep copy of the execution
func (e *Execution) Clone() *Execution {
	clone := &Execution{
		ID:             e.ID,
		StateMachineID: e.StateMachineID,
		Name:           e.Name,
		Status:         e.Status,
		StartTime:      e.StartTime,
		EndTime:        e.EndTime,
		CurrentState:   e.CurrentState,
		Error:          e.Error,
	}

	if e.Input != nil {
		clone.Input = cloneMap(e.Input)
	}
	if e.Output != nil {
		clone.Output = cloneMap(e.Output)
	}
	if e.Metadata != nil {
		clone.Metadata = cloneMap(e.Metadata)
	}

	return clone
}

// Clone creates a deep copy of the state history
func (sh *StateHistory) Clone() *StateHistory {
	clone := &StateHistory{
		ID:             sh.ID,
		ExecutionID:    sh.ExecutionID,
		StateName:      sh.StateName,
		StateType:      sh.StateType,
		Status:         sh.Status,
		StartTime:      sh.StartTime,
		EndTime:        sh.EndTime,
		Error:          sh.Error,
		RetryCount:     sh.RetryCount,
		SequenceNumber: sh.SequenceNumber,
	}

	if sh.Input != nil {
		clone.Input = cloneMap(sh.Input)
	}
	if sh.Output != nil {
		clone.Output = cloneMap(sh.Output)
	}
	if sh.Metadata != nil {
		clone.Metadata = cloneMap(sh.Metadata)
	}

	return clone
}

// cloneMap creates a shallow copy of a map
func cloneMap(m map[string]interface{}) map[string]interface{} {
	clone := make(map[string]interface{}, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}

// Validation helpers

// Validate validates the execution
func (e *Execution) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("execution ID is required")
	}
	if e.StateMachineID == "" {
		return fmt.Errorf("state machine ID is required")
	}
	if e.Name == "" {
		return fmt.Errorf("execution name is required")
	}
	if e.Status == "" {
		return fmt.Errorf("execution status is required")
	}
	if !isValidExecutionStatus(e.Status) {
		return fmt.Errorf("invalid execution status: %s", e.Status)
	}
	if e.StartTime.IsZero() {
		return fmt.Errorf("start time is required")
	}
	return nil
}

// Validate validates the state history
func (sh *StateHistory) Validate() error {
	if sh.ID == "" {
		return fmt.Errorf("state history ID is required")
	}
	if sh.ExecutionID == "" {
		return fmt.Errorf("execution ID is required")
	}
	if sh.StateName == "" {
		return fmt.Errorf("state name is required")
	}
	if sh.StateType == "" {
		return fmt.Errorf("state type is required")
	}
	if sh.Status == "" {
		return fmt.Errorf("state history status is required")
	}
	if !isValidStateHistoryStatus(sh.Status) {
		return fmt.Errorf("invalid state history status: %s", sh.Status)
	}
	if sh.StartTime.IsZero() {
		return fmt.Errorf("start time is required")
	}
	if sh.SequenceNumber < 0 {
		return fmt.Errorf("sequence number must be non-negative")
	}
	return nil
}

// isValidExecutionStatus checks if the execution status is valid
func isValidExecutionStatus(status string) bool {
	validStatuses := []string{
		StatusRunning,
		StatusSucceeded,
		StatusFailed,
		StatusCancelled,
		StatusTimedOut,
		StatusAborted,
	}
	for _, s := range validStatuses {
		if status == s {
			return true
		}
	}
	return false
}

// isValidStateHistoryStatus checks if the state history status is valid
func isValidStateHistoryStatus(status string) bool {
	validStatuses := []string{
		StateStatusSucceeded,
		StateStatusFailed,
		StateStatusRunning,
		StateStatusCancelled,
		StateStatusTimedOut,
		StateStatusRetrying,
	}
	for _, s := range validStatuses {
		if status == s {
			return true
		}
	}
	return false
}

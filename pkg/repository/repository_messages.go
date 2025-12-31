// pkg/repository/repository_extensions.go
package repository

import (
	"context"
	"fmt"
	"time"
)

// MessageCorrelationRecord represents correlation data for message states
type MessageCorrelationRecord struct {
	ID                 string      `json:"id"`
	ExecutionID        string      `json:"execution_id"`
	ExecutionStartTime *time.Time  `json:"execution_start_time,omitempty"`
	StateMachineID     string      `json:"state_machine_id"`
	StateName          string      `json:"state_name"`
	CorrelationKey     string      `json:"correlation_key"`
	CorrelationValue   interface{} `json:"correlation_value"`
	CreatedAt          int64       `json:"created_at"`           // Unix timestamp
	TimeoutAt          *int64      `json:"timeout_at,omitempty"` // Unix timestamp
	Status             string      `json:"status"`               // "WAITING", "RECEIVED", "TIMEOUT"
}

// MessageCorrelationFilter for querying correlation records
type MessageCorrelationFilter struct {
	CorrelationKey   string
	CorrelationValue interface{}
	Status           string
	StateMachineID   string
	Limit            int
	Offset           int
}

// Repository interface extensions for message state support
type MessageRepository interface {
	// SaveMessageCorrelation saves a message correlation record
	SaveMessageCorrelation(ctx context.Context, record *MessageCorrelationRecord) error

	// GetMessageCorrelation retrieves a correlation record by ID
	GetMessageCorrelation(ctx context.Context, id string) (*MessageCorrelationRecord, error)

	// FindWaitingCorrelations finds correlation records waiting for messages
	FindWaitingCorrelations(ctx context.Context, filter *MessageCorrelationFilter) ([]*MessageCorrelationRecord, error)

	// UpdateCorrelationStatus updates the status of a correlation record
	UpdateCorrelationStatus(ctx context.Context, id string, status string) error

	// DeleteMessageCorrelation deletes a correlation record
	DeleteMessageCorrelation(ctx context.Context, id string) error

	// ListTimedOutCorrelations finds correlations that have exceeded their timeout
	ListTimedOutCorrelations(ctx context.Context, currentTimestamp int64) ([]*MessageCorrelationRecord, error)
}

// SaveMessageCorrelation saves correlation data when a message state pauses execution
func (pm *Manager) SaveMessageCorrelation(ctx context.Context, record *MessageCorrelationRecord) error {
	if msgRepo, ok := pm.repository.(MessageRepository); ok {
		return msgRepo.SaveMessageCorrelation(ctx, record)
	}
	return fmt.Errorf("repository does not support message correlation")
}

// FindWaitingExecutionsByCorrelation finds executions waiting for a specific message
func (pm *Manager) FindWaitingExecutionsByCorrelation(ctx context.Context, correlationKey string, correlationValue interface{}) ([]*ExecutionRecord, error) {
	if msgRepo, ok := pm.repository.(MessageRepository); ok {
		// Find correlation records
		filter := &MessageCorrelationFilter{
			CorrelationKey:   correlationKey,
			CorrelationValue: correlationValue,
			Status:           "WAITING",
		}

		correlations, err := msgRepo.FindWaitingCorrelations(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to find correlations: %w", err)
		}

		// Fetch execution records for each correlation
		var executions []*ExecutionRecord
		for _, corr := range correlations {
			exec, err := pm.GetExecution(ctx, corr.ExecutionID)
			if err != nil {
				// Log error but continue with other executions
				continue
			}
			executions = append(executions, exec)
		}

		return executions, nil
	}

	return nil, fmt.Errorf("repository does not support message correlation")
}

// UpdateCorrelationStatus updates the status of a correlation record
func (pm *Manager) UpdateCorrelationStatus(ctx context.Context, correlationID string, status string) error {
	if msgRepo, ok := pm.repository.(MessageRepository); ok {
		return msgRepo.UpdateCorrelationStatus(ctx, correlationID, status)
	}
	return fmt.Errorf("repository does not support message correlation")
}

// ProcessTimedOutMessages finds and processes message correlations that have timed out
func (pm *Manager) ProcessTimedOutMessages(ctx context.Context, currentTimestamp int64) ([]*MessageCorrelationRecord, error) {
	if msgRepo, ok := pm.repository.(MessageRepository); ok {
		return msgRepo.ListTimedOutCorrelations(ctx, currentTimestamp)
	}
	return nil, fmt.Errorf("repository does not support message correlation")
}

// GetMessageCorrelation retrieves a correlation record
func (pm *Manager) GetMessageCorrelation(ctx context.Context, id string) (*MessageCorrelationRecord, error) {
	if msgRepo, ok := pm.repository.(MessageRepository); ok {
		return msgRepo.GetMessageCorrelation(ctx, id)
	}
	return nil, fmt.Errorf("repository does not support message correlation")
}

// DeleteMessageCorrelation deletes a correlation record
func (pm *Manager) DeleteMessageCorrelation(ctx context.Context, id string) error {
	if msgRepo, ok := pm.repository.(MessageRepository); ok {
		return msgRepo.DeleteMessageCorrelation(ctx, id)
	}
	return fmt.Errorf("repository does not support message correlation")
}

// pkg/repository/models.go
package repository

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// JSONB is a custom type for PostgreSQL JSONB fields
type JSONB map[string]interface{}

// Value implements the driver.Valuer interface for JSONB
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface for JSONB
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to unmarshal JSONB value")
	}

	result := make(map[string]interface{})
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}

	*j = result
	return nil
}

// ExecutionModel represents the executions table
type ExecutionModel struct {
	ExecutionID    string     `gorm:"primaryKey;size:255;not null"`
	StateMachineID string     `gorm:"size:255;not null;index:idx_state_machine"`
	Name           string     `gorm:"size:255;not null"`
	Input          JSONB      `gorm:"type:jsonb"`
	Output         JSONB      `gorm:"type:jsonb"`
	Status         string     `gorm:"size:50;not null;index:idx_status"`
	StartTime      time.Time  `gorm:"not null;index:idx_start_time"`
	EndTime        *time.Time `gorm:"index:idx_end_time"`
	CurrentState   string     `gorm:"size:255;not null"`
	Error          string     `gorm:"type:text"`
	Metadata       JSONB      `gorm:"type:jsonb;default:'{}'"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`

	// Relationships
	StateHistory []StateHistoryModel `gorm:"foreignKey:ExecutionID;references:ExecutionID;constraint:OnDelete:CASCADE"`
}

// TableName specifies the table name for ExecutionModel
func (ExecutionModel) TableName() string {
	return "executions"
}

// BeforeCreate hook validates execution before creation
func (e *ExecutionModel) BeforeCreate(tx *gorm.DB) error {
	validStatuses := []string{"RUNNING", "SUCCEEDED", "FAILED", "CANCELLED", "TIMED_OUT", "ABORTED"}
	for _, status := range validStatuses {
		if e.Status == status {
			return nil
		}
	}
	return errors.New("invalid execution status")
}

// StateHistoryModel represents the state_history table
type StateHistoryModel struct {
	ID             string     `gorm:"primaryKey;size:255;not null"`
	ExecutionID    string     `gorm:"size:255;not null;index:idx_execution_id"`
	StateName      string     `gorm:"size:255;not null;index:idx_state_name"`
	StateType      string     `gorm:"size:50;not null"`
	Input          JSONB      `gorm:"type:jsonb"`
	Output         JSONB      `gorm:"type:jsonb"`
	Status         string     `gorm:"size:50;not null;index:idx_state_status"`
	StartTime      time.Time  `gorm:"not null;index:idx_state_start_time"`
	EndTime        *time.Time `gorm:"index:idx_state_end_time"`
	Error          string     `gorm:"type:text"`
	RetryCount     int        `gorm:"default:0;not null"`
	SequenceNumber int        `gorm:"not null;index:idx_sequence"`
	Metadata       JSONB      `gorm:"type:jsonb;default:'{}'"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`

	// Foreign key relationship
	Execution ExecutionModel `gorm:"foreignKey:ExecutionID;references:ExecutionID"`
}

// TableName specifies the table name for StateHistoryModel
func (StateHistoryModel) TableName() string {
	return "state_history"
}

// BeforeCreate hook validates state history before creation
func (s *StateHistoryModel) BeforeCreate(tx *gorm.DB) error {
	validStatuses := []string{"SUCCEEDED", "FAILED", "RUNNING", "CANCELLED", "TIMED_OUT", "RETRYING"}
	for _, status := range validStatuses {
		if s.Status == status {
			return nil
		}
	}
	return errors.New("invalid state history status")
}

// ExecutionStatisticsModel represents aggregated execution statistics
type ExecutionStatisticsModel struct {
	ID                 uint      `gorm:"primaryKey;autoIncrement"`
	StateMachineID     string    `gorm:"size:255;not null;uniqueIndex:idx_stats_unique"`
	Status             string    `gorm:"size:50;not null;uniqueIndex:idx_stats_unique"`
	ExecutionCount     int64     `gorm:"not null;default:0"`
	AvgDurationSeconds float64   `gorm:"type:decimal(10,2)"`
	MinDurationSeconds float64   `gorm:"type:decimal(10,2)"`
	MaxDurationSeconds float64   `gorm:"type:decimal(10,2)"`
	FirstExecution     time.Time `gorm:"not null"`
	LastExecution      time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"autoUpdateTime"`
}

// TableName specifies the table name
func (ExecutionStatisticsModel) TableName() string {
	return "execution_statistics"
}

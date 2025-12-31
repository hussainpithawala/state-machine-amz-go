// pkg/repository/postgres_message_repository.go
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Message correlation table schema
const createMessageCorrelationTableSQL = `
CREATE TABLE IF NOT EXISTS message_correlations (
	id VARCHAR(255) PRIMARY KEY,
	execution_id VARCHAR(255) NOT NULL,
	execution_start_time TIMESTAMP NOT NULL,
	state_machine_id VARCHAR(255) NOT NULL,
	state_name VARCHAR(255) NOT NULL,
	correlation_key VARCHAR(255) NOT NULL,
	correlation_value JSONB NOT NULL,
	created_at BIGINT NOT NULL,
	timeout_at BIGINT,
	status VARCHAR(50) NOT NULL DEFAULT 'WAITING',
	CONSTRAINT fk_execution_message FOREIGN KEY (execution_id, execution_start_time) 
		REFERENCES executions(execution_id, start_time) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_message_correlations_correlation_key 
	ON message_correlations(correlation_key);
	
CREATE INDEX IF NOT EXISTS idx_message_correlations_status 
	ON message_correlations(status);
	
CREATE INDEX IF NOT EXISTS idx_message_correlations_execution_id 
	ON message_correlations(execution_id);
	
CREATE INDEX IF NOT EXISTS idx_message_correlations_timeout_at 
	ON message_correlations(timeout_at) WHERE timeout_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_correlations_key_value_status 
	ON message_correlations(correlation_key, correlation_value, status);
`

// SaveMessageCorrelation implements MessageRepository interface for PostgresRepository
func (r *PostgresRepository) SaveMessageCorrelation(ctx context.Context, record *MessageCorrelationRecord) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Serialize correlation value to JSON
	correlationValueJSON, err := json.Marshal(record.CorrelationValue)
	if err != nil {
		return fmt.Errorf("failed to marshal correlation value: %w", err)
	}

	query := `
		INSERT INTO message_correlations 
			(id, execution_id, execution_start_time, state_machine_id, state_name, correlation_key, 
			 correlation_value, created_at, timeout_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			timeout_at = EXCLUDED.timeout_at
	`

	_, err = r.db.ExecContext(ctx, query,
		record.ID,
		record.ExecutionID,
		record.ExecutionStartTime,
		record.StateMachineID,
		record.StateName,
		record.CorrelationKey,
		correlationValueJSON,
		record.CreatedAt,
		record.TimeoutAt,
		record.Status,
	)

	if err != nil {
		return fmt.Errorf("failed to save message correlation: %w", err)
	}

	return nil
}

// GetMessageCorrelation retrieves a correlation record by ID
func (r *PostgresRepository) GetMessageCorrelation(ctx context.Context, id string) (*MessageCorrelationRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, execution_id, execution_start_time, state_machine_id, state_name, correlation_key,
		       correlation_value, created_at, timeout_at, status
		FROM message_correlations
		WHERE id = $1
	`

	var record MessageCorrelationRecord
	var correlationValueJSON []byte
	var timeoutAt sql.NullInt64

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&record.ID,
		&record.ExecutionID,
		&record.ExecutionStartTime,
		&record.StateMachineID,
		&record.StateName,
		&record.CorrelationKey,
		&correlationValueJSON,
		&record.CreatedAt,
		&timeoutAt,
		&record.Status,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("correlation record not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get correlation record: %w", err)
	}

	// Deserialize correlation value
	if err := json.Unmarshal(correlationValueJSON, &record.CorrelationValue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal correlation value: %w", err)
	}

	if timeoutAt.Valid {
		timeout := timeoutAt.Int64
		record.TimeoutAt = &timeout
	}

	return &record, nil
}

// FindWaitingCorrelations finds correlation records waiting for messages
func (r *PostgresRepository) FindWaitingCorrelations(ctx context.Context, filter *MessageCorrelationFilter) ([]*MessageCorrelationRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query, args, err := r.buildFindWaitingCorrelationsQuery(filter)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query correlations: %w", err)
	}
	defer rows.Close()

	var records []*MessageCorrelationRecord

	for rows.Next() {
		var record MessageCorrelationRecord
		var correlationValueJSON []byte
		var timeoutAt sql.NullInt64

		err := rows.Scan(
			&record.ID,
			&record.ExecutionID,
			&record.ExecutionStartTime,
			&record.StateMachineID,
			&record.StateName,
			&record.CorrelationKey,
			&correlationValueJSON,
			&record.CreatedAt,
			&timeoutAt,
			&record.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan correlation record: %w", err)
		}

		// Deserialize correlation value
		if err := json.Unmarshal(correlationValueJSON, &record.CorrelationValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal correlation value: %w", err)
		}

		if timeoutAt.Valid {
			timeout := timeoutAt.Int64
			record.TimeoutAt = &timeout
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating correlation records: %w", err)
	}

	return records, nil
}

func (r *PostgresRepository) buildFindWaitingCorrelationsQuery(filter *MessageCorrelationFilter) (string, []interface{}, error) {
	query := `
		SELECT id, execution_id, execution_start_time, state_machine_id, state_name, correlation_key,
		       correlation_value, created_at, timeout_at, status
		FROM message_correlations
		WHERE 1=1
	`
	args := []interface{}{}
	argPos := 1

	if filter == nil {
		return query, args, nil
	}

	if filter.CorrelationKey != "" {
		query += fmt.Sprintf(" AND correlation_key = $%d", argPos)
		args = append(args, filter.CorrelationKey)
		argPos++
	}

	if filter.CorrelationValue != nil {
		// Serialize correlation value for comparison
		valueJSON, err := json.Marshal(filter.CorrelationValue)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal correlation value: %w", err)
		}
		query += fmt.Sprintf(" AND correlation_value = $%d::jsonb", argPos)
		args = append(args, valueJSON)
		argPos++
	}

	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, filter.Status)
		argPos++
	}

	if filter.StateMachineID != "" {
		query += fmt.Sprintf(" AND state_machine_id = $%d", argPos)
		args = append(args, filter.StateMachineID)
		argPos++
	}

	query += " ORDER BY created_at ASC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, filter.Limit)
		argPos++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, filter.Offset)
	}

	return query, args, nil
}

// UpdateCorrelationStatus updates the status of a correlation record
func (r *PostgresRepository) UpdateCorrelationStatus(ctx context.Context, id string, status string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `
		UPDATE message_correlations
		SET status = $1
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update correlation status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("correlation record not found: %s", id)
	}

	return nil
}

// DeleteMessageCorrelation deletes a correlation record
func (r *PostgresRepository) DeleteMessageCorrelation(ctx context.Context, id string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	query := `DELETE FROM message_correlations WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete correlation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("correlation record not found: %s", id)
	}

	return nil
}

// ListTimedOutCorrelations finds correlations that have exceeded their timeout
func (r *PostgresRepository) ListTimedOutCorrelations(ctx context.Context, currentTimestamp int64) ([]*MessageCorrelationRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, execution_id, execution_start_time, state_machine_id, state_name, correlation_key,
		       correlation_value, created_at, timeout_at, status
		FROM message_correlations
		WHERE status = 'WAITING'
		  AND timeout_at IS NOT NULL
		  AND timeout_at <= $1
	`

	rows, err := r.db.QueryContext(ctx, query, currentTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to query timed out correlations: %w", err)
	}
	defer rows.Close()

	var records []*MessageCorrelationRecord

	for rows.Next() {
		var record MessageCorrelationRecord
		var correlationValueJSON []byte
		var timeoutAt sql.NullInt64

		err := rows.Scan(
			&record.ID,
			&record.ExecutionID,
			&record.ExecutionStartTime,
			&record.StateMachineID,
			&record.StateName,
			&record.CorrelationKey,
			&correlationValueJSON,
			&record.CreatedAt,
			&timeoutAt,
			&record.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan correlation record: %w", err)
		}

		// Deserialize correlation value
		if err := json.Unmarshal(correlationValueJSON, &record.CorrelationValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal correlation value: %w", err)
		}

		if timeoutAt.Valid {
			timeout := timeoutAt.Int64
			record.TimeoutAt = &timeout
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating timed out correlations: %w", err)
	}

	return records, nil
}

// InitializeMessageCorrelationTable creates the message correlation table
func (r *PostgresRepository) InitializeMessageCorrelationTable(ctx context.Context) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	_, err := r.db.ExecContext(ctx, createMessageCorrelationTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create message correlation table: %w", err)
	}

	return nil
}

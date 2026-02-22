// Package repository pkg/repository/postgres.go
package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const NULL = "null"

type PostgresRepository struct {
	db     *sql.DB
	config *Config
}

// PostgresConfig extends Config with PostgreSQL-specific options
type PostgresConfig struct {
	MaxOpenConns     int
	MaxIdleConns     int
	ConnMaxLifetime  time.Duration
	ConnMaxIdleTime  time.Duration
	EnableWAL        bool   // Write-Ahead Logging
	StatementTimeout string // e.g., "30s"
	SearchPath       string // Schema search path
}

// NewPostgresRepository creates a new PostgreSQL repository repository
func NewPostgresRepository(config *Config) (*PostgresRepository, error) {
	if config.ConnectionURL == "" {
		return nil, errors.New("connection URL is required for PostgreSQL repository")
	}

	db, err := sql.Open("postgres", config.ConnectionURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Parse PostgreSQL-specific configuration
	pgConfig := parsePostgresConfig(config.Options)

	// Configure connection pool
	db.SetMaxOpenConns(pgConfig.MaxOpenConns)
	db.SetMaxIdleConns(pgConfig.MaxIdleConns)
	db.SetConnMaxLifetime(pgConfig.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pgConfig.ConnMaxIdleTime)

	strategy := &PostgresRepository{
		db:     db,
		config: config,
	}

	return strategy, nil
}

// parsePostgresConfig extracts PostgreSQL-specific options
func parsePostgresConfig(options map[string]interface{}) *PostgresConfig {
	config := &PostgresConfig{
		MaxOpenConns:     25,
		MaxIdleConns:     5,
		ConnMaxLifetime:  5 * time.Minute,
		ConnMaxIdleTime:  5 * time.Minute,
		EnableWAL:        true,
		StatementTimeout: "30s",
		SearchPath:       "public",
	}

	if options == nil {
		return config
	}

	if v, ok := options["max_open_conns"].(int); ok {
		config.MaxOpenConns = v
	}
	if v, ok := options["max_idle_conns"].(int); ok {
		config.MaxIdleConns = v
	}
	if v, ok := options["conn_max_lifetime"].(time.Duration); ok {
		config.ConnMaxLifetime = v
	}
	if v, ok := options["conn_max_idle_time"].(time.Duration); ok {
		config.ConnMaxIdleTime = v
	}
	if v, ok := options["enable_wal"].(bool); ok {
		config.EnableWAL = v
	}
	if v, ok := options["statement_timeout"].(string); ok {
		config.StatementTimeout = v
	}
	if v, ok := options["search_path"].(string); ok {
		config.SearchPath = v
	}

	return config
}

// Initialize creates the necessary database schema with optimizations
func (ps *PostgresRepository) Initialize(ctx context.Context) error {
	// Set statement timeout for DDL operations
	pgConfig := parsePostgresConfig(ps.config.Options)

	_, err := ps.db.ExecContext(ctx, fmt.Sprintf("SET statement_timeout = '%s'", pgConfig.StatementTimeout))
	if err != nil {
		return fmt.Errorf("failed to set statement timeout: %w", err)
	}

	// Create search path if specified
	if pgConfig.SearchPath != "public" && pgConfig.SearchPath != "" {
		schemaQuery := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", pgConfig.SearchPath)
		if _, err := ps.db.ExecContext(ctx, schemaQuery); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}

		// Set search path
		setPathQuery := fmt.Sprintf("SET search_path TO %s", pgConfig.SearchPath)
		if _, err := ps.db.ExecContext(ctx, setPathQuery); err != nil {
			return fmt.Errorf("failed to set search path: %w", err)
		}
	}

	stateMachinesSchema := `
	CREATE TABLE IF NOT EXISTS state_machines (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		definition TEXT NOT NULL,
		type VARCHAR(50),
		version VARCHAR(50) NOT NULL,
		metadata JSONB DEFAULT '{}'::jsonb,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_state_machines_name ON state_machines(name);
	`

	historySchema := `
	CREATE TABLE IF NOT EXISTS state_history (
		id VARCHAR(255) NOT NULL,
		execution_id VARCHAR(255) NOT NULL,
		execution_start_time TIMESTAMP NOT NULL,
		state_name VARCHAR(255) NOT NULL,
		state_type VARCHAR(50) NOT NULL,
		input JSONB,
		output JSONB,
		status VARCHAR(50) NOT NULL CHECK (status IN ('SUCCEEDED', 'FAILED', 'RUNNING', 'CANCELLED', 'TIMED_OUT', 'RETRYING', 'WAITING')),
		start_time TIMESTAMP NOT NULL,
		end_time TIMESTAMP,
		error TEXT,
		retry_count INTEGER DEFAULT 0 CHECK (retry_count >= 0),
		sequence_number INTEGER NOT NULL CHECK (sequence_number >= 0),
		metadata JSONB DEFAULT '{}'::jsonb,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id, start_time),
		CONSTRAINT fk_execution FOREIGN KEY (execution_id, execution_start_time) 
			REFERENCES executions(execution_id, start_time) 
			ON DELETE CASCADE
			ON UPDATE CASCADE
	) PARTITION BY RANGE (start_time);

	-- Create default partition
	CREATE TABLE IF NOT EXISTS state_history_default PARTITION OF state_history DEFAULT;

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_state_history_execution ON state_history(execution_id, start_time);
	CREATE INDEX IF NOT EXISTS idx_state_history_sequence ON state_history(execution_id, sequence_number);
	CREATE INDEX IF NOT EXISTS idx_state_history_start_time ON state_history(start_time DESC);
	CREATE INDEX IF NOT EXISTS idx_state_history_state_name ON state_history(state_name);
	CREATE INDEX IF NOT EXISTS idx_state_history_status ON state_history(status) WHERE status IN ('FAILED', 'RETRYING');
	`
	// Create executions table with partitioning support
	executionsSchema := `
	CREATE TABLE IF NOT EXISTS executions (
		execution_id VARCHAR(255) NOT NULL,
		state_machine_id VARCHAR(255) NOT NULL,
		name VARCHAR(255) NOT NULL,
		input JSONB,
		output JSONB,
		status VARCHAR(50) NOT NULL CHECK (status IN ('RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'TIMED_OUT', 'ABORTED', 'PAUSED')),
		start_time TIMESTAMP NOT NULL,
		end_time TIMESTAMP,
		current_state VARCHAR(255) NOT NULL,
		error TEXT,
		metadata JSONB DEFAULT '{}'::jsonb,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (execution_id, start_time)
	) PARTITION BY RANGE (start_time);

	-- Create default partition for current month
	CREATE TABLE IF NOT EXISTS executions_default PARTITION OF executions DEFAULT;

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_executions_status ON executions(status) WHERE status IN ('RUNNING', 'FAILED');
	CREATE INDEX IF NOT EXISTS idx_executions_start_time ON executions(start_time DESC);
	CREATE INDEX IF NOT EXISTS idx_executions_state_machine ON executions(state_machine_id, start_time DESC);
	CREATE INDEX IF NOT EXISTS idx_executions_end_time ON executions(end_time DESC) WHERE end_time IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_executions_name ON executions(name) WHERE name IS NOT NULL;
	
	-- JSONB indexes for metadata queries
	CREATE INDEX IF NOT EXISTS idx_executions_metadata_gin ON executions USING GIN (metadata);
	`

	// Create state_history table with partitioning

	// Create function for automatic updated_at timestamp
	triggerFunction := `
	CREATE OR REPLACE FUNCTION update_updated_at_column()
	RETURNS TRIGGER AS $$
	BEGIN
		NEW.updated_at = CURRENT_TIMESTAMP;
		RETURN NEW;
	END;
	$$ language 'plpgsql';
	`

	// Create trigger for executions table
	executionsTrigger := `
	DROP TRIGGER IF EXISTS update_executions_updated_at ON executions;
	CREATE TRIGGER update_executions_updated_at
		BEFORE UPDATE ON executions
		FOR EACH ROW
		EXECUTE FUNCTION update_updated_at_column();
	`

	// Create materialized view for execution statistics
	statsView := `
	CREATE MATERIALIZED VIEW IF NOT EXISTS execution_statistics AS
	SELECT 
		state_machine_id,
		status,
		COUNT(*) as execution_count,
		AVG(EXTRACT(EPOCH FROM (end_time - start_time))) as avg_duration_seconds,
		MIN(start_time) as first_execution,
		MAX(start_time) as last_execution
	FROM executions
	WHERE end_time IS NOT NULL
	GROUP BY state_machine_id, status;

	CREATE UNIQUE INDEX IF NOT EXISTS idx_execution_stats_unique 
		ON execution_statistics(state_machine_id, status);
	`

	// Create function to refresh statistics
	refreshStatsFunction := `
	CREATE OR REPLACE FUNCTION refresh_execution_statistics()
	RETURNS void AS $$
	BEGIN
		REFRESH MATERIALIZED VIEW CONCURRENTLY execution_statistics;
	END;
	$$ LANGUAGE plpgsql;
	`

	// Create linked_executions table
	linkedExecutionsSchema := `
	CREATE TABLE IF NOT EXISTS linked_executions (
		id VARCHAR(255) PRIMARY KEY,
		source_state_machine_id VARCHAR(255) NOT NULL,
		source_execution_id VARCHAR(255) NOT NULL,
		source_state_name VARCHAR(255),
		input_transformer_name VARCHAR(255),
		target_state_machine_name VARCHAR(255) NOT NULL,
		target_execution_id VARCHAR(255) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_linked_source_sm ON linked_executions(source_state_machine_id);
	CREATE INDEX IF NOT EXISTS idx_linked_source_exec ON linked_executions(source_execution_id);
	CREATE INDEX IF NOT EXISTS idx_linked_target_exec ON linked_executions(target_execution_id);
	`

	// Execute all schema creation statements
	schemas := []string{
		stateMachinesSchema,
		executionsSchema,
		historySchema,
		triggerFunction,
		executionsTrigger,
		statsView,
		refreshStatsFunction,
		linkedExecutionsSchema,
	}

	for i, schema := range schemas {
		if _, err := ps.db.ExecContext(ctx, schema); err != nil {
			return fmt.Errorf("failed to execute schema statement %d: %w", i+1, err)
		}
	}

	// Migrate existing tables to include new statuses if they exist
	migrationQueries := []string{
		`DO $$ 
		BEGIN 
			BEGIN
				ALTER TABLE executions DROP CONSTRAINT IF EXISTS executions_status_check;
				ALTER TABLE executions ADD CONSTRAINT executions_status_check CHECK (status IN ('RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'TIMED_OUT', 'ABORTED', 'PAUSED'));
			EXCEPTION WHEN OTHERS THEN 
				-- Ignore errors if table doesn't exist yet (though it should after the above loop)
			END;
			BEGIN
				ALTER TABLE state_history DROP CONSTRAINT IF EXISTS state_history_status_check;
				ALTER TABLE state_history ADD CONSTRAINT state_history_status_check CHECK (status IN ('SUCCEEDED', 'FAILED', 'RUNNING', 'CANCELLED', 'TIMED_OUT', 'RETRYING', 'WAITING'));
			EXCEPTION WHEN OTHERS THEN 
				-- Ignore errors
			END;
		END $$;`,
	}

	for _, query := range migrationQueries {
		if _, err := ps.db.ExecContext(ctx, query); err != nil {
			fmt.Printf("Warning: failed to run migration query: %v\n", err)
		}
	}

	// Initialize message correlation table
	if err := ps.InitializeMessageCorrelationTable(ctx); err != nil {
		return fmt.Errorf("failed to initialize message correlation table: %w", err)
	}

	return nil
}

// Close closes the database connection gracefully
func (ps *PostgresRepository) Close() error {
	if ps.db != nil {
		return ps.db.Close()
	}
	return nil
}

// SaveStateMachine saves a state machine definition
func (ps *PostgresRepository) SaveStateMachine(ctx context.Context, record *StateMachineRecord) error {
	if record.ID == "" {
		return errors.New("state machine id is required")
	}

	metadataJSON, err := json.Marshal(record.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO state_machines (
			id, name, description, definition, type, version, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			definition = EXCLUDED.definition,
			type = EXCLUDED.type,
			version = EXCLUDED.version,
			metadata = EXCLUDED.metadata,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = ps.db.ExecContext(ctx, query,
		record.ID,
		record.Name,
		record.Description,
		record.Definition,
		record.Type,
		record.Version,
		metadataJSON,
		record.CreatedAt,
		record.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save state machine: %w", err)
	}

	return nil
}

// GetStateMachine retrieves a state machine by ID
func (ps *PostgresRepository) GetStateMachine(ctx context.Context, stateMachineID string) (*StateMachineRecord, error) {
	query := `
		SELECT id, name, description, definition, type, version, metadata, created_at, updated_at
		FROM state_machines
		WHERE id = $1
	`

	var record StateMachineRecord
	var metadataJSON []byte

	err := ps.db.QueryRowContext(ctx, query, stateMachineID).Scan(
		&record.ID,
		&record.Name,
		&record.Description,
		&record.Definition,
		&record.Type,
		&record.Version,
		&metadataJSON,
		&record.CreatedAt,
		&record.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("state machine '%s' not found", stateMachineID)
		}
		return nil, fmt.Errorf("failed to get state machine: %w", err)
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &record.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &record, nil
}

// ListStateMachines lists all state machines with filtering
func (ps *PostgresRepository) ListStateMachines(ctx context.Context, filter *DefinitionFilter) ([]*StateMachineRecord, error) {
	query := `
		SELECT id, name, description, definition, type, version, metadata, created_at, updated_at
		FROM state_machines
		WHERE 1=1
	`
	args := []interface{}{}
	argCount := 0

	// Apply filters
	if filter != nil {
		if filter.StateMachineID != "" {
			argCount++
			query += fmt.Sprintf(" AND id = $%d", argCount)
			args = append(args, filter.StateMachineID)
		}
		if filter.Name != "" {
			argCount++
			query += fmt.Sprintf(" AND name ILIKE $%d", argCount)
			args = append(args, "%"+filter.Name+"%")
		}
	}

	// Order by created_at descending
	query += " ORDER BY created_at DESC"

	rows, err := ps.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list state machines: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Printf("Warning: failed to close state machine rows: %v\n", err)
		}
	}(rows)

	var records []*StateMachineRecord
	for rows.Next() {
		var record StateMachineRecord
		var metadataJSON []byte

		err := rows.Scan(
			&record.ID,
			&record.Name,
			&record.Description,
			&record.Definition,
			&record.Type,
			&record.Version,
			&metadataJSON,
			&record.CreatedAt,
			&record.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan state machine record: %w", err)
		}

		if len(metadataJSON) > 0 && string(metadataJSON) != NULL {
			if err := json.Unmarshal(metadataJSON, &record.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating state machine rows: %w", err)
	}

	return records, nil
}

// SaveExecution saves or updates an execution record with UPSERT
func (ps *PostgresRepository) SaveExecution(ctx context.Context, record *ExecutionRecord) error {
	if record.ExecutionID == "" {
		return errors.New("execution_id is required")
	}

	// Marshal JSON fields
	inputJSON, outputJSON, metadataJSON, err := ps.marshalExecutionRecord(record)
	if err != nil {
		return fmt.Errorf("failed to marhshal the execution record: %w", err)
	}

	// Use INSERT ... ON CONFLICT for UPSERT behavior
	query := `
		INSERT INTO executions (
			execution_id, state_machine_id, name, input, output, status,
			start_time, end_time, current_state, error, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (execution_id, start_time) DO UPDATE SET
			output = EXCLUDED.output,
			status = EXCLUDED.status,
			end_time = EXCLUDED.end_time,
			current_state = EXCLUDED.current_state,
			error = EXCLUDED.error,
			metadata = EXCLUDED.metadata,
			updated_at = CURRENT_TIMESTAMP
		RETURNING execution_id, created_at, updated_at
	`

	var returnedID string
	var createdAt, updatedAt time.Time

	err = ps.db.QueryRowContext(ctx, query,
		record.ExecutionID,
		record.StateMachineID,
		record.Name,
		inputJSON,
		outputJSON,
		record.Status,
		record.StartTime,
		record.EndTime,
		record.CurrentState,
		record.Error,
		metadataJSON,
	).Scan(&returnedID, &createdAt, &updatedAt)

	if err != nil {
		return fmt.Errorf("failed to save execution: %w", err)
	}

	return nil
}

func (ps *PostgresRepository) CountExecutions(ctx context.Context, filter *ExecutionFilter) (int64, error) {
	var conditions []string
	var args []interface{}

	// Delegate filter building to a helper
	conditions, args = ps.buildExecutionFilters(filter, conditions, args)

	// Build final query
	query := "SELECT COUNT(*) FROM executions"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	var count int64
	// Execute query
	err := ps.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return -1, fmt.Errorf("failed to scan count: %w", err)
	}

	return count, nil
}

func (ps *PostgresRepository) marshalExecutionRecord(record *ExecutionRecord) (
	inputJSON, outputJSON, metadataJSON []byte,
	err error,
) {
	inputJSON, err = json.Marshal(record.Input)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	outputJSON, err = json.Marshal(record.Output)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal output: %w", err)
	}

	metadataJSON = []byte("{}")
	if record.Metadata != nil {
		metadataJSON, err = json.Marshal(record.Metadata)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	return inputJSON, outputJSON, metadataJSON, nil
}

// GetExecution retrieves an execution by ID with proper error handling
func (ps *PostgresRepository) GetExecution(ctx context.Context, executionID string) (*ExecutionRecord, error) {
	if executionID == "" {
		return nil, errors.New("execution_id is required")
	}

	query := `
		SELECT 
			execution_id, state_machine_id, name, input, output, status,
			start_time, end_time, current_state, error, metadata
		FROM executions
		WHERE execution_id = $1
		ORDER BY start_time DESC
		LIMIT 1
	`

	row := ps.db.QueryRowContext(ctx, query, executionID)

	record := &ExecutionRecord{}
	var inputJSON, outputJSON, metadataJSON []byte
	var endTime sql.NullTime
	var errorStr sql.NullString

	err := row.Scan(
		&record.ExecutionID,
		&record.StateMachineID,
		&record.Name,
		&inputJSON,
		&outputJSON,
		&record.Status,
		&record.StartTime,
		&endTime,
		&record.CurrentState,
		&errorStr,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get execution: %w", err)
	}

	// Unmarshal JSON fields
	executionRecord, err2 := processJson(inputJSON, record, outputJSON, metadataJSON, endTime, errorStr)
	if err2 != nil {
		return executionRecord, err2
	}

	return record, nil
}

func processJson(inputJSON []byte, record *ExecutionRecord, outputJSON, metadataJSON []byte, endTime sql.NullTime, errorStr sql.NullString) (*ExecutionRecord, error) {
	if len(inputJSON) > 0 && string(inputJSON) != NULL {
		if err := json.Unmarshal(inputJSON, &record.Input); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input: %w", err)
		}
	}

	if len(outputJSON) > 0 && string(outputJSON) != NULL {
		if err := json.Unmarshal(outputJSON, &record.Output); err != nil {
			return nil, fmt.Errorf("failed to unmarshal output: %w", err)
		}
	}

	if len(metadataJSON) > 0 && string(metadataJSON) != NULL && string(metadataJSON) != "{}" {
		if err := json.Unmarshal(metadataJSON, &record.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	if endTime.Valid {
		record.EndTime = &endTime.Time
	}

	if errorStr.Valid {
		record.Error = errorStr.String
	}
	return nil, nil
}

// SaveStateHistory saves a state execution to history
func (ps *PostgresRepository) SaveStateHistory(ctx context.Context, record *StateHistoryRecord) error {
	if record.ExecutionID == "" {
		return errors.New("execution_id is required")
	}
	if record.ID == "" {
		return errors.New("history id is required")
	}

	// Marshal JSON fields
	inputJSON, err := json.Marshal(record.Input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	outputJSON, err := json.Marshal(record.Output)
	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}

	metadataJSON := []byte("{}")
	if record.Metadata != nil {
		metadataJSON, err = json.Marshal(record.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO state_history (
			id, execution_id, execution_start_time, state_name, state_type, input, output,
			status, start_time, end_time, error, retry_count, sequence_number, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id, start_time) DO UPDATE SET
			output = EXCLUDED.output,
			status = EXCLUDED.status,
			end_time = EXCLUDED.end_time,
			error = EXCLUDED.error,
			retry_count = EXCLUDED.retry_count
	`

	_, err = ps.db.ExecContext(ctx, query,
		record.ID,
		record.ExecutionID,
		record.ExecutionStartTime,
		record.StateName,
		record.StateType,
		inputJSON,
		outputJSON,
		record.Status,
		record.StartTime,
		record.EndTime,
		record.Error,
		record.RetryCount,
		record.SequenceNumber,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to save state history: %w", err)
	}

	return nil
}

// GetStateHistory retrieves all state history for an execution ordered by sequence
func (ps *PostgresRepository) GetStateHistory(ctx context.Context, executionID string) ([]*StateHistoryRecord, error) {
	if executionID == "" {
		return nil, errors.New("execution_id is required")
	}

	query := `
		SELECT 
			id, execution_id, state_name, state_type, input, output,
			status, start_time, end_time, error, retry_count, sequence_number, metadata
		FROM state_history
		WHERE execution_id = $1
		ORDER BY sequence_number ASC, start_time ASC
	`

	rows, err := ps.db.QueryContext(ctx, query, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query state history: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println("Failed to close state history rows")
		}
	}(rows)

	var history []*StateHistoryRecord

	for rows.Next() {
		record := &StateHistoryRecord{}
		var inputJSON, outputJSON, metadataJSON []byte
		var endTime sql.NullTime
		var errorStr sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.ExecutionID,
			&record.StateName,
			&record.StateType,
			&inputJSON,
			&outputJSON,
			&record.Status,
			&record.StartTime,
			&endTime,
			&errorStr,
			&record.RetryCount,
			&record.SequenceNumber,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan state history: %w", err)
		}

		// Delegate JSON and null handling
		if err := record.unmarshalFields(inputJSON, outputJSON, metadataJSON); err != nil {
			return nil, err
		}

		record.setNullableFields(endTime, errorStr)

		history = append(history, record)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating state history: %w", err)
	}

	return history, nil
}

// unmarshalFields handles unmarshaling JSON fields safely
func (r *StateHistoryRecord) unmarshalFields(input, output, metadata []byte) error {
	if len(input) > 0 && string(input) != NULL {
		if err := json.Unmarshal(input, &r.Input); err != nil {
			return fmt.Errorf("failed to unmarshal input: %w", err)
		}
	}
	if len(output) > 0 && string(output) != NULL {
		if err := json.Unmarshal(output, &r.Output); err != nil {
			return fmt.Errorf("failed to unmarshal output: %w", err)
		}
	}
	if len(metadata) > 0 && string(metadata) != NULL && string(metadata) != "{}" {
		if err := json.Unmarshal(metadata, &r.Metadata); err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}
	return nil
}

// setNullableFields handles nullable SQL types
func (r *StateHistoryRecord) setNullableFields(endTime sql.NullTime, errorStr sql.NullString) {
	if endTime.Valid {
		r.EndTime = &endTime.Time
	}
	if errorStr.Valid {
		r.Error = errorStr.String
	}
}

func (ps *PostgresRepository) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error) {
	query, args := ps.buildListExecutionsQuery(filter)

	rows, err := ps.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println("Failed to close executions rows")
		}
	}(rows) // Safe: log in production if needed, but defer is fine

	var results []*ExecutionRecord
	for rows.Next() {
		record, err := ps.scanExecutionRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, record)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating executions: %w", err)
	}

	return results, nil
}

// ListExecutionIDs returns only execution IDs matching the filter (more efficient than ListExecutions)
func (ps *PostgresRepository) ListExecutionIDs(ctx context.Context, filter *ExecutionFilter) ([]string, error) {
	query, args := ps.buildListExecutionIDsQuery(filter)

	rows, err := ps.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query execution IDs: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println("Failed to close execution IDs rows")
		}
	}(rows)

	var results []string
	for rows.Next() {
		var executionID string
		if err := rows.Scan(&executionID); err != nil {
			return nil, fmt.Errorf("failed to scan execution ID: %w", err)
		}
		results = append(results, executionID)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating execution IDs: %w", err)
	}

	return results, nil
}

func (ps *PostgresRepository) buildListExecutionIDsQuery(filter *ExecutionFilter) (query string, args []interface{}) {
	baseQuery := `
		SELECT DISTINCT execution_id
		FROM executions
	`

	var conditions []string
	args = []interface{}{}

	if filter != nil {
		conditions, args = ps.buildExecutionFilters(filter, conditions, args)
	}

	query = baseQuery
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY execution_id"

	// Handle pagination
	if filter != nil {
		if filter.Limit > 0 {
			query += fmt.Sprintf(" LIMIT $%d", len(args)+1)
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", len(args)+1)
			args = append(args, filter.Offset)
		}
	}

	return query, args
}

func (ps *PostgresRepository) buildListExecutionsQuery(filter *ExecutionFilter) (query string, args []interface{}) {
	baseQuery := `
		SELECT DISTINCT ON (execution_id)
			execution_id, state_machine_id, name, input, output, status,
			start_time, end_time, current_state, error, metadata
		FROM executions
	`

	var conditions []string
	args = []interface{}{} // initialize args since it's a named return

	if filter != nil {
		conditions, args = ps.buildExecutionFilters(filter, conditions, args)
	}

	query = baseQuery
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY execution_id, start_time DESC"

	// Handle pagination
	if filter != nil {
		if filter.Limit > 0 {
			query += fmt.Sprintf(" LIMIT $%d", len(args)+1)
			args = append(args, filter.Limit)
		}
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", len(args)+1)
			args = append(args, filter.Offset)
		}
	}

	return query, args
}

func (ps *PostgresRepository) scanExecutionRow(rows *sql.Rows) (*ExecutionRecord, error) {
	record := &ExecutionRecord{}
	var inputJSON, outputJSON, metadataJSON []byte
	var endTime sql.NullTime
	var errorStr sql.NullString

	err := rows.Scan(
		&record.ExecutionID,
		&record.StateMachineID,
		&record.Name,
		&inputJSON,
		&outputJSON,
		&record.Status,
		&record.StartTime,
		&endTime,
		&record.CurrentState,
		&errorStr,
		&metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan execution: %w", err)
	}

	if err := ps.unmarshalJSONField(inputJSON, &record.Input); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}
	if err := ps.unmarshalJSONField(outputJSON, &record.Output); err != nil {
		return nil, fmt.Errorf("unmarshal output: %w", err)
	}
	if err := ps.unmarshalJSONField(metadataJSON, &record.Metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	if endTime.Valid {
		record.EndTime = &endTime.Time
	}
	if errorStr.Valid {
		record.Error = errorStr.String
	}

	return record, nil
}

// Helper to safely unmarshal JSON, treating empty or "null" as skip
func (ps *PostgresRepository) unmarshalJSONField(data []byte, target interface{}) error {
	if len(data) == 0 {
		return nil
	}
	asStr := string(data)
	if asStr == "null" || asStr == "{}" {
		return nil
	}
	return json.Unmarshal(data, target)
}

// buildExecutionFilters constructs WHERE conditions and args from filters
func (ps *PostgresRepository) buildExecutionFilters(
	filter *ExecutionFilter,
	conditions []string,
	args []interface{},
) (newConditions []string, newArgs []interface{}) {
	if filter == nil {
		return conditions, args
	}

	argCount := len(args) + 1

	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argCount))
		args = append(args, filter.Status)
		argCount++
	}

	if filter.StateMachineID != "" {
		conditions = append(conditions, fmt.Sprintf("state_machine_id = $%d", argCount))
		args = append(args, filter.StateMachineID)
		argCount++
	}

	if filter.Name != "" {
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", argCount))
		args = append(args, "%"+filter.Name+"%")
		argCount++
	}

	if !filter.StartAfter.IsZero() {
		conditions = append(conditions, fmt.Sprintf("start_time >= $%d", argCount))
		args = append(args, filter.StartAfter)
	}

	if !filter.StartBefore.IsZero() {
		conditions = append(conditions, fmt.Sprintf("start_before <= $%d", argCount))
		args = append(args, filter.StartBefore)
		// argCount++
	}

	// need to solve this
	//  if filter.e {
	//	if metadataJSON, err := json.Marshal(metadataFilter); err == nil {
	//		conditions = append(conditions, fmt.Sprintf("metadata @> $%d::jsonb", argCount))
	//		args = append(args, string(metadataJSON))
	//	}
	//	}

	return conditions, args
}

// DeleteExecution removes an execution and its history (cascade)
func (ps *PostgresRepository) DeleteExecution(ctx context.Context, executionID string) error {
	if executionID == "" {
		return errors.New("execution_id is required")
	}

	query := `DELETE FROM executions WHERE execution_id = $1`

	result, err := ps.db.ExecContext(ctx, query, executionID)
	if err != nil {
		return fmt.Errorf("failed to delete execution: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("execution not found: %s", executionID)
	}

	return nil
}

// HealthCheck verifies the database connection is alive and responsive
func (ps *PostgresRepository) HealthCheck(ctx context.Context) error {
	// Simple ping
	if err := ps.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Check if we can query
	var result int
	err := ps.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("database query failed: %w", err)
	}

	if result != 1 {
		return errors.New("database health check returned unexpected result")
	}

	return nil
}

// Additional utility methods

// GetExecutionStats returns statistics for a state machine
func (ps *PostgresRepository) GetExecutionStats(ctx context.Context, stateMachineID string) (map[string]interface{}, error) {
	query := `
		SELECT 
			status,
			COUNT(*) as count,
			AVG(EXTRACT(EPOCH FROM (end_time - start_time))) as avg_duration_seconds,
			MIN(start_time) as first_execution,
			MAX(start_time) as last_execution
		FROM executions
		WHERE state_machine_id = $1
		AND end_time IS NOT NULL
		GROUP BY status
	`

	rows, err := ps.db.QueryContext(ctx, query, stateMachineID)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution stats: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			fmt.Println("Failed to close execution stats rows")
		}
	}(rows)

	stats := make(map[string]interface{})
	stats["by_status"] = make(map[string]interface{})

	for rows.Next() {
		var status string
		var count int
		var avgDuration sql.NullFloat64
		var firstExec, lastExec time.Time

		if err := rows.Scan(&status, &count, &avgDuration, &firstExec, &lastExec); err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}

		statusStats := map[string]interface{}{
			"count":           count,
			"first_execution": firstExec,
			"last_execution":  lastExec,
		}

		if avgDuration.Valid {
			statusStats["avg_duration_seconds"] = avgDuration.Float64
		}

		stats["by_status"].(map[string]interface{})[status] = statusStats
	}

	return stats, nil
}

// CreatePartition creates a new partition for a time range
func (ps *PostgresRepository) CreatePartition(ctx context.Context, tableName string, startTime, endTime time.Time) error {
	partitionName := fmt.Sprintf("%s_%s", tableName, startTime.Format("200601"))

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF %s
		FOR VALUES FROM ('%s') TO ('%s')
	`, partitionName, tableName,
		startTime.Format("2006-01-02"),
		endTime.Format("2006-01-02"))

	_, err := ps.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create partition: %w", err)
	}

	return nil
}

// ArchiveOldExecutions moves old executions to an archive table
func (ps *PostgresRepository) ArchiveOldExecutions(ctx context.Context, olderThan time.Time) (int64, error) {
	// Create an archive table if not exists
	archiveSchema := `
	CREATE TABLE IF NOT EXISTS executions_archive (LIKE executions INCLUDING ALL);
	CREATE TABLE IF NOT EXISTS state_history_archive (LIKE state_history INCLUDING ALL);
	`

	if _, err := ps.db.ExecContext(ctx, archiveSchema); err != nil {
		return 0, fmt.Errorf("failed to create archive tables: %w", err)
	}

	// Begin transaction
	tx, err := ps.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func(tx *sql.Tx) {
		err := tx.Rollback()
		if err != nil {
			fmt.Printf("failed to rollback transaction: %v\n", err)
		}
	}(tx)

	// Move executions to archive
	moveExecQuery := `
		WITH moved AS (
			DELETE FROM executions
			WHERE start_time < $1
			RETURNING *
		)
		INSERT INTO executions_archive SELECT * FROM moved
	`

	result, err := tx.ExecContext(ctx, moveExecQuery, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to archive executions: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()

	// Move state history to archive
	moveHistoryQuery := `
		WITH moved AS (
			DELETE FROM state_history
			WHERE start_time < $1
			RETURNING *
		)
		INSERT INTO state_history_archive SELECT * FROM moved
	`

	if _, err := tx.ExecContext(ctx, moveHistoryQuery, olderThan); err != nil {
		return 0, fmt.Errorf("failed to archive state history: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit archive transaction: %w", err)
	}

	return rowsAffected, nil
}

// VacuumTables runs VACUUM ANALYZE on the main tables for maintenance
func (ps *PostgresRepository) VacuumTables(ctx context.Context) error {
	tables := []string{"executions", "state_history"}

	for _, table := range tables {
		query := fmt.Sprintf("VACUUM ANALYZE %s", table)
		if _, err := ps.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to vacuum table %s: %w", table, err)
		}
	}

	return nil
}

// RefreshStatistics refreshes the materialized view for statistics
func (ps *PostgresRepository) RefreshStatistics(ctx context.Context) error {
	_, err := ps.db.ExecContext(ctx, "SELECT refresh_execution_statistics()")
	if err != nil {
		return fmt.Errorf("failed to refresh statistics: %w", err)
	}
	return nil
}

// GetExecutionOutput retrieves output from an execution (final or specific state)
// If stateName is empty, returns the final execution output
// If stateName is provided, returns the output of that specific state
func (ps *PostgresRepository) GetExecutionOutput(ctx context.Context, executionID, stateName string) (interface{}, error) {
	if stateName == "" {
		// Get final execution output
		query := `SELECT output FROM executions WHERE execution_id = $1`
		var outputJSON []byte
		err := ps.db.QueryRowContext(ctx, query, executionID).Scan(&outputJSON)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("execution '%s' not found", executionID)
			}
			return nil, fmt.Errorf("failed to get execution output: %w", err)
		}

		if len(outputJSON) == 0 || string(outputJSON) == NULL {
			return nil, nil
		}

		var output interface{}
		if err := json.Unmarshal(outputJSON, &output); err != nil {
			return nil, fmt.Errorf("failed to unmarshal execution output: %w", err)
		}
		return output, nil
	}

	// Get specific state output (most recent if state was executed multiple times)
	query := `
		SELECT output FROM state_history
		WHERE execution_id = $1 AND state_name = $2
		ORDER BY sequence_number DESC
		LIMIT 1
	`
	var outputJSON []byte
	err := ps.db.QueryRowContext(ctx, query, executionID, stateName).Scan(&outputJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("state '%s' not found in execution '%s'", stateName, executionID)
		}
		return nil, fmt.Errorf("failed to get state output: %w", err)
	}

	if len(outputJSON) == 0 || string(outputJSON) == NULL {
		return nil, nil
	}

	var output interface{}
	if err := json.Unmarshal(outputJSON, &output); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state output: %w", err)
	}
	return output, nil
}

// SaveLinkedExecution saves a linked execution record
func (ps *PostgresRepository) SaveLinkedExecution(ctx context.Context, linkedExec *LinkedExecutionRecord) error {
	if linkedExec.ID == "" {
		return errors.New("linked execution id is required")
	}
	if linkedExec.SourceExecutionID == "" {
		return errors.New("source execution id is required")
	}
	if linkedExec.TargetExecutionID == "" {
		return errors.New("target execution id is required")
	}

	query := `
		INSERT INTO linked_executions (
			id, source_state_machine_id, source_execution_id, source_state_name,
			input_transformer_name, target_state_machine_name, target_execution_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO NOTHING
	`

	_, err := ps.db.ExecContext(ctx, query,
		linkedExec.ID,
		linkedExec.SourceStateMachineID,
		linkedExec.SourceExecutionID,
		linkedExec.SourceStateName,
		linkedExec.InputTransformerName,
		linkedExec.TargetStateMachineName,
		linkedExec.TargetExecutionID,
	)

	if err != nil {
		return fmt.Errorf("failed to save linked execution: %w", err)
	}

	return nil
}

// ListLinkedExecutions lists linked executions with filtering and pagination
func (ps *PostgresRepository) ListLinkedExecutions(ctx context.Context, filter *LinkedExecutionFilter) ([]*LinkedExecutionRecord, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	// Build WHERE clause
	if filter != nil {
		if filter.SourceStateMachineID != "" {
			conditions = append(conditions, fmt.Sprintf("source_state_machine_id = $%d", argPos))
			args = append(args, filter.SourceStateMachineID)
			argPos++
		}
		if filter.SourceExecutionID != "" {
			conditions = append(conditions, fmt.Sprintf("source_execution_id = $%d", argPos))
			args = append(args, filter.SourceExecutionID)
			argPos++
		}
		if filter.SourceStateName != "" {
			conditions = append(conditions, fmt.Sprintf("source_state_name = $%d", argPos))
			args = append(args, filter.SourceStateName)
			argPos++
		}
		if filter.TargetStateMachineName != "" {
			conditions = append(conditions, fmt.Sprintf("target_state_machine_name = $%d", argPos))
			args = append(args, filter.TargetStateMachineName)
			argPos++
		}
		if filter.TargetExecutionID != "" {
			conditions = append(conditions, fmt.Sprintf("target_execution_id = $%d", argPos))
			args = append(args, filter.TargetExecutionID)
			argPos++
		}
		if !filter.CreatedAfter.IsZero() {
			conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argPos))
			args = append(args, filter.CreatedAfter)
			argPos++
		}
		if !filter.CreatedBefore.IsZero() {
			conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argPos))
			args = append(args, filter.CreatedBefore)
			argPos++
		}
	}

	// Build query
	query := `
		SELECT id, source_state_machine_id, source_execution_id, source_state_name,
		       input_transformer_name, target_state_machine_name, target_execution_id, created_at
		FROM linked_executions
	`

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY created_at DESC"

	// Apply pagination
	if filter != nil {
		if filter.Limit > 0 {
			query += fmt.Sprintf(" LIMIT $%d", argPos)
			args = append(args, filter.Limit)
			argPos++
		}
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argPos)
			args = append(args, filter.Offset)
		}
	}

	rows, err := ps.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list linked executions: %w", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Printf("Failed to close linked executions rows: %v\n", err)
		}
	}(rows)

	var records []*LinkedExecutionRecord
	for rows.Next() {
		record := &LinkedExecutionRecord{}
		err := rows.Scan(
			&record.ID,
			&record.SourceStateMachineID,
			&record.SourceExecutionID,
			&record.SourceStateName,
			&record.InputTransformerName,
			&record.TargetStateMachineName,
			&record.TargetExecutionID,
			&record.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan linked execution: %w", err)
		}
		records = append(records, record)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating linked executions: %w", err)
	}

	return records, nil
}

// CountLinkedExecutions returns the count of linked executions matching the filter
func (ps *PostgresRepository) CountLinkedExecutions(ctx context.Context, filter *LinkedExecutionFilter) (int64, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	// Build WHERE clause
	if filter != nil {
		if filter.SourceStateMachineID != "" {
			conditions = append(conditions, fmt.Sprintf("source_state_machine_id = $%d", argPos))
			args = append(args, filter.SourceStateMachineID)
			argPos++
		}
		if filter.SourceExecutionID != "" {
			conditions = append(conditions, fmt.Sprintf("source_execution_id = $%d", argPos))
			args = append(args, filter.SourceExecutionID)
			argPos++
		}
		if filter.SourceStateName != "" {
			conditions = append(conditions, fmt.Sprintf("source_state_name = $%d", argPos))
			args = append(args, filter.SourceStateName)
			argPos++
		}
		if filter.TargetStateMachineName != "" {
			conditions = append(conditions, fmt.Sprintf("target_state_machine_name = $%d", argPos))
			args = append(args, filter.TargetStateMachineName)
			argPos++
		}
		if filter.TargetExecutionID != "" {
			conditions = append(conditions, fmt.Sprintf("target_execution_id = $%d", argPos))
			args = append(args, filter.TargetExecutionID)
			argPos++
		}
		if !filter.CreatedAfter.IsZero() {
			conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argPos))
			args = append(args, filter.CreatedAfter)
			argPos++
		}
		if !filter.CreatedBefore.IsZero() {
			conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argPos))
			args = append(args, filter.CreatedBefore)
		}
	}

	// Build query
	query := "SELECT COUNT(*) FROM linked_executions"

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	var count int64
	err := ps.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count linked executions: %w", err)
	}

	return count, nil
}

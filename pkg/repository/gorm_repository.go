// pkg/repository/gorm_repository.go
package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GormStrategy implements Repository using GORM
type GormStrategy struct {
	db     *gorm.DB
	config *Config
}

// NewGormStrategy creates a new GORM-based repository
func NewGormStrategy(config *Config) (*GormStrategy, error) {
	if config.ConnectionURL == "" {
		return nil, fmt.Errorf("connection URL is required")
	}

	// Parse GORM-specific configuration
	gormCfg := parseGormConfig(config.Options)

	// Open database connection with GORM config
	db, err := gorm.Open(postgres.Open(config.ConnectionURL), &gorm.Config{
		Logger:                 gormCfg.Logger,
		SkipDefaultTransaction: true, // Better performance
		PrepareStmt:            true, // Prepared statement cache
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get generic database object for connection pool settings
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(gormCfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(gormCfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(gormCfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(gormCfg.ConnMaxIdleTime)

	return &GormStrategy{
		db:     db,
		config: config,
	}, nil
}

// GormConfig holds GORM-specific configuration
type GormConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	Logger          logger.Interface
	LogLevel        logger.LogLevel
}

// parseGormConfig extracts GORM-specific options from config
func parseGormConfig(options map[string]interface{}) *GormConfig {
	cfg := &GormConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
		LogLevel:        logger.Warn,
	}

	if options == nil {
		return cfg
	}

	if v, ok := options["max_open_conns"].(int); ok {
		cfg.MaxOpenConns = v
	}
	if v, ok := options["max_idle_conns"].(int); ok {
		cfg.MaxIdleConns = v
	}
	if v, ok := options["conn_max_lifetime"].(time.Duration); ok {
		cfg.ConnMaxLifetime = v
	}
	if v, ok := options["conn_max_idle_time"].(time.Duration); ok {
		cfg.ConnMaxIdleTime = v
	}
	if v, ok := options["log_level"].(string); ok {
		switch v {
		case "silent":
			cfg.LogLevel = logger.Silent
		case "error":
			cfg.LogLevel = logger.Error
		case "warn":
			cfg.LogLevel = logger.Warn
		case "info":
			cfg.LogLevel = logger.Info
		}
	}

	// Create logger with configured level
	cfg.Logger = logger.Default.LogMode(cfg.LogLevel)

	return cfg
}

// Initialize creates tables and indexes using GORM AutoMigrate
func (r *GormStrategy) Initialize(ctx context.Context) error {
	// Migrate tables in correct order (parent tables first)

	// Step 1: Migrate executions table first (no dependencies)
	err := r.db.WithContext(ctx).AutoMigrate(&ExecutionModel{})
	if err != nil {
		return fmt.Errorf("failed to migrate executions table: %w", err)
	}

	// Step 2: Migrate state_history table (depends on executions)
	err = r.db.WithContext(ctx).AutoMigrate(&StateHistoryModel{})
	if err != nil {
		return fmt.Errorf("failed to migrate state_history table: %w", err)
	}

	// Step 3: Add foreign key constraint manually after both tables exist
	if err := r.addForeignKeyConstraints(ctx); err != nil {
		// Log warning but don't fail if constraint already exists
		// This is safe because constraint might already exist from previous run
		fmt.Printf("Warning: could not add foreign key constraints: %v\n", err)
	}

	// Step 4: Migrate statistics table (independent)
	err = r.db.WithContext(ctx).AutoMigrate(&ExecutionStatisticsModel{})
	if err != nil {
		return fmt.Errorf("failed to migrate statistics table: %w", err)
	}

	// Step 5: Create additional indexes for better performance
	if err := r.createAdditionalIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create additional indexes: %w", err)
	}

	return nil
}

// addForeignKeyConstraints adds foreign key constraints after tables are created
func (r *GormStrategy) addForeignKeyConstraints(ctx context.Context) error {
	// Add foreign key from state_history to executions
	constraint_state_history := `
		ALTER TABLE state_history 
		DROP CONSTRAINT IF EXISTS fk_state_history_execution;
	`
	constraint_executions := `
			ALTER TABLE state_history 
		ADD CONSTRAINT fk_state_history_execution 
		FOREIGN KEY (execution_id) 
		REFERENCES executions(execution_id) 
		ON DELETE CASCADE;
	`

	if err := r.db.WithContext(ctx).Exec(constraint_state_history).Error; err != nil {
		return fmt.Errorf("failed to add foreign key constraint: %w", err)
	}
	if err := r.db.WithContext(ctx).Exec(constraint_executions).Error; err != nil {
		return fmt.Errorf("failed to add foreign key constraint: %w", err)
	}

	return nil
}

// createAdditionalIndexes creates composite and specialized indexes
func (r *GormStrategy) createAdditionalIndexes(ctx context.Context) error {
	indexes := []string{
		// Composite index for common query patterns
		`CREATE INDEX IF NOT EXISTS idx_executions_sm_status_time 
		 ON executions(state_machine_id, status, start_time DESC)`,

		// Partial index for active executions only
		`CREATE INDEX IF NOT EXISTS idx_executions_running 
		 ON executions(state_machine_id, start_time DESC) 
		 WHERE status = 'RUNNING'`,

		// Index for execution-history joins
		`CREATE INDEX IF NOT EXISTS idx_state_history_exec_seq 
		 ON state_history(execution_id, sequence_number ASC)`,

		// GIN indexes for JSONB searches
		`CREATE INDEX IF NOT EXISTS idx_executions_metadata_gin 
		 ON executions USING GIN (metadata)`,

		`CREATE INDEX IF NOT EXISTS idx_state_history_metadata_gin 
		 ON state_history USING GIN (metadata)`,
	}

	for _, indexSQL := range indexes {
		if err := r.db.WithContext(ctx).Exec(indexSQL).Error; err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// Close closes the database connection
func (r *GormStrategy) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// SaveExecution saves or updates an execution using GORM
func (r *GormStrategy) SaveExecution(ctx context.Context, exec *ExecutionRecord) error {
	model := toExecutionModel(exec)

	// GORM's Save handles both INSERT and UPDATE
	result := r.db.WithContext(ctx).Save(model)
	if result.Error != nil {
		return fmt.Errorf("failed to save execution: %w", result.Error)
	}

	return nil
}

// GetExecution retrieves an execution by ID
func (r *GormStrategy) GetExecution(ctx context.Context, executionID string) (*ExecutionRecord, error) {
	var model ExecutionModel

	result := r.db.WithContext(ctx).
		Where("execution_id = ?", executionID).
		First(&model)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("execution not found: %s", executionID)
		}
		return nil, fmt.Errorf("failed to get execution: %w", result.Error)
	}

	return fromExecutionModel(&model), nil
}

// SaveStateHistory saves a state history entry
func (r *GormStrategy) SaveStateHistory(ctx context.Context, history *StateHistoryRecord) error {
	model := toStateHistoryModel(history)

	result := r.db.WithContext(ctx).Save(model)
	if result.Error != nil {
		return fmt.Errorf("failed to save state history: %w", result.Error)
	}

	return nil
}

// GetStateHistory retrieves all state history for an execution
func (r *GormStrategy) GetStateHistory(ctx context.Context, executionID string) ([]*StateHistoryRecord, error) {
	var models []StateHistoryModel

	result := r.db.WithContext(ctx).
		Where("execution_id = ?", executionID).
		Order("sequence_number ASC, start_time ASC").
		Find(&models)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get state history: %w", result.Error)
	}

	histories := make([]*StateHistoryRecord, len(models))
	for i := range models {
		model := models[i]
		histories[i] = fromStateHistoryModel(&model)
	}

	return histories, nil
}

// ListExecutions lists executions with filtering and pagination
func (r *GormStrategy) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error) {
	query := r.db.WithContext(ctx).Model(&ExecutionModel{})

	// Apply filters
	if filter != nil {
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}

		if filter.StateMachineID != "" {
			query = query.Where("state_machine_id = ?", filter.StateMachineID)
		}

		if filter.Name != "" {
			query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
		}

		if !filter.StartAfter.IsZero() {
			query = query.Where("start_time >= ?", filter.StartAfter)
		}

		if !filter.StartBefore.IsZero() {
			query = query.Where("start_time <= ?", filter.StartBefore)
		}

		// Apply pagination
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	// Always order by start time descending
	query = query.Order("start_time DESC")

	var models []ExecutionModel
	result := query.Find(&models)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list executions: %w", result.Error)
	}

	executions := make([]*ExecutionRecord, len(models))
	for i := range models {
		model := models[i]
		executions[i] = fromExecutionModel(&model)
	}

	return executions, nil
}

// CountExecutions returns the count of executions matching the filter
func (r *GormStrategy) CountExecutions(ctx context.Context, filter *ExecutionFilter) (int64, error) {
	query := r.db.WithContext(ctx).Model(&ExecutionModel{})

	// Apply same filters as ListExecutions (without pagination)
	if filter != nil {
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.StateMachineID != "" {
			query = query.Where("state_machine_id = ?", filter.StateMachineID)
		}
		if filter.Name != "" {
			query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
		}
		if !filter.StartAfter.IsZero() {
			query = query.Where("start_time >= ?", filter.StartAfter)
		}
		if !filter.StartBefore.IsZero() {
			query = query.Where("start_time <= ?", filter.StartBefore)
		}
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count executions: %w", err)
	}

	return count, nil
}

// DeleteExecution removes an execution and its history (cascade handled by FK)
func (r *GormStrategy) DeleteExecution(ctx context.Context, executionID string) error {
	result := r.db.WithContext(ctx).
		Where("execution_id = ?", executionID).
		Delete(&ExecutionModel{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete execution: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("execution not found: %s", executionID)
	}

	return nil
}

// GetExecutionWithHistory retrieves an execution with its full state history using eager loading
func (r *GormStrategy) GetExecutionWithHistory(ctx context.Context, executionID string) (*ExecutionRecord, []*StateHistoryRecord, error) {
	// Get execution
	var execModel ExecutionModel
	result := r.db.WithContext(ctx).
		Where("execution_id = ?", executionID).
		First(&execModel)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil, fmt.Errorf("execution not found: %s", executionID)
		}
		return nil, nil, fmt.Errorf("failed to get execution: %w", result.Error)
	}

	// Get state history separately
	var historyModels []StateHistoryModel
	result = r.db.WithContext(ctx).
		Where("execution_id = ?", executionID).
		Order("sequence_number ASC").
		Find(&historyModels)

	if result.Error != nil {
		return nil, nil, fmt.Errorf("failed to get state history: %w", result.Error)
	}

	execution := fromExecutionModel(&execModel)

	histories := make([]*StateHistoryRecord, len(historyModels))
	for i := range historyModels {
		h := historyModels[i]
		histories[i] = fromStateHistoryModel(&h)
	}

	return execution, histories, nil
}

// GetStatistics returns aggregated statistics for a state machine
func (r *GormStrategy) GetStatistics(ctx context.Context, stateMachineID string) (*Statistics, error) {
	type StatsResult struct {
		Status             string
		Count              int64
		AvgDurationSeconds float64
		P50Duration        float64
		P95Duration        float64
		P99Duration        float64
		FirstExecution     time.Time
		LastExecution      time.Time
	}

	var results []StatsResult

	err := r.db.WithContext(ctx).Raw(`
		SELECT 
			status,
			COUNT(*) as count,
			AVG(EXTRACT(EPOCH FROM (end_time - start_time))) as avg_duration_seconds,
			PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))) as p50_duration,
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))) as p95_duration,
			PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))) as p99_duration,
			MIN(start_time) as first_execution,
			MAX(start_time) as last_execution
		FROM executions
		WHERE state_machine_id = ?
		AND end_time IS NOT NULL
		GROUP BY status
	`, stateMachineID).Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	stats := &Statistics{
		StateMachineID: stateMachineID,
		ByStatus:       make(map[string]*StatusStatistics),
	}

	for _, result := range results {
		stats.ByStatus[result.Status] = &StatusStatistics{
			Count:              result.Count,
			AvgDurationSeconds: result.AvgDurationSeconds,
			P50Duration:        result.P50Duration,
			P95Duration:        result.P95Duration,
			P99Duration:        result.P99Duration,
			FirstExecution:     result.FirstExecution,
			LastExecution:      result.LastExecution,
		}
	}

	return stats, nil
}

// UpdateStatistics refreshes the execution statistics table
func (r *GormStrategy) UpdateStatistics(ctx context.Context) error {
	// Delete old statistics
	if err := r.db.WithContext(ctx).Exec("DELETE FROM execution_statistics").Error; err != nil {
		return fmt.Errorf("failed to delete old statistics: %w", err)
	}

	// Insert new statistics
	sql := `
		INSERT INTO execution_statistics (
			state_machine_id, status, execution_count, 
			avg_duration_seconds, min_duration_seconds, max_duration_seconds,
			first_execution, last_execution
		)
		SELECT 
			state_machine_id,
			status,
			COUNT(*) as execution_count,
			AVG(EXTRACT(EPOCH FROM (end_time - start_time))) as avg_duration_seconds,
			MIN(EXTRACT(EPOCH FROM (end_time - start_time))) as min_duration_seconds,
			MAX(EXTRACT(EPOCH FROM (end_time - start_time))) as max_duration_seconds,
			MIN(start_time) as first_execution,
			MAX(start_time) as last_execution
		FROM executions
		WHERE end_time IS NOT NULL
		GROUP BY state_machine_id, status
	`

	if err := r.db.WithContext(ctx).Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to update statistics: %w", err)
	}

	return nil
}

// HealthCheck verifies database connectivity
func (r *GormStrategy) HealthCheck(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Conversion helpers

func toExecutionModel(exec *ExecutionRecord) *ExecutionModel {
	model := &ExecutionModel{
		ExecutionID:    exec.ExecutionID,
		StateMachineID: exec.StateMachineID,
		Name:           exec.Name,
		Status:         exec.Status,
		StartTime:      *exec.StartTime,
		CurrentState:   exec.CurrentState,
	}

	if exec.Input != nil {
		model.Input = toJSONB(exec.Input)
	}
	if exec.Output != nil {
		model.Output = toJSONB(exec.Output)
	}
	if exec.StartTime != nil && exec.StartTime.IsZero() {
		model.StartTime = *exec.StartTime
	}
	if exec.EndTime != nil && !exec.EndTime.IsZero() {
		model.EndTime = exec.EndTime
	}
	if exec.Error != NULL {
		model.Error = exec.Error
	}
	if exec.Metadata != nil {
		model.Metadata = toJSONB(exec.Metadata)
	}

	return model
}

func fromExecutionModel(model *ExecutionModel) *ExecutionRecord {
	exec := &ExecutionRecord{
		ExecutionID:    model.ExecutionID,
		StateMachineID: model.StateMachineID,
		Name:           model.Name,
		Status:         model.Status,
		StartTime:      &model.StartTime,
		CurrentState:   model.CurrentState,
	}

	if model.Input != nil {
		exec.Input = fromJSONB(model.Input)
	}
	if model.Output != nil {
		exec.Output = fromJSONB(model.Output)
	}
	if model.EndTime != nil {
		exec.EndTime = model.EndTime
	}
	if model.Error != "" {
		exec.Error = model.Error
	}
	if model.Metadata != nil {
		exec.Metadata = fromJSONB(model.Metadata)
	}

	return exec
}

func toStateHistoryModel(history *StateHistoryRecord) *StateHistoryModel {
	model := &StateHistoryModel{
		ID:             history.ID,
		ExecutionID:    history.ExecutionID,
		StateName:      history.StateName,
		StateType:      history.StateType,
		Status:         history.Status,
		StartTime:      *history.StartTime,
		RetryCount:     history.RetryCount,
		SequenceNumber: history.SequenceNumber,
	}

	if history.Input != nil {
		model.Input = toJSONB(history.Input)
	}
	if history.Output != nil {
		model.Output = toJSONB(history.Output)
	}
	if !history.EndTime.IsZero() {
		model.EndTime = history.EndTime
	}
	if history.Error != NULL {
		model.Error = history.Error
	}
	if history.Metadata != nil {
		model.Metadata = toJSONB(history.Metadata)
	}

	return model
}

func fromStateHistoryModel(model *StateHistoryModel) *StateHistoryRecord {
	history := &StateHistoryRecord{
		ID:             model.ID,
		ExecutionID:    model.ExecutionID,
		StateName:      model.StateName,
		StateType:      model.StateType,
		Status:         model.Status,
		StartTime:      &model.StartTime,
		RetryCount:     model.RetryCount,
		SequenceNumber: model.SequenceNumber,
	}

	if model.Input != nil {
		history.Input = fromJSONB(model.Input)
	}
	if model.Output != nil {
		history.Output = fromJSONB(model.Output)
	}
	if model.EndTime != nil {
		history.EndTime = model.EndTime
	}
	if model.Error != "" {
		history.Error = model.Error
	}
	if model.Metadata != nil {
		history.Metadata = fromJSONB(model.Metadata)
	}

	return history
}

func toJSONB(data interface{}) JSONB {
	if m, ok := data.(map[string]interface{}); ok {
		return JSONB(m)
	}
	return nil
}

func fromJSONB(jsonb JSONB) map[string]interface{} {
	if jsonb == nil {
		return nil
	}
	return map[string]interface{}(jsonb)
}

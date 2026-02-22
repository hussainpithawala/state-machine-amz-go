// pkg/repository/gorm_postgres.go
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// GormPostgresRepository implements Repository using GORM
type GormPostgresRepository struct {
	db     *gorm.DB
	config *Config
}

// NewGormPostgresRepository creates a new GORM-based repository
func NewGormPostgresRepository(config *Config) (*GormPostgresRepository, error) {
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

	return &GormPostgresRepository{
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
func (r *GormPostgresRepository) Initialize(ctx context.Context) error {
	migrator := r.db.Migrator()

	// Migrate tables in correct order (parent tables first)
	tables := []struct {
		model     interface{}
		tableName string
	}{
		{&StateMachineModel{}, "state_machines"},
		{&ExecutionModel{}, "executions"},
		{&StateHistoryModel{}, "state_history"},
		{&ExecutionStatisticsModel{}, "statistics"},
		{&MessageCorrelationModel{}, "message_correlations"},
		{&LinkedExecutionModel{}, "linked_executions"},
	}

	for _, table := range tables {
		if err := r.migrateTable(ctx, migrator, table.model, table.tableName); err != nil {
			return err
		}

		// Add foreign key constraints after state_history table is migrated
		if table.tableName == "state_history" {
			if err := r.addForeignKeyConstraints(ctx); err != nil {
				// Log warning but don't fail if constraint already exists
				fmt.Printf("Warning: could not add foreign key constraints: %v\n", err)
			}
		}
	}

	// Create additional indexes for better performance
	if err := r.createAdditionalIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create additional indexes: %w", err)
	}

	return nil
}

// migrateTable handles table creation or schema updates
func (r *GormPostgresRepository) migrateTable(ctx context.Context, migrator gorm.Migrator, model interface{}, tableName string) error {
	if !migrator.HasTable(model) {
		if err := migrator.CreateTable(model); err != nil {
			return fmt.Errorf("failed to create %s table: %w", tableName, err)
		}
	} else {
		if err := r.db.WithContext(ctx).AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to update %s table schema: %w", tableName, err)
		}
	}
	return nil
}

// addForeignKeyConstraints adds foreign key constraints after tables are created
func (r *GormPostgresRepository) addForeignKeyConstraints(ctx context.Context) error {
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
func (r *GormPostgresRepository) createAdditionalIndexes(ctx context.Context) error {
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

// SaveStateMachine saves a state machine definition
func (r *GormPostgresRepository) SaveStateMachine(ctx context.Context, sm *StateMachineRecord) error {
	model := toStateMachineModel(sm)
	return r.db.WithContext(ctx).Save(model).Error
}

// GetStateMachine retrieves a state machine by ID
func (r *GormPostgresRepository) GetStateMachine(ctx context.Context, stateMachineID string) (*StateMachineRecord, error) {
	var model StateMachineModel
	result := r.db.WithContext(ctx).Limit(1).Find(&model, "id = ?", stateMachineID)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("state machine '%s' not found", stateMachineID)
	}
	return fromStateMachineModel(&model), nil
}

// ListStateMachines lists all state machines with filtering
func (r *GormPostgresRepository) ListStateMachines(ctx context.Context, filter *DefinitionFilter) ([]*StateMachineRecord, error) {
	var models []StateMachineModel

	query := r.db.WithContext(ctx)

	// Apply filters
	if filter != nil {
		if filter.StateMachineID != "" {
			query = query.Where("id = ?", filter.StateMachineID)
		}
		if filter.Name != "" {
			query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
		}
	}

	// Order by created_at descending
	result := query.Order("created_at DESC").Find(&models)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list state machines: %w", result.Error)
	}

	records := make([]*StateMachineRecord, len(models))
	for i := range models {
		records[i] = fromStateMachineModel(&models[i])
	}

	return records, nil
}

func (r *GormPostgresRepository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// GetDB returns the underlying GORM database instance (for testing)
func (r *GormPostgresRepository) GetDB() *gorm.DB {
	return r.db
}

// SaveExecution saves or updates an execution using GORM
func (r *GormPostgresRepository) SaveExecution(ctx context.Context, exec *ExecutionRecord) error {
	model := toExecutionModel(exec)

	// Use Clauses with OnConflict for proper upsert behavior
	// This ensures we either insert a new record or update the existing one based on execution_id
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "execution_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "current_state", "output", "error", "end_time", "updated_at"}),
		}).
		Create(model)

	if result.Error != nil {
		return fmt.Errorf("failed to save execution: %w", result.Error)
	}

	return nil
}

// GetExecution retrieves an execution by ID
func (r *GormPostgresRepository) GetExecution(ctx context.Context, executionID string) (*ExecutionRecord, error) {
	var model ExecutionModel

	result := r.db.WithContext(ctx).
		Where("execution_id = ?", executionID).
		Limit(1).
		Find(&model)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get execution: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	return fromExecutionModel(&model), nil
}

// SaveStateHistory saves a state history entry
func (r *GormPostgresRepository) SaveStateHistory(ctx context.Context, history *StateHistoryRecord) error {
	model := toStateHistoryModel(history)

	// State history is immutable - use insert-only with ON CONFLICT DO NOTHING
	// This avoids expensive upserts since history records should never be updated
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoNothing: true,
		}).
		Create(model)

	if result.Error != nil {
		return fmt.Errorf("failed to save state history: %w", result.Error)
	}

	return nil
}

// GetStateHistory retrieves all state history for an execution
func (r *GormPostgresRepository) GetStateHistory(ctx context.Context, executionID string) ([]*StateHistoryRecord, error) {
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
func (r *GormPostgresRepository) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*ExecutionRecord, error) {
	query := r.db.WithContext(ctx).Model(&ExecutionModel{})

	// Apply filters
	if filter != nil {
		query = addFiltersToQuery(filter, query)

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

// ListExecutionIDs returns only execution IDs matching the filter (more efficient than ListExecutions)
func (r *GormPostgresRepository) ListExecutionIDs(ctx context.Context, filter *ExecutionFilter) ([]string, error) {
	query := r.db.WithContext(ctx).Model(&ExecutionModel{}).Select("execution_id").Distinct()

	// Apply filters
	if filter != nil {
		query = addFiltersToQuery(filter, query)

		// Apply pagination
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	// Always order by execution_id
	query = query.Order("execution_id")

	var executionIDs []string
	result := query.Pluck("execution_id", &executionIDs)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list execution IDs: %w", result.Error)
	}

	return executionIDs, nil
}

func addFiltersToQuery(filter *ExecutionFilter, query *gorm.DB) *gorm.DB {
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
	return query
}

// CountExecutions returns the count of executions matching the filter
func (r *GormPostgresRepository) CountExecutions(ctx context.Context, filter *ExecutionFilter) (int64, error) {
	query := r.db.WithContext(ctx).Model(&ExecutionModel{})

	// Apply same filters as ListExecutions (without pagination)
	if filter != nil {
		addFiltersToQuery(filter, query)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count executions: %w", err)
	}

	return count, nil
}

// DeleteExecution removes an execution and its history (cascade handled by FK)
func (r *GormPostgresRepository) DeleteExecution(ctx context.Context, executionID string) error {
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
func (r *GormPostgresRepository) GetExecutionWithHistory(ctx context.Context, executionID string) (*ExecutionRecord, []*StateHistoryRecord, error) {
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
func (r *GormPostgresRepository) GetStatistics(ctx context.Context, stateMachineID string) (*Statistics, error) {
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
func (r *GormPostgresRepository) UpdateStatistics(ctx context.Context) error {
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
func (r *GormPostgresRepository) HealthCheck(ctx context.Context) error {
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
		CurrentState:   exec.CurrentState,
	}

	if exec.Input != nil {
		model.Input = toJSONB(exec.Input)
	}
	if exec.Output != nil {
		model.Output = toJSONB(exec.Output)
	}
	if exec.StartTime != nil && !exec.StartTime.IsZero() {
		model.StartTime = *exec.StartTime
	}
	if exec.EndTime != nil && !exec.EndTime.IsZero() {
		model.EndTime = *exec.EndTime
	}
	if exec.Error != "" {
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
	if !model.StartTime.IsZero() {
		exec.StartTime = &model.StartTime
	}
	if !model.EndTime.IsZero() {
		exec.EndTime = &model.EndTime
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
		RetryCount:     history.RetryCount,
		SequenceNumber: history.SequenceNumber,
	}

	// Handle ExecutionStartTime with nil check
	if history.ExecutionStartTime != nil {
		model.ExecutionStartTime = *history.ExecutionStartTime
	}

	if history.Input != nil {
		model.Input = toJSONB(history.Input)
	}
	if history.Output != nil {
		model.Output = toJSONB(history.Output)
	}
	if history.StartTime != nil && !history.StartTime.IsZero() {
		model.StartTime = *history.StartTime
	}
	if history.EndTime != nil && !history.EndTime.IsZero() {
		model.EndTime = *history.EndTime
	}
	if history.Error != "" {
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
		EndTime:        &model.EndTime,
		RetryCount:     model.RetryCount,
		SequenceNumber: model.SequenceNumber,
	}

	if model.Input != nil {
		history.Input = fromJSONB(model.Input)
	}
	if model.Output != nil {
		history.Output = fromJSONB(model.Output)
	}

	if !model.StartTime.IsZero() {
		history.StartTime = &model.StartTime
	}

	if !model.EndTime.IsZero() {
		history.EndTime = &model.EndTime
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
	if data == nil {
		return nil
	}
	if m, ok := data.(map[string]interface{}); ok {
		return JSONB(m)
	}
	// If it's not a map, we still want to store it, but JSONB type expects a map.
	// However, GORM with our JSONB type (which is a map) might struggle with non-map types.
	// But correlation values can be strings or numbers.
	// Let's check how JSONB is defined.
	return JSONB{"$": data}
}

func fromJSONB(jsonb JSONB) map[string]interface{} {
	if jsonb == nil {
		return nil
	}
	return map[string]interface{}(jsonb)
}

func toStateMachineModel(sm *StateMachineRecord) *StateMachineModel {
	return &StateMachineModel{
		ID:          sm.ID,
		Name:        sm.Name,
		Description: sm.Description,
		Definition:  sm.Definition,
		Type:        sm.Type,
		Version:     sm.Version,
		Metadata:    toJSONB(sm.Metadata),
		CreatedAt:   sm.CreatedAt,
		UpdatedAt:   sm.UpdatedAt,
	}
}

func fromStateMachineModel(model *StateMachineModel) *StateMachineRecord {
	return &StateMachineRecord{
		ID:          model.ID,
		Name:        model.Name,
		Description: model.Description,
		Definition:  model.Definition,
		Type:        model.Type,
		Version:     model.Version,
		Metadata:    fromJSONB(model.Metadata),
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}
}

func toLinkedExecutionModel(record *LinkedExecutionRecord) *LinkedExecutionModel {
	return &LinkedExecutionModel{
		ID:                     record.ID,
		SourceStateMachineID:   record.SourceStateMachineID,
		SourceExecutionID:      record.SourceExecutionID,
		SourceStateName:        record.SourceStateName,
		InputTransformerName:   record.InputTransformerName,
		TargetStateMachineName: record.TargetStateMachineName,
		TargetExecutionID:      record.TargetExecutionID,
		CreatedAt:              record.CreatedAt,
	}
}

func fromLinkedExecutionModel(model *LinkedExecutionModel) *LinkedExecutionRecord {
	return &LinkedExecutionRecord{
		ID:                     model.ID,
		SourceStateMachineID:   model.SourceStateMachineID,
		SourceExecutionID:      model.SourceExecutionID,
		SourceStateName:        model.SourceStateName,
		InputTransformerName:   model.InputTransformerName,
		TargetStateMachineName: model.TargetStateMachineName,
		TargetExecutionID:      model.TargetExecutionID,
		CreatedAt:              model.CreatedAt,
	}
}

// SaveMessageCorrelation saves a message correlation record
func (r *GormPostgresRepository) SaveMessageCorrelation(ctx context.Context, record *MessageCorrelationRecord) error {
	model := toMessageCorrelationModel(record)
	return r.db.WithContext(ctx).Save(model).Error
}

// GetMessageCorrelation retrieves a correlation record by ID
func (r *GormPostgresRepository) GetMessageCorrelation(ctx context.Context, id string) (*MessageCorrelationRecord, error) {
	var model MessageCorrelationModel
	result := r.db.WithContext(ctx).Limit(1).Find(&model, "id = ?", id)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("correlation record not found: %s", id)
	}
	return fromMessageCorrelationModel(&model), nil
}

// FindWaitingCorrelations finds correlation records waiting for messages
func (r *GormPostgresRepository) FindWaitingCorrelations(ctx context.Context, filter *MessageCorrelationFilter) ([]*MessageCorrelationRecord, error) {
	query := r.db.WithContext(ctx).Model(&MessageCorrelationModel{})

	if filter != nil {
		if filter.CorrelationKey != "" {
			query = query.Where("correlation_key = ?", filter.CorrelationKey)
		}
		if filter.CorrelationValue != nil {
			query = query.Where("correlation_value = ?", toJSONB(filter.CorrelationValue))
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.StateMachineID != "" {
			query = query.Where("state_machine_id = ?", filter.StateMachineID)
		}

		query = query.Order("created_at ASC")

		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	var models []MessageCorrelationModel
	err := query.Find(&models).Error
	if err != nil {
		return nil, err
	}

	records := make([]*MessageCorrelationRecord, len(models))
	for i := range models {
		records[i] = fromMessageCorrelationModel(&models[i])
	}
	return records, nil
}

// UpdateCorrelationStatus updates the status of a correlation record
func (r *GormPostgresRepository) UpdateCorrelationStatus(ctx context.Context, id, status string) error {
	result := r.db.WithContext(ctx).Model(&MessageCorrelationModel{}).
		Where("id = ?", id).
		Update("status", status)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("correlation record not found: %s", id)
	}
	return nil
}

// DeleteMessageCorrelation deletes a correlation record
func (r *GormPostgresRepository) DeleteMessageCorrelation(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&MessageCorrelationModel{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("correlation record not found: %s", id)
	}
	return nil
}

// ListTimedOutCorrelations finds correlations that have exceeded their timeout
func (r *GormPostgresRepository) ListTimedOutCorrelations(ctx context.Context, currentTimestamp int64) ([]*MessageCorrelationRecord, error) {
	var models []MessageCorrelationModel
	err := r.db.WithContext(ctx).
		Where("status = 'WAITING' AND timeout_at IS NOT NULL AND timeout_at <= ?", currentTimestamp).
		Find(&models).Error

	if err != nil {
		return nil, err
	}

	records := make([]*MessageCorrelationRecord, len(models))
	for i := range models {
		records[i] = fromMessageCorrelationModel(&models[i])
	}
	return records, nil
}

// SaveLinkedExecution saves a linked execution record
func (r *GormPostgresRepository) SaveLinkedExecution(ctx context.Context, linkedExec *LinkedExecutionRecord) error {
	model := toLinkedExecutionModel(linkedExec)

	// Use insert-only with ON CONFLICT DO NOTHING since linkages are immutable
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoNothing: true,
		}).
		Create(model)

	if result.Error != nil {
		return fmt.Errorf("failed to save linked execution: %w", result.Error)
	}

	return nil
}

// ListLinkedExecutions lists linked executions with filtering and pagination
func (r *GormPostgresRepository) ListLinkedExecutions(ctx context.Context, filter *LinkedExecutionFilter) ([]*LinkedExecutionRecord, error) {
	var models []LinkedExecutionModel

	query := r.db.WithContext(ctx).Table("linked_executions")

	// Join with executions table if we need to filter by source execution status
	if filter != nil && filter.SourceExecutionStatus != "" {
		query = query.Joins("INNER JOIN executions ON linked_executions.source_execution_id = executions.execution_id").
			Where("executions.status = ?", filter.SourceExecutionStatus)
	}

	// Apply filters
	if filter != nil {
		if filter.SourceStateMachineID != "" {
			query = query.Where("linked_executions.source_state_machine_id = ?", filter.SourceStateMachineID)
		}
		if filter.SourceExecutionID != "" {
			query = query.Where("linked_executions.source_execution_id = ?", filter.SourceExecutionID)
		}
		if filter.SourceStateName != "" {
			query = query.Where("linked_executions.source_state_name = ?", filter.SourceStateName)
		}
		if filter.InputTransformerName != "" {
			query = query.Where("linked_executions.input_transformer_name = ?", filter.InputTransformerName)
		}
		if filter.TargetStateMachineName != "" {
			query = query.Where("linked_executions.target_state_machine_name = ?", filter.TargetStateMachineName)
		}
		if filter.TargetExecutionID != "" {
			query = query.Where("linked_executions.target_execution_id = ?", filter.TargetExecutionID)
		}
		if !filter.CreatedAfter.IsZero() {
			query = query.Where("linked_executions.created_at >= ?", filter.CreatedAfter)
		}
		if !filter.CreatedBefore.IsZero() {
			query = query.Where("linked_executions.created_at <= ?", filter.CreatedBefore)
		}

		// Apply pagination
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
	}

	// Select linked_executions columns explicitly
	query = query.Select("linked_executions.*")

	// Order by created_at descending
	result := query.Order("linked_executions.created_at DESC").Find(&models)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list linked executions: %w", result.Error)
	}

	records := make([]*LinkedExecutionRecord, len(models))
	for i := range models {
		records[i] = fromLinkedExecutionModel(&models[i])
	}

	return records, nil
}

// CountLinkedExecutions returns the count of linked executions matching the filter
func (r *GormPostgresRepository) CountLinkedExecutions(ctx context.Context, filter *LinkedExecutionFilter) (int64, error) {
	var count int64

	query := r.db.WithContext(ctx).Table("linked_executions")

	// Join with executions table if we need to filter by source execution status
	if filter != nil && filter.SourceExecutionStatus != "" {
		query = query.Joins("INNER JOIN executions ON linked_executions.source_execution_id = executions.execution_id").
			Where("executions.status = ?", filter.SourceExecutionStatus)
	}

	// Apply filters
	if filter != nil {
		if filter.SourceStateMachineID != "" {
			query = query.Where("linked_executions.source_state_machine_id = ?", filter.SourceStateMachineID)
		}
		if filter.SourceExecutionID != "" {
			query = query.Where("linked_executions.source_execution_id = ?", filter.SourceExecutionID)
		}
		if filter.SourceStateName != "" {
			query = query.Where("linked_executions.source_state_name = ?", filter.SourceStateName)
		}
		if filter.InputTransformerName != "" {
			query = query.Where("linked_executions.input_transformer_name = ?", filter.InputTransformerName)
		}
		if filter.TargetStateMachineName != "" {
			query = query.Where("linked_executions.target_state_machine_name = ?", filter.TargetStateMachineName)
		}
		if filter.TargetExecutionID != "" {
			query = query.Where("linked_executions.target_execution_id = ?", filter.TargetExecutionID)
		}
		if !filter.CreatedAfter.IsZero() {
			query = query.Where("linked_executions.created_at >= ?", filter.CreatedAfter)
		}
		if !filter.CreatedBefore.IsZero() {
			query = query.Where("linked_executions.created_at <= ?", filter.CreatedBefore)
		}
	}

	result := query.Count(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count linked executions: %w", result.Error)
	}

	return count, nil
}

// ListNonLinkedExecutions lists executions that have no linked executions matching the filter criteria
// This allows finding executions that don't have specific types of linked executions
// For example: executions with no SUCCEEDED linked executions from a specific state
func (r *GormPostgresRepository) ListNonLinkedExecutions(ctx context.Context, filter *LinkedExecutionFilter) ([]*ExecutionRecord, error) {
	// Build the subquery for linked executions with filters
	subQuery := r.db.Table("linked_executions le")

	if filter != nil {
		if filter.SourceStateName != "" {
			subQuery = subQuery.Where("le.source_state_name = ?", filter.SourceStateName)
		}
		if filter.InputTransformerName != "" {
			subQuery = subQuery.Where("le.input_transformer_name = ?", filter.InputTransformerName)
		}
		if filter.TargetStateMachineName != "" {
			subQuery = subQuery.Where("le.target_state_machine_name = ?", filter.TargetStateMachineName)
		}
		if !filter.CreatedAfter.IsZero() {
			subQuery = subQuery.Where("le.created_at >= ?", filter.CreatedAfter)
		}
		if !filter.CreatedBefore.IsZero() {
			subQuery = subQuery.Where("le.created_at <= ?", filter.CreatedBefore)
		}
	}

	// Main query with LEFT JOIN to executions (to get status filter)
	query := r.db.WithContext(ctx).Model(&ExecutionModel{}).
		Select("DISTINCT executions.*").
		Joins("LEFT JOIN (?) AS filtered_links ON executions.execution_id = filtered_links.source_execution_id",
			subQuery.Select("le.source_execution_id").
				Joins("INNER JOIN executions AS source_exec ON le.source_execution_id = source_exec.execution_id")).
		Where("filtered_links.source_execution_id IS NULL")

	// Apply source execution filters
	if filter != nil {
		if filter.SourceStateMachineID != "" {
			query = query.Where("executions.state_machine_id = ?", filter.SourceStateMachineID)
		}
		if filter.SourceExecutionID != "" {
			query = query.Where("executions.execution_id = ?", filter.SourceExecutionID)
		}
		if filter.SourceExecutionStatus != "" {
			query = query.Where("executions.status = ?", filter.SourceExecutionStatus)
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
	query = query.Order("executions.start_time DESC")

	var models []ExecutionModel
	result := query.Find(&models)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list non-linked executions: %w", result.Error)
	}

	executions := make([]*ExecutionRecord, len(models))
	for i := range models {
		model := models[i]
		executions[i] = fromExecutionModel(&model)
	}

	return executions, nil
}

func toMessageCorrelationModel(record *MessageCorrelationRecord) *MessageCorrelationModel {
	model := &MessageCorrelationModel{
		ID:               record.ID,
		ExecutionID:      record.ExecutionID,
		StateMachineID:   record.StateMachineID,
		StateName:        record.StateName,
		CorrelationKey:   record.CorrelationKey,
		CorrelationValue: toJSONB(record.CorrelationValue),
		CreatedAt:        record.CreatedAt,
		TimeoutAt:        record.TimeoutAt,
		Status:           record.Status,
	}
	if record.ExecutionStartTime != nil {
		model.ExecutionStartTime = *record.ExecutionStartTime
	}
	return model
}

func fromMessageCorrelationModel(model *MessageCorrelationModel) *MessageCorrelationRecord {
	record := &MessageCorrelationRecord{
		ID:                 model.ID,
		ExecutionID:        model.ExecutionID,
		ExecutionStartTime: &model.ExecutionStartTime,
		StateMachineID:     model.StateMachineID,
		StateName:          model.StateName,
		CorrelationKey:     model.CorrelationKey,
		CorrelationValue:   fromJSONB(model.CorrelationValue),
		CreatedAt:          model.CreatedAt,
		TimeoutAt:          model.TimeoutAt,
		Status:             model.Status,
	}
	return record
}

// GetExecutionOutput retrieves output from an execution (final or specific state)
// If stateName is empty, returns the final execution output
// If stateName is provided, returns the output of that specific state
func (r *GormPostgresRepository) GetExecutionOutput(ctx context.Context, executionID, stateName string) (interface{}, error) {
	if stateName == "" {
		// Get final execution output
		var execution ExecutionModel
		result := r.db.WithContext(ctx).
			Select("output").
			Where("execution_id = ?", executionID).
			First(&execution)

		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("execution '%s' not found", executionID)
			}
			return nil, fmt.Errorf("failed to get execution output: %w", result.Error)
		}

		return fromJSONB(execution.Output), nil
	}

	// Get specific state output (most recent if state was executed multiple times)
	var stateHistory StateHistoryModel
	result := r.db.WithContext(ctx).
		Select("output").
		Where("execution_id = ? AND state_name = ?", executionID, stateName).
		Order("sequence_number DESC").
		First(&stateHistory)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("state '%s' not found in execution '%s'", stateName, executionID)
		}
		return nil, fmt.Errorf("failed to get state output: %w", result.Error)
	}

	return fromJSONB(stateHistory.Output), nil
}

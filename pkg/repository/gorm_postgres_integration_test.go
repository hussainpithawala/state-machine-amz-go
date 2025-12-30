//go:build integration
// +build integration

// pkg/repository/gorm_postgres_integration_test.go
package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// GormPostgresIntegrationTestSuite contains all GORM PostgreSQL integration tests
type GormPostgresIntegrationTestSuite struct {
	suite.Suite
	repository *GormPostgresRepository
	ctx        context.Context
}

// SetupSuite runs once before all tests
func (suite *GormPostgresIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Get connection string from environment or use default
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_gorm_test?sslmode=disable"
	}

	config := Config{
		Strategy:      "postgres_gorm",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns":     10,
			"max_idle_conns":     2,
			"conn_max_lifetime":  5 * time.Minute,
			"conn_max_idle_time": 5 * time.Minute,
			"log_level":          "warn",
		},
	}

	var err error
	suite.repository, err = NewGormPostgresRepository(&config)
	require.NoError(suite.T(), err, "Failed to create GORM PostgreSQL repository")

	// Initialize schema (AutoMigrate + indexes)
	err = suite.repository.Initialize(suite.ctx)
	require.NoError(suite.T(), err, "Failed to initialize GORM PostgreSQL schema")
}

// TearDownSuite runs once after all tests
func (suite *GormPostgresIntegrationTestSuite) TearDownSuite() {
	if suite.repository != nil {
		suite.cleanupTestData()
		_ = suite.repository.Close()
	}
}

// SetupTest runs before each test
func (suite *GormPostgresIntegrationTestSuite) SetupTest() {
	suite.cleanupTestData()
}

// cleanupTestData removes all test data
func (suite *GormPostgresIntegrationTestSuite) cleanupTestData() {
	// Safe because we’re using a dedicated test database.
	// Truncate in FK-safe order.
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE state_machines CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE state_history CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE executions CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE execution_statistics CASCADE").Error
}

// TestSaveAndGetExecution tests basic save and retrieve operations
func (suite *GormPostgresIntegrationTestSuite) TestSaveAndGetExecution() {
	now := time.Now()
	record := &ExecutionRecord{
		ExecutionID:    "exec-gorm-001",
		StateMachineID: "sm-test-001",
		Name:           "test-execution",
		Input: map[string]interface{}{
			"orderId": "12345",
			"amount":  100.50,
		},
		Status:       "RUNNING",
		StartTime:    &now,
		CurrentState: "ProcessOrder",
		Metadata: map[string]interface{}{
			"version": "1.0",
			"source":  "api",
		},
	}

	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))

	retrieved, err := suite.repository.GetExecution(suite.ctx, "exec-gorm-001")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), record.ExecutionID, retrieved.ExecutionID)
	assert.Equal(suite.T(), record.Status, retrieved.Status)
	assert.Equal(suite.T(), record.StateMachineID, retrieved.StateMachineID)

	assert.NotNil(suite.T(), retrieved.Input)
	inputMap, ok := retrieved.Input.(map[string]interface{})
	require.True(suite.T(), ok)
	assert.Equal(suite.T(), "12345", inputMap["orderId"])
}

func (suite *GormPostgresIntegrationTestSuite) TestSaveAndGetStateMachine() {
	record := &StateMachineRecord{
		ID:          "sm-gorm-001",
		Name:        "test-sm",
		Description: "test description",
		Definition:  `{"StartAt": "State1", "States": {"State1": {"Type": "Pass", "End": true}}}`,
		Version:     "1.0",
		Metadata: map[string]interface{}{
			"owner": "team-a",
		},
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}

	// Save
	err := suite.repository.SaveStateMachine(suite.ctx, record)
	require.NoError(suite.T(), err)

	// Get
	retrieved, err := suite.repository.GetStateMachine(suite.ctx, record.ID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), record.ID, retrieved.ID)
	assert.Equal(suite.T(), record.Name, retrieved.Name)
	assert.Equal(suite.T(), record.Description, retrieved.Description)
	assert.Equal(suite.T(), record.Definition, retrieved.Definition)
	assert.Equal(suite.T(), record.Version, retrieved.Version)
	assert.Equal(suite.T(), record.Metadata["owner"], retrieved.Metadata["owner"])
	assert.WithinDuration(suite.T(), record.CreatedAt, retrieved.CreatedAt, time.Second)

	// Update
	record.Description = "updated description"
	err = suite.repository.SaveStateMachine(suite.ctx, record)
	require.NoError(suite.T(), err)

	retrieved, err = suite.repository.GetStateMachine(suite.ctx, record.ID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "updated description", retrieved.Description)
}

// TestUpdateExecution tests updating an existing execution
func (suite *GormPostgresIntegrationTestSuite) TestUpdateExecution() {
	now := time.Now()
	record := &ExecutionRecord{
		ExecutionID:    "exec-gorm-002",
		StateMachineID: "sm-test-001",
		Name:           "update-test",
		Input:          map[string]interface{}{"test": "data"},
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "State1",
	}

	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))

	record.Status = "SUCCEEDED"
	record.CurrentState = "State2"
	endTime := time.Now().Add(time.Minute)
	record.EndTime = &endTime
	record.Output = map[string]interface{}{"result": "success"}

	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))

	retrieved, err := suite.repository.GetExecution(suite.ctx, "exec-gorm-002")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "SUCCEEDED", retrieved.Status)
	assert.Equal(suite.T(), "State2", retrieved.CurrentState)
	assert.NotNil(suite.T(), retrieved.EndTime)
	assert.NotNil(suite.T(), retrieved.Output)
}

// TestSaveStateHistory tests saving state execution history
func (suite *GormPostgresIntegrationTestSuite) TestSaveStateHistory() {
	now := time.Now()

	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-gorm-003",
		StateMachineID: "sm-test-001",
		Name:           "history-test",
		Input:          map[string]interface{}{},
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "State1",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, execRecord))

	for i := 0; i < 3; i++ {
		startTime := time.Now().Add(time.Duration(i) * time.Second)
		endTime := startTime.Add(500 * time.Millisecond)

		histRecord := &StateHistoryRecord{
			ID:                 fmt.Sprintf("hist-gorm-%d", i),
			ExecutionID:        execRecord.ExecutionID,
			ExecutionStartTime: execRecord.StartTime,
			StateName:          fmt.Sprintf("State%d", i+1),
			StateType:          "Task",
			Input: map[string]interface{}{
				"step":  i,
				"value": i * 10,
			},
			Output: map[string]interface{}{
				"result": "processed",
			},
			Status:         "SUCCEEDED",
			StartTime:      &startTime,
			EndTime:        &endTime,
			SequenceNumber: i,
			RetryCount:     0,
		}

		require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, histRecord))
	}

	history, err := suite.repository.GetStateHistory(suite.ctx, execRecord.ExecutionID)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), history, 3)

	for i, h := range history {
		assert.Equal(suite.T(), i, h.SequenceNumber)
		assert.Equal(suite.T(), fmt.Sprintf("State%d", i+1), h.StateName)
		assert.Equal(suite.T(), "SUCCEEDED", h.Status)
		assert.NotNil(suite.T(), h.EndTime)
	}
}

// TestStateHistoryWithRetries tests retry tracking
func (suite *GormPostgresIntegrationTestSuite) TestStateHistoryWithRetries() {
	now := time.Now()

	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-gorm-004",
		StateMachineID: "sm-test-001",
		Name:           "retry-test",
		Input:          map[string]interface{}{},
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "FlakyState",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, execRecord))

	for retry := 0; retry < 3; retry++ {
		startTime := time.Now().Add(time.Duration(retry) * time.Second)
		endTime := startTime.Add(100 * time.Millisecond)

		status := "FAILED"
		errorStr := "Transient error"
		if retry == 2 {
			status = "SUCCEEDED"
			errorStr = ""
		}

		histRecord := &StateHistoryRecord{
			ID:                 fmt.Sprintf("hist-gorm-retry-%d", retry),
			ExecutionID:        execRecord.ExecutionID,
			ExecutionStartTime: execRecord.StartTime,
			StateName:          "FlakyState",
			StateType:          "Task",
			Input:              map[string]interface{}{},
			Status:             status,
			StartTime:          &startTime,
			EndTime:            &endTime,
			Error:              errorStr,
			RetryCount:         retry,
			SequenceNumber:     0,
		}

		require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, histRecord))
	}

	history, err := suite.repository.GetStateHistory(suite.ctx, execRecord.ExecutionID)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), history, 3)

	assert.Equal(suite.T(), 0, history[0].RetryCount)
	assert.Equal(suite.T(), "FAILED", history[0].Status)
	assert.Equal(suite.T(), 2, history[2].RetryCount)
	assert.Equal(suite.T(), "SUCCEEDED", history[2].Status)
}

// TestListExecutions tests listing with various filters
func (suite *GormPostgresIntegrationTestSuite) TestListExecutions() {
	baseTime := time.Now().Add(-1 * time.Hour)

	testCases := []struct {
		id     string
		smID   string
		status string
		offset time.Duration
	}{
		{"exec-gorm-list-001", "sm-order", "SUCCEEDED", 0},
		{"exec-gorm-list-002", "sm-order", "RUNNING", 10 * time.Minute},
		{"exec-gorm-list-003", "sm-payment", "SUCCEEDED", 20 * time.Minute},
		{"exec-gorm-list-004", "sm-order", "FAILED", 30 * time.Minute},
		{"exec-gorm-list-005", "sm-payment", "SUCCEEDED", 40 * time.Minute},
	}

	for _, tc := range testCases {
		startTime := baseTime.Add(tc.offset)
		record := &ExecutionRecord{
			ExecutionID:    tc.id,
			StateMachineID: tc.smID,
			Name:           "list-test",
			Input:          map[string]interface{}{},
			Status:         tc.status,
			StartTime:      &startTime,
			CurrentState:   "SomeState",
		}
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))
	}

	// List all
	executions, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:  100,
		Offset: 0,
	})
	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), len(executions), 5)

	// Filter by status
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:  100,
		Offset: 0,
		Status: "SUCCEEDED",
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))

	// Filter by state machine ID
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-order",
		Limit:          100,
		Offset:         0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))

	// Combined filters
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-order",
		Status:         "SUCCEEDED",
		Limit:          100,
		Offset:         0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(executions))

	// Pagination
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:  2,
		Offset: 0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(executions))

	// Time range filter
	timeLapse := baseTime.Add(15 * time.Minute)
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:      3,
		Offset:     0,
		StartAfter: timeLapse,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, len(executions))
}

// TestDeleteExecution tests cascade deletion
func (suite *GormPostgresIntegrationTestSuite) TestDeleteExecution() {
	startTime := time.Now()
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-gorm-delete",
		StateMachineID: "sm-test-001",
		Name:           "delete-test",
		Input:          map[string]interface{}{},
		Status:         "SUCCEEDED",
		StartTime:      &startTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, execRecord))

	startTimeHistory := time.Now()
	histRecord := &StateHistoryRecord{
		ID:                 "hist-gorm-delete",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateName:          "State1",
		StateType:          "Task",
		Input:              map[string]interface{}{},
		Status:             "SUCCEEDED",
		StartTime:          &startTimeHistory,
		SequenceNumber:     0,
	}
	require.NoError(suite.T(), suite.repository.SaveStateHistory(suite.ctx, histRecord))

	require.NoError(suite.T(), suite.repository.DeleteExecution(suite.ctx, execRecord.ExecutionID))

	_, err := suite.repository.GetExecution(suite.ctx, execRecord.ExecutionID)
	assert.Error(suite.T(), err)

	history, err := suite.repository.GetStateHistory(suite.ctx, execRecord.ExecutionID)
	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), history)
}

// TestHealthCheck tests database health check
func (suite *GormPostgresIntegrationTestSuite) TestHealthCheck() {
	err := suite.repository.HealthCheck(suite.ctx)
	assert.NoError(suite.T(), err)
}

// TestConcurrentWrites tests concurrent write operations
func (suite *GormPostgresIntegrationTestSuite) TestConcurrentWrites() {
	const numGoroutines = 10
	const writesPerGoroutine = 5

	errChan := make(chan error, numGoroutines*writesPerGoroutine)
	doneChan := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			for i := 0; i < writesPerGoroutine; i++ {
				startTime := time.Now()
				record := &ExecutionRecord{
					ExecutionID:    fmt.Sprintf("exec-gorm-concurrent-%d-%d", goroutineID, i),
					StateMachineID: "sm-concurrent",
					Name:           "concurrent-test",
					Input:          map[string]interface{}{"g": goroutineID, "i": i},
					Status:         "SUCCEEDED",
					StartTime:      &startTime,
					CurrentState:   "Final",
				}

				if err := suite.repository.SaveExecution(suite.ctx, record); err != nil {
					errChan <- err
					return
				}
			}
			doneChan <- true
		}(g)
	}

	for i := 0; i < numGoroutines; i++ {
		<-doneChan
	}
	close(errChan)

	for err := range errChan {
		require.NoError(suite.T(), err)
	}

	executions, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-concurrent",
		Limit:          1000,
		Offset:         0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), numGoroutines*writesPerGoroutine, len(executions))
}

// TestGetStatistics tests statistics generation (GORM repository variant)
func (suite *GormPostgresIntegrationTestSuite) TestGetStatistics() {
	statuses := []string{"SUCCEEDED", "SUCCEEDED", "FAILED", "RUNNING"}

	for i, status := range statuses {
		startTime := time.Now().Add(time.Duration(-i) * time.Minute)
		endTime := startTime.Add(30 * time.Second)

		record := &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-gorm-stats-%d", i),
			StateMachineID: "sm-stats-test",
			Name:           "stats-test",
			Input:          map[string]interface{}{},
			Status:         status,
			StartTime:      &startTime,
			CurrentState:   "Final",
		}
		if status != "RUNNING" {
			record.EndTime = &endTime
		}

		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))
	}

	stats, err := suite.repository.GetStatistics(suite.ctx, "sm-stats-test")
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), stats)
	require.NotNil(suite.T(), stats.ByStatus)

	succeededStats := stats.ByStatus["SUCCEEDED"]
	require.NotNil(suite.T(), succeededStats)
	assert.Equal(suite.T(), int64(2), succeededStats.Count)
	assert.GreaterOrEqual(suite.T(), succeededStats.AvgDurationSeconds, 0.0)

	failedStats := stats.ByStatus["FAILED"]
	require.NotNil(suite.T(), failedStats)
	assert.Equal(suite.T(), int64(1), failedStats.Count)
}

// TestErrorHandling tests various error scenarios
func (suite *GormPostgresIntegrationTestSuite) TestErrorHandling() {
	// Save with empty execution ID (should fail at DB/validation layer)
	err := suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
		ExecutionID: "",
	})
	assert.Error(suite.T(), err)

	// Get non-existent execution
	_, err = suite.repository.GetExecution(suite.ctx, "non-existent")
	assert.Error(suite.T(), err)

	// Delete non-existent execution
	err = suite.repository.DeleteExecution(suite.ctx, "non-existent")
	assert.Error(suite.T(), err)

	// Save state history without execution (should fail due to FK)
	now := time.Now()
	err = suite.repository.SaveStateHistory(suite.ctx, &StateHistoryRecord{
		ID:             "orphan-hist-gorm",
		ExecutionID:    "non-existent-exec",
		StateName:      "State1",
		StateType:      "Task",
		Status:         "SUCCEEDED",
		StartTime:      &now,
		SequenceNumber: 0,
	})
	assert.Error(suite.T(), err)
}

// Run the test suite
func TestGormPostgresIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL integration tests in short mode")
	}

	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
	}

	// Try to connect quickly; skip if not available.
	config := &Config{
		Strategy:      "postgres_gorm",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"log_level": "silent",
		},
	}

	repo, err := NewGormPostgresRepository(config)
	if err != nil {
		t.Skip("PostgreSQL not available, skipping GORM integration tests")
		return
	}
	_ = repo.Close()

	suite.Run(t, new(GormPostgresIntegrationTestSuite))
}

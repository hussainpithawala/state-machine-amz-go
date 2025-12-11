//go:build integration
// +build integration

// pkg/repository/postgres_integration_test.go
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

// PostgresIntegrationTestSuite contains all PostgreSQL integration tests
type PostgresIntegrationTestSuite struct {
	suite.Suite
	strategy *PostgresStrategy
	ctx      context.Context
}

// SetupSuite runs once before all tests
func (suite *PostgresIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Get connection string from environment or use default
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
	}

	config := Config{
		Strategy:      "postgres",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns":    10,
			"max_idle_conns":    2,
			"conn_max_lifetime": 5 * time.Minute,
			"statement_timeout": "30s",
			"search_path":       "public",
		},
	}

	var err error
	suite.strategy, err = NewPostgresStrategy(&config)
	require.NoError(suite.T(), err, "Failed to create PostgreSQL strategy")

	// Initialize schema
	err = suite.strategy.Initialize(suite.ctx)
	require.NoError(suite.T(), err, "Failed to initialize PostgreSQL schema")
}

// TearDownSuite runs once after all tests
func (suite *PostgresIntegrationTestSuite) TearDownSuite() {
	if suite.strategy != nil {
		// Clean up test data
		suite.cleanupTestData()
		err := suite.strategy.Close()
		if err != nil {
			return
		}
	}
}

// SetupTest runs before each test
func (suite *PostgresIntegrationTestSuite) SetupTest() {
	// Clean up before each test
	suite.cleanupTestData()
}

// cleanupTestData removes all test data
func (suite *PostgresIntegrationTestSuite) cleanupTestData() {
	// This is safe because we're using a test database
	_, err := suite.strategy.db.ExecContext(suite.ctx, "TRUNCATE TABLE state_history CASCADE")
	if err != nil {
		return
	}
	_, err2 := suite.strategy.db.ExecContext(suite.ctx, "TRUNCATE TABLE executions CASCADE")
	if err2 != nil {
		return
	}
}

// TestSaveAndGetExecution tests basic save and retrieve operations
func (suite *PostgresIntegrationTestSuite) TestSaveAndGetExecution() {
	record := &ExecutionRecord{
		ExecutionID:    "exec-test-001",
		StateMachineID: "sm-test-001",
		Name:           "test-execution",
		Input: map[string]interface{}{
			"orderId": "12345",
			"amount":  100.50,
		},
		Status:       "RUNNING",
		StartTime:    time.Now(),
		CurrentState: "ProcessOrder",
		Metadata: map[string]interface{}{
			"version": "1.0",
			"source":  "api",
		},
	}

	// Save execution
	err := suite.strategy.SaveExecution(suite.ctx, record)
	require.NoError(suite.T(), err)

	// Retrieve execution
	retrieved, err := suite.strategy.GetExecution(suite.ctx, "exec-test-001")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), record.ExecutionID, retrieved.ExecutionID)
	assert.Equal(suite.T(), record.Status, retrieved.Status)
	assert.Equal(suite.T(), record.StateMachineID, retrieved.StateMachineID)

	// Verify input was stored correctly
	assert.NotNil(suite.T(), retrieved.Input)
	inputMap := retrieved.Input.(map[string]interface{})
	assert.Equal(suite.T(), "12345", inputMap["orderId"])
}

// TestUpdateExecution tests updating an existing execution
func (suite *PostgresIntegrationTestSuite) TestUpdateExecution() {
	record := &ExecutionRecord{
		ExecutionID:    "exec-test-002",
		StateMachineID: "sm-test-001",
		Name:           "update-test",
		Input:          map[string]interface{}{"test": "data"},
		Status:         "RUNNING",
		StartTime:      time.Now(),
		CurrentState:   "State1",
	}

	// Save initial
	require.NoError(suite.T(), suite.strategy.SaveExecution(suite.ctx, record))

	// Update
	record.Status = "SUCCEEDED"
	record.CurrentState = "State2"
	endTime := time.Now().Add(time.Minute)
	record.EndTime = &endTime
	record.Output = map[string]interface{}{"result": "success"}

	require.NoError(suite.T(), suite.strategy.SaveExecution(suite.ctx, record))

	// Verify update
	retrieved, err := suite.strategy.GetExecution(suite.ctx, "exec-test-002")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "SUCCEEDED", retrieved.Status)
	assert.Equal(suite.T(), "State2", retrieved.CurrentState)
	assert.NotNil(suite.T(), retrieved.EndTime)
	assert.NotNil(suite.T(), retrieved.Output)
}

// TestSaveStateHistory tests saving state execution history
func (suite *PostgresIntegrationTestSuite) TestSaveStateHistory() {
	// First create an execution
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-test-003",
		StateMachineID: "sm-test-001",
		Name:           "history-test",
		Input:          map[string]interface{}{},
		Status:         "RUNNING",
		StartTime:      time.Now(),
		CurrentState:   "State1",
	}
	require.NoError(suite.T(), suite.strategy.SaveExecution(suite.ctx, execRecord))

	// Save multiple state history records
	for i := 0; i < 3; i++ {
		startTime := time.Now().Add(time.Duration(i) * time.Second)
		endTime := startTime.Add(500 * time.Millisecond)

		histRecord := &StateHistoryRecord{
			ID:                 fmt.Sprintf("hist-test-%d", i),
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
			StartTime:      startTime,
			EndTime:        &endTime,
			SequenceNumber: i,
			RetryCount:     0,
		}

		err := suite.strategy.SaveStateHistory(suite.ctx, histRecord)
		require.NoError(suite.T(), err)
	}

	// Retrieve history
	history, err := suite.strategy.GetStateHistory(suite.ctx, "exec-test-003")
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), history, 3)

	// Verify order and data
	for i, h := range history {
		assert.Equal(suite.T(), i, h.SequenceNumber)
		assert.Equal(suite.T(), fmt.Sprintf("State%d", i+1), h.StateName)
		assert.Equal(suite.T(), "SUCCEEDED", h.Status)
		assert.NotNil(suite.T(), h.EndTime)
	}
}

// TestStateHistoryWithRetries tests retry tracking
func (suite *PostgresIntegrationTestSuite) TestStateHistoryWithRetries() {
	// Create execution
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-test-004",
		StateMachineID: "sm-test-001",
		Name:           "retry-test",
		Input:          map[string]interface{}{},
		Status:         "RUNNING",
		StartTime:      time.Now(),
		CurrentState:   "FlakyState",
	}
	require.NoError(suite.T(), suite.strategy.SaveExecution(suite.ctx, execRecord))

	// Simulate retries
	for retry := 0; retry < 3; retry++ {
		startTime := time.Now().Add(time.Duration(retry) * time.Second)
		endTime := startTime.Add(100 * time.Millisecond)

		status := "FAILED"
		var errorStr string
		if retry == 2 {
			status = "SUCCEEDED"
		} else {
			errorStr = "Transient error"
		}

		histRecord := &StateHistoryRecord{
			ID:                 fmt.Sprintf("hist-retry-%d", retry),
			ExecutionID:        "exec-test-004",
			ExecutionStartTime: execRecord.StartTime,
			StateName:          "FlakyState",
			StateType:          "Task",
			Input:              map[string]interface{}{},
			Status:             status,
			StartTime:          startTime,
			EndTime:            &endTime,
			Error:              errorStr,
			RetryCount:         retry,
			SequenceNumber:     0, // Same state, multiple attempts
		}

		err := suite.strategy.SaveStateHistory(suite.ctx, histRecord)
		require.NoError(suite.T(), err)
	}

	// Verify retry history
	history, err := suite.strategy.GetStateHistory(suite.ctx, "exec-test-004")
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), history, 3)

	// Check retry progression
	assert.Equal(suite.T(), 0, history[0].RetryCount)
	assert.Equal(suite.T(), "FAILED", history[0].Status)
	assert.Equal(suite.T(), 2, history[2].RetryCount)
	assert.Equal(suite.T(), "SUCCEEDED", history[2].Status)
}

// TestListExecutions tests listing with various filters
func (suite *PostgresIntegrationTestSuite) TestListExecutions() {
	// Create multiple executions
	baseTime := time.Now().Add(-1 * time.Hour)

	testCases := []struct {
		id     string
		smID   string
		status string
		offset time.Duration
	}{
		{"exec-list-001", "sm-order", "SUCCEEDED", 0},
		{"exec-list-002", "sm-order", "RUNNING", 10 * time.Minute},
		{"exec-list-003", "sm-payment", "SUCCEEDED", 20 * time.Minute},
		{"exec-list-004", "sm-order", "FAILED", 30 * time.Minute},
		{"exec-list-005", "sm-payment", "SUCCEEDED", 40 * time.Minute},
	}

	for _, tc := range testCases {
		record := &ExecutionRecord{
			ExecutionID:    tc.id,
			StateMachineID: tc.smID,
			Name:           "list-test",
			Input:          map[string]interface{}{},
			Status:         tc.status,
			StartTime:      baseTime.Add(tc.offset),
			CurrentState:   "SomeState",
		}
		require.NoError(suite.T(), suite.strategy.SaveExecution(suite.ctx, record))
	}

	// Test: List all
	executions, err := suite.strategy.ListExecutions(suite.ctx, nil, 100, 0)
	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), len(executions), 5)

	// Test: Filter by status
	executions, err = suite.strategy.ListExecutions(suite.ctx, map[string]interface{}{
		"status": "SUCCEEDED",
	}, 100, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))

	// Test: Filter by state machine ID
	executions, err = suite.strategy.ListExecutions(suite.ctx, map[string]interface{}{
		"state_machine_id": "sm-order",
	}, 100, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))

	// Test: Combined filters
	executions, err = suite.strategy.ListExecutions(suite.ctx, map[string]interface{}{
		"state_machine_id": "sm-order",
		"status":           "SUCCEEDED",
	}, 100, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(executions))

	// Test: Pagination
	executions, err = suite.strategy.ListExecutions(suite.ctx, nil, 2, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(executions))

	executions, err = suite.strategy.ListExecutions(suite.ctx, nil, 2, 2)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(executions))

	// Test: Time range filter
	executions, err = suite.strategy.ListExecutions(suite.ctx, map[string]interface{}{
		"start_after": baseTime.Add(15 * time.Minute),
	}, 100, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))
}

// TestDeleteExecution tests cascade deletion
func (suite *PostgresIntegrationTestSuite) TestDeleteExecution() {
	// Create execution with history
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-test-delete",
		StateMachineID: "sm-test-001",
		Name:           "delete-test",
		Input:          map[string]interface{}{},
		Status:         "SUCCEEDED",
		StartTime:      time.Now(),
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.strategy.SaveExecution(suite.ctx, execRecord))

	// Add state history
	histRecord := &StateHistoryRecord{
		ID:                 "hist-delete",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateName:          "State1",
		StateType:          "Task",
		Input:              map[string]interface{}{},
		Status:             "SUCCEEDED",
		StartTime:          time.Now(),
		SequenceNumber:     0,
	}
	require.NoError(suite.T(), suite.strategy.SaveStateHistory(suite.ctx, histRecord))

	// Delete execution
	err := suite.strategy.DeleteExecution(suite.ctx, "exec-test-delete")
	require.NoError(suite.T(), err)

	// Verify deletion
	_, err = suite.strategy.GetExecution(suite.ctx, "exec-test-delete")
	assert.Error(suite.T(), err)

	// Verify history was also deleted (cascade)
	history, err := suite.strategy.GetStateHistory(suite.ctx, "exec-test-delete")
	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), history)
}

// TestHealthCheck tests database health check
func (suite *PostgresIntegrationTestSuite) TestHealthCheck() {
	err := suite.strategy.HealthCheck(suite.ctx)
	assert.NoError(suite.T(), err)
}

// TestConcurrentWrites tests concurrent write operations
func (suite *PostgresIntegrationTestSuite) TestConcurrentWrites() {
	const numGoroutines = 10
	const writesPerGoroutine = 5

	errChan := make(chan error, numGoroutines*writesPerGoroutine)
	doneChan := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			for i := 0; i < writesPerGoroutine; i++ {
				record := &ExecutionRecord{
					ExecutionID:    fmt.Sprintf("exec-concurrent-%d-%d", goroutineID, i),
					StateMachineID: "sm-concurrent",
					Name:           "concurrent-test",
					Input:          map[string]interface{}{"g": goroutineID, "i": i},
					Status:         "SUCCEEDED",
					StartTime:      time.Now(),
					CurrentState:   "Final",
				}

				if err := suite.strategy.SaveExecution(suite.ctx, record); err != nil {
					errChan <- err
					return
				}
			}
			doneChan <- true
		}(g)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-doneChan
	}

	close(errChan)

	// Check for errors
	for err := range errChan {
		require.NoError(suite.T(), err)
	}

	// Verify all records were created
	executions, err := suite.strategy.ListExecutions(suite.ctx, map[string]interface{}{
		"state_machine_id": "sm-concurrent",
	}, 1000, 0)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), numGoroutines*writesPerGoroutine, len(executions))
}

// TestGetExecutionStats tests statistics generation
func (suite *PostgresIntegrationTestSuite) TestGetExecutionStats() {
	// Create test executions with different statuses
	statuses := []string{"SUCCEEDED", "SUCCEEDED", "FAILED", "RUNNING"}

	for i, status := range statuses {
		startTime := time.Now().Add(time.Duration(-i) * time.Minute)
		endTime := startTime.Add(30 * time.Second)

		record := &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-stats-%d", i),
			StateMachineID: "sm-stats-test",
			Name:           "stats-test",
			Input:          map[string]interface{}{},
			Status:         status,
			StartTime:      startTime,
			CurrentState:   "Final",
		}

		if status != "RUNNING" {
			record.EndTime = &endTime
		}

		require.NoError(suite.T(), suite.strategy.SaveExecution(suite.ctx, record))
	}

	// Get statistics
	stats, err := suite.strategy.GetExecutionStats(suite.ctx, "sm-stats-test")
	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), stats)

	byStatus := stats["by_status"].(map[string]interface{})

	// Check SUCCEEDED stats
	succeededStats := byStatus["SUCCEEDED"].(map[string]interface{})
	assert.Equal(suite.T(), 2, succeededStats["count"])
	assert.NotNil(suite.T(), succeededStats["avg_duration_seconds"])

	// Check FAILED stats
	failedStats := byStatus["FAILED"].(map[string]interface{})
	assert.Equal(suite.T(), 1, failedStats["count"])
}

// TestErrorHandling tests various error scenarios
func (suite *PostgresIntegrationTestSuite) TestErrorHandling() {
	// Test: Save with empty execution ID
	err := suite.strategy.SaveExecution(suite.ctx, &ExecutionRecord{
		ExecutionID: "",
	})
	assert.Error(suite.T(), err)

	// Test: Get non-existent execution
	_, err = suite.strategy.GetExecution(suite.ctx, "non-existent")
	assert.Error(suite.T(), err)

	// Test: Delete non-existent execution
	err = suite.strategy.DeleteExecution(suite.ctx, "non-existent")
	assert.Error(suite.T(), err)

	// Test: Save state history without execution
	err = suite.strategy.SaveStateHistory(suite.ctx, &StateHistoryRecord{
		ID:             "orphan-hist",
		ExecutionID:    "non-existent-exec",
		StateName:      "State1",
		StateType:      "Task",
		Status:         "SUCCEEDED",
		StartTime:      time.Now(),
		SequenceNumber: 0,
	})
	assert.Error(suite.T(), err) // Should fail due to foreign key constraint
}

// Run the test suite
func TestPostgresIntegrationSuite(t *testing.T) {

	if testing.Short() {
		t.Skip("Skipping PostgreSQL integration tests in short mode")
	}

	// Check if PostgreSQL is available
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
	}

	// Try to connect
	config := &Config{
		Strategy:      "postgres",
		ConnectionURL: connURL,
	}

	strategy, err := NewPostgresStrategy(config)
	if err != nil {
		t.Skip("PostgreSQL not available, skipping integration tests")
		return
	}
	err = strategy.Close()
	if err != nil {
		return
	}

	suite.Run(t, new(PostgresIntegrationTestSuite))
}

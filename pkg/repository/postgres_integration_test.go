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
	repository *PostgresRepository
	ctx        context.Context
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
	suite.repository, err = NewPostgresRepository(&config)
	require.NoError(suite.T(), err, "Failed to create PostgreSQL repository")

	// Initialize schema
	err = suite.repository.Initialize(suite.ctx)
	require.NoError(suite.T(), err, "Failed to initialize PostgreSQL schema")
}

// TearDownSuite runs once after all tests
func (suite *PostgresIntegrationTestSuite) TearDownSuite() {
	if suite.repository != nil {
		// Clean up test data
		suite.cleanupTestData()
		err := suite.repository.Close()
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
	_, err := suite.repository.db.ExecContext(suite.ctx, "TRUNCATE TABLE state_machines CASCADE")
	if err != nil {
		return
	}
	_, err = suite.repository.db.ExecContext(suite.ctx, "TRUNCATE TABLE state_history CASCADE")
	if err != nil {
		return
	}
	_, err = suite.repository.db.ExecContext(suite.ctx, "TRUNCATE TABLE executions CASCADE")
	if err != nil {
		return
	}
	_, err = suite.repository.db.ExecContext(suite.ctx, "TRUNCATE TABLE message_correlations CASCADE")
	if err != nil {
		return
	}
}

// TestSaveAndGetExecution tests basic save and retrieve operations
func (suite *PostgresIntegrationTestSuite) TestSaveAndGetExecution() {
	var now = time.Now()
	record := &ExecutionRecord{
		ExecutionID:    "exec-test-001",
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

	// Save execution
	err := suite.repository.SaveExecution(suite.ctx, record)
	require.NoError(suite.T(), err)

	// Retrieve execution
	retrieved, err := suite.repository.GetExecution(suite.ctx, "exec-test-001")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), record.ExecutionID, retrieved.ExecutionID)
	assert.Equal(suite.T(), record.Status, retrieved.Status)
	assert.Equal(suite.T(), record.StateMachineID, retrieved.StateMachineID)

	// Verify input was stored correctly
	assert.NotNil(suite.T(), retrieved.Input)
	inputMap := retrieved.Input.(map[string]interface{})
	assert.Equal(suite.T(), "12345", inputMap["orderId"])
}

func (suite *PostgresIntegrationTestSuite) TestSaveAndGetStateMachine() {
	record := &StateMachineRecord{
		ID:          "sm-test-001",
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
func (suite *PostgresIntegrationTestSuite) TestUpdateExecution() {
	var now = time.Now()
	record := &ExecutionRecord{
		ExecutionID:    "exec-test-002",
		StateMachineID: "sm-test-001",
		Name:           "update-test",
		Input:          map[string]interface{}{"test": "data"},
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "State1",
	}

	// Save initial
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))

	// Update
	record.Status = "SUCCEEDED"
	record.CurrentState = "State2"
	endTime := time.Now().Add(time.Minute)
	record.EndTime = &endTime
	record.Output = map[string]interface{}{"result": "success"}

	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))

	// Verify update
	retrieved, err := suite.repository.GetExecution(suite.ctx, "exec-test-002")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "SUCCEEDED", retrieved.Status)
	assert.Equal(suite.T(), "State2", retrieved.CurrentState)
	assert.NotNil(suite.T(), retrieved.EndTime)
	assert.NotNil(suite.T(), retrieved.Output)
}

// TestSaveStateHistory tests saving state execution history
func (suite *PostgresIntegrationTestSuite) TestSaveStateHistory() {
	var now = time.Now()
	// First create an execution
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-test-003",
		StateMachineID: "sm-test-001",
		Name:           "history-test",
		Input:          map[string]interface{}{},
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "State1",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, execRecord))

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
			StartTime:      &startTime,
			EndTime:        &endTime,
			SequenceNumber: i,
			RetryCount:     0,
		}

		err := suite.repository.SaveStateHistory(suite.ctx, histRecord)
		require.NoError(suite.T(), err)
	}

	// Retrieve history
	history, err := suite.repository.GetStateHistory(suite.ctx, "exec-test-003")
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
	var now = time.Now()
	// Create execution
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-test-004",
		StateMachineID: "sm-test-001",
		Name:           "retry-test",
		Input:          map[string]interface{}{},
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "FlakyState",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, execRecord))

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
			StartTime:          &startTime,
			EndTime:            &endTime,
			Error:              errorStr,
			RetryCount:         retry,
			SequenceNumber:     0, // Same state, multiple attempts
		}

		err := suite.repository.SaveStateHistory(suite.ctx, histRecord)
		require.NoError(suite.T(), err)
	}

	// Verify retry history
	history, err := suite.repository.GetStateHistory(suite.ctx, "exec-test-004")
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
		var startTime = baseTime.Add(tc.offset)
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

	// Test: List all
	executions, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:  100,
		Offset: 0,
	})
	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), len(executions), 5)

	// Test: Filter by status
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:  100,
		Offset: 0,
		Status: "SUCCEEDED",
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))

	// Test: Filter by state machine ID
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-order",
		Limit:          100,
		Offset:         0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))

	// Test: Combined filters
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-order",
		Status:         "SUCCEEDED",
		Limit:          100,
		Offset:         0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 1, len(executions))

	// Test: Pagination
	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:  2,
		Offset: 0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(executions))

	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:  2,
		Offset: 0,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 2, len(executions))

	// Test: Time range filter
	timeLapse := baseTime.Add(15 * time.Minute)

	executions, err = suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		Limit:      3,
		Offset:     0,
		StartAfter: timeLapse,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(executions))
}

// TestDeleteExecution tests cascade deletion
func (suite *PostgresIntegrationTestSuite) TestDeleteExecution() {
	// Create execution with history
	startTime := time.Now()
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-test-delete",
		StateMachineID: "sm-test-001",
		Name:           "delete-test",
		Input:          map[string]interface{}{},
		Status:         "SUCCEEDED",
		StartTime:      &startTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, execRecord))

	// Add state history
	startTimeHistory := time.Now()
	histRecord := &StateHistoryRecord{
		ID:                 "hist-delete",
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

	// Delete execution
	err := suite.repository.DeleteExecution(suite.ctx, "exec-test-delete")
	require.NoError(suite.T(), err)

	// Verify deletion
	_, err = suite.repository.GetExecution(suite.ctx, "exec-test-delete")
	assert.Error(suite.T(), err)

	// Verify history was also deleted (cascade)
	history, err := suite.repository.GetStateHistory(suite.ctx, "exec-test-delete")
	require.NoError(suite.T(), err)
	assert.Empty(suite.T(), history)
}

// TestHealthCheck tests database health check
func (suite *PostgresIntegrationTestSuite) TestHealthCheck() {
	err := suite.repository.HealthCheck(suite.ctx)
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
				startTime := time.Now()
				record := &ExecutionRecord{
					ExecutionID:    fmt.Sprintf("exec-concurrent-%d-%d", goroutineID, i),
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
	executions, err := suite.repository.ListExecutions(suite.ctx, &ExecutionFilter{
		StateMachineID: "sm-concurrent",
		Limit:          1000,
		Offset:         0,
	})
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
			StartTime:      &startTime,
			CurrentState:   "Final",
		}

		if status != "RUNNING" {
			record.EndTime = &endTime
		}

		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, record))
	}

	// Get statistics
	stats, err := suite.repository.GetExecutionStats(suite.ctx, "sm-stats-test")
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
	err := suite.repository.SaveExecution(suite.ctx, &ExecutionRecord{
		ExecutionID: "",
	})
	assert.Error(suite.T(), err)

	// Test: Get non-existent execution
	_, err = suite.repository.GetExecution(suite.ctx, "non-existent")
	assert.Error(suite.T(), err)

	// Test: Delete non-existent execution
	err = suite.repository.DeleteExecution(suite.ctx, "non-existent")
	assert.Error(suite.T(), err)

	var now = time.Now()
	// Test: Save state history without execution
	err = suite.repository.SaveStateHistory(suite.ctx, &StateHistoryRecord{
		ID:             "orphan-hist",
		ExecutionID:    "non-existent-exec",
		StateName:      "State1",
		StateType:      "Task",
		Status:         "SUCCEEDED",
		StartTime:      &now,
		SequenceNumber: 0,
	})
	assert.Error(suite.T(), err) // Should fail due to foreign key constraint
}

func (suite *PostgresIntegrationTestSuite) TestMessageCorrelationLifecycle() {
	// First, we need an execution because message_correlations has a FK to executions
	now := time.Now().UTC().Truncate(time.Second)
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-msg-test-001",
		StateMachineID: "sm-test-001",
		Name:           "msg-test-execution",
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "WaitState",
	}
	err := suite.repository.SaveExecution(suite.ctx, execRecord)
	require.NoError(suite.T(), err)

	// 1. Test SaveMessageCorrelation
	corrRecord := &MessageCorrelationRecord{
		ID:                 "corr-001",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateMachineID:     execRecord.StateMachineID,
		StateName:          "WaitState",
		CorrelationKey:     "orderId",
		CorrelationValue:   "12345",
		CreatedAt:          time.Now().Unix(),
		Status:             "WAITING",
	}

	err = suite.repository.SaveMessageCorrelation(suite.ctx, corrRecord)
	require.NoError(suite.T(), err)

	// 2. Test GetMessageCorrelation
	retrieved, err := suite.repository.GetMessageCorrelation(suite.ctx, "corr-001")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), corrRecord.ID, retrieved.ID)
	assert.Equal(suite.T(), corrRecord.CorrelationValue, retrieved.CorrelationValue)
	assert.Equal(suite.T(), "WAITING", retrieved.Status)
	assert.NotNil(suite.T(), retrieved.ExecutionStartTime)
	assert.WithinDuration(suite.T(), *execRecord.StartTime, *retrieved.ExecutionStartTime, time.Second)

	// 3. Test UpdateCorrelationStatus
	err = suite.repository.UpdateCorrelationStatus(suite.ctx, "corr-001", "RECEIVED")
	require.NoError(suite.T(), err)

	retrieved, err = suite.repository.GetMessageCorrelation(suite.ctx, "corr-001")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "RECEIVED", retrieved.Status)

	// 4. Test FindWaitingCorrelations
	// Add another waiting correlation
	corrRecord2 := &MessageCorrelationRecord{
		ID:                 "corr-002",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateMachineID:     execRecord.StateMachineID,
		StateName:          "WaitState",
		CorrelationKey:     "userId",
		CorrelationValue:   "user-1",
		CreatedAt:          time.Now().Unix(),
		Status:             "WAITING",
	}
	err = suite.repository.SaveMessageCorrelation(suite.ctx, corrRecord2)
	require.NoError(suite.T(), err)

	filter := &MessageCorrelationFilter{
		CorrelationKey: "userId",
		Status:         "WAITING",
	}
	found, err := suite.repository.FindWaitingCorrelations(suite.ctx, filter)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), found, 1)
	assert.Equal(suite.T(), "corr-002", found[0].ID)
	assert.NotNil(suite.T(), found[0].ExecutionStartTime)

	// 5. Test ListTimedOutCorrelations
	timeoutAt := time.Now().Unix() - 10 // 10 seconds ago
	corrRecord3 := &MessageCorrelationRecord{
		ID:                 "corr-003",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateMachineID:     execRecord.StateMachineID,
		StateName:          "WaitState",
		CorrelationKey:     "timeoutKey",
		CorrelationValue:   "val",
		CreatedAt:          time.Now().Unix() - 20,
		TimeoutAt:          &timeoutAt,
		Status:             "WAITING",
	}
	err = suite.repository.SaveMessageCorrelation(suite.ctx, corrRecord3)
	require.NoError(suite.T(), err)

	timedOut, err := suite.repository.ListTimedOutCorrelations(suite.ctx, time.Now().Unix())
	require.NoError(suite.T(), err)
	assert.True(suite.T(), len(timedOut) >= 1)

	foundTimedOut := false
	for _, t := range timedOut {
		if t.ID == "corr-003" {
			foundTimedOut = true
			break
		}
	}
	assert.True(suite.T(), foundTimedOut)

	// 6. Test DeleteMessageCorrelation
	err = suite.repository.DeleteMessageCorrelation(suite.ctx, "corr-001")
	require.NoError(suite.T(), err)

	_, err = suite.repository.GetMessageCorrelation(suite.ctx, "corr-001")
	assert.Error(suite.T(), err)
}

func (suite *PostgresIntegrationTestSuite) TestFindWaitingCorrelations_Complexity() {
	// First, we need an execution
	now := time.Now().UTC().Truncate(time.Second)
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-msg-test-002",
		StateMachineID: "sm-test-002",
		Name:           "msg-test-execution-2",
		Status:         "RUNNING",
		StartTime:      &now,
		CurrentState:   "WaitState",
	}
	err := suite.repository.SaveExecution(suite.ctx, execRecord)
	require.NoError(suite.T(), err)

	// Add correlations with complex values (JSONB)
	val1 := map[string]interface{}{"a": 1.0, "b": "c"}
	corr1 := &MessageCorrelationRecord{
		ID:                 "corr-complex-1",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateMachineID:     execRecord.StateMachineID,
		StateName:          "WaitState",
		CorrelationKey:     "complex",
		CorrelationValue:   val1,
		CreatedAt:          time.Now().Unix(),
		Status:             "WAITING",
	}
	err = suite.repository.SaveMessageCorrelation(suite.ctx, corr1)
	require.NoError(suite.T(), err)

	// Filter by complex value
	filter := &MessageCorrelationFilter{
		CorrelationKey:   "complex",
		CorrelationValue: val1,
		Status:           "WAITING",
	}
	found, err := suite.repository.FindWaitingCorrelations(suite.ctx, filter)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), found, 1)
	assert.Equal(suite.T(), "corr-complex-1", found[0].ID)

	// Deserialize check
	retVal := found[0].CorrelationValue.(map[string]interface{})
	assert.Equal(suite.T(), 1.0, retVal["a"])
	assert.Equal(suite.T(), "c", retVal["b"])
}

// TestListExecutionIDs tests the ListExecutionIDs method
func (suite *PostgresIntegrationTestSuite) TestListExecutionIDs() {
	// Create multiple test executions
	executions := []*ExecutionRecord{
		{
			ExecutionID:    "exec-list-001",
			StateMachineID: "sm-list-test",
			Name:           "exec-001",
			Status:         "SUCCEEDED",
			StartTime:      timePtr(time.Now().Add(-5 * time.Hour)),
			CurrentState:   "Final",
		},
		{
			ExecutionID:    "exec-list-002",
			StateMachineID: "sm-list-test",
			Name:           "exec-002",
			Status:         "SUCCEEDED",
			StartTime:      timePtr(time.Now().Add(-4 * time.Hour)),
			CurrentState:   "Final",
		},
		{
			ExecutionID:    "exec-list-003",
			StateMachineID: "sm-list-test",
			Name:           "exec-003",
			Status:         "FAILED",
			StartTime:      timePtr(time.Now().Add(-3 * time.Hour)),
			CurrentState:   "ErrorState",
		},
		{
			ExecutionID:    "exec-list-004",
			StateMachineID: "sm-list-other",
			Name:           "exec-004",
			Status:         "SUCCEEDED",
			StartTime:      timePtr(time.Now().Add(-2 * time.Hour)),
			CurrentState:   "Final",
		},
		{
			ExecutionID:    "exec-list-005",
			StateMachineID: "sm-list-test",
			Name:           "exec-005",
			Status:         "SUCCEEDED",
			StartTime:      timePtr(time.Now().Add(-1 * time.Hour)),
			CurrentState:   "Final",
		},
	}

	// Save all executions
	for _, exec := range executions {
		err := suite.repository.SaveExecution(suite.ctx, exec)
		require.NoError(suite.T(), err)
	}

	// Test 1: List all execution IDs
	suite.Run("ListAll", func() {
		ids, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{})
		require.NoError(suite.T(), err)
		assert.GreaterOrEqual(suite.T(), len(ids), 5)
	})

	// Test 2: Filter by state machine ID
	suite.Run("FilterByStateMachineID", func() {
		ids, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-list-test",
		})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 4, len(ids))
		// Should include: exec-list-001, 002, 003, 005
		assert.Contains(suite.T(), ids, "exec-list-001")
		assert.Contains(suite.T(), ids, "exec-list-002")
		assert.Contains(suite.T(), ids, "exec-list-003")
		assert.Contains(suite.T(), ids, "exec-list-005")
		assert.NotContains(suite.T(), ids, "exec-list-004")
	})

	// Test 3: Filter by status
	suite.Run("FilterByStatus", func() {
		ids, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-list-test",
			Status:         "SUCCEEDED",
		})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 3, len(ids))
		// Should include: exec-list-001, 002, 005
		assert.Contains(suite.T(), ids, "exec-list-001")
		assert.Contains(suite.T(), ids, "exec-list-002")
		assert.Contains(suite.T(), ids, "exec-list-005")
		assert.NotContains(suite.T(), ids, "exec-list-003") // FAILED
	})

	// Test 4: Use limit
	suite.Run("WithLimit", func() {
		ids, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-list-test",
			Limit:          2,
		})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 2, len(ids))
	})

	// Test 5: Use offset
	suite.Run("WithOffset", func() {
		// Get all IDs first
		allIDs, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-list-test",
		})
		require.NoError(suite.T(), err)

		// Get with offset
		idsWithOffset, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-list-test",
			Offset:         2,
		})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), len(allIDs)-2, len(idsWithOffset))
	})

	// Test 6: Time range filter
	suite.Run("FilterByTimeRange", func() {
		ids, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{
			StateMachineID: "sm-list-test",
			StartAfter:     time.Now().Add(-3*time.Hour - 30*time.Minute),
			StartBefore:    time.Now().Add(-30 * time.Minute),
		})
		require.NoError(suite.T(), err)
		// Should include: exec-list-003 and exec-list-005
		assert.GreaterOrEqual(suite.T(), len(ids), 2)
		assert.Contains(suite.T(), ids, "exec-list-003")
		assert.Contains(suite.T(), ids, "exec-list-005")
	})

	// Test 7: Empty result
	suite.Run("EmptyResult", func() {
		ids, err := suite.repository.ListExecutionIDs(suite.ctx, &ExecutionFilter{
			StateMachineID: "non-existent-sm",
		})
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 0, len(ids))
	})
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

	strategy, err := NewPostgresRepository(config)
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

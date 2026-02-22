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
	// Safe because we're using a dedicated test database.
	// Truncate in FK-safe order.
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE state_machines CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE state_history CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE executions CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE execution_statistics CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE message_correlations CASCADE").Error
	_ = suite.repository.db.WithContext(suite.ctx).Exec("TRUNCATE TABLE linked_executions CASCADE").Error
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
	assert.Equal(suite.T(), 3, len(executions))
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

// TestMessageCorrelationLifecycle tests the lifecycle of message correlations
func (suite *GormPostgresIntegrationTestSuite) TestMessageCorrelationLifecycle() {
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)

	// 1. Create an execution that will be waiting for a message
	execRecord := &ExecutionRecord{
		ExecutionID:    "exec-msg-gorm-001",
		StateMachineID: "sm-msg-test",
		Name:           "msg-test",
		Status:         "PAUSED",
		StartTime:      &startTime,
		CurrentState:   "WaitForPayment",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, execRecord))

	// 2. Save a message correlation
	corrRecord := &MessageCorrelationRecord{
		ID:                 "corr-gorm-001",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateMachineID:     execRecord.StateMachineID,
		StateName:          "WaitForPayment",
		CorrelationKey:     "orderId",
		CorrelationValue:   "ORD-12345",
		CreatedAt:          now.Unix(),
		Status:             "WAITING",
	}
	require.NoError(suite.T(), suite.repository.SaveMessageCorrelation(suite.ctx, corrRecord))

	// 3. Find waiting correlations
	filter := &MessageCorrelationFilter{
		CorrelationKey:   "orderId",
		CorrelationValue: "ORD-12345",
		Status:           "WAITING",
	}
	correlations, err := suite.repository.FindWaitingCorrelations(suite.ctx, filter)
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), correlations, 1)
	assert.Equal(suite.T(), corrRecord.ID, correlations[0].ID)

	// 4. Update status to RECEIVED
	err = suite.repository.UpdateCorrelationStatus(suite.ctx, corrRecord.ID, "RECEIVED")
	require.NoError(suite.T(), err)

	// 5. Verify status updated
	updated, err := suite.repository.GetMessageCorrelation(suite.ctx, corrRecord.ID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "RECEIVED", updated.Status)

	// 6. Test timeout listing
	timeoutAt := now.Add(-10 * time.Minute).Unix()
	timedOutRecord := &MessageCorrelationRecord{
		ID:                 "corr-gorm-timeout",
		ExecutionID:        execRecord.ExecutionID,
		ExecutionStartTime: execRecord.StartTime,
		StateMachineID:     execRecord.StateMachineID,
		StateName:          "WaitForPayment",
		CorrelationKey:     "orderId",
		CorrelationValue:   "ORD-TIMEOUT",
		CreatedAt:          now.Add(-20 * time.Minute).Unix(),
		TimeoutAt:          &timeoutAt,
		Status:             "WAITING",
	}
	require.NoError(suite.T(), suite.repository.SaveMessageCorrelation(suite.ctx, timedOutRecord))

	timedOut, err := suite.repository.ListTimedOutCorrelations(suite.ctx, now.Unix())
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), timedOut)

	found := false
	for _, c := range timedOut {
		if c.ID == timedOutRecord.ID {
			found = true
			break
		}
	}
	assert.True(suite.T(), found, "Expected to find timed out correlation")

	// 7. Delete correlation
	require.NoError(suite.T(), suite.repository.DeleteMessageCorrelation(suite.ctx, corrRecord.ID))
	_, err = suite.repository.GetMessageCorrelation(suite.ctx, corrRecord.ID)
	assert.Error(suite.T(), err)
}

// TestListExecutionIDs tests the ListExecutionIDs method
func (suite *GormPostgresIntegrationTestSuite) TestListExecutionIDs() {
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
func TestGormPostgresIntegrationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL integration tests in short mode")
	}

	connURL := os.Getenv("POSTGRES_TEST_URL_GORM")
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

// TestSaveAndGetLinkedExecution tests saving and retrieving linked execution records
func (suite *GormPostgresIntegrationTestSuite) TestSaveAndGetLinkedExecution() {
	// Create source execution first
	sourceStartTime := time.Now().Add(-1 * time.Hour)
	sourceExec := &ExecutionRecord{
		ExecutionID:    "exec-source-001",
		StateMachineID: "sm-source",
		Name:           "source-execution",
		Input:          map[string]interface{}{"data": "source"},
		Status:         "SUCCEEDED",
		StartTime:      &sourceStartTime,
		CurrentState:   "Final",
		Output:         map[string]interface{}{"result": "success"},
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, sourceExec))

	// Create target execution
	targetStartTime := time.Now()
	targetExec := &ExecutionRecord{
		ExecutionID:    "exec-target-001",
		StateMachineID: "sm-target",
		Name:           "target-execution",
		Input:          map[string]interface{}{"data": "target"},
		Status:         "RUNNING",
		StartTime:      &targetStartTime,
		CurrentState:   "Processing",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, targetExec))

	// Create linked execution record
	linkedExec := &LinkedExecutionRecord{
		ID:                     "link-001",
		SourceStateMachineID:   "sm-source",
		SourceExecutionID:      "exec-source-001",
		SourceStateName:        "ProcessData",
		InputTransformerName:   "custom_transformer",
		TargetStateMachineName: "TargetStateMachine",
		TargetExecutionID:      "exec-target-001",
		CreatedAt:              time.Now().UTC(),
	}

	// Save linked execution
	err := suite.repository.SaveLinkedExecution(suite.ctx, linkedExec)
	require.NoError(suite.T(), err)

	// Verify it was saved by querying directly
	var result LinkedExecutionModel
	err = suite.repository.db.WithContext(suite.ctx).
		Where("id = ?", "link-001").
		First(&result).Error
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "sm-source", result.SourceStateMachineID)
	assert.Equal(suite.T(), "exec-source-001", result.SourceExecutionID)
	assert.Equal(suite.T(), "ProcessData", result.SourceStateName)
	assert.Equal(suite.T(), "custom_transformer", result.InputTransformerName)
	assert.Equal(suite.T(), "TargetStateMachine", result.TargetStateMachineName)
	assert.Equal(suite.T(), "exec-target-001", result.TargetExecutionID)
}

// TestSaveLinkedExecutionIdempotent tests that saving the same linked execution twice is idempotent
func (suite *GormPostgresIntegrationTestSuite) TestSaveLinkedExecutionIdempotent() {
	// Create executions
	sourceStartTime := time.Now().Add(-1 * time.Hour)
	sourceExec := &ExecutionRecord{
		ExecutionID:    "exec-source-002",
		StateMachineID: "sm-source",
		Name:           "source-execution",
		Input:          map[string]interface{}{"data": "source"},
		Status:         "SUCCEEDED",
		StartTime:      &sourceStartTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, sourceExec))

	targetStartTime := time.Now()
	targetExec := &ExecutionRecord{
		ExecutionID:    "exec-target-002",
		StateMachineID: "sm-target",
		Name:           "target-execution",
		Input:          map[string]interface{}{"data": "target"},
		Status:         "RUNNING",
		StartTime:      &targetStartTime,
		CurrentState:   "Processing",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, targetExec))

	linkedExec := &LinkedExecutionRecord{
		ID:                     "link-002",
		SourceStateMachineID:   "sm-source",
		SourceExecutionID:      "exec-source-002",
		SourceStateName:        "",
		InputTransformerName:   "",
		TargetStateMachineName: "TargetStateMachine",
		TargetExecutionID:      "exec-target-002",
		CreatedAt:              time.Now().UTC(),
	}

	// Save first time
	err := suite.repository.SaveLinkedExecution(suite.ctx, linkedExec)
	require.NoError(suite.T(), err)

	// Save second time (should not error due to ON CONFLICT DO NOTHING)
	err = suite.repository.SaveLinkedExecution(suite.ctx, linkedExec)
	require.NoError(suite.T(), err)

	// Verify only one record exists
	var count int64
	err = suite.repository.db.WithContext(suite.ctx).
		Model(&LinkedExecutionModel{}).
		Where("id = ?", "link-002").
		Count(&count).Error
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), count)
}

// TestSaveLinkedExecutionWithOptionalFields tests linked execution with empty optional fields
func (suite *GormPostgresIntegrationTestSuite) TestSaveLinkedExecutionWithOptionalFields() {
	// Create executions
	sourceStartTime := time.Now().Add(-1 * time.Hour)
	sourceExec := &ExecutionRecord{
		ExecutionID:    "exec-source-003",
		StateMachineID: "sm-source",
		Name:           "source-execution",
		Input:          map[string]interface{}{"data": "source"},
		Status:         "SUCCEEDED",
		StartTime:      &sourceStartTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, sourceExec))

	targetStartTime := time.Now()
	targetExec := &ExecutionRecord{
		ExecutionID:    "exec-target-003",
		StateMachineID: "sm-target",
		Name:           "target-execution",
		Input:          map[string]interface{}{"data": "target"},
		Status:         "RUNNING",
		StartTime:      &targetStartTime,
		CurrentState:   "Processing",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, targetExec))

	// Linked execution with optional fields empty
	linkedExec := &LinkedExecutionRecord{
		ID:                     "link-003",
		SourceStateMachineID:   "sm-source",
		SourceExecutionID:      "exec-source-003",
		SourceStateName:        "", // Optional
		InputTransformerName:   "", // Optional
		TargetStateMachineName: "TargetStateMachine",
		TargetExecutionID:      "exec-target-003",
		CreatedAt:              time.Now().UTC(),
	}

	err := suite.repository.SaveLinkedExecution(suite.ctx, linkedExec)
	require.NoError(suite.T(), err)

	// Verify it was saved with empty optional fields
	var result LinkedExecutionModel
	err = suite.repository.db.WithContext(suite.ctx).
		Where("id = ?", "link-003").
		First(&result).Error
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), "", result.SourceStateName)
	assert.Equal(suite.T(), "", result.InputTransformerName)
}

// TestQueryLinkedExecutionsBySource tests querying linked executions by source
func (suite *GormPostgresIntegrationTestSuite) TestQueryLinkedExecutionsBySource() {
	// Create multiple linked executions from same source
	sourceStartTime := time.Now().Add(-1 * time.Hour)
	sourceExec := &ExecutionRecord{
		ExecutionID:    "exec-source-004",
		StateMachineID: "sm-source",
		Name:           "source-execution",
		Input:          map[string]interface{}{"data": "source"},
		Status:         "SUCCEEDED",
		StartTime:      &sourceStartTime,
		CurrentState:   "Final",
	}
	require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, sourceExec))

	// Create 3 target executions linked to the same source
	for i := 1; i <= 3; i++ {
		targetStartTime := time.Now()
		targetExec := &ExecutionRecord{
			ExecutionID:    fmt.Sprintf("exec-target-00%d", 3+i),
			StateMachineID: "sm-target",
			Name:           fmt.Sprintf("target-execution-%d", i),
			Input:          map[string]interface{}{"data": "target"},
			Status:         "RUNNING",
			StartTime:      &targetStartTime,
			CurrentState:   "Processing",
		}
		require.NoError(suite.T(), suite.repository.SaveExecution(suite.ctx, targetExec))

		linkedExec := &LinkedExecutionRecord{
			ID:                     fmt.Sprintf("link-00%d", 3+i),
			SourceStateMachineID:   "sm-source",
			SourceExecutionID:      "exec-source-004",
			SourceStateName:        "ProcessData",
			InputTransformerName:   "",
			TargetStateMachineName: "TargetStateMachine",
			TargetExecutionID:      fmt.Sprintf("exec-target-00%d", 3+i),
			CreatedAt:              time.Now().UTC(),
		}
		require.NoError(suite.T(), suite.repository.SaveLinkedExecution(suite.ctx, linkedExec))
	}

	// Query by source execution ID
	var links []LinkedExecutionModel
	err := suite.repository.db.WithContext(suite.ctx).
		Where("source_execution_id = ?", "exec-source-004").
		Find(&links).Error
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 3, len(links))

	// Verify all have the same source
	for _, link := range links {
		assert.Equal(suite.T(), "exec-source-004", link.SourceExecutionID)
		assert.Equal(suite.T(), "sm-source", link.SourceStateMachineID)
	}
}

// Helper function to create time pointers
func timePtr(t time.Time) *time.Time {
	return &t
}

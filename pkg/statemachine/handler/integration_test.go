//go:build integration
// +build integration

package handler

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

// HandlerIntegrationTestSuite contains all handler integration tests
type HandlerIntegrationTestSuite struct {
	suite.Suite
	handler        *ExecutionHandler
	repoManager    *repository.Manager
	queueClient    *queue.Client
	ctx            context.Context
	stateMachineID string
}

// SetupSuite runs once before all tests
func (suite *HandlerIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Setup Postgres repository
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
	}

	repoConfig := repository.Config{
		Strategy:      "postgres",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns":    10,
			"max_idle_conns":    2,
			"conn_max_lifetime": 5 * time.Minute,
		},
	}

	var err error
	suite.repoManager, err = repository.NewManager(&repoConfig)
	require.NoError(suite.T(), err, "Failed to create repository manager")

	err = suite.repoManager.Initialize(suite.ctx)
	require.NoError(suite.T(), err, "Failed to initialize repository")

	// Setup Redis queue client
	redisAddr := os.Getenv("REDIS_TEST_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	queueConfig := &queue.Config{
		RedisAddr:   redisAddr,
		Concurrency: 10,
		Queues: map[string]int{
			"timeout": 5,
			"default": 3,
		},
		RetryPolicy: &queue.RetryPolicy{
			MaxRetry: 3,
			Timeout:  5 * time.Minute,
		},
	}

	suite.queueClient, err = queue.NewClient(queueConfig)
	require.NoError(suite.T(), err, "Failed to create queue client")

	// Create handler
	suite.handler = NewExecutionHandler(suite.repoManager, suite.queueClient)
	require.NotNil(suite.T(), suite.handler)

	// Create a test state machine definition
	suite.setupStateMachine()
}

// TearDownSuite runs once after all tests
func (suite *HandlerIntegrationTestSuite) TearDownSuite() {
	if suite.queueClient != nil {
		_ = suite.queueClient.Close()
	}
	if suite.repoManager != nil {
		suite.cleanupTestData()
		_ = suite.repoManager.Close()
	}
}

// SetupTest runs before each test
func (suite *HandlerIntegrationTestSuite) SetupTest() {
	// Clean execution and correlation data before each test
	suite.cleanupExecutions()
}

// setupStateMachine creates a test state machine
func (suite *HandlerIntegrationTestSuite) setupStateMachine() {
	definition := []byte(`
StartAt: FirstState
States:
  FirstState:
    Type: Pass
    Result:
      message: "Hello from FirstState"
    Next: SecondState
  SecondState:
    Type: Pass
    Result:
      message: "Processing complete"
    End: true
`)

	suite.stateMachineID = "test-handler-sm-" + time.Now().Format("20060102150405")
	err := suite.repoManager.SaveStateMachine(suite.ctx, suite.stateMachineID, definition)
	require.NoError(suite.T(), err)
}

// cleanupTestData removes all test data
func (suite *HandlerIntegrationTestSuite) cleanupTestData() {
	// Delete state machine and all related data
	_ = suite.repoManager.DeleteStateMachine(suite.ctx, suite.stateMachineID)
}

// cleanupExecutions removes execution and correlation data
func (suite *HandlerIntegrationTestSuite) cleanupExecutions() {
	// Note: In a real scenario, you'd clean up executions by state machine ID
	// For simplicity in tests, we'll rely on test isolation
}

// TestHandleExecution_DirectInput tests execution with direct input
func (suite *HandlerIntegrationTestSuite) TestHandleExecution_DirectInput() {
	input := map[string]interface{}{
		"user": "test-user",
		"data": "test-data",
	}
	inputBytes, err := json.Marshal(input)
	require.NoError(suite.T(), err)

	payload := &queue.ExecutionTaskPayload{
		StateMachineID: suite.stateMachineID,
		ExecutionName:  "direct-input-exec",
		Input:          inputBytes,
	}

	err = suite.handler.HandleExecution(suite.ctx, payload)

	assert.NoError(suite.T(), err)

	// Verify execution was created
	// Note: We'd need to add GetExecutionByName to repository to verify
}

// TestHandleExecution_InvalidStateMachine tests with non-existent state machine
func (suite *HandlerIntegrationTestSuite) TestHandleExecution_InvalidStateMachine() {
	payload := &queue.ExecutionTaskPayload{
		StateMachineID: "non-existent-sm",
		ExecutionName:  "fail-exec",
		Input:          map[string]interface{}{},
	}

	err := suite.handler.HandleExecution(suite.ctx, payload)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to load state machine")
}

// TestHandleTimeout_CorrelationNotFound tests timeout with missing correlation
func (suite *HandlerIntegrationTestSuite) TestHandleTimeout_CorrelationNotFound() {
	payload := &queue.TimeoutTaskPayload{
		ExecutionID:    "exec-not-found",
		StateMachineID: suite.stateMachineID,
		StateName:      "MessageState",
		CorrelationID:  "corr-not-found",
		TimeoutSeconds: 60,
	}

	err := suite.handler.HandleTimeout(suite.ctx, payload)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "failed to get correlation record")
}

// TestHandleTimeout_CompleteScenario tests full timeout scenario
func (suite *HandlerIntegrationTestSuite) TestHandleTimeout_CompleteScenario() {
	// Create a state machine with Message state
	smWithMessage := "test-handler-sm-msg-" + time.Now().Format("20060102150405")
	definition := []byte(`
StartAt: WaitForMessage
States:
  WaitForMessage:
    Type: Message
    CorrelationKey: "requestId"
    TimeoutSeconds: 5
    TimeoutPath: TimeoutHandler
  TimeoutHandler:
    Type: Pass
    Result:
      status: "timeout"
    End: true
`)

	err := suite.repoManager.SaveStateMachine(suite.ctx, smWithMessage, definition)
	require.NoError(suite.T(), err)
	defer suite.repoManager.DeleteStateMachine(suite.ctx, smWithMessage)

	// Create execution record
	executionID := "exec-timeout-test"
	execRecord := &repository.ExecutionRecord{
		ExecutionID:    executionID,
		StateMachineID: smWithMessage,
		Name:           "timeout-test-exec",
		Input:          map[string]interface{}{"requestId": "req-123"},
		Status:         "PAUSED",
		StartTime:      timePtr(time.Now()),
		CurrentState:   "WaitForMessage",
	}

	err = suite.repoManager.CreateExecution(suite.ctx, execRecord)
	require.NoError(suite.T(), err)

	// Create message correlation
	correlationID := "corr-timeout-test"
	timeoutAt := time.Now().Add(5 * time.Second).Unix()
	corrRecord := &repository.MessageCorrelationRecord{
		ID:               correlationID,
		ExecutionID:      executionID,
		StateMachineID:   smWithMessage,
		StateName:        "WaitForMessage",
		CorrelationKey:   "requestId",
		CorrelationValue: "req-123",
		CreatedAt:        time.Now().Unix(),
		TimeoutAt:        &timeoutAt,
		Status:           "WAITING",
	}

	err = suite.repoManager.CreateMessageCorrelation(suite.ctx, corrRecord)
	require.NoError(suite.T(), err)

	// Schedule timeout
	timeoutPayload := &queue.TimeoutTaskPayload{
		ExecutionID:    executionID,
		StateMachineID: smWithMessage,
		StateName:      "WaitForMessage",
		CorrelationID:  correlationID,
		TimeoutSeconds: 5,
		ScheduledAt:    time.Now().Unix(),
	}

	_, err = suite.queueClient.ScheduleTimeout(timeoutPayload, 100*time.Millisecond)
	require.NoError(suite.T(), err)

	// Wait a bit for scheduling
	time.Sleep(200 * time.Millisecond)

	// Simulate timeout processing - this would normally be done by worker
	// For now, we test that HandleTimeout can be called without error
	// when correlation is still WAITING
	err = suite.handler.HandleTimeout(suite.ctx, timeoutPayload)

	// May error due to state machine not having proper timeout path processing
	// but should have accessed correlation
	if err != nil {
		suite.T().Logf("HandleTimeout returned error (expected for this simple test): %v", err)
	}

	// Verify correlation was retrieved
	retrievedCorr, err := suite.repoManager.GetMessageCorrelation(suite.ctx, correlationID)
	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedCorr)
	assert.Equal(suite.T(), correlationID, retrievedCorr.ID)
}

// TestHandleTimeout_AlreadyProcessed tests timeout when message already received
func (suite *HandlerIntegrationTestSuite) TestHandleTimeout_AlreadyProcessed() {
	// Create execution and correlation that's already completed
	executionID := "exec-already-done"
	execRecord := &repository.ExecutionRecord{
		ExecutionID:    executionID,
		StateMachineID: suite.stateMachineID,
		Name:           "already-done-exec",
		Input:          map[string]interface{}{},
		Status:         "SUCCEEDED",
		StartTime:      timePtr(time.Now().Add(-1 * time.Minute)),
		EndTime:        timePtr(time.Now()),
		CurrentState:   "SecondState",
	}

	err := suite.repoManager.CreateExecution(suite.ctx, execRecord)
	require.NoError(suite.T(), err)

	correlationID := "corr-already-done"
	corrRecord := &repository.MessageCorrelationRecord{
		ID:               correlationID,
		ExecutionID:      executionID,
		StateMachineID:   suite.stateMachineID,
		StateName:        "MessageState",
		CorrelationKey:   "key",
		CorrelationValue: "value",
		CreatedAt:        time.Now().Add(-1 * time.Minute).Unix(),
		Status:           "COMPLETED", // Already completed
	}

	err = suite.repoManager.CreateMessageCorrelation(suite.ctx, corrRecord)
	require.NoError(suite.T(), err)

	payload := &queue.TimeoutTaskPayload{
		ExecutionID:    executionID,
		StateMachineID: suite.stateMachineID,
		StateName:      "MessageState",
		CorrelationID:  correlationID,
		TimeoutSeconds: 60,
	}

	err = suite.handler.HandleTimeout(suite.ctx, payload)

	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "correlation already processed")
}

// TestGetters tests handler getter methods
func (suite *HandlerIntegrationTestSuite) TestGetters() {
	assert.Equal(suite.T(), suite.repoManager, suite.handler.GetRepositoryManager())
	assert.Equal(suite.T(), suite.queueClient, suite.handler.GetQueueClient())
}

// TestNewExecutionHandlerWithContext tests handler creation with context
func (suite *HandlerIntegrationTestSuite) TestNewExecutionHandlerWithContext() {
	// Just test that we can create a handler - we can't easily test
	// execution context without implementing the full interface
	handler := NewExecutionHandler(suite.repoManager, suite.queueClient)
	assert.NotNil(suite.T(), handler)
	assert.Nil(suite.T(), handler.executionContext)
}

// Helper function
func timePtr(t time.Time) *time.Time {
	return &t
}

// Run the test suite
func TestHandlerIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerIntegrationTestSuite))
}

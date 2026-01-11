//go:build integration
// +build integration

package queue

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// QueueIntegrationTestSuite contains all queue integration tests with real Redis
type QueueIntegrationTestSuite struct {
	suite.Suite
	client *Client
	config *Config
	ctx    context.Context
}

// SetupSuite runs once before all tests
func (suite *QueueIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Get Redis address from environment or use default
	redisAddr := os.Getenv("REDIS_TEST_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	suite.config = &Config{
		RedisAddr:   redisAddr,
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"timeout":  5,
			"default":  3,
			"low":      1,
		},
		RetryPolicy: &RetryPolicy{
			MaxRetry: 3,
			Timeout:  10 * time.Minute,
		},
	}

	var err error
	suite.client, err = NewClient(suite.config)
	require.NoError(suite.T(), err, "Failed to create queue client")
}

// TearDownSuite runs once after all tests
func (suite *QueueIntegrationTestSuite) TearDownSuite() {
	if suite.client != nil {
		err := suite.client.Close()
		assert.NoError(suite.T(), err)
	}
}

// SetupTest runs before each test
func (suite *QueueIntegrationTestSuite) SetupTest() {
	// Clean up any existing tasks from previous tests
	suite.cleanupTasks()
}

// cleanupTasks removes all tasks from queues
func (suite *QueueIntegrationTestSuite) cleanupTasks() {
	// List and delete tasks from all queues
	for queue := range suite.config.Queues {
		tasks, _ := suite.client.ListScheduledTasks(queue)
		for _, task := range tasks {
			_ = suite.client.inspector.DeleteTask(queue, task.ID)
		}
	}
}

// TestEnqueueExecution tests basic execution enqueuing
func (suite *QueueIntegrationTestSuite) TestEnqueueExecution() {
	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm-1",
		ExecutionName:  "test-execution-1",
		Input:          map[string]interface{}{"key": "value"},
	}

	info, err := suite.client.EnqueueExecution(payload)

	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), info)
	assert.Equal(suite.T(), TypeExecutionTask, info.Type)
	assert.Equal(suite.T(), "test-sm-1", info.Queue)
}

// TestEnqueueExecutionWithPriority tests execution enqueuing with priority
func (suite *QueueIntegrationTestSuite) TestEnqueueExecutionWithPriority() {
	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm-2",
		ExecutionName:  "critical-execution",
	}

	info, err := suite.client.EnqueueExecutionWithPriority(payload, "critical")

	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), info)
	assert.Equal(suite.T(), "critical", info.Queue)
}

// TestScheduleExecution tests scheduling an execution for later
func (suite *QueueIntegrationTestSuite) TestScheduleExecution() {
	payload := &ExecutionTaskPayload{
		StateMachineID: "test-sm-3",
		ExecutionName:  "scheduled-execution",
		IsTimeout:      true,
		CorrelationID:  "corr-schedule-1",
	}

	delay := 10 * time.Second
	info, err := suite.client.ScheduleExecution(payload, delay)

	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), info)
	assert.Equal(suite.T(), "timeout-corr-schedule-1", info.ID)
	assert.Equal(suite.T(), "default", info.Queue)
}

// TestScheduleTimeout tests scheduling a timeout task
func (suite *QueueIntegrationTestSuite) TestScheduleTimeout() {
	payload := &TimeoutTaskPayload{
		ExecutionID:    "exec-timeout-1",
		StateMachineID: "test-sm-4",
		StateName:      "MessageState",
		CorrelationID:  "corr-timeout-1",
		TimeoutSeconds: 60,
		ScheduledAt:    time.Now().Unix(),
	}

	delay := 60 * time.Second
	info, err := suite.client.ScheduleTimeout(payload, delay)

	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), info)
	assert.Equal(suite.T(), TypeTimeoutTask, info.Type)
	assert.Equal(suite.T(), "timeout", info.Queue)
	assert.Equal(suite.T(), "timeout-corr-timeout-1", info.ID)
	assert.Equal(suite.T(), 24*time.Hour, info.Retention)
}

// TestCancelTimeout tests canceling a scheduled timeout
func (suite *QueueIntegrationTestSuite) TestCancelTimeout() {
	// First, schedule a timeout
	payload := &TimeoutTaskPayload{
		ExecutionID:    "exec-cancel-1",
		StateMachineID: "test-sm-5",
		StateName:      "MessageState",
		CorrelationID:  "corr-cancel-1",
		TimeoutSeconds: 60,
		ScheduledAt:    time.Now().Unix(),
	}

	_, err := suite.client.ScheduleTimeout(payload, 30*time.Second)
	require.NoError(suite.T(), err)

	// Verify it was scheduled
	taskInfo, err := suite.client.GetTaskInfo("timeout", "timeout-corr-cancel-1")
	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), taskInfo)

	// Now cancel it
	cancelled, err := suite.client.CancelTimeout("corr-cancel-1")

	require.NoError(suite.T(), err)
	assert.True(suite.T(), cancelled)

	// Verify it's gone
	_, err = suite.client.GetTaskInfo("timeout", "timeout-corr-cancel-1")
	assert.Error(suite.T(), err) // Should not be found
}

// TestCancelTimeout_NonExistent tests canceling a non-existent timeout
func (suite *QueueIntegrationTestSuite) TestCancelTimeout_NonExistent() {
	cancelled, err := suite.client.CancelTimeout("non-existent-corr")

	require.NoError(suite.T(), err)
	assert.False(suite.T(), cancelled)
}

// TestListScheduledTasks tests listing scheduled tasks
func (suite *QueueIntegrationTestSuite) TestListScheduledTasks() {
	// Schedule multiple tasks
	for i := 0; i < 3; i++ {
		payload := &TimeoutTaskPayload{
			ExecutionID:    "exec-list-test",
			StateMachineID: "test-sm-6",
			StateName:      "MessageState",
			CorrelationID:  "corr-list-" + string(rune('a'+i)),
			TimeoutSeconds: 60,
			ScheduledAt:    time.Now().Unix(),
		}

		_, err := suite.client.ScheduleTimeout(payload, 30*time.Second)
		require.NoError(suite.T(), err)
	}

	// List scheduled tasks
	tasks, err := suite.client.ListScheduledTasks("timeout")

	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), len(tasks), 3)
}

// TestGetTaskInfo tests retrieving task information
func (suite *QueueIntegrationTestSuite) TestGetTaskInfo() {
	payload := &TimeoutTaskPayload{
		ExecutionID:    "exec-info-1",
		StateMachineID: "test-sm-7",
		StateName:      "MessageState",
		CorrelationID:  "corr-info-1",
		TimeoutSeconds: 60,
		ScheduledAt:    time.Now().Unix(),
	}

	schedInfo, err := suite.client.ScheduleTimeout(payload, 30*time.Second)
	require.NoError(suite.T(), err)

	// Get task info
	info, err := suite.client.GetTaskInfo("timeout", schedInfo.ID)

	require.NoError(suite.T(), err)
	assert.NotNil(suite.T(), info)
	assert.Equal(suite.T(), schedInfo.ID, info.ID)
	assert.Equal(suite.T(), "timeout", info.Queue)
}

// TestTimeoutScenario_MessageReceivedBeforeTimeout tests the scenario where
// a message is received before timeout triggers (timeout is cancelled)
func (suite *QueueIntegrationTestSuite) TestTimeoutScenario_MessageReceivedBeforeTimeout() {
	correlationID := "corr-msg-before-timeout"

	// Schedule timeout
	payload := &TimeoutTaskPayload{
		ExecutionID:    "exec-msg-before",
		StateMachineID: "test-sm-8",
		StateName:      "MessageState",
		CorrelationID:  correlationID,
		TimeoutSeconds: 30,
		ScheduledAt:    time.Now().Unix(),
	}

	_, err := suite.client.ScheduleTimeout(payload, 30*time.Second)
	require.NoError(suite.T(), err)

	// Simulate message received - cancel timeout
	cancelled, err := suite.client.CancelTimeout(correlationID)

	require.NoError(suite.T(), err)
	assert.True(suite.T(), cancelled, "Timeout should be cancelled when message received")

	// Verify task is no longer scheduled
	_, err = suite.client.GetTaskInfo("timeout", "timeout-"+correlationID)
	assert.Error(suite.T(), err, "Cancelled task should not be found")
}

// TestTimeoutScenario_MultipleTimeouts tests multiple concurrent timeouts
func (suite *QueueIntegrationTestSuite) TestTimeoutScenario_MultipleTimeouts() {
	timeouts := []struct {
		correlationID string
		delay         time.Duration
		shouldCancel  bool
	}{
		{"corr-multi-1", 5 * time.Second, false},
		{"corr-multi-2", 10 * time.Second, true}, // This one will be cancelled
		{"corr-multi-3", 15 * time.Second, false},
	}

	// Schedule all timeouts
	for _, to := range timeouts {
		payload := &TimeoutTaskPayload{
			ExecutionID:    "exec-multi",
			StateMachineID: "test-sm-9",
			StateName:      "MessageState",
			CorrelationID:  to.correlationID,
			TimeoutSeconds: int(to.delay.Seconds()),
			ScheduledAt:    time.Now().Unix(),
		}

		info, err := suite.client.ScheduleTimeout(payload, to.delay)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), "timeout-"+to.correlationID, info.ID)
	}

	// Cancel the middle one
	cancelled, err := suite.client.CancelTimeout("corr-multi-2")
	require.NoError(suite.T(), err)
	assert.True(suite.T(), cancelled)

	// Verify only 2 remain scheduled
	tasks, err := suite.client.ListScheduledTasks("timeout")
	require.NoError(suite.T(), err)

	// Count our test tasks
	count := 0
	for _, task := range tasks {
		if task.ID == "timeout-corr-multi-1" || task.ID == "timeout-corr-multi-3" {
			count++
		}
	}
	assert.Equal(suite.T(), 2, count, "Should have 2 scheduled timeouts after cancellation")
}

// TestTimeoutScenario_DifferentDelays tests timeouts with various delays
func (suite *QueueIntegrationTestSuite) TestTimeoutScenario_DifferentDelays() {
	testCases := []struct {
		name           string
		correlationID  string
		delay          time.Duration
		timeoutSeconds int
	}{
		{"ShortTimeout", "corr-short", 1 * time.Second, 1},
		{"MediumTimeout", "corr-medium", 30 * time.Second, 30},
		{"LongTimeout", "corr-long", 5 * time.Minute, 300},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			payload := &TimeoutTaskPayload{
				ExecutionID:    "exec-delays",
				StateMachineID: "test-sm-10",
				StateName:      "MessageState",
				CorrelationID:  tc.correlationID,
				TimeoutSeconds: tc.timeoutSeconds,
				ScheduledAt:    time.Now().Unix(),
			}

			info, err := suite.client.ScheduleTimeout(payload, tc.delay)

			require.NoError(t, err)
			assert.NotNil(t, info)
			assert.Equal(t, "timeout-"+tc.correlationID, info.ID)
			assert.Equal(t, "timeout", info.Queue)
		})
	}
}

// TestTimeoutScenario_HighVolume tests system under high volume
func (suite *QueueIntegrationTestSuite) TestTimeoutScenario_HighVolume() {
	numTimeouts := 50
	scheduledIDs := make([]string, 0, numTimeouts)

	// Schedule many timeouts
	for i := 0; i < numTimeouts; i++ {
		correlationID := "corr-volume-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		payload := &TimeoutTaskPayload{
			ExecutionID:    "exec-volume",
			StateMachineID: "test-sm-11",
			StateName:      "MessageState",
			CorrelationID:  correlationID,
			TimeoutSeconds: 60,
			ScheduledAt:    time.Now().Unix(),
		}

		info, err := suite.client.ScheduleTimeout(payload, time.Duration(i+1)*time.Second)
		require.NoError(suite.T(), err)
		scheduledIDs = append(scheduledIDs, info.ID)
	}

	// Verify all are scheduled
	tasks, err := suite.client.ListScheduledTasks("timeout")
	require.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), len(tasks), numTimeouts)

	// Cancel half of them
	for i := 0; i < numTimeouts/2; i++ {
		correlationID := "corr-volume-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		_, err := suite.client.CancelTimeout(correlationID)
		// Don't require no error as some might already be processed
		if err != nil {
			suite.T().Logf("Could not cancel %s: %v", correlationID, err)
		}
	}
}

// TestTimeoutScenario_ConcurrentOperations tests concurrent scheduling and cancellation
func (suite *QueueIntegrationTestSuite) TestTimeoutScenario_ConcurrentOperations() {
	done := make(chan bool, 10)

	// Concurrent scheduling
	for i := 0; i < 5; i++ {
		go func(idx int) {
			correlationID := "corr-concurrent-" + string(rune('a'+idx))
			payload := &TimeoutTaskPayload{
				ExecutionID:    "exec-concurrent",
				StateMachineID: "test-sm-12",
				StateName:      "MessageState",
				CorrelationID:  correlationID,
				TimeoutSeconds: 60,
				ScheduledAt:    time.Now().Unix(),
			}

			_, err := suite.client.ScheduleTimeout(payload, 30*time.Second)
			assert.NoError(suite.T(), err)
			done <- true
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < 5; i++ {
		<-done
	}

	// Concurrent cancellation of the same task
	correlationID := "corr-cancel-race"
	payload := &TimeoutTaskPayload{
		ExecutionID:    "exec-race",
		StateMachineID: "test-sm-13",
		StateName:      "MessageState",
		CorrelationID:  correlationID,
		TimeoutSeconds: 60,
		ScheduledAt:    time.Now().Unix(),
	}

	_, err := suite.client.ScheduleTimeout(payload, 30*time.Second)
	require.NoError(suite.T(), err)

	// Try to cancel concurrently
	results := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func() {
			cancelled, _ := suite.client.CancelTimeout(correlationID)
			results <- cancelled
		}()
	}

	// Collect results - only one should succeed
	successCount := 0
	for i := 0; i < 3; i++ {
		if <-results {
			successCount++
		}
	}
	assert.LessOrEqual(suite.T(), successCount, 1, "Only one cancellation should succeed")
}

// TestConfig runs all queue configuration tests
func (suite *QueueIntegrationTestSuite) TestConfig() {
	// Test default config
	defaultCfg := DefaultConfig()
	assert.NotNil(suite.T(), defaultCfg)
	assert.Equal(suite.T(), "localhost:6379", defaultCfg.RedisAddr)
	assert.Contains(suite.T(), defaultCfg.Queues, "timeout")

	// Test config validation
	validCfg := &Config{
		RedisAddr:   "localhost:6379",
		Concurrency: 5,
		Queues:      map[string]int{"default": 1},
	}
	err := validCfg.Validate()
	assert.NoError(suite.T(), err)
	assert.Contains(suite.T(), validCfg.Queues, "timeout") // Should be added automatically

	// Test invalid configs
	invalidCfgs := []struct {
		name   string
		config *Config
		errMsg string
	}{
		{
			"MissingRedisAddr",
			&Config{Concurrency: 5, Queues: map[string]int{"default": 1}},
			"redis address is required",
		},
		{
			"InvalidConcurrency",
			&Config{RedisAddr: "localhost:6379", Concurrency: 0, Queues: map[string]int{"default": 1}},
			"concurrency must be greater than 0",
		},
		{
			"NoQueues",
			&Config{RedisAddr: "localhost:6379", Concurrency: 5, Queues: map[string]int{}},
			"at least one queue must be configured",
		},
	}

	for _, tc := range invalidCfgs {
		suite.T().Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

// Run the test suite
func TestQueueIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(QueueIntegrationTestSuite))
}

package queue

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Client wraps asynq.Client for enqueuing state machine execution tasks
type Client struct {
	client      *asynq.Client
	inspector   *asynq.Inspector
	retryPolicy *RetryPolicy
}

// NewClient creates a new queue client for enqueuing tasks
func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := asynq.NewClient(config.GetRedisClientOpt())
	inspector := asynq.NewInspector(config.GetRedisClientOpt())

	return &Client{
		client:      client,
		inspector:   inspector,
		retryPolicy: config.RetryPolicy,
	}, nil
}

// EnqueueExecution enqueues a state machine execution task
func (c *Client) EnqueueExecution(payload *ExecutionTaskPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	task, err := NewExecutionTask(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Apply default options if none provided
	if len(opts) == 0 && c.retryPolicy != nil {
		opts = []asynq.Option{
			asynq.MaxRetry(c.retryPolicy.MaxRetry),
			asynq.Timeout(c.retryPolicy.Timeout),
			asynq.Queue(payload.StateMachineID),
		}
	}

	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	return info, nil
}

// EnqueueExecutionWithPriority enqueues a task with specific priority queue
func (c *Client) EnqueueExecutionWithPriority(payload *ExecutionTaskPayload, priority string) (*asynq.TaskInfo, error) {
	opts := []asynq.Option{
		asynq.Queue(priority),
	}

	if c.retryPolicy != nil {
		opts = append(opts,
			asynq.MaxRetry(c.retryPolicy.MaxRetry),
			asynq.Timeout(c.retryPolicy.Timeout),
		)
	}

	return c.EnqueueExecution(payload, opts...)
}

// ScheduleExecution schedules a state machine execution task for later
func (c *Client) ScheduleExecution(payload *ExecutionTaskPayload, delay time.Duration) (*asynq.TaskInfo, error) {
	task, err := NewExecutionTask(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	opts := []asynq.Option{
		asynq.ProcessIn(delay),
		asynq.Queue("default"),
		asynq.TaskID(fmt.Sprintf("timeout-%s", payload.CorrelationID)), // Use correlation ID for task cancellation
	}

	if c.retryPolicy != nil {
		opts = append(opts,
			asynq.MaxRetry(c.retryPolicy.MaxRetry),
			asynq.Timeout(c.retryPolicy.Timeout),
		)
	}

	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule task: %w", err)
	}

	return info, nil
}

// ScheduleTimeout schedules a timeout boundary event task
func (c *Client) ScheduleTimeout(payload *TimeoutTaskPayload, delay time.Duration) (*asynq.TaskInfo, error) {
	task, err := NewTimeoutTask(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create timeout task: %w", err)
	}

	opts := []asynq.Option{
		asynq.ProcessIn(delay),
		asynq.Queue("timeout"), // Dedicated queue for timeout tasks
		asynq.TaskID(fmt.Sprintf("timeout-%s", payload.CorrelationID)), // Unique ID for cancellation
		asynq.Retention(24 * time.Hour),                                // Keep completed tasks for 24 hours for debugging
	}

	if c.retryPolicy != nil {
		opts = append(opts,
			asynq.MaxRetry(1), // Timeouts should only retry once
			asynq.Timeout(c.retryPolicy.Timeout),
		)
	}

	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to schedule timeout task: %w", err)
	}

	return info, nil
}

// CancelTimeout attempts to cancel a scheduled timeout task
// Returns true if the task was successfully cancelled, false if it was already processed or not found
func (c *Client) CancelTimeout(correlationID string) (bool, error) {
	taskID := fmt.Sprintf("timeout-%s", correlationID)

	// Try to delete from scheduled queue
	err := c.inspector.DeleteTask("timeout", taskID)
	if err != nil {
		// Check if task not found or already processed
		if err == asynq.ErrTaskNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to cancel timeout task: %w", err)
	}

	return true, nil
}

// GetTaskInfo retrieves information about a task
func (c *Client) GetTaskInfo(queue, taskID string) (*asynq.TaskInfo, error) {
	info, err := c.inspector.GetTaskInfo(queue, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task info: %w", err)
	}
	return info, nil
}

// ListScheduledTasks lists all scheduled tasks in a queue
func (c *Client) ListScheduledTasks(queue string) ([]*asynq.TaskInfo, error) {
	tasks, err := c.inspector.ListScheduledTasks(queue)
	if err != nil {
		return nil, fmt.Errorf("failed to list scheduled tasks: %w", err)
	}
	return tasks, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	if err := c.client.Close(); err != nil {
		return err
	}
	if err := c.inspector.Close(); err != nil {
		return err
	}
	return nil
}

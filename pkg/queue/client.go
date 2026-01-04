package queue

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Client wraps asynq.Client for enqueuing state machine execution tasks
type Client struct {
	client      *asynq.Client
	retryPolicy *RetryPolicy
}

// NewClient creates a new queue client for enqueuing tasks
func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := asynq.NewClient(config.GetRedisClientOpt())

	return &Client{
		client:      client,
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
func (c *Client) ScheduleExecution(payload *ExecutionTaskPayload, processAt time.Time) (*asynq.TaskInfo, error) {
	task, err := NewExecutionTask(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	opts := []asynq.Option{
		asynq.ProcessAt(processAt),
		asynq.Queue("default"),
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

// Close closes the client connection
func (c *Client) Close() error {
	return c.client.Close()
}

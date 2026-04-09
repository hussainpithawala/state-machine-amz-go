package queue

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Config represents the queue configuration
type Config struct {
	RedisClientOpt *asynq.RedisClientOpt
	Concurrency    int
	Queues         map[string]int
	RetryPolicy    *RetryPolicy

	// GroupAggregation enables task aggregation for group-based processing
	GroupAggregation *GroupAggregationConfig
}

// GroupAggregationConfig configures task grouping for batch processing
type GroupAggregationConfig struct {
	// Enabled turns on group aggregation
	Enabled bool

	// GroupMaxSize triggers group processing when this many tasks are collected
	// If 0, defaults to asynq default (typically 100)
	GroupMaxSize int

	// GroupMaxDelay is the maximum time to wait before processing a group
	// since the first task was added. If 0, defaults to 5 minutes.
	GroupMaxDelay time.Duration

	// GroupGracePeriod is the time to wait after the last task before processing.
	// This resets with each new task. If 0, defaults to 15 seconds.
	GroupGracePeriod time.Duration
}

// RetryPolicy defines retry behavior for failed tasks
type RetryPolicy struct {
	MaxRetry int
	Timeout  time.Duration
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr:         "localhost:6379",
			Password:     "redispassword",
			DB:           0,
			DialTimeout:  10 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			PoolSize:     20,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6, // Highest priority
			"timeout":  5, // High priority for timeout events
			"default":  3, // Normal priority
			"low":      1, // Lowest priority
		},
		RetryPolicy: &RetryPolicy{
			MaxRetry: 3,
			Timeout:  10 * time.Minute,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.RedisClientOpt.Addr == "" {
		return fmt.Errorf("redis address is required")
	}
	if c.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than 0")
	}
	if len(c.Queues) == 0 {
		return fmt.Errorf("at least one queue must be configured")
	}
	// Ensure timeout queue exists
	if _, exists := c.Queues["timeout"]; !exists {
		c.Queues["timeout"] = 5 // Add with high priority
	}
	return nil
}

// GetRedisClientOpt returns asynq Redis client options
func (c *Config) GetRedisClientOpt() asynq.RedisClientOpt {
	return *c.RedisClientOpt
}

// GetServerConfig returns asynq server configuration
func (c *Config) GetServerConfig() asynq.Config {
	cfg := asynq.Config{
		Concurrency: c.Concurrency,
		Queues:      c.Queues,
	}

	// Apply group aggregation settings if enabled
	if c.GroupAggregation != nil && c.GroupAggregation.Enabled {
		cfg.GroupMaxSize = c.GroupAggregation.GroupMaxSize
		cfg.GroupMaxDelay = c.GroupAggregation.GroupMaxDelay
		cfg.GroupGracePeriod = c.GroupAggregation.GroupGracePeriod
		cfg.GroupAggregator = &ExecutionTaskAggregator{}
	}

	return cfg
}

package queue

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Config represents the queue configuration
type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	Concurrency   int
	Queues        map[string]int
	RetryPolicy   *RetryPolicy
}

// RetryPolicy defines retry behavior for failed tasks
type RetryPolicy struct {
	MaxRetry int
	Timeout  time.Duration
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		RedisAddr:     "localhost:6379",
		RedisPassword: "",
		RedisDB:       0,
		Concurrency:   10,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		RetryPolicy: &RetryPolicy{
			MaxRetry: 3,
			Timeout:  10 * time.Minute,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.RedisAddr == "" {
		return fmt.Errorf("redis address is required")
	}
	if c.Concurrency <= 0 {
		return fmt.Errorf("concurrency must be greater than 0")
	}
	if len(c.Queues) == 0 {
		return fmt.Errorf("at least one queue must be configured")
	}
	return nil
}

// GetRedisClientOpt returns asynq Redis client options
func (c *Config) GetRedisClientOpt() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     c.RedisAddr,
		Password: c.RedisPassword,
		DB:       c.RedisDB,
	}
}

// GetServerConfig returns asynq server configuration
func (c *Config) GetServerConfig() asynq.Config {
	return asynq.Config{
		Concurrency: c.Concurrency,
		Queues:      c.Queues,
	}
}

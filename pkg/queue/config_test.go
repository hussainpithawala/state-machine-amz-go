package queue

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "localhost:6379", config.RedisClientOpt.Addr)
	assert.Equal(t, "redispassword", config.RedisClientOpt.Password)
	assert.Equal(t, 0, config.RedisClientOpt.DB)
	assert.Equal(t, 10, config.Concurrency)
	assert.NotNil(t, config.Queues)
	assert.Equal(t, 6, config.Queues["critical"])
	assert.Equal(t, 5, config.Queues["timeout"])
	assert.Equal(t, 3, config.Queues["default"])
	assert.Equal(t, 1, config.Queues["low"])
	assert.NotNil(t, config.RetryPolicy)
	assert.Equal(t, 3, config.RetryPolicy.MaxRetry)
	assert.Equal(t, 10*time.Minute, config.RetryPolicy.Timeout)
}

func TestConfig_Validate_Success(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
			DB:   0,
		},
		Concurrency: 5,
		Queues: map[string]int{
			"default": 1,
		},
	}

	err := config.Validate()
	assert.NoError(t, err)
}

func TestConfig_Validate_MissingRedisAddr(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			DB: 0,
		},
		Concurrency: 5,
		Queues: map[string]int{
			"default": 1,
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis address is required")
}

func TestConfig_Validate_InvalidConcurrency(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
			DB:   0,
		},
		Concurrency: 0,
		Queues: map[string]int{
			"default": 1,
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "concurrency must be greater than 0")
}

func TestConfig_Validate_NegativeConcurrency(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
			DB:   0,
		},
		Concurrency: -1,
		Queues: map[string]int{
			"default": 1,
		},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "concurrency must be greater than 0")
}

func TestConfig_Validate_NoQueues(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
			DB:   0,
		},
		Concurrency: 5,
		Queues:      map[string]int{},
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one queue must be configured")
}

func TestConfig_Validate_NilQueues(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
			DB:   0,
		},
		Concurrency: 5,
		Queues:      nil,
	}

	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one queue must be configured")
}

func TestConfig_Validate_AddsTimeoutQueue(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
			DB:   0,
		},
		Concurrency: 5,
		Queues: map[string]int{
			"default": 3,
		},
	}

	err := config.Validate()
	assert.NoError(t, err)

	// Verify timeout queue was added
	priority, exists := config.Queues["timeout"]
	assert.True(t, exists)
	assert.Equal(t, 5, priority)
}

func TestConfig_Validate_PreservesExistingTimeoutQueue(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
			DB:   0,
		},
		Concurrency: 5,
		Queues: map[string]int{
			"default": 3,
			"timeout": 8, // Custom priority
		},
	}

	err := config.Validate()
	assert.NoError(t, err)

	// Verify timeout queue priority was not changed
	priority, exists := config.Queues["timeout"]
	assert.True(t, exists)
	assert.Equal(t, 8, priority)
}

func TestConfig_GetRedisClientOpt(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr:     "redis.example.com:6379",
			Password: "secret",
			DB:       2,
		},
	}

	opts := config.GetRedisClientOpt()

	assert.Equal(t, "redis.example.com:6379", opts.Addr)
	assert.Equal(t, "secret", opts.Password)
	assert.Equal(t, 2, opts.DB)
}

func TestConfig_GetRedisClientOptWithTls(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr:     "redis.example.com:6379",
			Password: "secret",
			DB:       2,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	opts := config.GetRedisClientOpt()

	assert.Equal(t, "redis.example.com:6379", opts.Addr)
	assert.Equal(t, "secret", opts.Password)
	assert.Equal(t, 2, opts.DB)
}

func TestConfig_GetServerConfig(t *testing.T) {
	queues := map[string]int{
		"critical": 6,
		"default":  3,
		"low":      1,
	}

	config := &Config{
		Concurrency: 15,
		Queues:      queues,
	}

	serverConfig := config.GetServerConfig()

	assert.Equal(t, 15, serverConfig.Concurrency)
	assert.Equal(t, queues, serverConfig.Queues)
}

func TestRetryPolicy(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetry: 5,
		Timeout:  30 * time.Second,
	}

	assert.Equal(t, 5, policy.MaxRetry)
	assert.Equal(t, 30*time.Second, policy.Timeout)
}

func TestConfig_WithCustomRetryPolicy(t *testing.T) {
	customPolicy := &RetryPolicy{
		MaxRetry: 10,
		Timeout:  5 * time.Minute,
	}

	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
		},
		Concurrency: 5,
		Queues: map[string]int{
			"default": 1,
		},
		RetryPolicy: customPolicy,
	}

	err := config.Validate()
	require.NoError(t, err)

	assert.Equal(t, 10, config.RetryPolicy.MaxRetry)
	assert.Equal(t, 5*time.Minute, config.RetryPolicy.Timeout)
}

func TestConfig_WithoutRetryPolicy(t *testing.T) {
	config := &Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
		},
		Concurrency: 5,
		Queues: map[string]int{
			"default": 1,
		},
		RetryPolicy: nil,
	}

	err := config.Validate()
	assert.NoError(t, err)
	assert.Nil(t, config.RetryPolicy)
}

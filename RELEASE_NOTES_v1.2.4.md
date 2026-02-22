# Release Notes - v1.2.4

## 🔒 TLS/SSL Support for Redis

We're excited to announce v1.2.4, which brings comprehensive TLS/SSL support for secure Redis connections, along with a more flexible configuration system.

---

## 🎉 What's New

### TLS/SSL Support for Redis Connections

This release adds full support for secure Redis connections with TLS/SSL encryption:

- **TLS Configuration**: Connect to Redis instances with TLS encryption using client certificates
- **Flexible Authentication**: Support for password authentication combined with TLS
- **Connection Pooling**: Enhanced connection pool configuration with configurable timeouts
- **Development Mode**: `InsecureSkipVerify` option for testing environments

**Example Usage:**

```go
import (
    "crypto/tls"
    "github.com/hibiken/asynq"
    "github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
)

queueConfig := &queue.Config{
    RedisClientOpt: &asynq.RedisClientOpt{
        Addr:     "redis.example.com:6380",
        Password: "your-secure-password",
        DB:       0,
        TLSConfig: &tls.Config{
            InsecureSkipVerify: false,
            // Add your CA, client cert, and key here
        },
        DialTimeout:  10 * time.Second,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
        PoolSize:     20,
    },
    Concurrency: 10,
    Queues: map[string]int{
        "critical": 6,
        "default":  3,
        "low":      1,
    },
}
```

### Enhanced Redis Configuration

The queue configuration system has been refactored for greater flexibility:

- **Direct Access to Asynq Options**: New `RedisClientOpt` field provides direct access to all Asynq Redis client options
- **Better Timeout Control**: Configure dial, read, and write timeouts independently
- **Connection Pool Sizing**: Control the size of the Redis connection pool
- **Unified Configuration**: All Redis options in a single, cohesive structure

### New Examples

Two new examples demonstrate TLS configuration:

- **`examples/test_secure_redis/`** - Complete example with TLS-enabled Redis
- **`examples/test_tls_conn/`** - TLS connection testing example

### Docker Compose Support

Added a secure Redis service to the Docker Compose setup:

- **Service Name**: `redis-secured`
- **Ports**: 7379 (non-TLS), 7380 (TLS)
- **Authentication**: Password-protected (`redispassword`)
- **Health Checks**: Automated TLS connection verification

---

## ⚠️ Breaking Changes

### Queue Configuration Refactoring

The `queue.Config` struct has been refactored to use `asynq.RedisClientOpt`:

**Old Configuration (v1.2.3 and earlier):**

```go
config := &queue.Config{
    RedisAddr:     "localhost:6379",
    RedisPassword: "password",
    RedisDB:       0,
    TlsConfig:     &tls.Config{...},
    Concurrency:   10,
}
```

**New Configuration (v1.2.4):**

```go
config := &queue.Config{
    RedisClientOpt: &asynq.RedisClientOpt{
        Addr:      "localhost:6379",
        Password:  "password",
        DB:        0,
        TLSConfig: &tls.Config{...},
    },
    Concurrency: 10,
}
```

**Migration Guide:**

1. Replace `RedisAddr`, `RedisPassword`, `RedisDB`, and `TlsConfig` fields with a single `RedisClientOpt` field
2. Move your Redis configuration values into the `asynq.RedisClientOpt` struct
3. Note: `TlsConfig` is now `TLSConfig` (capitalized "LS")
4. Consider adding timeout and pool size configurations for better performance

---

## 🐛 Bug Fixes

### State Machine Name Handling

Fixed an issue in `StateMachine.SaveDefinition()` where the name field wasn't properly handled:

- Only sets `record.ID` when `stateMachineID` is non-empty
- Only sets `record.Name` when `statemachine.Name` is non-empty
- `StateMachine.ToRecord()` now uses the `Name` field when available, falling back to `ID`

**Impact**: State machines can now have explicit names separate from their IDs, improving readability in logs and UIs.

### State Machine Name Field

Added the `Name` field to `StateMachine` and `rawStateMachine` structs:

```go
type StateMachine struct {
    Name           string                  `json:"Name"`
    Comment        string                  `json:"Comment,omitempty"`
    StartAt        string                  `json:"StartAt"`
    // ... other fields
}
```

**Impact**: State machine definitions can now include a `Name` field that will be properly serialized/deserialized.

### Example Timeout Fix

Fixed timeout duration in `ExampleTestTimeoutScenario()` from 12s to 10s for consistency with the configured timeout value.

### Makefile Syntax

Fixed environment variable export syntax in the `test-examples` Makefile target.

---

## 📦 Dependency Updates

- **Go Version**: Updated to 1.24.0
- **github.com/redis/go-redis/v9**: Updated from v9.7.0 to v9.7.3
- **golang.org/x/crypto**: Updated from v0.31.0 to v0.45.0
- **golang.org/x/sync**: Updated from v0.10.0 to v0.18.0
- **golang.org/x/sys**: Updated from v0.28.0 to v0.38.0
- **golang.org/x/text**: Updated from v0.21.0 to v0.31.0

---

## 🔧 Infrastructure Improvements

### Enhanced .gitignore

Added entries to exclude TLS certificates and Redis data:

```
/examples/test_secure_redis/redis/tls/*.crt
/examples/test_secure_redis/redis/tls/*.key
/examples/test_tls_conn/redis/tls/*.crt
/examples/test_tls_conn/redis/tls/*.key
/docker-examples/redis/tls/*.crt
/docker-examples/redis/tls/*.key
/redis/
```

### Makefile Enhancements

- Added `REDIS_PASSWORD`, `REDIS_SECURED_ADDR`, and `REDIS_SECURED_PASSWORD` variables
- Updated health checks for both standard and secure Redis instances
- Excluded TLS examples from automated CI test runs

### CI/CD Updates

Updated GitHub Actions workflow to exclude TLS examples (`test_tls_conn` and `test_secure_redis`) from automated runs, as they require specific TLS certificate setup.

---

## 🧪 Testing

All queue configuration tests have been updated to use the new `RedisClientOpt` structure:

- Updated 15+ test cases in `pkg/queue/config_test.go`
- Added new test `TestConfig_GetRedisClientOptWithTls` to verify TLS configuration
- All tests passing with the new configuration structure

---

## 📚 Documentation

- Updated all examples to demonstrate new configuration format
- Added comprehensive inline comments for TLS configuration
- Docker Compose includes health check examples for secure Redis

---

## 🔐 Security Improvements

This release significantly enhances security capabilities:

- **Encryption in Transit**: TLS/SSL encryption for all Redis communications
- **Password Authentication**: Secure password-based authentication for Redis
- **Certificate-Based Auth**: Support for CA and client certificate authentication
- **Flexible Security**: Configurable TLS verification for different security requirements

---

## 📈 Migration Path

### For Most Users (No TLS)

If you're not using TLS, simply update your configuration structure:

```go
// Before
config := &queue.Config{
    RedisAddr:   "localhost:6379",
    Concurrency: 10,
}

// After
config := &queue.Config{
    RedisClientOpt: &asynq.RedisClientOpt{
        Addr: "localhost:6379",
    },
    Concurrency: 10,
}
```

### For TLS Users

If you were using the `TlsConfig` field:

```go
// Before
config := &queue.Config{
    RedisAddr: "localhost:6379",
    TlsConfig: &tls.Config{
        InsecureSkipVerify: true,
    },
}

// After
config := &queue.Config{
    RedisClientOpt: &asynq.RedisClientOpt{
        Addr: "localhost:6379",
        TLSConfig: &tls.Config{
            InsecureSkipVerify: true,
        },
    },
}
```

### Recommended Configuration

For production deployments, we recommend:

```go
config := &queue.Config{
    RedisClientOpt: &asynq.RedisClientOpt{
        Addr:         "redis.example.com:6380",
        Password:     os.Getenv("REDIS_PASSWORD"),
        DB:           0,
        DialTimeout:  10 * time.Second,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
        PoolSize:     20,
        TLSConfig: &tls.Config{
            InsecureSkipVerify: false, // Set to true only in dev/test
            // Load your certificates here
        },
    },
    Concurrency: 10,
    Queues: map[string]int{
        "critical": 6,
        "timeout":  5,
        "default":  3,
        "low":      1,
    },
    RetryPolicy: &queue.RetryPolicy{
        MaxRetry: 3,
        Timeout:  10 * time.Minute,
    },
}
```

---

## 🙏 Contributors

Thank you to everyone who contributed to this release!

---

## 📝 Full Changelog

See [CHANGELOG.md](CHANGELOG.md) for the complete list of changes.

---

## 🚀 Getting Started

### Installation

```bash
go get -u github.com/hussainpithawala/state-machine-amz-go@v1.2.4
```

### Quick Start

Check out our examples:

```bash
# Basic Redis example
cd examples/distributed_queue
go run main.go

# Secure Redis with TLS
cd examples/test_secure_redis
go run main.go --tls
```

---

## 🐛 Known Issues

None at this time.

---

## 🔮 What's Next

We're planning for future releases:

- Redis Cluster support
- Redis Sentinel support
- Additional queue backends (RabbitMQ, Kafka)
- Enhanced monitoring and observability

Stay tuned for updates!

---

**Full Release**: [v1.2.4](https://github.com/hussainpithawala/state-machine-amz-go/releases/tag/v1.2.4)

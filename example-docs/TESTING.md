# Testing Guide

This document explains how to run tests for the state machine project, including both unit tests and integration tests.

## Test Structure

The project has two types of tests:

1. **Unit Tests**: Fast tests that don't require external dependencies (no Docker, no databases)
2. **Integration Tests**: Tests that require real Postgres and Redis instances

Integration tests are marked with the `integration` build tag:
```go
//go:build integration
// +build integration
```

## Running Unit Tests

Unit tests run without any external dependencies and are fast:

```bash
# Run all unit tests
go test ./...

# Run unit tests for specific packages
go test ./pkg/queue/...
go test ./pkg/statemachine/handler/...

# Run with coverage
go test ./pkg/queue/... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run with verbose output
go test -v ./pkg/queue/...
```

## Running Integration Tests

Integration tests require Postgres and Redis to be running. The easiest way is to use Docker Compose.

### Prerequisites

1. Docker and Docker Compose installed
2. Ports 5432 (Postgres) and 6379 (Redis) available

### Step 1: Start Docker Services

```bash
# Start Postgres and Redis
cd docker-examples
docker-compose up -d postgres redis

# Wait for services to be healthy
docker-compose ps

# Check logs if needed
docker-compose logs postgres
docker-compose logs redis
```

### Step 2: Run Integration Tests

```bash
# Run all integration tests
go test -tags=integration ./...

# Run integration tests for specific packages
go test -tags=integration ./pkg/queue/...
go test -tags=integration ./pkg/statemachine/handler/...

# Run with verbose output
go test -v -tags=integration ./pkg/queue/...

# Run with coverage
go test -tags=integration ./pkg/queue/... -coverprofile=integration_coverage.out
go tool cover -html=integration_coverage.out
```

### Step 3: Stop Docker Services

```bash
# Stop services
cd docker-examples
docker-compose down

# Stop and remove volumes (clean slate)
docker-compose down -v
```

## Environment Variables

Integration tests use environment variables to configure database connections:

### Postgres Configuration

```bash
export POSTGRES_TEST_URL="postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
```

Default: `postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable`

### Redis Configuration

```bash
export REDIS_TEST_ADDR="localhost:6379"
```

Default: `localhost:6379`

## Test Coverage

### Current Test Coverage

The test suite provides comprehensive coverage for:

#### Queue Package (`pkg/queue/`)
- ✅ Configuration validation and defaults
- ✅ Task creation and parsing (Execution and Timeout tasks)
- ✅ Queue client operations (enqueue, schedule, cancel)
- ✅ Timeout scheduling and cancellation
- ✅ Multiple concurrent timeout scenarios
- ✅ High volume timeout handling
- ✅ Message received before timeout scenarios
- ✅ Concurrent operations and race conditions

#### Handler Package (`pkg/statemachine/handler/`)
- ✅ Handler creation and initialization
- ✅ Execution handling with direct input
- ✅ Timeout handling complete scenarios
- ✅ Correlation management (waiting, completed, not found)
- ✅ Error handling for invalid state machines
- ✅ Getter methods for repository and queue client

### Checking Coverage

```bash
# Unit test coverage
go test ./pkg/queue/... -coverprofile=unit_coverage.out
go tool cover -func=unit_coverage.out | grep total

# Integration test coverage
go test -tags=integration ./pkg/queue/... -coverprofile=int_coverage.out
go tool cover -func=int_coverage.out | grep total

# Combined coverage report
go test -tags=integration ./... -coverprofile=combined_coverage.out
go tool cover -html=combined_coverage.out
```

## Continuous Integration

For CI/CD pipelines, use the following workflow:

```yaml
# Example GitHub Actions workflow
steps:
  - name: Start Services
    run: |
      cd docker-examples
      docker-compose up -d postgres redis

  - name: Wait for Services
    run: |
      docker-compose exec -T postgres pg_isready -U postgres
      docker-compose exec -T redis redis-cli ping

  - name: Run Unit Tests
    run: go test ./...

  - name: Run Integration Tests
    run: go test -tags=integration ./...
    env:
      POSTGRES_TEST_URL: postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable
      REDIS_TEST_ADDR: localhost:6379

  - name: Stop Services
    run: docker-compose down -v
```

## Timeout Event Test Scenarios

The integration tests include comprehensive timeout event scenarios:

### 1. Message Received Before Timeout
Tests the happy path where a message arrives before the timeout triggers, and the timeout is cancelled.

### 2. Timeout Triggers First
Tests the scenario where no message arrives and the timeout event is processed.

### 3. Multiple Concurrent Timeouts
Tests handling of multiple timeout events with different delays, including selective cancellation.

### 4. High Volume Timeouts
Tests system behavior under load with 50+ concurrent timeout tasks.

### 5. Concurrent Operations
Tests race conditions when multiple goroutines schedule and cancel timeouts simultaneously.

### 6. Already Processed Correlations
Tests graceful handling when timeout fires but correlation was already completed.

### 7. Different Timeout Delays
Tests timeouts ranging from 1 second to 5 minutes to ensure proper scheduling.

## Troubleshooting

### Integration Tests Fail to Connect

If integration tests fail with connection errors:

```bash
# Check if services are running
docker-compose ps

# Check service logs
docker-compose logs postgres
docker-compose logs redis

# Restart services
docker-compose restart postgres redis

# Full reset
docker-compose down -v
docker-compose up -d postgres redis
```

### Port Conflicts

If ports 5432 or 6379 are already in use:

```bash
# Check what's using the ports
lsof -i :5432
lsof -i :6379

# Either stop the conflicting service or modify docker-compose.yml
# to use different ports, then update environment variables
```

### Database Schema Issues

If you encounter schema-related errors:

```bash
# The tests automatically initialize the schema, but if needed:
docker-compose exec postgres psql -U postgres -d statemachine_test -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
```

## Best Practices

1. **Always run unit tests first** - They're fast and catch most issues
2. **Run integration tests before committing** - Ensures nothing breaks with real dependencies
3. **Clean up between test runs** - Use `docker-compose down -v` for a clean slate
4. **Use verbose mode for debugging** - Add `-v` flag to see detailed test output
5. **Check coverage regularly** - Aim for >80% coverage on critical paths

## Writing New Tests

### Unit Tests
Place in the same package without build tags:
```go
package queue

func TestMyFeature(t *testing.T) {
    // Test code
}
```

### Integration Tests
Add build tag and use real connections:
```go
//go:build integration
// +build integration

package queue

func (suite *MyTestSuite) TestWithRealDB(t *testing.T) {
    // Test with real Postgres/Redis
}
```

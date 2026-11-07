// examples/parallel_persistent/main.go
//
// Demonstrates persistent parallel state execution with Redis + Postgres.
//
// Architecture:
//
//	Leader  — creates the SM definition, enqueues a single execution
//	Worker  — processes tasks from the queue (parent + branch executions)
//
// The parent SM hits a Parallel state, which dispatches each branch as a
// separate queued execution. The parent pauses (WAITING/PAUSED). When all
// branches complete, the ParallelOrchestrator resumes the parent.
//
// Prerequisites:
//   - PostgreSQL  (default: localhost:5432/statemachine_test_gorm)
//   - Redis       (default: localhost:6379)
//
// Usage:
//
//	# Terminal 1 — start the worker
//	go run ./examples/parallel_persistent --mode worker
//
//	# Terminal 2 — enqueue the execution
//	go run ./examples/parallel_persistent --mode leader
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/handler"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/parallel"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

var (
	mode          = flag.String("mode", "leader", "'leader' or 'worker'")
	redisAddr     = flag.String("redis", "localhost:6379", "Redis address")
	redisPassword = flag.String("redis-password", "", "Redis password")
	redisDB       = flag.Int("redis-db", 0, "Redis DB number")
	postgresURL   = flag.String("postgres", "", "PostgreSQL URL (overrides DATABASE_URL_GORM)")
	concurrency   = flag.Int("concurrency", 10, "Worker concurrency")
)

const smID = "parallel-persistent-example"

// State machine definition: ValidateOrder -> ParallelProcessing (3 branches) -> GenerateReport
const workflowJSON = `{
  "Comment": "Persistent Parallel State Example",
  "StartAt": "ValidateOrder",
  "States": {
    "ValidateOrder": {
      "Type": "Task",
      "Resource": "validate-order",
      "Next": "ParallelProcessing"
    },
    "ParallelProcessing": {
      "Type": "Parallel",
      "FailurePolicy": "WAIT_FOR_ALL",
      "Next": "GenerateReport",
      "Branches": [
        {
          "StartAt": "ProcessPayment",
          "States": {
            "ProcessPayment": {
              "Type": "Task",
              "Resource": "process-payment",
              "Next": "ApplyDiscount"
            },
            "ApplyDiscount": {
              "Type": "Task",
              "Resource": "apply-discount",
              "End": true
            }
          }
        },
        {
          "StartAt": "CheckInventory",
          "States": {
            "CheckInventory": {
              "Type": "Task",
              "Resource": "check-inventory",
              "Next": "ReserveStock"
            },
            "ReserveStock": {
              "Type": "Task",
              "Resource": "reserve-stock",
              "End": true
            }
          }
        },
        {
          "StartAt": "CalculateShipping",
          "States": {
            "CalculateShipping": {
              "Type": "Task",
              "Resource": "calculate-shipping",
              "End": true
            }
          }
        }
      ],
      "ResultPath": "$.parallelResults"
    },
    "GenerateReport": {
      "Type": "Task",
      "Resource": "generate-report",
      "End": true
    }
  }
}`

func main() {
	flag.Parse()

	ctx := context.Background()

	repoManager, err := setupRepository(ctx)
	if err != nil {
		log.Fatalf("Repository setup failed: %v", err)
	}
	defer repoManager.Close()

	queueConfig := newQueueConfig()
	rdb := newRedisClient()

	switch *mode {
	case "leader":
		runLeader(ctx, queueConfig, repoManager, rdb)
	case "worker":
		runWorker(ctx, queueConfig, repoManager, rdb)
	default:
		log.Fatalf("Invalid mode %q — use 'leader' or 'worker'", *mode)
	}
}

// ---------------------------------------------------------------------------
// Leader: create SM, enqueue one execution
// ---------------------------------------------------------------------------

func runLeader(ctx context.Context, queueConfig *queue.Config, repoManager *repository.Manager, rdb *redis.Client) {
	log.Println("[leader] Starting...")

	sm, err := getOrCreateStateMachine(ctx, repoManager)
	if err != nil {
		log.Fatalf("[leader] SM setup failed: %v", err)
	}

	queueClient, err := queue.NewClient(queueConfig)
	if err != nil {
		log.Fatalf("[leader] Queue client failed: %v", err)
	}
	defer queueClient.Close()

	sm.SetQueueClient(queueClient)

	if err := sm.SetRedisClient(ctx, rdb); err != nil {
		log.Fatalf("[leader] Redis setup failed: %v", err)
	}

	log.Printf("[leader] SM loaded: %s", sm.GetID())

	input := map[string]interface{}{
		"orderId":    fmt.Sprintf("ORD-%d", time.Now().UnixNano()),
		"customerId": "CUST-42",
		"amount":     249.99,
		"items":      []string{"headphones", "cable", "case"},
	}

	execName := fmt.Sprintf("parallel-exec-%d", time.Now().Unix())

	payload := &queue.ExecutionTaskPayload{
		StateMachineID: sm.GetID(),
		ExecutionName:  execName,
		Input:          input,
	}

	taskInfo, err := queueClient.EnqueueExecution(payload)
	if err != nil {
		log.Fatalf("[leader] Enqueue failed: %v", err)
	}

	log.Printf("[leader] Enqueued execution: TaskID=%s, Name=%s", taskInfo.ID, execName)
	log.Println("[leader] Done. Start a worker to process the execution.")
}

// ---------------------------------------------------------------------------
// Worker: process queue tasks (parent + branch executions)
// ---------------------------------------------------------------------------

func runWorker(ctx context.Context, queueConfig *queue.Config, repoManager *repository.Manager, rdb *redis.Client) {
	log.Println("[worker] Starting...")

	exec := setupExecutor()
	execAdapter := executor.NewExecutionContextAdapter(exec)

	queueClient, err := queue.NewClient(queueConfig)
	if err != nil {
		log.Fatalf("[worker] Queue client failed: %v", err)
	}
	defer queueClient.Close()

	smFactory := func(ctx2 context.Context, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
		sm, err := persistent.NewFromDefnId(ctx2, smID, mgr)
		if err != nil {
			return nil, err
		}
		sm.SetQueueClient(queueClient)
		if err := sm.SetRedisClient(ctx2, rdb); err != nil {
			return nil, err
		}
		return sm, nil
	}

	orch, err := parallel.NewOrchestrator(ctx, rdb, repoManager, queueClient, smFactory)
	if err != nil {
		log.Fatalf("[worker] Orchestrator failed: %v", err)
	}

	execHandler := handler.NewExecutionHandlerWithContext(repoManager, queueClient, execAdapter, nil)
	execHandler.SetParallelOrchestrator(orch)

	worker, err := queue.NewWorker(queueConfig, execHandler)
	if err != nil {
		log.Fatalf("[worker] Worker creation failed: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := worker.Run(); err != nil {
			log.Fatalf("[worker] Run failed: %v", err)
		}
	}()

	log.Printf("[worker] Running with concurrency=%d. Ctrl+C to stop.", *concurrency)
	<-sigChan
	log.Println("[worker] Shutting down...")
	worker.Shutdown()
	log.Println("[worker] Stopped.")
}

// ---------------------------------------------------------------------------
// Task handlers
// ---------------------------------------------------------------------------

func setupExecutor() *executor.BaseExecutor {
	exec := executor.NewBaseExecutor()

	exec.RegisterGoFunction("validate-order", func(_ context.Context, input interface{}) (interface{}, error) {
		log.Println("  [task] validate-order")
		m := asMap(input)
		m["validated"] = true
		return m, nil
	})

	exec.RegisterGoFunction("process-payment", func(_ context.Context, input interface{}) (interface{}, error) {
		log.Println("  [task] process-payment")
		time.Sleep(200 * time.Millisecond)
		m := asMap(input)
		m["paymentStatus"] = "COMPLETED"
		m["transactionId"] = fmt.Sprintf("TXN-%d", time.Now().UnixNano())
		return m, nil
	})

	exec.RegisterGoFunction("apply-discount", func(_ context.Context, input interface{}) (interface{}, error) {
		log.Println("  [task] apply-discount")
		m := asMap(input)
		m["discountApplied"] = true
		m["finalAmount"] = 224.99
		return m, nil
	})

	exec.RegisterGoFunction("check-inventory", func(_ context.Context, input interface{}) (interface{}, error) {
		log.Println("  [task] check-inventory")
		time.Sleep(150 * time.Millisecond)
		m := asMap(input)
		m["inStock"] = true
		return m, nil
	})

	exec.RegisterGoFunction("reserve-stock", func(_ context.Context, input interface{}) (interface{}, error) {
		log.Println("  [task] reserve-stock")
		m := asMap(input)
		m["reservationId"] = fmt.Sprintf("RES-%d", time.Now().UnixNano())
		return m, nil
	})

	exec.RegisterGoFunction("calculate-shipping", func(_ context.Context, input interface{}) (interface{}, error) {
		log.Println("  [task] calculate-shipping")
		time.Sleep(100 * time.Millisecond)
		m := asMap(input)
		m["shippingCost"] = 12.99
		m["estimatedDelivery"] = time.Now().Add(48 * time.Hour).Format("2006-01-02")
		return m, nil
	})

	exec.RegisterGoFunction("generate-report", func(_ context.Context, input interface{}) (interface{}, error) {
		log.Println("  [task] generate-report")
		m := asMap(input)
		m["reportId"] = fmt.Sprintf("RPT-%d", time.Now().UnixNano())
		m["completedAt"] = time.Now().Format(time.RFC3339)
		return m, nil
	})

	return exec
}

// ---------------------------------------------------------------------------
// Infrastructure helpers
// ---------------------------------------------------------------------------

func getOrCreateStateMachine(ctx context.Context, repoManager *repository.Manager) (*persistent.StateMachine, error) {
	sm, err := persistent.NewFromDefnId(ctx, smID, repoManager)
	if err == nil {
		return sm, nil
	}

	log.Printf("[leader] Creating new SM definition: %s", smID)

	sm, err = persistent.New([]byte(workflowJSON), true, smID, repoManager)
	if err != nil {
		return nil, fmt.Errorf("failed to create SM: %w", err)
	}
	if err := sm.SaveDefinition(ctx); err != nil {
		return nil, fmt.Errorf("failed to save SM definition: %w", err)
	}
	return sm, nil
}

func setupRepository(ctx context.Context) (*repository.Manager, error) {
	connURL := *postgresURL
	if connURL == "" {
		connURL = os.Getenv("DATABASE_URL_GORM")
	}
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_test_gorm?sslmode=disable"
	}

	mgr, err := repository.NewPersistenceManager(&repository.Config{
		Strategy:      "gorm-postgres",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns":    25,
			"max_idle_conns":    5,
			"conn_max_lifetime": 5 * time.Minute,
		},
	})
	if err != nil {
		return nil, err
	}
	if err := mgr.Initialize(ctx); err != nil {
		return nil, err
	}
	return mgr, nil
}

func newQueueConfig() *queue.Config {
	return &queue.Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr:     *redisAddr,
			Password: *redisPassword,
			DB:       *redisDB,
		},
		Concurrency: *concurrency,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		RetryPolicy: &queue.RetryPolicy{
			MaxRetry: 3,
			Timeout:  5 * time.Minute,
		},
	}
}

func newRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: *redisPassword,
		DB:       *redisDB,
	})
}

func asMap(input interface{}) map[string]interface{} {
	if m, ok := input.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

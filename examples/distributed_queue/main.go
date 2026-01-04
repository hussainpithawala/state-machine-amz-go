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

	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

var (
	mode           = flag.String("mode", "leader", "Application mode: 'leader' or 'worker'")
	redisAddr      = flag.String("redis", "localhost:6379", "Redis address")
	redisPassword  = flag.String("redis-password", "", "Redis password")
	redisDB        = flag.Int("redis-db", 0, "Redis database number")
	concurrency    = flag.Int("concurrency", 10, "Worker concurrency")
	postgresURL    = flag.String("postgres", "postgres://user:password@localhost:5432/statemachine?sslmode=disable", "PostgreSQL connection URL")
	stateMachineID = flag.String("sm-id", "order-processing-sm", "State machine ID")
)

func main() {
	flag.Parse()

	ctx := context.Background()

	// Setup task executor with handlers
	exec := setupExecutor()

	// Setup repository manager
	repoManager, err := setupRepository(ctx)
	if err != nil {
		log.Fatalf("Failed to setup repository: %v", err)
	}
	defer repoManager.Close()

	// Setup queue configuration
	queueConfig := &queue.Config{
		RedisAddr:     *redisAddr,
		RedisPassword: *redisPassword,
		RedisDB:       *redisDB,
		Concurrency:   *concurrency,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		RetryPolicy: &queue.RetryPolicy{
			MaxRetry: 3,
			Timeout:  10 * time.Minute,
		},
	}

	allStateMachines, err := repoManager.ListStateMachines(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to list state machines: %v", err)
	}

	for i := 0; i < len(allStateMachines); i++ {
		queueName := allStateMachines[i].ID
		queueConfig.Queues[queueName] = 5
	}

	switch *mode {
	case "leader":
		runLeader(ctx, queueConfig, repoManager)
	case "worker":
		runWorker(ctx, queueConfig, repoManager, exec)
	default:
		log.Fatalf("Invalid mode: %s. Use 'leader' or 'worker'", *mode)
	}
}

// runLeader runs the application in leader mode (enqueues tasks)
func runLeader(ctx context.Context, queueConfig *queue.Config, repoManager *repository.Manager) {
	log.Println("Starting application in LEADER mode...")

	// Create queue client
	queueClient, err := queue.NewClient(queueConfig)
	if err != nil {
		log.Fatalf("Failed to create queue client: %v", err)
	}
	defer queueClient.Close()

	// Load or create state machine
	sm, err := getOrCreateStateMachine(ctx, repoManager)
	if err != nil {
		log.Fatalf("Failed to get state machine: %v", err)
	}

	// Set queue client for distributed execution
	sm.SetQueueClient(queueClient)

	log.Printf("State machine loaded: ID=%s", sm.GetID())

	// Generate 100 sample execution tasks and enqueue them directly
	log.Println("Generating 100 sample execution tasks...")

	batchName := fmt.Sprintf("batch-exec-%d", time.Now().Unix())
	enqueuedCount := 0
	failedCount := 0

	for i := 0; i < 10000; i++ {
		// Create task payload
		payload := &queue.ExecutionTaskPayload{
			StateMachineID: sm.GetID(),
			ExecutionName:  fmt.Sprintf("%s-%d", batchName, i),
			ExecutionIndex: i,
			Input: map[string]interface{}{
				"orderId":    fmt.Sprintf("ORDER-%d", 10000+i),
				"customerId": fmt.Sprintf("CUST-%d", 1000+i),
				"amount":     100.00 + float64(i),
				"items":      []string{"item1", "item2", "item3"},
				"timestamp":  time.Now().Unix(),
			},
		}

		// Enqueue the task
		taskInfo, err := queueClient.EnqueueExecution(payload)
		if err != nil {
			log.Printf("Failed to enqueue task %d: %v", i, err)
			failedCount++
		} else {
			enqueuedCount++
			if (i+1)%10 == 0 {
				log.Printf("Enqueued %d/%d tasks... (TaskID: %s, Queue: %s)",
					enqueuedCount, 100, taskInfo.ID, taskInfo.Queue)
			}
		}
	}

	log.Printf("Batch enqueued: %d tasks submitted successfully, %d failed", enqueuedCount, failedCount)

	// Keep leader running to monitor or submit more tasks
	log.Println("Leader is running. Press Ctrl+C to exit.")
	waitForShutdown()
}

// runWorker runs the application in worker mode (processes tasks from queue)
func runWorker(ctx context.Context, queueConfig *queue.Config, repoManager *repository.Manager, exec *executor.BaseExecutor) {
	log.Println("Starting application in WORKER mode...")

	// Create execution context adapter
	execAdapter := executor.NewExecutionContextAdapter(exec)

	// Create execution handler with executor
	handler := persistent.NewExecutionHandlerWithContext(repoManager, execAdapter)

	// Create worker with handler
	worker, err := queue.NewWorker(queueConfig, handler)
	if err != nil {
		log.Fatalf("Failed to create worker: %v", err)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start worker in goroutine
	go func() {
		if err := worker.Run(); err != nil {
			log.Fatalf("Worker failed: %v", err)
		}
	}()

	log.Println("Worker is running. Press Ctrl+C to shutdown gracefully.")

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutdown signal received, stopping worker...")
	worker.Shutdown()
	log.Println("Worker stopped successfully")
}

// setupRepository initializes the repository manager
func setupRepository(ctx context.Context) (*repository.Manager, error) {
	// Configure persistence
	persistenceConfig := &repository.Config{
		Strategy:      "gorm-postgres",
		ConnectionURL: *postgresURL,
		Options: map[string]interface{}{
			"max_open_conns":    25,
			"max_idle_conns":    5,
			"conn_max_lifetime": 5 * time.Minute,
		},
	}

	// Create persistence manager
	persistenceManager, err := repository.NewPersistenceManager(persistenceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistence manager: %w", err)
	}

	// Initialize schema
	if err := persistenceManager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize persistence: %w", err)
	}

	return persistenceManager, nil
}

// getOrCreateStateMachine loads an existing state machine or creates a new one
func getOrCreateStateMachine(ctx context.Context, repoManager *repository.Manager) (*persistent.StateMachine, error) {
	// Try to load existing state machine
	sm, err := persistent.NewFromDefnId(ctx, *stateMachineID, repoManager)
	if err == nil {
		return sm, nil
	}

	// Create a new state machine if it doesn't exist
	log.Printf("State machine not found, creating new one: %s", *stateMachineID)

	definition := []byte(`{
		"Comment": "Order Processing State Machine for Distributed Queue",
		"StartAt": "ValidateOrder",
		"States": {
			"ValidateOrder": {
				"Type": "Task",
				"Resource": "arn:aws:lambda:::validate:order",
				"Next": "ProcessPayment"
			},
			"ProcessPayment": {
				"Type": "Task",
				"Resource": "arn:aws:lambda:::process:payment",
				"Next": "ShipOrder"
			},
			"ShipOrder": {
				"Type": "Task",
				"Resource": "arn:aws:lambda:::ship:order",
				"Next": "SendConfirmation"
			},
			"SendConfirmation": {
				"Type": "Task",
				"Resource": "arn:aws:lambda:::send:confirmation",
				"End": true
			}
		}
	}`)

	sm, err = persistent.New(definition, true, *stateMachineID, repoManager)
	if err != nil {
		return nil, fmt.Errorf("failed to create state machine: %w", err)
	}

	// Save the definition
	if err := sm.SaveDefinition(ctx); err != nil {
		return nil, fmt.Errorf("failed to save state machine definition: %w", err)
	}

	return sm, nil
}

// waitForShutdown waits for a shutdown signal
func waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Println("Shutdown signal received, exiting...")
}

// setupExecutor registers task handlers for the state machine
func setupExecutor() *executor.BaseExecutor {
	exec := executor.NewBaseExecutor()

	// Register handler for ValidateOrder
	exec.RegisterGoFunction("validate:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		log.Printf("→ Validating order: %v", input)
		//time.Sleep(100 * time.Millisecond) // Simulate processing

		// Add validation result to input
		if inputMap, ok := input.(map[string]interface{}); ok {
			inputMap["validationStatus"] = "VALID"
			inputMap["validatedAt"] = time.Now().Unix()
			return inputMap, nil
		}
		return input, nil
	})

	// Register handler for ProcessPayment
	exec.RegisterGoFunction("process:payment", func(ctx context.Context, input interface{}) (interface{}, error) {
		log.Printf("→ Processing payment: %v", input)
		//time.Sleep(150 * time.Millisecond) // Simulate processing

		if inputMap, ok := input.(map[string]interface{}); ok {
			inputMap["paymentStatus"] = "COMPLETED"
			inputMap["transactionId"] = fmt.Sprintf("TXN-%d", time.Now().Unix())
			inputMap["processedAt"] = time.Now().Unix()
			return inputMap, nil
		}
		return input, nil
	})

	// Register handler for ShipOrder
	exec.RegisterGoFunction("ship:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		log.Printf("→ Shipping order: %v", input)
		//time.Sleep(100 * time.Millisecond) // Simulate processing

		if inputMap, ok := input.(map[string]interface{}); ok {
			inputMap["shippingStatus"] = "SHIPPED"
			inputMap["trackingNumber"] = fmt.Sprintf("TRK-%d", time.Now().Unix())
			inputMap["shippedAt"] = time.Now().Unix()
			return inputMap, nil
		}
		return input, nil
	})

	// Register handler for SendConfirmation
	exec.RegisterGoFunction("send:confirmation", func(ctx context.Context, input interface{}) (interface{}, error) {
		log.Printf("→ Sending confirmation: %v", input)
		//time.Sleep(50 * time.Millisecond) // Simulate processing

		if inputMap, ok := input.(map[string]interface{}); ok {
			inputMap["confirmationStatus"] = "SENT"
			inputMap["confirmedAt"] = time.Now().Unix()
			return inputMap, nil
		}
		return input, nil
	})

	log.Println("Task executor configured with 4 handlers")
	return exec
}

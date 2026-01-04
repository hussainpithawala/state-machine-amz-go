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

	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
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

	repoManager.ListExecutionIDs()

	switch *mode {
	case "leader":
		runLeader(ctx, queueConfig, repoManager)
	case "worker":
		runWorker(ctx, queueConfig, repoManager)
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

	// Example: Execute batch with distributed queue
	filter := &repository.ExecutionFilter{
		StateMachineID: sm.GetID(),
		Status:         "SUCCEEDED",
		Limit:          100,
	}

	batchOpts := &statemachine.BatchExecutionOptions{
		NamePrefix:        fmt.Sprintf("batch-exec-%d", time.Now().Unix()),
		ConcurrentBatches: 10,
		StopOnError:       false,
		OnExecutionStart: func(sourceExecID string, index int) {
			log.Printf("Enqueuing task %d for source execution: %s", index, sourceExecID)
		},
		OnExecutionComplete: func(sourceExecID string, index int, err error) {
			if err != nil {
				log.Printf("Failed to enqueue task %d: %v", index, err)
			} else {
				log.Printf("Successfully enqueued task %d", index)
			}
		},
	}

	results, err := sm.ExecuteBatch(ctx, filter, "ProcessOrder", batchOpts)
	if err != nil {
		log.Fatalf("Batch execution failed: %v", err)
	}

	log.Printf("Batch enqueued: %d tasks submitted", len(results))

	// Keep leader running to monitor or submit more tasks
	log.Println("Leader is running. Press Ctrl+C to exit.")
	waitForShutdown()
}

// runWorker runs the application in worker mode (processes tasks from queue)
func runWorker(ctx context.Context, queueConfig *queue.Config, repoManager *repository.Manager) {
	log.Println("Starting application in WORKER mode...")

	// Create execution handler
	handler := persistent.NewExecutionHandler(repoManager)

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

	definition := []byte(`
Comment: "Workflow with message pause and resume using GORM"
StartAt: InitialTask
States:
  InitialTask:
    Type: Task
    Resource: "arn:aws:states:::lambda:function:initial-task"
    Next: WaitForApproval

  WaitForApproval:
    Type: Message
    CorrelationKey: "orderId"
    CorrelationValuePath: "$.orderId"
    Next: FinalTask

  FinalTask:
    Type: Task
    Resource: "arn:aws:states:::lambda:function:final-task"
    End: true
	`)

	sm, err = persistent.New(definition, false, *stateMachineID, repoManager)
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

// Package main demonstrates crash recovery for batch/micro-batch orchestrations.
//
// This example shows:
//   1. An orchestrator managing multiple micro-batch executions
//   2. Simulated crash of both orchestrator and worker executions
//   3. Recovery scanner detecting and resuming orphaned executions
//   4. Barrier mechanism properly signaling orchestrator after worker recovery
//   5. Orchestrator resuming normal operation after barrier completion
//
// Key Scenario:
//   - Orchestrator dispatches micro-batch and waits at barrier
//   - Workers process tasks and decrement barrier
//   - CRASH: Last worker crashes before signaling barrier completion
//   - Recovery: Both workers and orchestrator are resumed
//   - Barrier properly signals orchestrator to continue
//
// Usage:
//   go run main.go
//
// Requirements:
//   - PostgreSQL database
//   - Redis instance (for barrier/orchestration)
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/recovery"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
	"github.com/redis/go-redis/v9"
)

const (
	// Database and Redis configuration
	databaseURL        = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
	redisURL           = "redis://localhost:6379/0"
	
	// Batch configuration
	batchID            = "crash-recovery-batch-001"
	microBatchSize     = 5
	totalExecutions    = 15 // 3 micro-batches of 5 each
	
	// State machine IDs
	workerSMID         = "crash-recovery-worker-sm"
	orchestratorSMID   = batch.OrchestratorStateMachineID
	
	// Recovery configuration
	scanInterval       = 2 * time.Second
	orphanedThreshold  = 4 * time.Second
	maxRecoveryAttempts = 3
	
	// Keys for Redis
	barrierKeyPrefix = "batch:barrier:"
)

func main() {
	fmt.Println("=== Crash Recovery for Batch/Micro-Batch Orchestration ===")
	fmt.Println()
	fmt.Println("This example demonstrates:")
	fmt.Println("  1. Orchestrator managing micro-batch executions")
	fmt.Println("  2. Simulated crash of orchestrator and workers")
	fmt.Println("  3. Recovery scanner resuming orphaned executions")
	fmt.Println("  4. Barrier mechanism signaling orchestrator completion")
	fmt.Println("  5. Orchestrator continuing after barrier is cleared")
	fmt.Println()

	ctx := context.Background()

	// Initialize components
	persistenceManager, redisClient, err := initializeComponents(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	defer persistenceManager.Close()
	defer redisClient.Close()

	// Clean up previous run
	if err := cleanupPreviousRun(ctx, persistenceManager, redisClient); err != nil {
		fmt.Printf("Warning: cleanup failed: %v\n", err)
	}

	// Setup state machines
	_, orchestratorSM, err := setupStateMachines(ctx, persistenceManager, redisClient)
	if err != nil {
		log.Fatalf("Failed to setup state machines: %v", err)
	}

	fmt.Println("=== Phase 1: Creating Source Executions ===")
	fmt.Println()

	// Create source executions in database
	sourceExecutionIDs, err := createSourceExecutions(ctx, persistenceManager)
	if err != nil {
		log.Fatalf("Failed to create source executions: %v", err)
	}

	fmt.Printf("Created %d source executions\n", len(sourceExecutionIDs))
	fmt.Println()

	fmt.Println("=== Phase 2: Starting Batch Orchestration ===")
	fmt.Println()

	// Create orchestrator
	orchestrator, err := batch.NewOrchestrator(ctx, redisClient, orchestratorSM,
		func(ctx context.Context, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
			return persistent.NewFromDefnId(ctx, smID, mgr)
		},
		func(def []byte, isJSON bool, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
			return persistent.New(def, isJSON, smID, mgr)
		},
	)
	if err != nil {
		log.Fatalf("Failed to create orchestrator: %v", err)
	}

	// Start batch execution
	doneCh, err := orchestrator.Run(ctx, batchID, sourceExecutionIDs, workerSMID, "ProcessData", &statemachine.BatchExecutionOptions{
		NamePrefix:        "crash-recovery-batch",
		ConcurrentBatches: 1,
		MicroBatchSize:    microBatchSize,
	}, nil)
	if err != nil {
		log.Fatalf("Failed to start batch: %v", err)
	}

	fmt.Printf("Batch orchestration started: %s\n", batchID)
	fmt.Printf("Total executions: %d, Micro-batch size: %d\n", totalExecutions, microBatchSize)
	fmt.Println()

	// Wait for first micro-batch to be dispatched and orchestrator to pause
	time.Sleep(2 * time.Second)

	fmt.Println("=== Phase 3: Simulating Crash ===")
	fmt.Println()
	fmt.Println("Simulating crash scenario:")
	fmt.Println("  - Orchestrator: PAUSED at WaitForMicroBatchCompletion")
	fmt.Println("  - Workers: Mid-execution (some completed, some not)")
	fmt.Println()

	// Simulate partial worker completion (some workers complete, last one crashes)
	simulatedWorkerCompletions := 3 // Out of 5 in first micro-batch
	fmt.Printf("Simulating %d/%d workers completing before crash...\n",
		simulatedWorkerCompletions, microBatchSize)

	for i := 0; i < simulatedWorkerCompletions; i++ {
		mbID := fmt.Sprintf("%s-mb-0", batchID)
		_ = simulateWorkerCompletion(ctx, redisClient, mbID, i)
	}

	// Check barrier status
	remaining, _ := redisClient.Get(ctx, fmt.Sprintf("%s%s", barrierKeyPrefix, batchID)).Int64()
	fmt.Printf("Barrier remaining after partial completions: %d\n", remaining)
	fmt.Println()

	fmt.Println(">>> CRASH SIMULATED <<<")
	fmt.Println("   - Last 2 workers crashed before decrementing barrier")
	fmt.Println("   - Orchestrator waiting at barrier")
	fmt.Println("   - Barrier counter stuck at non-zero value")
	fmt.Println()

	// Wait for orphaned threshold
	fmt.Printf("Waiting %v for executions to be considered orphaned...\n", orphanedThreshold+1*time.Second)
	time.Sleep(orphanedThreshold + 1*time.Second)

	fmt.Println()
	fmt.Println("=== Phase 4: Starting Recovery Scanner ===")
	fmt.Println()

	// Configure and start recovery scanner
	recoveryConfig := &recovery.RecoveryConfig{
		Enabled:                  true,
		ScanInterval:             scanInterval,
		OrphanedThreshold:        orphanedThreshold,
		DefaultRecoveryStrategy:  recovery.StrategyRetry,
		DefaultMaxRecoveryAttempts: maxRecoveryAttempts,
	}

	if err := orchestratorSM.StartRecoveryScanner(recoveryConfig); err != nil {
		log.Fatalf("Failed to start recovery scanner: %v", err)
	}
	defer orchestratorSM.StopRecoveryScanner()

	fmt.Printf("Recovery scanner started (interval: %v, threshold: %v)\n",
		scanInterval, orphanedThreshold)
	fmt.Println()

	// Monitor recovery
	fmt.Println("Monitoring recovery process...")
	fmt.Println()

	maxWait := 60 * time.Second
	startTime := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var recoveryComplete bool
	var orchestratorStatus string

	for !recoveryComplete {
		select {
		case <-ticker.C:
			elapsed := time.Since(startTime).Round(time.Second)

			// Check orchestrator execution status
			orchExecs, err := orchestratorSM.ListExecutions(ctx, &repository.ExecutionFilter{
				StateMachineID: orchestratorSMID,
			})
			if err != nil {
				fmt.Printf("Error checking orchestrator: %v\n", err)
				continue
			}

			if len(orchExecs) > 0 {
				orchestratorStatus = orchExecs[0].Status
				fmt.Printf("[%v] Orchestrator: %-12s  Barrier: %d\n",
					elapsed, orchestratorStatus, getBarrierRemaining(ctx, redisClient, batchID))
			}

			// Check if orchestrator completed
			if orchestratorStatus == "SUCCEEDED" {
				fmt.Println()
				fmt.Printf(">>> Recovery Complete! Orchestrator finished successfully <<<\n")
				fmt.Println()
				recoveryComplete = true
			}

			// Check timeout
			if elapsed > maxWait {
				fmt.Println()
				fmt.Printf(">>> Recovery timed out after %v <<<\n", maxWait)
				fmt.Printf("Final orchestrator status: %s\n", orchestratorStatus)
				return
			}

		case err := <-doneCh:
			if err != nil {
				fmt.Printf("Batch execution error: %v\n", err)
			}
			recoveryComplete = true
		}
	}

	// Display final results
	displayFinalResults(ctx, persistenceManager, redisClient, batchID)
}

// initializeComponents creates and initializes PostgreSQL and Redis connections
func initializeComponents(ctx context.Context) (*repository.Manager, *redis.Client, error) {
	fmt.Println("Initializing components...")

	// PostgreSQL
	persistenceConfig := &repository.Config{
		Strategy:      "postgres",
		ConnectionURL: databaseURL,
		Options: map[string]interface{}{
			"max_open_conns": 25,
			"max_idle_conns": 5,
		},
	}

	persistenceManager, err := repository.NewPersistenceManager(persistenceConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create persistence manager: %w", err)
	}

	if err := persistenceManager.Initialize(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to initialize persistence: %w", err)
	}

	// Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	fmt.Printf("Connected to PostgreSQL: %s\n", databaseURL)
	fmt.Printf("Connected to Redis: %s\n", redisURL)
	fmt.Println()

	return persistenceManager, redisClient, nil
}

// setupStateMachines creates worker and orchestrator state machines
func setupStateMachines(ctx context.Context, pm *repository.Manager, rdb *redis.Client) (
	*persistent.StateMachine, *persistent.StateMachine, error) {

	// Worker state machine (processes individual items)
	workerYAML := `
Comment: "Worker state machine for crash recovery example"
StartAt: ProcessData
States:
  ProcessData:
    Type: Task
    Resource: "worker:process-data"
    ResultPath: "$.processResult"
    End: true
`

	workerSM, err := persistent.New([]byte(workerYAML), false, workerSMID, pm)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create worker SM: %w", err)
	}

	if err := workerSM.SaveDefinition(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to save worker SM: %w", err)
	}

	// Register worker handler
	workerExec := executor.NewBaseExecutor()
	workerExec.RegisterGoFunction("worker:process-data", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("    → Worker processing data...")
		time.Sleep(500 * time.Millisecond) // Simulate work

		return map[string]interface{}{
			"processed": true,
			"timestamp": time.Now().Format(time.RFC3339),
		}, nil
	})

	// Set up queue client for worker (needed for micro-batch signaling)
	queueClient, err := queue.NewClient(&queue.Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: "localhost:6379",
		},
		Concurrency: 10,
		Queues: map[string]int{
			"default": 10,
		},
	})
	if err != nil {
		log.Printf("Warning: failed to create queue client: %v", err)
	} else {
		workerSM.SetQueueClient(queueClient)
	}
	workerSM.SetExecutor(workerExec)

	// Load orchestrator definition
	orchestratorSM, err := persistent.NewFromDefnId(ctx, orchestratorSMID, pm)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load orchestrator SM: %w", err)
	}

	fmt.Println("State machines initialized:")
	fmt.Printf("  - Worker SM: %s\n", workerSMID)
	fmt.Printf("  - Orchestrator SM: %s\n", orchestratorSMID)
	fmt.Println()

	return workerSM, orchestratorSM, nil
}

// createSourceExecutions creates dummy source executions in the database
func createSourceExecutions(ctx context.Context, pm *repository.Manager) ([]string, error) {
	var ids []string

	for i := 0; i < totalExecutions; i++ {
		execID := fmt.Sprintf("source-exec-%s-%03d", batchID, i)
		ids = append(ids, execID)

		execRecord := &repository.ExecutionRecord{
			ExecutionID:    execID,
			StateMachineID: "source-sm",
			Name:           fmt.Sprintf("SourceExecution-%03d", i),
			Status:         "SUCCEEDED",
			StartTime:      ptrTime(time.Now().Add(-1 * time.Hour)),
			EndTime:        ptrTime(time.Now().Add(-59 * time.Minute)),
			CurrentState:   "Complete",
			Input: map[string]interface{}{
				"id":    execID,
				"value": fmt.Sprintf("data-%03d", i),
			},
			Output: map[string]interface{}{
				"id":      execID,
				"processed": true,
				"data":    fmt.Sprintf("result-%03d", i),
			},
		}

		if err := pm.SaveExecution(ctx, &execution.Execution{
			ID:             execRecord.ExecutionID,
			StateMachineID: execRecord.StateMachineID,
			Name:           execRecord.Name,
			Status:         execRecord.Status,
			StartTime:      *execRecord.StartTime,
			EndTime:        *execRecord.EndTime,
			CurrentState:   execRecord.CurrentState,
			Input:          execRecord.Input,
			Output:         execRecord.Output,
		}); err != nil {
			return nil, fmt.Errorf("failed to save source execution %s: %w", execID, err)
		}
	}

	fmt.Printf("Created %d source executions\n", len(ids))
	return ids, nil
}

// simulateWorkerCompletion simulates a worker completing and decrementing barrier
func simulateWorkerCompletion(ctx context.Context, rdb *redis.Client, mbID string, workerIndex int) error {
	// Decrement barrier
	barrierKey := fmt.Sprintf("%s%s", barrierKeyPrefix, mbID)
	remaining, err := rdb.Decr(ctx, barrierKey).Result()
	if err != nil {
		return fmt.Errorf("failed to decrement barrier: %w", err)
	}

	fmt.Printf("    Worker %d completed, barrier remaining: %d\n", workerIndex+1, remaining)
	return nil
}

// getBarrierRemaining returns the current barrier counter value
func getBarrierRemaining(ctx context.Context, rdb *redis.Client, batchID string) int64 {
	// Check first micro-batch barrier
	mbID := fmt.Sprintf("%s-mb-0", batchID)
	barrierKey := fmt.Sprintf("%s%s", barrierKeyPrefix, mbID)
	
	remaining, err := rdb.Get(ctx, barrierKey).Int64()
	if err == redis.Nil {
		return 0
	}
	if err != nil {
		return -1
	}
	return remaining
}

// displayFinalResults shows the final state after recovery
func displayFinalResults(ctx context.Context, pm *repository.Manager, rdb *redis.Client, batchID string) {
	fmt.Println("=== Final Results ===")
	fmt.Println()

	// Orchestrator executions
	orchExecs, _ := pm.ListExecutions(ctx, &repository.ExecutionFilter{
		StateMachineID: orchestratorSMID,
	})
	
	fmt.Println("Orchestrator Executions:")
	for _, exec := range orchExecs {
		fmt.Printf("  ID: %s\n", exec.ExecutionID)
		fmt.Printf("  Status: %s\n", exec.Status)
		fmt.Printf("  Current State: %s\n", exec.CurrentState)
		if exec.RecoveryMetadata != nil {
			fmt.Printf("  Recovery Attempts: %d\n", exec.RecoveryMetadata.RecoveryAttemptCount)
		}
		fmt.Println()
	}

	// Worker executions
	workerExecs, _ := pm.ListExecutions(ctx, &repository.ExecutionFilter{
		StateMachineID: workerSMID,
	})

	fmt.Printf("Worker Executions: %d total\n", len(workerExecs))
	
	succeeded := 0
	failed := 0
	for _, exec := range workerExecs {
		if exec.Status == "SUCCEEDED" {
			succeeded++
		} else if exec.Status == "FAILED" {
			failed++
		}
	}
	fmt.Printf("  Succeeded: %d, Failed: %d\n", succeeded, failed)
	fmt.Println()

	// Redis keys status
	fmt.Println("Redis Keys Status:")
	cursorKey := fmt.Sprintf("batch:cursor:%s", batchID)
	cursor, _ := rdb.Get(ctx, cursorKey).Int()
	fmt.Printf("  Cursor: %d / %d\n", cursor, totalExecutions)
	
	idsListKey := fmt.Sprintf("batch:ids:%s", batchID)
	idsLen, _ := rdb.LLen(ctx, idsListKey).Result()
	fmt.Printf("  IDs List: %d items\n", idsLen)
	fmt.Println()

	// Barrier status for all micro-batches
	fmt.Println("Barrier Status:")
	for i := 0; i < (totalExecutions+microBatchSize-1)/microBatchSize; i++ {
		mbID := fmt.Sprintf("%s-mb-%d", batchID, i)
		barrierKey := fmt.Sprintf("%s%s", barrierKeyPrefix, mbID)
		remaining, err := rdb.Get(ctx, barrierKey).Int64()
		if err == redis.Nil {
			fmt.Printf("  Micro-batch %d: CLEARED (key deleted)\n", i)
		} else if err != nil {
			fmt.Printf("  Micro-batch %d: Error reading: %v\n", i, err)
		} else {
			fmt.Printf("  Micro-batch %d: %d remaining\n", i, remaining)
		}
	}
}

// cleanupPreviousRun removes Redis keys and optionally database records from previous runs
func cleanupPreviousRun(ctx context.Context, pm *repository.Manager, rdb *redis.Client) error {
	fmt.Println("Cleaning up previous run...")

	// Delete Redis keys
	keys := []string{
		fmt.Sprintf("batch:cursor:%s", batchID),
		fmt.Sprintf("batch:ids:%s", batchID),
		fmt.Sprintf("batch:metrics:%s", batchID),
	}

	for i := 0; i < 10; i++ {
		keys = append(keys, fmt.Sprintf("%s%s-mb-%d", barrierKeyPrefix, batchID, i))
	}

	if err := rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("failed to delete Redis keys: %w", err)
	}

	// Delete previous executions (optional - in production you'd archive)
	// For this example, we'll just use unique batch IDs
	
	fmt.Println("Cleanup complete")
	fmt.Println()
	return nil
}

// Helper functions
func ptrTime(t time.Time) *time.Time {
	return &t
}

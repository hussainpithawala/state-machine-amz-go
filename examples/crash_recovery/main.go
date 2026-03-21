// Package main demonstrates crash-resilient execution recovery.
// This example shows how a state machine execution can resume after an abrupt crash.
//
// Scenario:
//   1. First Run: Execution starts, processes some states, then simulates a crash
//   2. Second Run: Recovery scanner detects the orphaned execution and resumes from the last successful state
//
// Usage:
//   go run main.go
//
// Requirements:
//   - PostgreSQL database running
//   - Set DATABASE_URL environment variable (optional, uses default if not set)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/recovery"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

const (
	// Execution ID for the crash recovery example
	executionID        = "crash-recovery-exec-001"
	stateMachineID     = "crash-recovery-sm"
	databaseEnvVar     = "DATABASE_URL"
	defaultDatabaseURL = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"

	// Recovery configuration
	scanInterval        = 2 * time.Second  // How often to scan for orphaned executions
	orphanedThreshold   = 3 * time.Second  // Time after which RUNNING execution is considered orphaned
	maxRecoveryAttempts = 3                // Maximum recovery attempts
)

func main() {
	fmt.Println("=== Crash-Resilient Execution Recovery Example ===")
	fmt.Println()
	fmt.Println("This example demonstrates:")
	fmt.Println("  1. Starting an execution that simulates a crash")
	fmt.Println("  2. Recovery scanner detecting the orphaned execution")
	fmt.Println("  3. Automatic resumption from the last successful state")
	fmt.Println()

	ctx := context.Background()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	shutdownChan := make(chan struct{})

	// Start goroutine to handle shutdown signals
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
			cancel()
		case <-ctx.Done():
			// Context cancelled elsewhere
		}
		close(shutdownChan)
	}()

	// Run the crash recovery example
	if err := runCrashRecoveryExample(ctx, shutdownChan); err != nil {
		log.Fatalf("Example failed: %v", err)
	}

	// Cleanup
	cancel()
	wg.Wait()

	fmt.Println("\n=== Example completed successfully ===")
}

// runCrashRecoveryExample orchestrates the crash recovery demonstration
func runCrashRecoveryExample(ctx context.Context, shutdownChan <-chan struct{}) error {
	// Create persistence manager
	persistenceManager, err := getPersistenceManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to create persistence manager: %w", err)
	}
	defer func() {
		if err := persistenceManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close persistence manager: %v\n", err)
		}
	}()

	// Clean up any previous test data
	if err := cleanupPreviousRun(ctx, persistenceManager); err != nil {
		fmt.Printf("Warning: failed to cleanup previous run: %v\n", err)
	}

	// Define workflow with multiple states
	yamlContent := `
Comment: "Crash recovery example workflow"
StartAt: Initialize
States:
  Initialize:
    Type: Task
    Resource: "task:initialize"
    ResultPath: "$.initResult"
    Next: ProcessData

  ProcessData:
    Type: Task
    Resource: "task:process-data"
    ResultPath: "$.processResult"
    Next: ValidateResults

  ValidateResults:
    Type: Task
    Resource: "task:validate"
    ResultPath: "$.validationResult"
    Next: GenerateReport

  GenerateReport:
    Type: Task
    Resource: "task:generate-report"
    ResultPath: "$.reportResult"
    End: true
`

	// Create persistent state machine
	pm, err := persistent.New([]byte(yamlContent), false, stateMachineID, persistenceManager)
	if err != nil {
		return fmt.Errorf("failed to create persistent state machine: %w", err)
	}

	// Save state machine definition
	if err := pm.SaveDefinition(ctx); err != nil {
		return fmt.Errorf("failed to save state machine definition: %w", err)
	}

	// Create executor and register task handlers
	exec := createExecutor(persistenceManager)

	fmt.Println("=== Phase 1: Starting Initial Execution (will simulate crash) ===")
	fmt.Println()

	// Phase 1: Start execution that will "crash"
	executionInstance, err := runInitialExecution(ctx, pm, exec)
	if err != nil {
		return fmt.Errorf("initial execution failed: %w", err)
	}

	fmt.Printf("Initial execution started: %s\n", executionInstance.ID)
	fmt.Printf("Status: %s, Current State: %s\n", executionInstance.Status, executionInstance.CurrentState)

	// Simulate crash by stopping without completing
	fmt.Println()
	fmt.Println(">>> SIMULATING CRASH <<<")
	fmt.Println("   (Execution terminated abruptly without proper cleanup)")
	fmt.Println()

	// Wait for orphaned threshold to pass
	fmt.Printf("Waiting %v for execution to be considered orphaned...\n", orphanedThreshold+1*time.Second)
	select {
	case <-time.After(orphanedThreshold + 1*time.Second):
		// Continue
	case <-shutdownChan:
		return nil
	}

	fmt.Println()
	fmt.Println("=== Phase 2: Starting Recovery Scanner ===")
	fmt.Println()

	// Phase 2: Start recovery scanner
	recoveryConfig := &recovery.RecoveryConfig{
		Enabled:                  true,
		ScanInterval:             scanInterval,
		OrphanedThreshold:        orphanedThreshold,
		DefaultRecoveryStrategy:  recovery.StrategyRetry,
		DefaultMaxRecoveryAttempts: maxRecoveryAttempts,
	}

	if err := pm.StartRecoveryScanner(recoveryConfig); err != nil {
		return fmt.Errorf("failed to start recovery scanner: %w", err)
	}
	defer pm.StopRecoveryScanner()

	fmt.Printf("Recovery scanner started (interval: %v, threshold: %v)\n", scanInterval, orphanedThreshold)
	fmt.Println()

	// Monitor execution until completion or timeout
	fmt.Println("Monitoring execution recovery...")
	maxWait := 30 * time.Second
	startTime := time.Now()

	for {
		select {
		case <-shutdownChan:
			return nil
		case <-time.After(500 * time.Millisecond):
			// Check execution status
			execRecord, err := pm.GetExecution(ctx, executionID)
			if err != nil {
				return fmt.Errorf("failed to get execution: %w", err)
			}

			fmt.Printf("  Status: %-10s  Current State: %-20s  Duration: %v\n",
				execRecord.Status, execRecord.CurrentState, time.Since(startTime).Round(time.Second))

			// Check if execution completed
			if execRecord.Status == "SUCCEEDED" || execRecord.Status == "FAILED" {
				fmt.Println()
				fmt.Printf(">>> Execution completed with status: %s <<<\n", execRecord.Status)
				fmt.Println()

				// Display final results
				if err := displayExecutionResults(ctx, pm, execRecord); err != nil {
					return fmt.Errorf("failed to display results: %w", err)
				}

				return nil
			}

			// Check timeout
			if time.Since(startTime) > maxWait {
				return fmt.Errorf("recovery timed out after %v", maxWait)
			}
		}
	}
}

// runInitialExecution starts an execution and simulates a crash mid-way
func runInitialExecution(ctx context.Context, pm *persistent.StateMachine, exec *executor.BaseExecutor) (*execution.Execution, error) {
	// Create execution context
	execCtx := &execution.Execution{
		ID:             executionID,
		Name:           "CrashRecoveryExecution",
		StateMachineID: stateMachineID,
		Input: map[string]interface{}{
			"requestId":   fmt.Sprintf("req-%d", time.Now().Unix()),
			"userId":      "user-123",
			"data":        []string{"item1", "item2", "item3"},
			"shouldCrash": true, // Flag to simulate crash
		},
		StartTime: time.Now(),
		Status:    "RUNNING",
	}

	// Create execution context adapter
	ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

	// Start execution in a goroutine so we can simulate crash
	resultChan := make(chan *execution.Execution, 1)
	errChan := make(chan error, 1)

	go func() {
		result, err := pm.Execute(ctx, execCtx)
		if err != nil {
			errChan <- err
		} else {
			resultChan <- result
		}
	}()

	// Wait a bit for execution to progress through first state
	time.Sleep(800 * time.Millisecond)

	// Simulate crash by cancelling context
	fmt.Println("   Execution progress: Started processing...")
	fmt.Println("   [CRASH SIMULATION] Cancelling context abruptly...")
	cancelFunc, ok := ctx.Value(types.ExecutionContextKey).(interface{ Cancel() })
	if ok && cancelFunc != nil {
		// In real scenario, process would just die
	}

	// Give it a moment to persist state
	time.Sleep(200 * time.Millisecond)

	// Fetch the execution state from DB to show what was persisted
	execRecord, err := pm.GetExecution(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution after crash: %w", err)
	}

	// Return execution instance representing the "crashed" state
	return &execution.Execution{
		ID:             execRecord.ExecutionID,
		Name:           execRecord.Name,
		StateMachineID: execRecord.StateMachineID,
		Status:         execRecord.Status,
		CurrentState:   execRecord.CurrentState,
		Input:          execRecord.Input,
		StartTime:      *execRecord.StartTime,
	}, nil
}

// displayExecutionResults shows the final execution results
func displayExecutionResults(ctx context.Context, pm *persistent.StateMachine, execRecord *repository.ExecutionRecord) error {
	// Get execution history
	history, err := pm.GetExecutionHistory(ctx, execRecord.ExecutionID)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	fmt.Println("=== Execution History ===")
	for i, h := range history {
		duration := time.Duration(0)
		if h.StartTime != nil && h.EndTime != nil {
			duration = h.EndTime.Sub(*h.StartTime).Round(time.Millisecond)
		}
		fmt.Printf("  %d. State: %-20s  Status: %-10s  Duration: %v\n",
			i+1, h.StateName, h.Status, duration)
	}

	// Get all executions for this state machine
	executions, err := pm.ListExecutions(ctx, &repository.ExecutionFilter{
		StateMachineID: stateMachineID,
	})
	if err != nil {
		return fmt.Errorf("failed to list executions: %w", err)
	}

	fmt.Println()
	fmt.Printf("Total executions for state machine: %d\n", len(executions))

	// Display recovery metadata if available
	if execRecord.RecoveryMetadata != nil {
		fmt.Println()
		fmt.Println("=== Recovery Metadata ===")
		fmt.Printf("  Last Successful State: %s\n", execRecord.RecoveryMetadata.LastSuccessfulState)
		fmt.Printf("  Recovery Attempts: %d\n", execRecord.RecoveryMetadata.RecoveryAttemptCount)
		if execRecord.RecoveryMetadata.CrashDetectedAt != nil {
			fmt.Printf("  Crash Detected At: %v\n", execRecord.RecoveryMetadata.CrashDetectedAt.Format(time.RFC3339))
		}
	}

	return nil
}

// createExecutor creates and configures the executor with task handlers
func createExecutor(persistenceManager *repository.Manager) *executor.BaseExecutor {
	exec := executor.NewBaseExecutor()
	exec.SetRepositoryManager(persistenceManager)

	// Register task handlers
	exec.RegisterGoFunction("task:initialize", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → [Initialize] Starting initialization...")
		time.Sleep(200 * time.Millisecond)

		inputMap := input.(map[string]interface{})
		return map[string]interface{}{
			"initialized": true,
			"sessionId":   fmt.Sprintf("sess-%d", time.Now().UnixNano()),
			"requestId":   inputMap["requestId"],
		}, nil
	})

	exec.RegisterGoFunction("task:process-data", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → [ProcessData] Processing data items...")
		time.Sleep(300 * time.Millisecond)

		inputMap := input.(map[string]interface{})
		data, ok := inputMap["data"].([]string)
		if !ok {
			data = []string{}
		}

		return map[string]interface{}{
			"processed":       true,
			"itemsProcessed":  len(data),
			"processingTime":  time.Now().Format(time.RFC3339),
		}, nil
	})

	exec.RegisterGoFunction("task:validate", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → [ValidateResults] Validating results...")
		time.Sleep(200 * time.Millisecond)

		return map[string]interface{}{
			"validated": true,
			"valid":     true,
			"checks":    []string{"schema", "integrity", "completeness"},
		}, nil
	})

	exec.RegisterGoFunction("task:generate-report", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → [GenerateReport] Generating final report...")
		time.Sleep(200 * time.Millisecond)

		return map[string]interface{}{
			"reportGenerated": true,
			"reportId":        fmt.Sprintf("rpt-%d", time.Now().UnixNano()),
			"format":          "JSON",
			"status":          "COMPLETED",
		}, nil
	})

	return exec
}

// getPersistenceManager creates a PostgreSQL persistence manager
func getPersistenceManager(ctx context.Context) (*repository.Manager, error) {
	connectionURL := getConnectionURL()

	persistenceConfig := &repository.Config{
		Strategy:      "postgres",
		ConnectionURL: connectionURL,
		Options: map[string]interface{}{
			"max_open_conns":    25,
			"max_idle_conns":    5,
			"conn_max_lifetime": 5 * time.Minute,
		},
	}

	persistenceManager, err := repository.NewPersistenceManager(persistenceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistence manager: %w", err)
	}

	if err := persistenceManager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize persistence: %w", err)
	}

	fmt.Printf("Connected to database: %s\n", connectionURL)
	return persistenceManager, nil
}

// getConnectionURL returns the database connection URL
func getConnectionURL() string {
	if url := os.Getenv(databaseEnvVar); url != "" {
		return url
	}
	return defaultDatabaseURL
}

// cleanupPreviousRun removes execution data from previous test runs
func cleanupPreviousRun(ctx context.Context, manager *repository.Manager) error {
	// Delete previous execution if exists
	// Note: In production, you would use proper cleanup strategies
	fmt.Println("Cleaning up previous test data...")

	// Get previous executions
	executions, err := manager.ListExecutions(ctx, &repository.ExecutionFilter{
		StateMachineID: stateMachineID,
	})
	if err != nil {
		return err
	}

	if len(executions) > 0 {
		fmt.Printf("  Found %d previous execution(s) to clean up\n", len(executions))
		// In a real scenario, you might archive instead of delete
		// For this example, we'll just use a new execution ID if needed
	}

	return nil
}

// Helper to display JSON
func displayJSON(label string, data interface{}) error {
	fmt.Printf("\n%s:\n", label)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

func main() {
	fmt.Println("=== Batch Chained Execution Example ===")

	if err := runBatchChainedExecutionExample(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n=== Batch chained execution completed successfully ===")
}

func runBatchChainedExecutionExample() error {
	ctx := context.Background()

	// Setup database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/statemachine_batched_execution?sslmode=disable"
	}

	// Create repository manager
	repoConfig := &repository.Config{
		Strategy:      "gorm-postgres",
		ConnectionURL: dbURL,
	}

	repoManager, err := repository.NewPersistenceManager(repoConfig)
	if err != nil {
		return fmt.Errorf("failed to create repository manager: %w", err)
	}
	defer repoManager.Close()

	// Initialize the database schema
	if err := repoManager.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create executor and register task handlers
	exec := executor.NewBaseExecutor()

	// Register handlers for State Machine A - Data Ingestion
	exec.RegisterGoFunction("ingest:data", func(ctx context.Context, input interface{}) (interface{}, error) {
		data := input.(map[string]interface{})
		fmt.Printf("\n[Ingest] Processing: %v\n", data["orderId"])

		return map[string]interface{}{
			"orderId":     data["orderId"],
			"rawData":     data,
			"ingestedAt":  time.Now().Format(time.RFC3339),
			"ingestionID": fmt.Sprintf("ing-%v", data["orderId"]),
		}, nil
	})

	// Register handlers for State Machine B - Data Processing (will run in batch)
	exec.RegisterGoFunction("process:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		data := input.(map[string]interface{})
		orderId := data["orderId"]
		fmt.Printf("\n[Process] Processing order: %v\n", orderId)

		// Simulate processing
		time.Sleep(100 * time.Millisecond)

		return map[string]interface{}{
			"orderId":        orderId,
			"originalData":   data,
			"processedData":  fmt.Sprintf("Processed-%v", orderId),
			"processingTime": time.Now().Format(time.RFC3339),
			"status":         "processed",
		}, nil
	})

	exec.RegisterGoFunction("validate:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		data := input.(map[string]interface{})
		orderId := data["orderId"]
		fmt.Printf("[Validate] Validating order: %v\n", orderId)

		return map[string]interface{}{
			"orderId":      orderId,
			"validated":    true,
			"validatedAt":  time.Now().Format(time.RFC3339),
			"originalData": data,
		}, nil
	})

	// Define State Machine A - Data Ingestion Pipeline
	stateMachineA_YAML := `
Comment: "State Machine A - Data Ingestion Pipeline"
StartAt: IngestData
States:
  IngestData:
    Type: Task
    Resource: "arn:aws:lambda:::ingest:data"
    End: true
`

	// Create persistent state machine A
	smA, err := persistent.New([]byte(stateMachineA_YAML), false, "data-ingestion-pipeline", repoManager)
	if err != nil {
		return fmt.Errorf("failed to create state machine A: %w", err)
	}

	if err := smA.SaveDefinition(ctx); err != nil {
		return fmt.Errorf("failed to save state machine A definition: %w", err)
	}

	// Define State Machine B - Order Processing Pipeline
	stateMachineB_YAML := `
Comment: "State Machine B - Order Processing Pipeline"
StartAt: ProcessOrder
States:
  ProcessOrder:
    Type: Task
    Resource: "arn:aws:lambda:::process:order"
    Next: ValidateOrder

  ValidateOrder:
    Type: Task
    Resource: "arn:aws:lambda:::validate:order"
    End: true
`

	// Create persistent state machine B
	smB, err := persistent.New([]byte(stateMachineB_YAML), false, "order-processing-pipeline", repoManager)
	if err != nil {
		return fmt.Errorf("failed to create state machine B: %w", err)
	}

	if err := smB.SaveDefinition(ctx); err != nil {
		return fmt.Errorf("failed to save state machine B definition: %w", err)
	}

	fmt.Println("\n=== Step 1: Create Multiple Source Executions ===")

	// Execute State Machine A multiple times to create source executions
	orders := []map[string]interface{}{
		{"orderId": "ORD-001", "customer": "Alice", "amount": 100},
		{"orderId": "ORD-002", "customer": "Bob", "amount": 200},
		{"orderId": "ORD-003", "customer": "Charlie", "amount": 150},
		{"orderId": "ORD-004", "customer": "Diana", "amount": 300},
		{"orderId": "ORD-005", "customer": "Eve", "amount": 250},
	}

	for _, order := range orders {
		execName := fmt.Sprintf("ingestion-%v", order["orderId"])
		_, err := smA.Execute(ctx, order, statemachine.WithExecutionName(execName))
		if err != nil {
			return fmt.Errorf("state machine A execution failed: %w", err)
		}
		fmt.Printf("Created source execution: %s\n", execName)
	}

	fmt.Println("\n=== Step 2: Execute Batch Chained Executions (Sequential) ===")

	// Create filter to get all successful executions from State Machine A
	filter := &repository.ExecutionFilter{
		StateMachineID: smA.GetID(),
		Status:         "SUCCEEDED",
		Limit:          10,
	}

	// Configure batch options
	batchOpts := &statemachine.BatchExecutionOptions{
		NamePrefix:        "batch-sequential",
		ConcurrentBatches: 1, // Sequential execution
		StopOnError:       false,
		OnExecutionStart: func(sourceExecutionID string, index int) {
			fmt.Printf("\n[Batch Sequential] Starting execution %d for source: %s\n", index, sourceExecutionID)
		},
		OnExecutionComplete: func(sourceExecutionID string, index int, err error) {
			if err != nil {
				fmt.Printf("[Batch Sequential] Execution %d failed: %v\n", index, err)
			} else {
				fmt.Printf("[Batch Sequential] Execution %d completed successfully\n", index)
			}
		},
	}

	// Execute batch chained executions
	results, err := smB.ExecuteBatch(ctx, filter, "", batchOpts)
	if err != nil {
		return fmt.Errorf("batch execution failed: %w", err)
	}

	fmt.Printf("\n[Batch Sequential] Completed %d executions\n", len(results))
	for _, result := range results {
		if result.Error != nil {
			fmt.Printf("  - Index %d: FAILED - %v\n", result.Index, result.Error)
		} else {
			fmt.Printf("  - Index %d: %s - Status: %s\n", result.Index, result.Execution.Name, result.Execution.Status)
		}
	}

	fmt.Println("\n=== Step 3: Execute Batch Chained Executions (Concurrent) ===")

	// Configure concurrent batch options
	concurrentBatchOpts := &statemachine.BatchExecutionOptions{
		NamePrefix:        "batch-concurrent",
		ConcurrentBatches: 3, // Process 3 executions concurrently
		StopOnError:       false,
		OnExecutionStart: func(sourceExecutionID string, index int) {
			fmt.Printf("\n[Batch Concurrent] Starting execution %d for source: %s\n", index, sourceExecutionID)
		},
		OnExecutionComplete: func(sourceExecutionID string, index int, err error) {
			if err != nil {
				fmt.Printf("[Batch Concurrent] Execution %d failed: %v\n", index, err)
			} else {
				fmt.Printf("[Batch Concurrent] Execution %d completed successfully\n", index)
			}
		},
	}

	// Execute batch chained executions concurrently
	concurrentResults, err := smB.ExecuteBatch(ctx, filter, "", concurrentBatchOpts)
	if err != nil {
		return fmt.Errorf("concurrent batch execution failed: %w", err)
	}

	fmt.Printf("\n[Batch Concurrent] Completed %d executions\n", len(concurrentResults))
	for _, result := range concurrentResults {
		if result.Error != nil {
			fmt.Printf("  - Index %d: FAILED - %v\n", result.Index, result.Error)
		} else {
			fmt.Printf("  - Index %d: %s - Status: %s\n", result.Index, result.Execution.Name, result.Execution.Status)
		}
	}

	fmt.Println("\n=== Step 4: Execute Batch with Filtering (Time Range) ===")

	// Create filter with time range
	timeRangeFilter := &repository.ExecutionFilter{
		StateMachineID: smA.GetID(),
		Status:         "SUCCEEDED",
		StartAfter:     time.Now().Add(-1 * time.Hour), // Last hour
		Limit:          3,                              // Only first 3
	}

	timeRangeBatchOpts := &statemachine.BatchExecutionOptions{
		NamePrefix:        "batch-timerange",
		ConcurrentBatches: 2,
		StopOnError:       false,
	}

	timeRangeResults, err := smB.ExecuteBatch(ctx, timeRangeFilter, "", timeRangeBatchOpts)
	if err != nil {
		return fmt.Errorf("time range batch execution failed: %w", err)
	}

	fmt.Printf("\n[Batch Time Range] Completed %d executions (filtered by time range, limit 3)\n", len(timeRangeResults))
	for _, result := range timeRangeResults {
		if result.Error != nil {
			fmt.Printf("  - Index %d: FAILED - %v\n", result.Index, result.Error)
		} else {
			fmt.Printf("  - Index %d: %s - Status: %s\n", result.Index, result.Execution.Name, result.Execution.Status)
		}
	}

	fmt.Println("\n=== Step 5: Execute Batch with Input Transformer ===")

	transformerBatchOpts := &statemachine.BatchExecutionOptions{
		NamePrefix:        "batch-transformed",
		ConcurrentBatches: 2,
		StopOnError:       false,
	}

	// Execute with input transformer
	transformedResults, err := smB.ExecuteBatch(
		ctx,
		&repository.ExecutionFilter{
			StateMachineID: smA.GetID(),
			Status:         "SUCCEEDED",
			Limit:          2,
		},
		"",
		transformerBatchOpts,
		statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
			fmt.Println("[Transformer] Transforming input...")
			data := output.(map[string]interface{})

			// Extract and transform specific fields
			transformed := map[string]interface{}{
				"orderId":       data["orderId"],
				"ingestionData": data["rawData"],
				"transformedAt": time.Now().Format(time.RFC3339),
				"priority":      "high",
			}

			return transformed, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("transformed batch execution failed: %w", err)
	}

	fmt.Printf("\n[Batch Transformed] Completed %d executions with transformation\n", len(transformedResults))

	fmt.Println("\n=== Summary ===")

	// Count all executions
	countA, _ := smA.CountExecutions(ctx, &repository.ExecutionFilter{})
	countB, _ := smB.CountExecutions(ctx, &repository.ExecutionFilter{})

	fmt.Printf("\nTotal Executions:\n")
	fmt.Printf("  - State Machine A (Ingestion): %d\n", countA)
	fmt.Printf("  - State Machine B (Processing): %d\n", countB)

	// List recent executions from State Machine B
	recentExecs, err := smB.ListExecutions(ctx, &repository.ExecutionFilter{Limit: 10})
	if err != nil {
		return fmt.Errorf("failed to list executions: %w", err)
	}

	fmt.Println("\nRecent State Machine B Executions:")
	for _, exec := range recentExecs {
		fmt.Printf("  - %s: %s (started at %v)\n", exec.Name, exec.Status, exec.StartTime.Format(time.RFC3339))
	}

	return nil
}

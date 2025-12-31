package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

func main() {
	fmt.Println("=== Chained Execution Example ===")

	if err := runChainedExecutionExample(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n=== Chained execution completed successfully ===")
}

// runChainedExecutionExample demonstrates chaining state machine executions
func runChainedExecutionExample() error {
	ctx := context.Background()

	// Setup database connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/statemachine_db?sslmode=disable"
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

	// Register handlers for state machine A
	exec.RegisterGoFunction("process:data", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("\n[State Machine A] Processing data...")
		data := input.(map[string]interface{})

		// Process the data
		result := map[string]interface{}{
			"rawData":       data,
			"processedData": fmt.Sprintf("Processed: %v", data["input"]),
			"timestamp":     "2024-01-01T12:00:00Z",
		}

		fmt.Printf("[State Machine A] Data processed: %v\n", result["processedData"])
		return result, nil
	})

	exec.RegisterGoFunction("validate:data", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("\n[State Machine A] Validating data...")
		data := input.(map[string]interface{})

		result := map[string]interface{}{
			"validated": true,
			"data":      data,
			"status":    "valid",
		}

		fmt.Println("[State Machine A] Data validated successfully")
		return result, nil
	})

	// Register handlers for state machine B
	exec.RegisterGoFunction("enrich:data", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("\n[State Machine B] Enriching data from previous execution...")
		data := input.(map[string]interface{})

		result := map[string]interface{}{
			"original":     data,
			"enrichedData": "Additional information added",
			"score":        95,
		}

		fmt.Printf("[State Machine B] Data enriched: %v\n", result)
		return result, nil
	})

	exec.RegisterGoFunction("store:data", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("\n[State Machine B] Storing final data...")
		data := input.(map[string]interface{})

		result := map[string]interface{}{
			"stored":  true,
			"data":    data,
			"message": "Data successfully stored in database",
		}

		fmt.Println("[State Machine B] Data stored successfully")
		return result, nil
	})

	// Define State Machine A - Data Processing Pipeline
	stateMachineA_YAML := `
Comment: "State Machine A - Data Processing Pipeline"
StartAt: ProcessData
States:
  ProcessData:
    Type: Task
    Resource: "arn:aws:lambda:::process:data"
    ResultPath: "$.processResult"
    Next: ValidateData

  ValidateData:
    Type: Task
    Resource: "arn:aws:lambda:::validate:data"
    ResultPath: "$.validationResult"
    End: true
`

	// Create persistent state machine A
	smA, err := persistent.New([]byte(stateMachineA_YAML), false, "data-processing-pipeline", repoManager)
	if err != nil {
		return fmt.Errorf("failed to create state machine A: %w", err)
	}

	// Save state machine A definition
	if err := smA.SaveDefinition(ctx); err != nil {
		return fmt.Errorf("failed to save state machine A definition: %w", err)
	}

	fmt.Println("\n=== Step 1: Execute State Machine A ===")

	// Execute State Machine A
	inputA := map[string]interface{}{
		"input":    "Raw sensor data from device 123",
		"deviceId": "device-123",
	}

	execA, err := smA.Execute(ctx, inputA, statemachine.WithExecutionName("execution-A-001"))
	if err != nil {
		return fmt.Errorf("state machine A execution failed: %w", err)
	}

	fmt.Printf("\n[Execution A] Status: %s\n", execA.Status)
	fmt.Printf("[Execution A] ID: %s\n", execA.ID)
	outputJSON, _ := json.MarshalIndent(execA.Output, "", "  ")
	fmt.Printf("[Execution A] Output:\n%s\n", string(outputJSON))

	// Define State Machine B - Data Enrichment Pipeline
	stateMachineB_YAML := `
Comment: "State Machine B - Data Enrichment Pipeline"
StartAt: EnrichData
States:
  EnrichData:
    Type: Task
    Resource: "arn:aws:lambda:::enrich:data"
    ResultPath: "$.enrichResult"
    Next: StoreData

  StoreData:
    Type: Task
    Resource: "arn:aws:lambda:::store:data"
    ResultPath: "$.storeResult"
    End: true
`

	// Create persistent state machine B
	smB, err := persistent.New([]byte(stateMachineB_YAML), false, "data-enrichment-pipeline", repoManager)
	if err != nil {
		return fmt.Errorf("failed to create state machine B: %w", err)
	}

	// Save state machine B definition
	if err := smB.SaveDefinition(ctx); err != nil {
		return fmt.Errorf("failed to save state machine B definition: %w", err)
	}

	fmt.Println("\n=== Step 2: Execute State Machine B using output from State Machine A ===")

	// Example 1: Chain using final output from State Machine A
	fmt.Println("\n--- Example 1: Using final output from Execution A ---")
	execB1, err := smB.Execute(ctx, nil,
		statemachine.WithExecutionName("execution-B-001"),
		statemachine.WithSourceExecution(execA.ID),
	)
	if err != nil {
		return fmt.Errorf("state machine B execution (example 1) failed: %w", err)
	}

	fmt.Printf("\n[Execution B1] Status: %s\n", execB1.Status)
	fmt.Printf("[Execution B1] ID: %s\n", execB1.ID)
	outputJSON, _ = json.MarshalIndent(execB1.Output, "", "  ")
	fmt.Printf("[Execution B1] Output:\n%s\n", string(outputJSON))

	// Example 2: Chain using specific state output from State Machine A
	fmt.Println("\n--- Example 2: Using output from 'ProcessData' state of Execution A ---")
	execB2, err := smB.Execute(ctx, nil,
		statemachine.WithExecutionName("execution-B-002"),
		statemachine.WithSourceExecution(execA.ID, "ProcessData"),
	)
	if err != nil {
		return fmt.Errorf("state machine B execution (example 2) failed: %w", err)
	}

	fmt.Printf("\n[Execution B2] Status: %s\n", execB2.Status)
	fmt.Printf("[Execution B2] ID: %s\n", execB2.ID)
	outputJSON, _ = json.MarshalIndent(execB2.Output, "", "  ")
	fmt.Printf("[Execution B2] Output:\n%s\n", string(outputJSON))

	// Example 3: Chain with input transformation
	fmt.Println("\n--- Example 3: Using transformation function ---")
	execB3, err := smB.Execute(ctx, nil,
		statemachine.WithExecutionName("execution-B-003"),
		statemachine.WithSourceExecution(execA.ID, "ValidateData"),
		statemachine.WithInputTransformer(func(output interface{}) (interface{}, error) {
			fmt.Println("[Transformer] Transforming input from Execution A...")
			data := output.(map[string]interface{})

			// Extract only specific fields and add metadata
			transformed := map[string]interface{}{
				"validatedData": data["validationResult"],
				"source":        "execution-A-001",
				"transformedAt": "2024-01-01T12:05:00Z",
			}

			fmt.Printf("[Transformer] Transformed: %v\n", transformed)
			return transformed, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("state machine B execution (example 3) failed: %w", err)
	}

	fmt.Printf("\n[Execution B3] Status: %s\n", execB3.Status)
	fmt.Printf("[Execution B3] ID: %s\n", execB3.ID)
	outputJSON, _ = json.MarshalIndent(execB3.Output, "", "  ")
	fmt.Printf("[Execution B3] Output:\n%s\n", string(outputJSON))

	// List all executions to show the chain
	fmt.Println("\n=== Summary of All Executions ===")

	executionsA, err := smA.ListExecutions(ctx, &repository.ExecutionFilter{Limit: 10})
	if err != nil {
		return fmt.Errorf("failed to list executions for State Machine A: %w", err)
	}

	fmt.Println("\nState Machine A Executions:")
	for _, exec := range executionsA {
		fmt.Printf("  - ID: %s, Name: %s, Status: %s\n", exec.ExecutionID, exec.Name, exec.Status)
	}

	executionsB, err := smB.ListExecutions(ctx, &repository.ExecutionFilter{Limit: 10})
	if err != nil {
		return fmt.Errorf("failed to list executions for State Machine B: %w", err)
	}

	fmt.Println("\nState Machine B Executions (all chained from A):")
	for _, exec := range executionsB {
		fmt.Printf("  - ID: %s, Name: %s, Status: %s\n", exec.ExecutionID, exec.Name, exec.Status)
	}

	return nil
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
	"gopkg.in/yaml.v3"
)

func main() {
	fmt.Println("=== PostgreSQL with YAML Configuration Example ===")

	if err := runSimpleWorkflowExample(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== All examples completed successfully ===")
}

// runSimpleWorkflowExample demonstrates basic workflow with YAML
func runSimpleWorkflowExample() error {
	fmt.Println("--- Simple Workflow Example ---")

	ctx := context.Background()

	// 1. Load YAML workflow definition
	yamlContent := `
Comment: "Simple order processing workflow"
StartAt: ProcessOrder
States:
  ProcessOrder:
    Type: Task
    Resource: "arn:aws:lambda:::process:order"
    ResultPath: "$.orderResult"
    Next: ValidatePayment

  ValidatePayment:
    Type: Task
    Resource: "arn:aws:lambda:::validate:payment"
    ResultPath: "$.paymentResult"
    Next: SendNotification

  SendNotification:
    Type: Task
    Resource: "arn:aws:lambda:::send:notification"
    ResultPath: "$.notificationResult"
    End: true
`

	// 1. Create persistence manager
	persistenceManager, err2 := getPersistenceManager(ctx)
	if err2 != nil {
		return err2
	}

	// 2. Create state machine from definition
	pm, err := persistent.New([]byte(yamlContent), false, "sm-1", persistenceManager)
	if err != nil {
		return fmt.Errorf("failed to create a peristent state machine: %w", err)
	}

	// 5. Create executor and register task handlers
	exec := executor.NewBaseExecutor()

	// Register handlers for the resources defined in YAML
	exec.RegisterGoFunction("process:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → Processing order...")
		time.Sleep(100 * time.Millisecond)

		inputMap := input.(map[string]interface{})
		return map[string]interface{}{
			"orderId":   inputMap["orderId"],
			"processed": true,
			"timestamp": time.Now().Format(time.RFC3339),
		}, nil
	})

	exec.RegisterGoFunction("validate:payment", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → Validating payment...")
		time.Sleep(100 * time.Millisecond)

		return map[string]interface{}{
			"valid":         true,
			"transactionId": "TXN-" + time.Now().Format("20060102150405"),
		}, nil
	})

	exec.RegisterGoFunction("send:notification", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → Sending notification...")
		time.Sleep(100 * time.Millisecond)

		return map[string]interface{}{
			"notificationSent": true,
			"channel":          "email",
		}, nil
	})

	// 6. Create execution context with persistence
	execCtx := &execution.Execution{
		ID:             "exec-simple-001",
		Name:           "SimpleOrderProcessing",
		StateMachineID: "simple-workflow-v1",
		Input: map[string]interface{}{
			"orderId": "ORD-12345",
			"amount":  150.00,
			"customer": map[string]interface{}{
				"id":   "CUST-001",
				"name": "John Doe",
			},
		},
		StartTime: time.Now(),
		Status:    "RUNNING",
	}

	// Save initial execution state
	//if err := pm.SaveExecution(ctx, execCtx); err != nil {
	//	return fmt.Errorf("failed to save initial execution: %w", err)
	//}

	// 7. Execute the workflow
	fmt.Println("\nExecuting workflow...")

	executionInstance, err := pm.Execute(ctx, execCtx)

	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	fmt.Printf("\n✓ Execution completed: %s (Status: %s)\n", executionInstance.ID, executionInstance.Status)

	// 8. Retrieve and display history from database
	history, err := pm.GetExecutionHistory(ctx, executionInstance.ID)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	} else {
		fmt.Println("\nExecution History:")
		err := yaml.NewEncoder(os.Stdout).Encode(history)
		if err != nil {
			return fmt.Errorf("failed to marshal history: %w", err)
		}
	}
	defer func(pm *repository.Manager) {
		err := pm.Close()
		if err != nil {
			fmt.Printf("Warning: failed to close persistence manager: %v\n", err)
		} else {
			fmt.Printf("Info: Closed the persistence manager: %v\n", err)
		}
	}(persistenceManager)

	return nil
}

func getPersistenceManager(ctx context.Context) (*repository.Manager, error) {
	// 4. Configure PostgreSQL persistence
	persistenceConfig := &repository.Config{
		Strategy:      "postgres",
		ConnectionURL: getConnectionURL(),
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

	// Initialize schema
	if err := persistenceManager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize persistence: %w", err)
	}
	return persistenceManager, nil
}

func getConnectionURL() string {
	// Check environment variable first
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	// Default for local development
	return "postgres://postgres:postgres@localhost:5432/statemachine_example_simple?sslmode=disable"
}

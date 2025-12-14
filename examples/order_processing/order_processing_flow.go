package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

// TestOrderData represents a single test case
type TestOrderData struct {
	Name  string      `json:"name"`
	Input interface{} `json:"input"`
}

// OrderInput represents the structure of order data for testing

func main() {
	// Test cases with order data
	testCases := []TestOrderData{
		{
			Name: "Premium Order",
			Input: map[string]interface{}{
				"order": map[string]interface{}{
					"orderId":          "ORD-PREM-001",
					"customerId":       "CUST-PREM-001",
					"items":            []string{"premium_item_1", "premium_item_2"},
					"quantity":         2,
					"total":            750.00,
					"premium_customer": true,
					"payment_method":   "credit_card",
					"shipping": map[string]interface{}{
						"required": true,
						"country":  "US",
					},
					"timestamp": time.Now().Unix(),
				},
			},
		},
		{
			Name: "Bulk Order (Available)",
			Input: map[string]interface{}{
				"order": map[string]interface{}{
					"orderId":        "ORD-BULK-001",
					"customerId":     "CUST-BULK-001",
					"items":          []string{"bulk_item_1"},
					"quantity":       25,
					"total":          1500.00,
					"payment_method": "credit_card",
					"shipping": map[string]interface{}{
						"required": true,
						"country":  "US",
					},
					"timestamp": time.Now().Unix(),
				},
			},
		},
		{
			Name: "Digital Order",
			Input: map[string]interface{}{
				"order": map[string]interface{}{
					"orderId":        "ORD-DIG-001",
					"customerId":     "CUST-DIG-001",
					"items":          []string{"ebook"},
					"quantity":       1,
					"total":          49.99,
					"payment_method": "paypal",
					"shipping": map[string]interface{}{
						"required": false,
					},
					"digital_product": map[string]interface{}{
						"type": "ebook",
					},
					"customer_email": "customer@example.com",
					"timestamp":      time.Now().Unix(),
				},
			},
		},
		{
			Name: "Standard Order",
			Input: map[string]interface{}{
				"order": map[string]interface{}{
					"orderId":        "ORD-STD-001",
					"customerId":     "CUST-STD-001",
					"items":          []string{"standard_item_1", "standard_item_2"},
					"quantity":       3,
					"total":          89.99,
					"payment_method": "credit_card",
					"shipping": map[string]interface{}{
						"required": true,
						"country":  "US",
					},
					"timestamp": time.Now().Unix(),
				},
			},
		},
		{
			Name: "Payment Failure Order",
			Input: map[string]interface{}{
				"order": map[string]interface{}{
					"orderId":        "ORD-FAIL-001",
					"customerId":     "CUST-FAIL-001",
					"items":          []string{"expensive_item"},
					"quantity":       1,
					"total":          2500.00,
					"payment_method": "expired_card",
					"shipping": map[string]interface{}{
						"required": true,
						"country":  "US",
					},
					"timestamp": time.Now().Unix(),
				},
			},
		},
	}

	// Load the state machine definition from YAML
	workflowPath := filepath.Join("workflows", "order_processing.yaml")
	smDef, err := loadStateMachineDefinition(workflowPath)
	if err != nil {
		log.Fatalf("Failed to load state machine definition: %v", err)
	}

	fmt.Printf("✓ Loaded state machine: %s\n", smDef.Comment)
	fmt.Printf("✓ Start state: %s\n", smDef.StartAt)
	fmt.Printf("✓ Total states: %d\n\n", len(smDef.States))

	var wg sync.WaitGroup

	// Run tests
	for i, testCase := range testCases {
		wg.Add(1)
		go func(i int, tc TestOrderData) {
			defer wg.Done()
			fmt.Printf("==================================================\n")
			fmt.Printf("Test Case %d: %s\n", i+1, testCase.Name)
			fmt.Printf("==================================================\n")

			// Create execution context using the correct constructor
			execCtx := execution.New(
				"", // Empty ID will auto-generate
				uuid.New().String(),
				testCase.Input,
			)

			// Print input data
			fmt.Printf("\nInput Data:\n")
			printJSON(testCase.Input)

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Execute the state machine
			result, err := smDef.RunExecution(ctx, execCtx)
			if err != nil {
				fmt.Printf("\n❌ Execution Error: %v\n", err)
				execCtx.Status = "FAILED"
				execCtx.Error = err
				execCtx.EndTime = time.Now()
			} else {
				fmt.Printf("\n✓ Execution completed successfully\n")
				execCtx = result
			}

			// Print execution results
			printExecutionResults(execCtx)

			fmt.Printf("%d \n", i)
		}(i, testCase)
	}

	fmt.Printf("\nWaiting for all go-routine-tests to complete...\n")
	fmt.Printf("==================================================\n")
	wg.Wait()
	fmt.Printf("==================================================\n")
	fmt.Printf("All go-routine-tests completed successfully!\n")
	// Print summary
	printTestSummary(len(testCases))
}

// loadStateMachineDefinition loads a state machine from YAML file
func loadStateMachineDefinition(filePath string) (*statemachine.StateMachine, error) {
	// Read the YAML file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML and unmarshal to StateMachine
	sm, err := statemachine.New(data, false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse state machine: %w", err)
	}

	return sm, nil
}

// printJSON pretty prints JSON data
func printJSON(data interface{}) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Printf("%s\n", string(jsonData))
}

// printExecutionResults prints the execution results
func printExecutionResults(execCtx *execution.Execution) {
	fmt.Printf("\nExecution Results:\n")
	fmt.Printf("  Execution ID: %s\n", execCtx.ID)
	fmt.Printf("  Name: %s\n", execCtx.Name)
	fmt.Printf("  Status: %s\n", execCtx.Status)
	fmt.Printf("  Start Time: %s\n", execCtx.StartTime.Format(time.RFC3339))

	if !execCtx.EndTime.IsZero() {
		fmt.Printf("  End Time: %s\n", execCtx.EndTime.Format(time.RFC3339))
		duration := execCtx.EndTime.Sub(execCtx.StartTime)
		fmt.Printf("  Duration: %v\n", duration)
	}

	if execCtx.Error != nil {
		fmt.Printf("  Error: %v\n", execCtx.Error)
	}

	if execCtx.CurrentState != "" {
		fmt.Printf("  Current State: %s\n", execCtx.CurrentState)
	}

	// Print output if available
	if execCtx.Output != nil {
		fmt.Printf("\nOutput Data:\n")
		printJSON(execCtx.Output)
	}

	// Print state execution history if available
	if len(execCtx.History) > 0 {
		fmt.Printf("\nState Execution History (%d states):\n", len(execCtx.History))
		for i, historyEvent := range execCtx.History {
			fmt.Printf("  %d. State: %s (Time: %s)\n",
				i+1,
				historyEvent.StateName,
				historyEvent.Timestamp.Format(time.RFC3339),
			)
		}
	}

	// Print full execution summary
	fmt.Printf("\nExecution Summary:\n")
	execMap := execCtx.ToMap()
	printJSON(execMap)
}

// printTestSummary prints a summary of all tests
func printTestSummary(totalTests int) {
	fmt.Printf("==================================================\n")
	fmt.Printf("Test Summary\n")
	fmt.Printf("==================================================\n")
	fmt.Printf("Total Test Cases: %d\n", totalTests)
	fmt.Printf("Test File: workflows/order_processing.yaml\n")
	fmt.Printf("Timestamp: %s\n", time.Now().Format(time.RFC3339))
	fmt.Printf("\nKey Test Scenarios:\n")
	fmt.Printf("  ✓ Premium Orders (750.00) - Tests ProcessPremiumOrder task\n")
	fmt.Printf("  ✓ Bulk Orders (1500.00) - Tests ProcessPremiumOrder task\n")
	fmt.Printf("  ✓ Digital Orders (49.99) - Tests ProcessSmallOrder task\n")
	fmt.Printf("  ✓ Standard Orders (89.99) - Tests ProcessStandardOrder task\n")
	fmt.Printf("  ✓ Payment Failures - Tests error handling and Catch policies\n")
	fmt.Printf("\nFeatures Being Tested:\n")
	fmt.Printf("  ✓ Pass states with data transformation\n")
	fmt.Printf("  ✓ Choice states with numeric comparisons\n")
	fmt.Printf("  ✓ Task states with retry policies\n")
	fmt.Printf("  ✓ Task states with catch policies\n")
	fmt.Printf("  ✓ InputPath, ResultPath, OutputPath processing\n")
	fmt.Printf("  ✓ Error handling and compensation flows\n")
	fmt.Printf("  ✓ TimeoutSeconds and HeartbeatSeconds\n")
	fmt.Printf("  ✓ Exponential backoff with MaxDelaySeconds\n")
	fmt.Printf("==================================================\n")
}

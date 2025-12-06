// examples/parallel_workflow_yaml.go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

// TaskRegistry that implements states.ExecutionContext interface
type TaskRegistry struct {
	handlers map[string]func(context.Context, interface{}) (interface{}, error)
}

func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{
		handlers: make(map[string]func(context.Context, interface{}) (interface{}, error)),
	}
}

func (r *TaskRegistry) RegisterTaskHandler(resourceURI string, handler func(context.Context, interface{}) (interface{}, error)) {
	r.handlers[resourceURI] = handler
}

func (r *TaskRegistry) GetTaskHandler(resourceURI string) (func(context.Context, interface{}) (interface{}, error), bool) {
	handler, exists := r.handlers[resourceURI]
	return handler, exists
}

// MockExecutor wraps BaseExecutor and adds task registry support
type MockExecutor struct {
	*executor.BaseExecutor
	taskRegistry *TaskRegistry
}

func NewMockExecutor() *MockExecutor {
	base := executor.NewBaseExecutor()
	return &MockExecutor{
		BaseExecutor: base,
		taskRegistry: NewTaskRegistry(),
	}
}

func (e *MockExecutor) RegisterTaskHandler(resourceURI string, handler func(context.Context, interface{}) (interface{}, error)) {
	e.taskRegistry.RegisterTaskHandler(resourceURI, handler)
	// Also register with the executor's registry
	e.RegisterGoFunction(resourceURI, handler)
}

func (e *MockExecutor) GetTaskRegistry() *TaskRegistry {
	return e.taskRegistry
}

// YAML State Machine Definition
const parallelWorkflowYAML = `
Comment: "Parallel Order Processing Workflow"
StartAt: ValidateOrder
States:
  ValidateOrder:
    Type: Task
    Resource: "arn:aws:lambda:::validate:order"
    Next: ParallelProcessing
    ResultPath: "$.validation"
    TimeoutSeconds: 10

  ParallelProcessing:
    Type: Parallel
    Next: GenerateInvoice
    Branches:
      - StartAt: ProcessPayment
        States:
          ProcessPayment:
            Type: Task
            Resource: "arn:aws:lambda:::process:payment"
            Next: ApplyDiscounts
            ResultPath: "$.paymentResult"
            Retry:
              - ErrorEquals: ["States.TaskFailed"]
                MaxAttempts: 3
                BackoffRate: 2.0
          ApplyDiscounts:
            Type: Task
            Resource: "arn:aws:lambda:::apply:discounts"
            End: true
            ResultPath: "$.discounts"

      - StartAt: CheckInventory
        States:
          CheckInventory:
            Type: Task
            Resource: "arn:aws:lambda:::check:inventory"
            Next: CalculateShipping
            ResultPath: "$.inventory"
            Retry:
              - ErrorEquals: ["States.TaskFailed"]
                MaxAttempts: 5
                IntervalSeconds: 1
                BackoffRate: 1.5
            Catch:
              - ErrorEquals: ["States.ALL"]
                ResultPath: "$.inventoryError"
                Next: HandleInventoryError
          CalculateShipping:
            Type: Task
            Resource: "arn:aws:lambda:::calculate:shipping"
            End: true
            ResultPath: "$.shipping"
          HandleInventoryError:
            Type: Pass
            Result:
              message: "Inventory check failed, using backup stock"
            ResultPath: "$.inventoryFallback"
            End: true

      - StartAt: UpdateCRM
        States:
          UpdateCRM:
            Type: Task
            Resource: "arn:aws:lambda:::update:crm"
            Next: LogAnalytics
            ResultPath: "$.crm"
            TimeoutSeconds: 5
          LogAnalytics:
            Type: Task
            Resource: "arn:aws:lambda:::log:analytics"
            End: true
            ResultPath: "$.analytics"

    ResultPath: "$.parallelResults"

  GenerateInvoice:
    Type: Task
    Resource: "arn:aws:lambda:::generate:invoice"
    Next: SendNotification
    ResultPath: "$.invoice"
    Parameters:
      orderDetails.$: "$"
      customer.$: "$.customer"
      payment.$: "$.paymentResult"
      shipping.$: "$.shipping"

  SendNotification:
    Type: Task
    Resource: "arn:aws:lambda:::send:notification"
    End: true
    ResultPath: "$.notification"
    Retry:
      - ErrorEquals: ["States.TaskFailed"]
        MaxAttempts: 2
`

// Choice Workflow YAML
const choiceWorkflowYAML = `
Comment: "Choice Workflow Example"
StartAt: CheckOrderValue
States:
  CheckOrderValue:
    Type: Task
    Resource: "arn:aws:lambda:::validate:order"
    Next: RouteOrder
    ResultPath: "$.validation"

  RouteOrder:
    Type: Choice
    Choices:
      - Variable: "$.validation.totals.total"
        NumericGreaterThan: 1000
        Next: ProcessLargeOrder
      - Variable: "$.validation.customer.membership"
        StringEquals: "PREMIUM"
        Next: ProcessPremium
      - Variable: "$.validation.items[0].category"
        StringEquals: "ELECTRONICS"
        Next: ProcessElectronics
    Default: ProcessStandard

  ProcessStandard:
    Type: Task
    Resource: "arn:aws:lambda:::process:payment"
    End: true

  ProcessLargeOrder:
    Type: Task
    Resource: "arn:aws:lambda:::check:inventory"
    End: true

  ProcessPremium:
    Type: Task
    Resource: "arn:aws:lambda:::apply:discounts"
    End: true

  ProcessElectronics:
    Type: Task
    Resource: "arn:aws:lambda:::calculate:shipping"
    End: true
`

// Simple Workflow YAML for testing
const simpleWorkflowYAML = `
Comment: "Simple Workflow Example"
StartAt: ValidateOrder
States:
  ValidateOrder:
    Type: Task
    Resource: "arn:aws:lambda:::validate:order"
    Next: ProcessPayment
    ResultPath: "$.validation"

  ProcessPayment:
    Type: Task
    Resource: "arn:aws:lambda:::process:payment"
    Next: SendNotification
    ResultPath: "$.payment"
    Retry:
      - ErrorEquals: ["States.TaskFailed"]
        MaxAttempts: 3

  SendNotification:
    Type: Task
    Resource: "arn:aws:lambda:::send:notification"
    End: true
    ResultPath: "$.notification"
`

func main() {
	fmt.Println("=== Testing Parallel Workflow with YAML Definition ===")

	// Create executor with task handlers
	exec := NewMockExecutor()
	registerTaskHandlers(exec)

	// Test 1: Parse and execute simple workflow
	fmt.Println("\n--- Test 1: Simple Workflow ---")
	testSimpleWorkflow(exec)

	// Test 2: Parse and execute parallel workflow
	fmt.Println("\n--- Test 2: Parallel Workflow ---")
	testParallelWorkflow(exec)

	// Test 3: Parse and execute choice workflow
	fmt.Println("\n--- Test 3: Choice Workflow ---")
	testChoiceWorkflow(exec)
}

func registerTaskHandlers(exec *MockExecutor) {
	fmt.Println("Registering task handlers...")

	// Handler 1: Validate Order
	exec.RegisterTaskHandler("arn:aws:lambda:::validate:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  📋 Validating order...")
		time.Sleep(100 * time.Millisecond)

		order, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid order format")
		}

		order["validated"] = true
		order["validationTimestamp"] = time.Now().Format(time.RFC3339)
		order["validationId"] = fmt.Sprintf("VAL-%d", time.Now().UnixNano())

		fmt.Printf("  ✅ Order validated: %s\n", order["validationId"])
		return order, nil
	})

	// Handler 2: Process Payment
	exec.RegisterTaskHandler("arn:aws:lambda:::process:payment", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  💳 Processing payment...")
		time.Sleep(200 * time.Millisecond)

		payment, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid payment format")
		}

		payment["paymentStatus"] = "COMPLETED"
		payment["transactionId"] = fmt.Sprintf("TXN-%d", time.Now().UnixNano())
		payment["processedAt"] = time.Now().Format(time.RFC3339)

		fmt.Printf("  ✅ Payment processed: %s\n", payment["transactionId"])
		return payment, nil
	})

	// Handler 3: Check Inventory
	exec.RegisterTaskHandler("arn:aws:lambda:::check:inventory", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  📦 Checking inventory...")
		time.Sleep(150 * time.Millisecond)

		inventory, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid inventory format")
		}

		inventory["inStock"] = true
		inventory["reserved"] = true
		inventory["reservationId"] = fmt.Sprintf("RES-%d", time.Now().UnixNano())
		inventory["checkedAt"] = time.Now().Format(time.RFC3339)

		fmt.Printf("  ✅ Inventory checked: %s\n", inventory["reservationId"])
		return inventory, nil
	})

	// Handler 4: Calculate Shipping
	exec.RegisterTaskHandler("arn:aws:lambda:::calculate:shipping", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  🚚 Calculating shipping...")
		time.Sleep(100 * time.Millisecond)

		shipping, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid shipping format")
		}

		shipping["shippingMethod"] = "EXPRESS"
		shipping["shippingCost"] = 12.99
		shipping["estimatedDelivery"] = time.Now().Add(48 * time.Hour).Format("2006-01-02")
		shipping["calculatedAt"] = time.Now().Format(time.RFC3339)

		fmt.Println("  ✅ Shipping calculated")
		return shipping, nil
	})

	// Handler 5: Apply Discounts
	exec.RegisterTaskHandler("arn:aws:lambda:::apply:discounts", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  🏷️  Applying discounts...")
		time.Sleep(50 * time.Millisecond)

		order, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid order format")
		}

		order["discountApplied"] = true
		order["discountAmount"] = 15.00
		order["finalAmount"] = 135.75
		order["promoCode"] = "SUMMER2024"

		fmt.Println("  ✅ Discounts applied")
		return order, nil
	})

	// Handler 6: Generate Invoice
	exec.RegisterTaskHandler("arn:aws:lambda:::generate:invoice", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  📄 Generating invoice...")
		time.Sleep(250 * time.Millisecond)

		invoice, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid invoice format")
		}

		invoice["invoiceNumber"] = fmt.Sprintf("INV-%d", time.Now().UnixNano())
		invoice["generatedAt"] = time.Now().Format(time.RFC3339)
		invoice["status"] = "GENERATED"

		fmt.Printf("  ✅ Invoice generated: %s\n", invoice["invoiceNumber"])
		return invoice, nil
	})

	// Handler 7: Send Notification
	exec.RegisterTaskHandler("arn:aws:lambda:::send:notification", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  📧 Sending notification...")
		time.Sleep(100 * time.Millisecond)

		notification, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid notification format")
		}

		notification["sent"] = true
		notification["sentAt"] = time.Now().Format(time.RFC3339)
		notification["recipient"] = "customer@example.com"
		notification["messageId"] = fmt.Sprintf("MSG-%d", time.Now().UnixNano())

		fmt.Printf("  ✅ Notification sent: %s\n", notification["messageId"])
		return notification, nil
	})

	// Handler 8: Update CRM
	exec.RegisterTaskHandler("arn:aws:lambda:::update:crm", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  👥 Updating CRM...")
		time.Sleep(300 * time.Millisecond)

		crm, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid CRM format")
		}

		crm["crmUpdated"] = true
		crm["crmRecordId"] = fmt.Sprintf("CRM-%d", time.Now().UnixNano())
		crm["updatedAt"] = time.Now().Format(time.RFC3339)

		fmt.Printf("  ✅ CRM updated: %s\n", crm["crmRecordId"])
		return crm, nil
	})

	// Handler 9: Log Analytics
	exec.RegisterTaskHandler("arn:aws:lambda:::log:analytics", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  📊 Logging analytics...")
		time.Sleep(50 * time.Millisecond)

		analytics, ok := input.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid analytics format")
		}

		analytics["logged"] = true
		analytics["analyticsId"] = fmt.Sprintf("ANA-%d", time.Now().UnixNano())
		analytics["timestamp"] = time.Now().Unix()

		fmt.Printf("  ✅ Analytics logged: %s\n", analytics["analyticsId"])
		return analytics, nil
	})

	fmt.Println("✅ All task handlers registered")
}

func testSimpleWorkflow(exec *MockExecutor) {
	// Create state machine from definition
	sm, err := statemachine.New([]byte(simpleWorkflowYAML), false)
	if err != nil {
		log.Fatalf("Failed to create state machine: %v", err)
	}

	// Create execution context
	order := createSampleOrder()
	executionCtx := &execution.Execution{
		ID:        fmt.Sprintf("EXEC-SIMPLE-%d", time.Now().UnixNano()),
		Name:      "SimpleOrderProcessing",
		Input:     order,
		StartTime: time.Now(),
		Status:    "RUNNING",
	}

	// Execute
	ctx := context.Background()
	fmt.Println("Starting simple workflow execution...")
	startTime := time.Now()

	result, err := exec.Execute(ctx, sm, executionCtx)
	if err != nil {
		log.Printf("Workflow execution failed: %v", err)
	} else {
		duration := time.Since(startTime)
		fmt.Printf("✅ Simple workflow completed in %v\n", duration)
		fmt.Printf("Final status: %s\n", result.Status)
	}
}

func testParallelWorkflow(exec *MockExecutor) {
	// Create state machine from definition
	sm, err := statemachine.New([]byte(parallelWorkflowYAML), false)
	if err != nil {
		log.Fatalf("Failed to create state machine: %v", err)
	}

	// Create execution context
	order := createSampleOrder()
	executionCtx := &execution.Execution{
		ID:        fmt.Sprintf("EXEC-PARALLEL-%d", time.Now().UnixNano()),
		Name:      "ParallelOrderProcessing",
		Input:     order,
		StartTime: time.Now(),
		Status:    "RUNNING",
	}

	// Execute
	ctx := context.Background()
	fmt.Println("Starting parallel workflow execution...")
	startTime := time.Now()

	result, err := exec.Execute(ctx, sm, executionCtx)
	if err != nil {
		log.Printf("Workflow execution failed: %v", err)
	} else {
		duration := time.Since(startTime)
		fmt.Printf("✅ Parallel workflow completed in %v\n", duration)
		fmt.Printf("Final status: %s\n", result.Status)

		// Parallel tasks should complete in ~300ms (max of individual branch durations)
		// not the sum of all durations
		if duration < 500*time.Millisecond {
			fmt.Println("✅ Parallel execution verified!")
		}
	}
}

func testChoiceWorkflow(exec *MockExecutor) {
	// Create state machine from definition
	sm, err := statemachine.New([]byte(choiceWorkflowYAML), false)
	if err != nil {
		log.Fatalf("Failed to create state machine: %v", err)
	}

	// Test different scenarios
	testCases := []struct {
		name   string
		order  map[string]interface{}
		expect string // Expected final state
	}{
		{
			name: "Large Order (> $1000)",
			order: func() map[string]interface{} {
				order := createSampleOrder()
				order["totals"].(map[string]interface{})["total"] = 1500.00
				return order
			}(),
			expect: "ProcessLargeOrder", // Should go to check inventory
		},
		{
			name: "Premium Customer",
			order: func() map[string]interface{} {
				order := createSampleOrder()
				order["customer"].(map[string]interface{})["membership"] = "PREMIUM"
				return order
			}(),
			expect: "ProcessPremium", // Should apply discounts
		},
		{
			name:   "Standard Order",
			order:  createSampleOrder(),
			expect: "ProcessStandard", // Default path
		},
	}

	for _, tc := range testCases {
		fmt.Printf("\nTesting scenario: %s\n", tc.name)

		executionCtx := &execution.Execution{
			ID:        fmt.Sprintf("EXEC-CHOICE-%s-%d", tc.name, time.Now().UnixNano()),
			Name:      "ChoiceWorkflowTest",
			Input:     tc.order,
			StartTime: time.Now(),
			Status:    "RUNNING",
		}

		ctx := context.Background()
		result, err := exec.Execute(ctx, sm, executionCtx)

		if err != nil {
			log.Printf("  ❌ Choice workflow failed: %v", err)
		} else {
			fmt.Printf("  ✅ Choice workflow completed: %s\n", result.Status)
			// Note: In a real implementation, you would check which state was executed
		}
	}
}

func createSampleOrder() map[string]interface{} {
	return map[string]interface{}{
		"orderId": fmt.Sprintf("ORD-%d", time.Now().UnixNano()),
		"customer": map[string]interface{}{
			"id":         "CUST-12345",
			"name":       "John Doe",
			"email":      "john.doe@example.com",
			"phone":      "+1-555-123-4567",
			"membership": "STANDARD",
		},
		"items": []interface{}{
			map[string]interface{}{
				"id":       "ITEM-001",
				"name":     "Wireless Headphones",
				"quantity": 1,
				"price":    99.99,
				"category": "ELECTRONICS",
			},
			map[string]interface{}{
				"id":       "ITEM-002",
				"name":     "USB-C Cable",
				"quantity": 2,
				"price":    24.99,
				"category": "ACCESSORIES",
			},
		},
		"shippingAddress": map[string]interface{}{
			"street":  "123 Main St",
			"city":    "San Francisco",
			"state":   "CA",
			"zip":     "94105",
			"country": "USA",
		},
		"payment": map[string]interface{}{
			"method":         "CREDIT_CARD",
			"lastFour":       "4242",
			"billingAddress": "same",
		},
		"totals": map[string]interface{}{
			"subtotal":         149.97,
			"tax":              14.99,
			"shippingEstimate": 0,
			"total":            164.96,
			"currency":         "USD",
			"discountEligible": true,
		},
		"metadata": map[string]interface{}{
			"source":    "web",
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}
}

// Helper to write YAML to file for testing
func writeYAMLToFile() {
	//yamlContent := parallelWorkflowYAML
	filename := "parallel_workflow.yaml"

	// You would write to file here
	fmt.Printf("YAML definition written to: %s\n", filename)
	fmt.Println("You can use this file with your state machine loader.")
}

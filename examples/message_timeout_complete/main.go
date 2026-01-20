// example/message_timeout_complete_example.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/handler"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
)

var (
	mode           = flag.String("mode", "apiserver", "Application mode: 'leader' or 'worker'")
	redisAddr      = flag.String("redis", "localhost:6379", "Redis address")
	redisPassword  = flag.String("redis-password", "", "Redis password")
	redisDB        = flag.Int("redis-db", 0, "Redis database number")
	concurrency    = flag.Int("concurrency", 10, "Worker concurrency")
	postgresURL    = flag.String("postgres", "postgres://postgres:postgres@localhost:5432/statemachine?sslmode=disable", "PostgreSQL connection URL")
	stateMachineID = flag.String("sm-id", "order-processing-sm", "State machine ID")
)

// State machine definition with Message state and timeout handling
const stateMachineDefinition = `
{
  "Comment": "Order processing with payment confirmation and timeout",
  "StartAt": "CreateOrder",
  "States": {
    "CreateOrder": {
      "Type": "Task",
      "Resource": "arn:aws:states:::lambda:invoke",
      "Next": "WaitForPayment"
    },
    "WaitForPayment": {
      "Type": "Message",
      "CorrelationKey": "orderId",
      "CorrelationValuePath": "$.orderId",
      "TimeoutSeconds": 15,
      "TimeoutPath": "PaymentTimeout",
      "Next": "ProcessPayment",
      "Catch": [
        {
          "ErrorEquals": ["States.ALL"],
          "Next": "HandleError"
        }
      ]
    },
    "ProcessPayment": {
      "Type": "Task",
      "Resource": "arn:aws:states:::lambda:invoke",
      "Next": "SendConfirmation"
    },
    "PaymentTimeout": {
      "Type": "Task",
      "Resource": "arn:aws:states:::lambda:invoke",
      "Comment": "Handle payment timeout - cancel order",
      "Next": "SendTimeoutNotification"
    },
    "SendConfirmation": {
      "Type": "Task",
      "Resource": "arn:aws:states:::lambda:invoke",
      "End": true
    },
    "SendTimeoutNotification": {
      "Type": "Task",
      "Resource": "arn:aws:states:::lambda:invoke",
      "End": true
    },
    "HandleError": {
      "Type": "Fail",
      "Error": "PaymentProcessingError",
      "Cause": "Failed to process payment"
    }
  }
}
`

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

// Setup initializes all components
func Setup() (stateMachine *persistent.StateMachine, repository *repository.Manager, queueClient *queue.Client, worker *queue.Worker, err error) {
	ctx := context.Background()

	// Initialize repository
	persistenceManager, err := setupRepository(ctx)

	// Initialize queue client
	queueConfig := &queue.Config{
		RedisAddr:   "localhost:6379",
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"timeout":  5, // High priority for timeout events
			"default":  3,
			"low":      1,
		},
		RetryPolicy: &queue.RetryPolicy{
			MaxRetry: 3,
			Timeout:  10 * time.Minute,
		},
	}
	queueClient, err = queue.NewClient(queueConfig)

	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create queue client: %w", err)
	}

	// Create state machine
	sm, err := persistent.New(
		[]byte(stateMachineDefinition),
		true,
		"order-processing-sm",
		persistenceManager,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create state machine: %w", err)
	}

	// Set queue client
	sm.SetQueueClient(queueClient)

	// Save state machine definition
	if err := sm.SaveDefinition(ctx); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to save definition: %w", err)
	}

	// Create execution handler
	executionHandler := handler.NewExecutionHandler(persistenceManager, queueClient)
	//executionHandler := handler.NewStateMachineExecutionHandler(repoManager, queueClient)

	// Create worker
	worker, err = queue.NewWorker(queueConfig, executionHandler)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create worker: %w", err)
	}

	return sm, persistenceManager, queueClient, worker, nil
}

// Example 1: Start an execution that will wait for payment
func ExampleStartExecution() {
	ctx := context.Background()

	sm, persistenceManager, queueClient, worker, err := Setup()

	defer func() {
		worker.Shutdown()
		err := persistenceManager.Close()
		if err != nil {
			return
		}
		err2 := queueClient.Close()
		if err2 != nil {
			return
		}
	}()
	if err != nil {
		panic(err)
	}

	// Start execution with order data
	input := map[string]interface{}{
		"orderId":    "ORD-12345",
		"customerId": "CUST-67890",
		"amount":     99.99,
		"currency":   "USD",
		"items": []map[string]interface{}{
			{
				"sku":      "ITEM-001",
				"quantity": 2,
				"price":    49.99,
			},
		},
		"createdAt": time.Now().Unix(),
	}

	sm.SaveDefinition(ctx)
	exec, err := sm.Execute(ctx, input)
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Execution started:\n")
	fmt.Printf("   ID: %s\n", exec.ID)
	fmt.Printf("   Status: %s\n", exec.Status)
	fmt.Printf("   Current State: %s\n", exec.CurrentState)
	fmt.Printf("   Waiting for payment confirmation for order: ORD-12345\n")
	fmt.Printf("   ⏰ Timeout will trigger at: %s\n", time.Now().Add(5*time.Minute).Format(time.RFC3339))
	fmt.Printf("   💡 Two possible outcomes:\n")
	fmt.Printf("      1. Payment arrives → ProcessPayment → SendConfirmation\n")
	fmt.Printf("      2. Timeout occurs → PaymentTimeout → SendTimeoutNotification\n")
}

// Example 2: Worker process that handles both regular executions and timeouts
func ExampleRunWorker() {
	_, persistenceManager, queueClient, worker, err := Setup()
	if err != nil {
		fmt.Println(err)
	}

	defer func() {
		worker.Shutdown()
		err := persistenceManager.Close()
		if err != nil {
			return
		}
		err2 := queueClient.Close()
		if err2 != nil {
			return
		}
	}()
	if err != nil {
		panic(err)
	}

	// Run worker in the background
	go func() {
		log.Println("🚀 Starting worker to process tasks...")
		if err := worker.Run(); err != nil {
			log.Fatalf("Worker failed: %v", err)
		}
	}()

	// Keep worker running
	select {}
}

// Example 3: Payment webhook receives payment before timeout (HAPPY PATH)
func HandlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	var webhook struct {
		OrderID       string  `json:"order_id"`
		PaymentID     string  `json:"payment_id"`
		Status        string  `json:"status"`
		Amount        float64 `json:"amount"`
		PaymentMethod string  `json:"payment_method"`
		Timestamp     int64   `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&webhook); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	repoManager, err1 := setupRepository(ctx)
	defer func(repoManager *repository.Manager) {
		err := repoManager.Close()
		if err != nil {
			fmt.Println("Failed to close repository manager")
		}
	}(repoManager)

	if err1 != nil {
		http.Error(w, "Failed to initialize repository", http.StatusInternalServerError)
		return
	}

	sm, err := persistent.NewFromDefnId(ctx, "order-processing-sm", repoManager)
	if err != nil {
		http.Error(w, "State machine not found", http.StatusInternalServerError)
		return
	}

	// Find waiting execution by correlation
	waitingExecs, err := sm.FindWaitingExecutionsByCorrelation(
		ctx,
		"orderId",
		webhook.OrderID,
	)
	if err != nil || len(waitingExecs) == 0 {
		http.Error(w, "No waiting execution found", http.StatusNotFound)
		return
	}

	execRecord := waitingExecs[0]
	log.Printf("💳 Payment received for order %s, resuming execution %s", webhook.OrderID, execRecord.ExecutionID)

	// Prepare message data with received payment information
	resumeInput := map[string]interface{}{
		"__received_message__": map[string]interface{}{
			"correlation_key":   "orderId",
			"correlation_value": webhook.OrderID,
			"data": map[string]interface{}{
				"paymentId":     webhook.PaymentID,
				"status":        webhook.Status,
				"amount":        webhook.Amount,
				"paymentMethod": webhook.PaymentMethod,
				"receivedAt":    webhook.Timestamp,
			},
		},
	}

	processor := states.JSONPathProcessor{}
	mergedInput, err := sm.MergeInputs(&processor, execRecord.Input, resumeInput)
	// Create execution context for resumption
	execCtx := &execution.Execution{
		ID:             execRecord.ExecutionID,
		StateMachineID: execRecord.StateMachineID,
		Name:           execRecord.Name,
		Status:         execRecord.Status,
		CurrentState:   execRecord.CurrentState,
		Input:          mergedInput,
		StartTime:      *execRecord.StartTime,
	}

	// Resume execution with payment data
	resumedExec, err := sm.ResumeExecution(ctx, execCtx)
	if err != nil {
		log.Printf("❌ Failed to resume execution: %v", err)
		http.Error(w, "Failed to process payment", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Execution resumed successfully:")
	log.Printf("   Execution ID: %s", resumedExec.ID)
	log.Printf("   Status: %s", resumedExec.Status)
	log.Printf("   Payment processed, timeout task cancelled")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "payment_received",
		"execution_id": execRecord.ExecutionID,
		"order_id":     webhook.OrderID,
		"message":      "Payment processed successfully",
	})
}

// Example 4: Monitor executions and check for timeouts
func ExampleMonitorExecutions() {
	ctx := context.Background()
	repoManager, err2 := setupRepository(ctx)
	if err2 != nil {
		panic(err2)
	}

	sm, err := persistent.NewFromDefnId(ctx, "order-processing-sm", repoManager)
	if err != nil {
		panic(err)
	}

	// List all paused executions waiting for messages
	filter := &repository.ExecutionFilter{
		StateMachineID: sm.GetID(),
		Status:         "PAUSED",
	}

	executions, err := sm.ListExecutions(ctx, filter)
	if err != nil {
		panic(err)
	}

	fmt.Printf("📊 Monitoring Paused Executions:\n")
	fmt.Printf("   Total waiting: %d\n\n", len(executions))

	for _, exec := range executions {
		fmt.Printf("   Execution: %s\n", exec.ExecutionID)
		fmt.Printf("   Started: %s\n", exec.StartTime.Format(time.RFC3339))
		fmt.Printf("   Current State: %s\n", exec.CurrentState)
		fmt.Printf("   Age: %s\n", time.Since(*exec.StartTime).Round(time.Second))

		// Get correlation info
		correlationID := fmt.Sprintf("corr-%s-%s", exec.ExecutionID, exec.CurrentState)
		correlation, err := repoManager.GetMessageCorrelation(ctx, correlationID)
		if err == nil {
			fmt.Printf("   Correlation: %s = %v\n", correlation.CorrelationKey, correlation.CorrelationValue)
			if correlation.TimeoutAt != nil {
				timeoutTime := time.Unix(*correlation.TimeoutAt, 0)
				if time.Now().Before(timeoutTime) {
					remaining := time.Until(timeoutTime).Round(time.Second)
					fmt.Printf("   ⏰ Timeout in: %s\n", remaining)
				} else {
					fmt.Printf("   ⚠️  Timeout should have triggered\n")
				}
			}
		}
		fmt.Println()
	}
}

// Example 5: Test timeout scenario (for testing without waiting 5 minutes)
func ExampleTestTimeoutScenario() {
	ctx := context.Background()

	// Create state machine with short timeout for testing
	testDefinition := `
	{
	  "StartAt": "WaitForPayment",
	  "States": {
	    "WaitForPayment": {
	      "Type": "Message",
	      "CorrelationKey": "orderId",
	      "TimeoutSeconds": 10,
	      "TimeoutPath": "PaymentTimeout",
	      "Next": "ProcessPayment"
	    },
	    "ProcessPayment": {
	      "Type": "Pass",
	      "Result": "Payment processed",
	      "End": true
	    },
	    "PaymentTimeout": {
	      "Type": "Pass",
	      "Result": "Payment timed out - order cancelled",
	      "End": true
	    }
	  }
	}
	`

	repoManager, err2 := setupRepository(ctx)
	if err2 != nil {
		fmt.Println(err2)
		return
	}
	defer func(repoManager *repository.Manager) {
		err := repoManager.Close()
		if err != nil {
			fmt.Println("Failed to close repository manager")
		}
	}(repoManager)

	queueClient, _ := queue.NewClient(queue.DefaultConfig())

	defer func(queueClient *queue.Client) {
		err := queueClient.Close()
		if err != nil {
			fmt.Println("Failed to close queue client")
		}
	}(queueClient)

	sm, err := persistent.New([]byte(testDefinition), true, "test-timeout-sm", repoManager)
	if err != nil {
		fmt.Println(err)
		return
	}
	sm.SetQueueClient(queueClient)

	err = sm.SaveDefinition(ctx)
	if err != nil {
		fmt.Println("Failed to save state machine definition")
		return
	}

	// Start execution
	input := map[string]interface{}{"orderId": "TEST-001"}
	exec, err := sm.Execute(ctx, input)
	if err != nil {
		panic(err)
	}

	fmt.Printf("🧪 Test execution started: %s\n", exec.ID)
	fmt.Printf("   Waiting 10 seconds for timeout...\n")

	// Wait for timeout to trigger
	time.Sleep(12 * time.Second)

	// Check execution status
	execRecord, _ := sm.GetExecution(ctx, exec.ID)
	fmt.Printf("   Final status: %s\n", execRecord.Status)

	// Get history to see the path taken
	history, _ := sm.GetExecutionHistory(ctx, exec.ID)
	fmt.Printf("   States executed:\n")
	for _, h := range history {
		fmt.Printf("      - %s (%s)\n", h.StateName, h.Status)
	}
}

// Example 6: API Server with payment webhook endpoint
func ExampleRunAPIServer() {
	http.HandleFunc("/webhook/payment", HandlePaymentWebhook)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
		if err != nil {
			fmt.Println("Failed to encode health check response")
			return
		}
	})

	log.Println("🌐 Starting API server on :6565")
	log.Println("   Payment webhook: POST /webhook/payment")
	log.Fatal(http.ListenAndServe(":6565", nil))
}

// Main function showing complete setup
func main() {
	flag.Parse()
	// Start worker in background
	fmt.Printf("\n Mode %s", *mode)

	switch *mode {
	case "apiserver":
		ExampleRunAPIServer()
	case "worker":
		ExampleRunWorker()
	case "ExampleStartExecution":
		ExampleStartExecution()
	case "ExampleMonitorExecutions":
		ExampleMonitorExecutions()
	case "ExampleTestTimeoutScenario":
		ExampleTestTimeoutScenario()
	default:
		log.Fatalf("Invalid mode: %s. Use 'apiserver' or 'worker'", *mode)
	}

}

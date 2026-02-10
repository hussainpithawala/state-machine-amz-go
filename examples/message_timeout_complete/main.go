// example/message_timeout_complete_example.go
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"time"

	"github.com/hibiken/asynq"
	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/handler"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

var (
	mode           = flag.String("mode", "apiserver", "Application mode: 'leader' or 'worker'")
	redisAddr      = flag.String("redis", "localhost:6379", "Redis address")
	redisPassword  = flag.String("redis-password", "", "Redis password")
	useTls         = flag.Bool("tls", false, "Use TLS for Redis connection")
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
      "Resource": "create:order",
      "Next": "WaitForPayment"
    },
    "WaitForPayment": {
      "Type": "Message",
      "CorrelationKey": "orderId",
      "CorrelationValuePath": "$.order.orderId",
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
      "Resource": "process:payment",
      "Next": "SendConfirmation"
    },
    "PaymentTimeout": {
      "Type": "Task",
      "Resource": "payment:timeout",
      "Comment": "Handle payment timeout - cancel order",
      "Next": "SendTimeoutNotification"
    },
    "SendConfirmation": {
      "Type": "Task",
      "Resource": "send:confirmation",
      "End": true
    },
    "SendTimeoutNotification": {
      "Type": "Task",
      "Resource": "send:timeout-notification",
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

func setupExecutor() *executor.BaseExecutor {
	exec := executor.NewBaseExecutor()

	// Register handler for ValidateOrder
	exec.RegisterGoFunction("create:order", func(ctx context.Context, input interface{}) (interface{}, error) {
		log.Printf("→ Creating order: %v", input)
		//time.Sleep(100 * time.Millisecond) // Simulate processing

		// Add validation result to input
		if inputMap, ok := input.(map[string]interface{}); ok {
			inputMap["validationStatus"] = "VALID"
			inputMap["order"] = map[string]interface{}{
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
			}
			return inputMap, nil
		}
		return input, nil
	})

	// Register handler for ProcessPayment
	exec.RegisterGoFunction("process:payment", func(ctx context.Context, input interface{}) (interface{}, error) {
		log.Printf("→ Processing payment: %v", input)
		//time.Sleep(150 * time.Millisecond) // Simulate processing

		if inputMap, ok := input.(map[string]interface{}); ok {
			reflect.DeepEqual(map[string]interface{}{
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
			}, inputMap["order"])

			inputMap["paymentStatus"] = "COMPLETED"
			inputMap["transactionId"] = fmt.Sprintf("TXN-%d", time.Now().Unix())
			inputMap["processedAt"] = time.Now().Unix()
			return inputMap, nil
		}
		return input, nil
	})

	// Register handler for ProcessPayment
	exec.RegisterGoFunction("send:timeout-notification", func(ctx context.Context, input interface{}) (interface{}, error) {
		log.Printf("→ Handling timeout-notification: %v", input)
		//time.Sleep(150 * time.Millisecond) // Simulate processing

		if inputMap, ok := input.(map[string]interface{}); ok {
			reflect.DeepEqual(map[string]interface{}{
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
			}, inputMap["order"])
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

func getQueueConfig() *queue.Config {
	if *useTls {
		return &queue.Config{
			RedisClientOpt: &asynq.RedisClientOpt{
				Addr:         *redisAddr,
				Password:     *redisPassword,
				DB:           *redisDB,
				DialTimeout:  10 * time.Second,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				PoolSize:     20,
				TLSConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6, // Highest priority
				"timeout":  5, // High priority for timeout events
				"default":  3, // Normal priority
				"low":      1, // Lowest priority
			},
			RetryPolicy: &queue.RetryPolicy{
				MaxRetry: 3,
				Timeout:  10 * time.Minute,
			},
		}
	} else {
		return &queue.Config{
			RedisClientOpt: &asynq.RedisClientOpt{
				Addr:         *redisAddr,
				DB:           *redisDB,
				DialTimeout:  10 * time.Second,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				PoolSize:     20,
			},
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6, // Highest priority
				"timeout":  5, // High priority for timeout events
				"default":  3, // Normal priority
				"low":      1, // Lowest priority
			},
			RetryPolicy: &queue.RetryPolicy{
				MaxRetry: 3,
				Timeout:  10 * time.Minute,
			},
		}
	}
}

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
func Setup(baseExecutor *executor.BaseExecutor) (stateMachine *persistent.StateMachine, repository *repository.Manager, queueClient *queue.Client, worker *queue.Worker, err error) {
	ctx := context.Background()

	// Initialize repository
	persistenceManager, err := setupRepository(ctx)

	// Initialize queue client
	queueConfig := getQueueConfig()
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
	ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(baseExecutor))
	// Create worker
	worker, err = queue.NewWorker(queueConfig, executionHandler)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create worker: %w", err)
	}

	return sm, persistenceManager, queueClient, worker, nil
}

// Example 1: Start an execution that will wait for payment
func ExampleStartExecution(baseExecutor *executor.BaseExecutor) {
	ctx := context.Background()

	sm, persistenceManager, queueClient, worker, err := Setup(baseExecutor)

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

	saveError := sm.SaveDefinition(ctx)
	if saveError != nil {
		return
	}

	ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(baseExecutor))
	executionResult, err := sm.Execute(ctx, input)
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Execution started:\n")
	fmt.Printf("   ID: %s\n", executionResult.ID)
	fmt.Printf("   Status: %s\n", executionResult.Status)
	fmt.Printf("   Current State: %s\n", executionResult.CurrentState)
	fmt.Printf("   Waiting for payment confirmation for order: ORD-12345\n")
	fmt.Printf("   ⏰ Timeout will trigger at: %s\n", time.Now().Add(5*time.Minute).Format(time.RFC3339))
	fmt.Printf("   💡 Two possible outcomes:\n")
	fmt.Printf("      1. Payment arrives → ProcessPayment → SendConfirmation\n")
	fmt.Printf("      2. Timeout occurs → PaymentTimeout → SendTimeoutNotification\n")
}

// Example 2: Worker process that handles both regular executions and timeouts
func ExampleRunWorker(baseExecutor *executor.BaseExecutor) {
	_, persistenceManager, queueClient, worker, err := Setup(baseExecutor)
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

	received_message_key := fmt.Sprintf("%s_%s", states.ReceivedMessageBase, execRecord.CurrentState)
	// Prepare message data with received payment information
	resumeInput := map[string]interface{}{
		received_message_key: map[string]interface{}{
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
func ExampleMonitorExecutions(exec *executor.BaseExecutor) {
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
func ExampleTestTimeoutScenario(baseExecutor *executor.BaseExecutor) {
	ctx := context.Background()

	// Create state machine with short timeout for testing
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

	config := getQueueConfig()

	queueClient, _ := queue.NewClient(config)

	defer func(queueClient *queue.Client) {
		err := queueClient.Close()
		if err != nil {
			fmt.Println("Failed to close queue client")
		}
	}(queueClient)

	sm, err := persistent.New([]byte(stateMachineDefinition), true, "test-timeout-sm", repoManager)

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

	ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(baseExecutor))
	executionResult, err := sm.Execute(ctx, input)
	if err != nil {
		panic(err)
	}

	fmt.Printf("🧪 Test execution started: %s\n", executionResult.ID)
	fmt.Printf("   Waiting 10 seconds for timeout...\n")

	// Wait for timeout to trigger
	time.Sleep(10 * time.Second)

	// Check execution status
	execRecord, _ := sm.GetExecution(ctx, executionResult.ID)
	fmt.Printf("   Final status: %s\n", execRecord.Status)

	// Get history to see the path taken
	history, _ := sm.GetExecutionHistory(ctx, executionResult.ID)
	fmt.Printf("   States executed:\n")
	for _, h := range history {
		fmt.Printf("      - %s (%s)\n", h.StateName, h.Status)
	}
}

// Example 6: API Server with payment webhook endpoint
func ExampleRunAPIServer(exec *executor.BaseExecutor) {
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
	fmt.Printf("\n RedisAddress %s", *redisAddr)
	fmt.Printf("\n RedisPassword %s", *redisPassword)
	fmt.Printf("\n UseTls %t", *useTls)

	// Setup task executor with handlers
	baseExecutor := setupExecutor()

	switch *mode {
	case "apiserver":
		ExampleRunAPIServer(baseExecutor)
	case "worker":
		ExampleRunWorker(baseExecutor)
	case "ExampleStartExecution":
		ExampleStartExecution(baseExecutor)
	case "ExampleMonitorExecutions":
		ExampleMonitorExecutions(baseExecutor)
	case "ExampleTestTimeoutScenario":
		ExampleTestTimeoutScenario(baseExecutor)
	default:
		log.Fatalf("Invalid mode: %s. Use 'apiserver' or 'worker'", *mode)
	}

}

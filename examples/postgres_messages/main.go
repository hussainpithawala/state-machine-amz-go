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
	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

func main() {
	fmt.Println("=== PostgreSQL Message Pause and Resume Example ===")
	registryMap := make(executor.RegistryMap)
	stateMachineId := "msg-sm-" + fmt.Sprintf("%d", time.Now().Unix())
	registryMap[stateMachineId] = prepareStateRegistry()
	if err := runMessageWorkflowExample(&registryMap, stateMachineId); err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== Example completed successfully ===")
}

func runMessageWorkflowExample(registryMap *executor.RegistryMap, stateMachineId string) error {
	ctx := context.Background()

	// 1. Define workflow with a Message state
	// It starts with a Task, then waits for a message, then ends with another Task
	yamlContent := `
Comment: "Workflow with message pause and resume"
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
`
	// 1. state machine id

	smID := stateMachineId

	// 2. Create persistence manager
	persistenceManager, err := getPersistenceManager(ctx)
	if err != nil {
		return err
	}
	defer func(persistenceManager *repository.Manager) {
		err := persistenceManager.Close()
		if err != nil {
			fmt.Println("Failed to close persistence manager")
		}
	}(persistenceManager)

	// 3. Create persistent state machine

	pm, err := persistent.New([]byte(yamlContent), false, smID, persistenceManager)
	if err != nil {
		return fmt.Errorf("failed to create persistent state machine: %w", err)
	}

	// Save state machine definition to repository so it can be loaded during resumption
	if err := pm.SaveDefinition(ctx); err != nil {
		return fmt.Errorf("failed to save state machine definition: %w", err)
	}

	// 4. Create executor and register handlers
	exec := executor.NewBaseExecutor()
	exec.SetRepositoryManager(persistenceManager)

	exec.RegisterGoFunction("initial-task", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → Executing initial task...")
		inputMap := input.(map[string]interface{})
		return map[string]interface{}{
			"orderId": inputMap["orderId"],
			"status":  "INITIAL_DONE",
		}, nil
	})

	exec.RegisterGoFunction("final-task", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → Executing final task...")
		return map[string]interface{}{
			"status": "COMPLETED",
		}, nil
	})

	// 5. Start execution
	execID := "exec-msg-" + fmt.Sprintf("%d", time.Now().Unix())
	orderID := "ORD-" + fmt.Sprintf("%d", time.Now().Unix())

	execCtx := &execution.Execution{
		ID:             execID,
		Name:           "MessageWorkflowExecution",
		StateMachineID: smID,
		Input: map[string]interface{}{
			"orderId": orderID,
		},
		StartTime: time.Now(),
		Status:    "RUNNING",
	}

	fmt.Printf("\n1. Starting execution %s...\n", execID)

	ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

	executionInstance, err := pm.Execute(ctx, execCtx)
	if err != nil {
		return fmt.Errorf("initial execution failed: %w", err)
	}

	fmt.Printf("   Execution status: %s, Current state: %s\n", executionInstance.Status, executionInstance.CurrentState)

	if executionInstance.Status != "PAUSED" {
		return fmt.Errorf("expected execution to be PAUSED, but got %s", executionInstance.Status)
	}
	fmt.Println("   ✓ Execution successfully paused at WaitForApproval state.")

	// 6. Simulate external message arrival
	fmt.Println("\n2. Simulating external message arrival...")
	time.Sleep(1 * time.Second) // Simulate some delay

	// Use the same orderID as used in start
	messageRequest := &executor.MessageRequest{
		CorrelationKey:   "orderId",
		CorrelationValue: orderID,
		Data: map[string]interface{}{
			"approved": true,
			"approver": "admin",
		},
	}

	// We don't need to provide the state machine to the Message method anymore
	fmt.Println("   Sending message to resume execution...")
	secondExecutor := executor.BaseExecutor{}
	secondExecutor.SetRepositoryManager(persistenceManager)
	response, err := secondExecutor.Message(ctx, messageRequest, registryMap)
	if err != nil {
		fmt.Printf("   Note: Message processing returned: %v\n", err)
	}

	if response != nil {
		fmt.Printf("   Message response: Status=%s, Resumed=%v\n", response.Status, response.MessageReceived)
	}

	// 7. Verify final status
	// Since we are using persistent state machine, we can fetch the latest state from DB
	// We wait a bit to ensure the async resumption (if any, though here it is sync) has updated the DB
	time.Sleep(500 * time.Millisecond)
	finalExec, err := pm.GetExecution(ctx, execID)
	if err != nil {
		return fmt.Errorf("failed to fetch final execution: %w", err)
	}

	fmt.Printf("\n3. Final execution result:\n")
	fmt.Printf("   Execution ID: %s\n", finalExec.ExecutionID)
	fmt.Printf("   Status: %s\n", finalExec.Status)
	fmt.Printf("   Current State: %s\n", finalExec.CurrentState)

	if finalExec.Status != "PAUSED" {
		return fmt.Errorf("expected execution to be PAUSED, but got %s", finalExec.Status)
	}
	fmt.Println("   ✓ Execution successfully resumed and completed.")

	return nil
}

func prepareStateRegistry() *executor.StateRegistry {
	// 4. Create executor and register handlers
	stateRegisry := executor.NewStateRegistry()
	stateRegisry.RegisterTaskHandler("initial-task", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → Executing initial task...")
		inputMap := input.(map[string]interface{})
		return map[string]interface{}{
			"orderId": inputMap["orderId"],
			"status":  "INITIAL_DONE",
		}, nil
	})
	stateRegisry.RegisterTaskHandler("final-task", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  → Executing final task...")
		return map[string]interface{}{
			"status": "COMPLETED",
		}, nil
	})

	return stateRegisry
}

func getPersistenceManager(ctx context.Context) (*repository.Manager, error) {
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

	if err := persistenceManager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize persistence: %w", err)
	}
	return persistenceManager, nil
}

func getConnectionURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
}

// examples/parallel_express/main.go
//
// Demonstrates express (in-process) parallel state execution.
//
// No Redis or Postgres required — branches run as goroutines inside a single
// process. Each branch gets its own BranchExecution with full state history,
// and Task handlers resolve via ExecutionContextKey in the context.
//
// The example runs three scenarios:
//  1. Happy path  — all branches succeed, branch histories are printed
//  2. Branch error — one branch fails, the error propagates to the parent
//  3. Timing proof — parallel branches run concurrently (wall-clock < sum)
//
// Usage:
//
//	go run ./examples/parallel_express
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
)

const workflowYAML = `
Comment: "Express Parallel Order Processing"
StartAt: ValidateOrder
States:
  ValidateOrder:
    Type: Task
    Resource: "validate-order"
    Next: ParallelProcessing

  ParallelProcessing:
    Type: Parallel
    Next: GenerateReport
    Branches:
      - StartAt: ProcessPayment
        States:
          ProcessPayment:
            Type: Task
            Resource: "process-payment"
            Next: ApplyDiscount
          ApplyDiscount:
            Type: Task
            Resource: "apply-discount"
            End: true

      - StartAt: CheckInventory
        States:
          CheckInventory:
            Type: Task
            Resource: "check-inventory"
            Next: ReserveStock
          ReserveStock:
            Type: Task
            Resource: "reserve-stock"
            End: true

      - StartAt: CalculateShipping
        States:
          CalculateShipping:
            Type: Task
            Resource: "calculate-shipping"
            End: true
    ResultPath: "$.parallelResults"

  GenerateReport:
    Type: Task
    Resource: "generate-report"
    End: true
`

const failingBranchYAML = `
Comment: "Parallel with a failing branch"
StartAt: ParallelStep
States:
  ParallelStep:
    Type: Parallel
    End: true
    Branches:
      - StartAt: SucceedBranch
        States:
          SucceedBranch:
            Type: Pass
            Result:
              ok: true
            End: true

      - StartAt: FailBranch
        States:
          FailBranch:
            Type: Task
            Resource: "always-fail"
            End: true
`

func main() {
	fmt.Println("=== Express Parallel State Execution ===")

	fmt.Println("\n--- Scenario 1: Happy Path ---")
	runHappyPath()

	fmt.Println("\n--- Scenario 2: Branch Error ---")
	runBranchError()

	fmt.Println("\n--- Scenario 3: Concurrency Timing ---")
	runTimingProof()
}

// ---------------------------------------------------------------------------
// Scenario 1: all branches succeed, inspect branch histories
// ---------------------------------------------------------------------------

func runHappyPath() {
	sm, err := statemachine.New([]byte(workflowYAML), false)
	if err != nil {
		log.Fatalf("Failed to create SM: %v", err)
	}

	ctx := contextWithHandlers(happyHandlers())

	result, err := sm.Execute(ctx, sampleOrder())
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}

	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("States executed: %d\n", len(result.History))

	printBranchHistories(sm, "ParallelProcessing")

	fmt.Println("OK")
}

// ---------------------------------------------------------------------------
// Scenario 2: one branch fails, error propagates
// ---------------------------------------------------------------------------

func runBranchError() {
	exec := executor.NewBaseExecutor()
	exec.RegisterGoFunction("always-fail", func(_ context.Context, _ interface{}) (interface{}, error) {
		return nil, fmt.Errorf("intentional branch failure")
	})

	sm, err := statemachine.New([]byte(failingBranchYAML), false)
	if err != nil {
		log.Fatalf("Failed to create SM: %v", err)
	}

	ctx := context.WithValue(context.Background(), types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

	result, err := sm.Execute(ctx, map[string]interface{}{"test": true})
	if err != nil {
		fmt.Printf("Expected error: %v\n", err)
	}
	fmt.Printf("Status: %s\n", result.Status)
	fmt.Println("OK")
}

// ---------------------------------------------------------------------------
// Scenario 3: prove branches run concurrently
// ---------------------------------------------------------------------------

func runTimingProof() {
	// Each branch sleeps 200ms. Sequential = 600ms+, parallel < 400ms.
	const branchSleep = 200 * time.Millisecond

	yaml := `
Comment: "Timing proof"
StartAt: ParallelTiming
States:
  ParallelTiming:
    Type: Parallel
    End: true
    Branches:
      - StartAt: SlowA
        States:
          SlowA:
            Type: Task
            Resource: "slow-task"
            End: true
      - StartAt: SlowB
        States:
          SlowB:
            Type: Task
            Resource: "slow-task"
            End: true
      - StartAt: SlowC
        States:
          SlowC:
            Type: Task
            Resource: "slow-task"
            End: true
`

	exec := executor.NewBaseExecutor()
	exec.RegisterGoFunction("slow-task", func(_ context.Context, input interface{}) (interface{}, error) {
		time.Sleep(branchSleep)
		return input, nil
	})

	sm, err := statemachine.New([]byte(yaml), false)
	if err != nil {
		log.Fatalf("Failed to create SM: %v", err)
	}

	ctx := context.WithValue(context.Background(), types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))

	start := time.Now()
	result, err := sm.Execute(ctx, map[string]interface{}{"timing": true})
	elapsed := time.Since(start)

	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}

	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Wall-clock: %v\n", elapsed.Round(time.Millisecond))

	sequential := 3 * branchSleep
	if elapsed < sequential {
		fmt.Printf("Parallel confirmed: %v < sequential %v\n", elapsed.Round(time.Millisecond), sequential)
	} else {
		fmt.Printf("WARNING: took %v, expected less than %v\n", elapsed, sequential)
	}
	fmt.Println("OK")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func sampleOrder() map[string]interface{} {
	return map[string]interface{}{
		"orderId":    fmt.Sprintf("ORD-%d", time.Now().UnixNano()),
		"customerId": "CUST-42",
		"amount":     249.99,
		"items":      []string{"headphones", "cable", "case"},
	}
}

func happyHandlers() *executor.BaseExecutor {
	exec := executor.NewBaseExecutor()

	exec.RegisterGoFunction("validate-order", func(_ context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  [task] validate-order")
		m := asMap(input)
		m["validated"] = true
		return m, nil
	})

	exec.RegisterGoFunction("process-payment", func(_ context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  [task] process-payment")
		time.Sleep(150 * time.Millisecond)
		m := asMap(input)
		m["paymentStatus"] = "COMPLETED"
		m["transactionId"] = fmt.Sprintf("TXN-%d", time.Now().UnixNano())
		return m, nil
	})

	exec.RegisterGoFunction("apply-discount", func(_ context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  [task] apply-discount")
		m := asMap(input)
		m["discountApplied"] = true
		m["finalAmount"] = 224.99
		return m, nil
	})

	exec.RegisterGoFunction("check-inventory", func(_ context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  [task] check-inventory")
		time.Sleep(100 * time.Millisecond)
		m := asMap(input)
		m["inStock"] = true
		return m, nil
	})

	exec.RegisterGoFunction("reserve-stock", func(_ context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  [task] reserve-stock")
		m := asMap(input)
		m["reservationId"] = fmt.Sprintf("RES-%d", time.Now().UnixNano())
		return m, nil
	})

	exec.RegisterGoFunction("calculate-shipping", func(_ context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  [task] calculate-shipping")
		time.Sleep(100 * time.Millisecond)
		m := asMap(input)
		m["shippingCost"] = 12.99
		m["estimatedDelivery"] = time.Now().Add(48 * time.Hour).Format("2006-01-02")
		return m, nil
	})

	exec.RegisterGoFunction("generate-report", func(_ context.Context, input interface{}) (interface{}, error) {
		fmt.Println("  [task] generate-report")
		m := asMap(input)
		m["reportId"] = fmt.Sprintf("RPT-%d", time.Now().UnixNano())
		return m, nil
	})

	return exec
}

func contextWithHandlers(exec *executor.BaseExecutor) context.Context {
	return context.WithValue(context.Background(), types.ExecutionContextKey, executor.NewExecutionContextAdapter(exec))
}

func printBranchHistories(sm *statemachine.StateMachine, parallelStateName string) {
	state, err := sm.GetState(parallelStateName)
	if err != nil {
		fmt.Printf("Could not find state %s: %v\n", parallelStateName, err)
		return
	}

	ps, ok := state.(*states.ParallelState)
	if !ok {
		fmt.Printf("%s is not a ParallelState\n", parallelStateName)
		return
	}

	branchExecs := ps.GetLastBranchExecutions()
	if branchExecs == nil {
		fmt.Println("No branch executions recorded.")
		return
	}

	fmt.Printf("\nBranch execution histories (%d branches):\n", len(branchExecs))
	for i, be := range branchExecs {
		fmt.Printf("  Branch %d: name=%s, status=%s, states=%d\n", i, be.Name, be.Status, len(be.History))
		for j, h := range be.History {
			fmt.Printf("    [%d] %s -> %s\n", j, h.StateName, h.Status)
		}
	}
}

func asMap(input interface{}) map[string]interface{} {
	if m, ok := input.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

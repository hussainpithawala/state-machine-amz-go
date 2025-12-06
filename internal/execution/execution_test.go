// cmd/demo/test_demo_execution_flow.go
package execution

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// Mock execution context
type DemoExecutionContext struct {
	handlers map[string]func(context.Context, interface{}) (interface{}, error)
}

func NewDemoExecutionContext() *DemoExecutionContext {
	return &DemoExecutionContext{
		handlers: make(map[string]func(context.Context, interface{}) (interface{}, error)),
	}
}

func (d *DemoExecutionContext) RegisterHandler(name string, fn func(context.Context, interface{}) (interface{}, error)) {
	d.handlers[name] = fn
}

func (d *DemoExecutionContext) GetTaskHandler(resource string) (func(context.Context, interface{}) (interface{}, error), bool) {
	fn, exists := d.handlers[resource]
	return fn, exists
}

func Test_demo_execution_flow(t *testing.T) {
	fmt.Println("=== Task Execution Flow Demo ===")

	// Create execution context
	execCtx := NewDemoExecutionContext()

	// Register demo handlers
	execCtx.RegisterHandler("greet", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("👋 Greeting handler called")
		data := input.(map[string]interface{})
		data["greeting"] = fmt.Sprintf("Hello, %s!", data["name"])
		data["timestamp"] = time.Now().Format("2006-01-02 15:04:05")
		return data, nil
	})

	execCtx.RegisterHandler("process", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("⚙️  Processing handler called")
		data := input.(map[string]interface{})
		data["processed"] = true
		data["stage"] = "completed"
		return data, nil
	})

	execCtx.RegisterHandler("validate", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("✅ Validation handler called")
		data := input.(map[string]interface{})

		if data["required"] != true {
			return nil, fmt.Errorf("validation failed: required field missing")
		}

		data["valid"] = true
		return data, nil
	})

	// Create context with execution context
	ctx := states.WithExecutionContext(context.Background(), execCtx)

	// Create task handler
	handler := states.NewDefaultTaskHandler()

	// Demo 1: Simple greeting
	fmt.Println("\n--- Demo 1: Simple Greeting ---")
	input1 := map[string]interface{}{
		"name": "World",
		"type": "demo",
	}

	result1, err := handler.Execute(ctx, "greet", input1, nil)
	if err != nil {
		log.Fatalf("Demo 1 failed: %v", err)
	}
	fmt.Printf("Result: %v\n", result1)

	// Demo 2: Processing with parameters
	fmt.Println("\n--- Demo 2: Processing with Parameters ---")
	input2 := map[string]interface{}{
		"data": map[string]interface{}{
			"values": []int{1, 2, 3},
			"source": "demo",
		},
	}

	parameters := map[string]interface{}{
		"values.$": "$.data.values",
		"source.$": "$.data.source",
		"extra":    "parameter",
	}

	result2, err := handler.Execute(ctx, "process", input2, parameters)
	if err != nil {
		log.Fatalf("Demo 2 failed: %v", err)
	}
	fmt.Printf("Result: %v\n", result2)

	// Demo 3: Validation (success case)
	fmt.Println("\n--- Demo 3: Validation Success ---")
	input3 := map[string]interface{}{
		"required": true,
		"optional": "value",
	}

	result3, err := handler.Execute(ctx, "validate", input3, nil)
	if err != nil {
		log.Fatalf("Demo 3 failed: %v", err)
	}
	fmt.Printf("Result: %v\n", result3)

	// Demo 4: Validation (failure case)
	fmt.Println("\n--- Demo 4: Validation Failure ---")
	input4 := map[string]interface{}{
		"optional": "value",
		// Missing required field
	}

	result4, err := handler.Execute(ctx, "validate", input4, nil)
	if err != nil {
		fmt.Printf("Expected error: %v\n", err)
	} else {
		fmt.Printf("Result: %v\n", result4)
	}

	// Demo 5: Task with timeout
	fmt.Println("\n--- Demo 5: Task with Timeout ---")

	// Register a slow handler
	execCtx.RegisterHandler("slow", func(ctx context.Context, input interface{}) (interface{}, error) {
		fmt.Println("🐌 Slow handler starting...")

		select {
		case <-time.After(3 * time.Second):
			fmt.Println("🐌 Slow handler completed")
			return map[string]interface{}{"status": "slow_success"}, nil
		case <-ctx.Done():
			fmt.Printf("🐌 Slow handler cancelled: %v\n", ctx.Err())
			return nil, ctx.Err()
		}
	})

	timeout := 1 // 1 second
	result5, err := handler.ExecuteWithTimeout(ctx, "slow", nil, nil, &timeout)
	if err != nil {
		fmt.Printf("Timeout occurred (expected): %v\n", err)
	} else {
		fmt.Printf("Result: %v\n", result5)
	}

	// Demo 6: Fallback behavior
	fmt.Println("\n--- Demo 6: Fallback (No Handler) ---")
	input6 := map[string]interface{}{
		"test": "no_handler",
	}

	result6, err := handler.Execute(ctx, "nonexistent", input6, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Result (input returned as-is): %v\n", result6)
	}

	fmt.Println("\n✅ Demo completed successfully!")
}

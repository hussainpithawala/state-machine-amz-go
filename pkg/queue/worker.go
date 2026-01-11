package queue

import (
	"context"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
)

// ExecutionHandler is an interface for handling state machine executions
// This interface breaks the circular dependency between queue and persistent packages
type ExecutionHandler interface {
	HandleExecution(ctx context.Context, payload *ExecutionTaskPayload) error
	HandleTimeout(ctx context.Context, payload *TimeoutTaskPayload) error
}

// Worker wraps asynq.Server for processing state machine execution tasks
type Worker struct {
	server           *asynq.Server
	mux              *asynq.ServeMux
	executionHandler ExecutionHandler
}

// NewWorker creates a new queue worker for processing tasks
func NewWorker(config *Config, handler ExecutionHandler) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	server := asynq.NewServer(
		config.GetRedisClientOpt(),
		config.GetServerConfig(),
	)

	mux := asynq.NewServeMux()

	worker := &Worker{
		server:           server,
		mux:              mux,
		executionHandler: handler,
	}

	// Register task handlers
	worker.registerHandlers()

	return worker, nil
}

// registerHandlers registers all task type handlers
func (w *Worker) registerHandlers() {
	w.mux.HandleFunc(TypeExecutionTask, w.handleExecutionTask)
	w.mux.HandleFunc(TypeTimeoutTask, w.handleTimeoutTask)
}

// handleExecutionTask processes a state machine execution task
func (w *Worker) handleExecutionTask(ctx context.Context, task *asynq.Task) error {
	// Parse the task payload
	payload, err := ParseExecutionTaskPayload(task)
	if err != nil {
		return fmt.Errorf("failed to parse task payload: %w", err)
	}

	log.Printf("Processing execution task: StateMachineID=%s, ExecutionID=%s, ExecutionName=%s, IsTimeout=%v",
		payload.StateMachineID, payload.ExecutionID, payload.ExecutionName, payload.IsTimeout)

	// Delegate to execution handler
	if err := w.executionHandler.HandleExecution(ctx, payload); err != nil {
		log.Printf("Execution failed: ExecutionName=%s, Error=%v", payload.ExecutionName, err)
		return fmt.Errorf("execution failed: %w", err)
	}

	log.Printf("Execution completed successfully: ExecutionName=%s", payload.ExecutionName)
	return nil
}

// handleTimeoutTask processes a timeout boundary event task
func (w *Worker) handleTimeoutTask(ctx context.Context, task *asynq.Task) error {
	// Parse the task payload
	payload, err := ParseTimeoutTaskPayload(task)
	if err != nil {
		return fmt.Errorf("failed to parse timeout task payload: %w", err)
	}

	log.Printf("Processing timeout task: ExecutionID=%s, StateName=%s, CorrelationID=%s",
		payload.ExecutionID, payload.StateName, payload.CorrelationID)

	// Delegate to execution handler
	if err := w.executionHandler.HandleTimeout(ctx, payload); err != nil {
		// Check if this is a benign error (correlation already processed)
		if err.Error() == "correlation already processed" {
			log.Printf("Timeout task skipped (correlation already processed): CorrelationID=%s", payload.CorrelationID)
			return nil // Don't retry
		}

		log.Printf("Timeout task failed: CorrelationID=%s, Error=%v", payload.CorrelationID, err)
		return fmt.Errorf("timeout task failed: %w", err)
	}

	log.Printf("Timeout task completed successfully: CorrelationID=%s", payload.CorrelationID)
	return nil
}

// Run starts the worker to process tasks from the queue
func (w *Worker) Run() error {
	log.Println("Starting worker to process state machine execution tasks...")
	if err := w.server.Run(w.mux); err != nil {
		return fmt.Errorf("worker failed to run: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the worker
func (w *Worker) Shutdown() {
	log.Println("Shutting down worker...")
	w.server.Shutdown()
}

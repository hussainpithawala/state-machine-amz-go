// cmd/microbatch/main.go
//
// Complete, runnable example of the micro-batch streaming framework integrated
// with state-machine-amz-go.  Run with:
//
//	go run ./cmd/microbatch/
//
// Environment variables (all optional, shown with defaults):
//
//	REDIS_ADDR          localhost:6379
//	POSTGRES_URL        (empty → uses in-memory repository)
//	SOURCE_EXEC_COUNT   5000   number of source executions to seed
//	MICRO_BATCH_SIZE    500    IDs per micro-batch
//	WORKER_CONCURRENCY  10     parallel queue workers
//	PAUSE_AT_BATCH      2      micro-batch index that simulates a soft-pause
//	                           (set to -1 to disable the pause demonstration)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/types"
	"github.com/redis/go-redis/v9"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/executor"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/handler"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/queue"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine/persistent"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/registry"
)

// ─── Workflow definitions ─────────────────────────────────────────────────────

// sourceWorkflowYAML is the upstream state machine whose SUCCEEDED executions
// are the "source execution IDs" fed into the micro-batch run.
const sourceWorkflowYAML = `
Comment: "Source workflow – produces executions that become micro-batch inputs"
StartAt: IngestRecord
States:
  IngestRecord:
    Type: Task
    Resource: "source:ingest"
    ResultPath: "$.ingestResult"
    Next: ValidateRecord
  ValidateRecord:
    Type: Task
    Resource: "source:validate"
    ResultPath: "$.validateResult"
    End: true
`

// targetWorkflowYAML is the downstream state machine that processes each source
// execution.  executeBatchConcurrent (with DoMicroBatch=true) drives this.
const targetWorkflowYAML = `
Comment: "Target workflow – enriches and stores each ingested record"
StartAt: EnrichRecord
States:
  EnrichRecord:
    Type: Task
    Resource: "target:enrich"
    ResultPath: "$.enrichResult"
    Next: StoreRecord
    Retry:
      - ErrorEquals: ["States.TaskFailed"]
        MaxAttempts: 2
        IntervalSeconds: 1
        BackoffRate: 2.0
  StoreRecord:
    Type: Task
    Resource: "target:store"
    ResultPath: "$.storeResult"
    End: true
`

// ─── Application wiring ───────────────────────────────────────────────────────

func main() {
	// Root context – cancelled on SIGINT/SIGTERM for clean shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
	slog.Info("shutdown complete")
}

func run(ctx context.Context) error {
	cfg := loadConfig()

	// ── Shared infrastructure ─────────────────────────────────────────────────
	manager, err := buildRepository(ctx, cfg)
	if err != nil {
		return fmt.Errorf("repository: %w", err)
	}
	//defer manager.Close()

	queueClient, err := buildQueueClient(cfg)
	if err != nil {
		return fmt.Errorf("queue client: %w", err)
	}

	orchRegistry := registry.New()

	// ─────────────────────────────────────────────────────────────────────────
	// STEP 1 – Create the persistent state machine.
	//
	// This is the TARGET state machine – the one that processes each individual
	// source execution record.  It is also the machine on which we call
	// ExecuteBatch, so it owns the micro-batch orchestration lifecycle.
	// ─────────────────────────────────────────────────────────────────────────
	slog.Info("step 1: creating target persistent state machine")

	targetSM, err := persistent.New(
		[]byte(targetWorkflowYAML),
		false, // YAML, not JSON
		"target-workflow-v1",
		manager,
	)
	if err != nil {
		return fmt.Errorf("step 1: %w", err)
	}

	// Register the target workflow's local task handlers so executions resolve.
	// Note: For distributed execution, task handlers are registered globally
	// and attached via ExecutionContextAdapter in the worker setup
	targetSM.SetQueueClient(queueClient)

	// Persist the definition (idempotent – safe to call on every startup).
	if err := targetSM.SaveDefinition(ctx); err != nil {
		return fmt.Errorf("step 1: save target definition: %w", err)
	}
	slog.Info("step 1 ✓", "sm_id", targetSM.GetID())

	// ─────────────────────────────────────────────────────────────────────────
	// STEP 2 – Create the Redis client.
	// ─────────────────────────────────────────────────────────────────────────
	slog.Info("step 2: connecting to Redis", "addr", cfg.redisAddr)

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.redisAddr,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
		MinIdleConns: 5,
	})
	// Verify connectivity before continuing.
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("step 2: redis ping: %w", err)
	}
	//defer rdb.Close()
	slog.Info("step 2 ✓", "addr", cfg.redisAddr)

	// ─────────────────────────────────────────────────────────────────────────
	// STEP 3 – Create the orchestrator.
	//
	// NewOrchestrator:
	//   • Loads the Lua barrier scripts into Redis (SCRIPT LOAD – idempotent).
	//   • Wires Barrier, MetricsRecorder, Evaluator, and ResumeController.
	//   • Holds a reference to targetSM so it can call ResumeExecution when
	//     the last worker in a micro-batch fires the barrier.
	// ─────────────────────────────────────────────────────────────────────────
	slog.Info("step 3: creating orchestrator (loading Lua scripts)")

	// Create factory functions for the orchestrator
	smFactory := func(ctx context.Context, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
		sm, err := persistent.NewFromDefnId(ctx, smID, mgr)
		return sm, err
	}

	smCreator := func(def []byte, isJSON bool, smID string, mgr *repository.Manager) (batch.StateMachine, error) {
		sm, err := persistent.New(def, isJSON, smID, mgr)
		return sm, err
	}

	orchestrator, err := batch.NewOrchestrator(ctx, rdb, targetSM, smFactory, smCreator)
	if err != nil {
		return fmt.Errorf("step 3: %w", err)
	}
	slog.Info("step 3 ✓")

	// ─────────────────────────────────────────────────────────────────────────
	// STEP 4 – Register the orchestrator state machine definition.
	//
	// EnsureDefinition saves the orchestrator's JSON into the repository so
	// that persistent.NewFromDefnId can retrieve it when Orchestrator.Run
	// spins up an orchestrator execution.  This call is fully idempotent –
	// safe to run on every application restart.
	// ─────────────────────────────────────────────────────────────────────────
	slog.Info("step 4: registering orchestrator state machine definition")

	if err := orchestrator.EnsureDefinition(ctx, batch.OrchestratorDefinitionJSON()); err != nil {
		return fmt.Errorf("step 4: %w", err)
	}
	slog.Info("step 4 ✓", "orchestrator_sm_id", batch.OrchestratorStateMachineID)

	// ─────────────────────────────────────────────────────────────────────────
	// STEP 5 – Register the orchestrator in the app-level registry.
	//
	// Queue workers only know the OrchestratorSMID from the task payload – they
	// don't hold a direct Go reference. The registry provides the lookup so the
	// worker can call orchestrator.SignalMicroBatchComplete without a global variable.
	// ─────────────────────────────────────────────────────────────────────────
	slog.Info("step 5: registering orchestrator in app registry",
		"key", batch.OrchestratorStateMachineID)

	orchRegistry.Register(batch.OrchestratorStateMachineID, orchestrator)
	slog.Info("step 5 ✓")

	// ── Seed source executions ────────────────────────────────────────────────
	// Seed the source state machine with N completed executions so there is
	// something for ExecuteBatch to filter on.  In a real system these already
	// exist – this block is only here to make the example self-contained.

	// Setup global executor with task handlers
	globalExec := setupExecutor()

	// setup context with executor
	ctx = context.WithValue(ctx, types.ExecutionContextKey, executor.NewExecutionContextAdapter(globalExec))

	sourceExecutionIDs, err := seedSourceExecutions(ctx, cfg, manager)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	slog.Info("source executions seeded", "count", len(sourceExecutionIDs))

	// ── Start queue worker pool ───────────────────────────────────────────────
	// Launch workers that pull tasks from the queue, run the target state
	// machine for each source execution, and drive the Redis barrier hook.
	workerWg, workerShutdown := startWorkerPool(ctx, cfg, manager, queueClient, globalExec, orchestrator, orchRegistry)

	// ── Start dedicated orchestrator continuation worker ──────────────────────
	// This worker watches for orchestrator executions that have completed a
	// Message state and need to continue to the next batch
	orchWorkerWg, orchWorkerShutdown := startOrchestratorContinuationWorker(ctx, rdb, orchestrator, manager, queueClient, smFactory)

	// ─────────────────────────────────────────────────────────────────────────
	// STEP 6 – Invoke the micro-batch execution.
	//
	// ExecuteBatch internally calls executeBatchConcurrent, which detects
	// DoMicroBatch=true and delegates to executeMicroBatch (persistent_microbatch.go).
	// That function:
	//   • Calls Orchestrator.Run → stores IDs in Redis, launches the orchestrator SM.
	//   • Returns immediately with a synthetic BatchExecutionResult.
	//
	// The orchestrator SM runs until WaitForMicroBatchCompletion (Message state),
	// at which point it pauses.  Workers drive it forward micro-batch by
	// micro-batch via the Redis barrier.
	// ─────────────────────────────────────────────────────────────────────────
	slog.Info("step 6: invoking micro-batch execution",
		"total_ids", len(sourceExecutionIDs),
		"micro_batch_size", cfg.microBatchSize,
	)

	filter := &repository.ExecutionFilter{
		StateMachineID: "source-workflow-v1",
		Status:         "SUCCEEDED",
	}

	batchOpts := &statemachine2.BatchExecutionOptions{
		NamePrefix:     fmt.Sprintf("nightly-run-%s", time.Now().Format("20060102-1504")),
		StopOnError:    false,
		DoMicroBatch:   true,
		MicroBatchSize: cfg.microBatchSize,
		RedisClient:    rdb,

		// Progress callbacks – wired to structured logging.
		OnExecutionStart: func(sourceExecID string, index int) {
			slog.Debug("task enqueued", "source_exec_id", sourceExecID, "index", index)
		},
		OnExecutionComplete: func(sourceExecID string, index int, err error) {
			if err != nil {
				slog.Warn("task enqueue failed", "source_exec_id", sourceExecID, "index", index, "err", err)
			}
		},
	}

	results, err := targetSM.ExecuteBatch(ctx, filter, "IngestRecord", batchOpts)
	if err != nil {
		return fmt.Errorf("step 6: ExecuteBatch: %w", err)
	}

	// The result slice contains one synthetic entry per orchestrator launch.
	// Execution=nil is expected – the orchestrator is still running asynchronously.
	for _, r := range results {
		if r.Error != nil {
			slog.Error("orchestration launch failed", "batch_id", r.SourceExecutionID, "err", r.Error)
		} else {
			slog.Info("step 6 ✓ – orchestrator launched",
				"batch_id", r.SourceExecutionID,
				"async", r.Execution == nil,
			)
		}
	}

	// Derive the batchID from the result so Step 7 can reference it.
	// (In production this ID would come from a database record, webhook payload,
	// or an audit log – here we read it straight from the synthetic result.)
	var activeBatchID string
	if len(results) > 0 {
		activeBatchID = results[0].SourceExecutionID
	}

	// ─────────────────────────────────────────────────────────────────────────
	// STEP 7 – Demonstrate the pause/resume path.
	//
	// If the evaluator detects that the rolling failure rate crossed the soft
	// threshold after a micro-batch, the orchestrator SM transitions to
	// PauseBatch and enters the CheckResumeSignal poll loop.
	//
	// An operator (or automated remediation system) calls orchestrator.Signal to set
	// the Redis key.  The next CheckResumeSignal poll reads-and-deletes it
	// (GETDEL) and sets shouldResume=true, allowing the orchestrator to
	// re-enter DispatchMicroBatch for the next micro-batch.
	//
	// Here we simulate this by waiting a few seconds and then sending the
	// resume signal so the example completes without human intervention.
	// ─────────────────────────────────────────────────────────────────────────
	if cfg.pauseAtBatch >= 0 && activeBatchID != "" {
		slog.Info("step 7: scheduling simulated operator resume signal",
			"batch_id", activeBatchID,
			"pause_at_batch", cfg.pauseAtBatch,
			"signal_delay", "8s",
		)
		go func() {
			// Give the orchestrator time to reach the PauseBatch state before
			// we send the resume signal.
			select {
			case <-ctx.Done():
				return
			case <-time.After(8 * time.Second):
			}
			slog.Info("step 7: operator sending resume signal",
				"batch_id", activeBatchID,
				"operator", "ops-engineer@company.com",
			)
			signalCtx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			//defer cancel()
			if err := orchestrator.Signal(
				signalCtx,
				activeBatchID,
				"ops-engineer@company.com",
				"Downstream API rate limit resolved – safe to continue",
			); err != nil {
				slog.Error("step 7: signal failed", "err", err)
				return
			}
			slog.Info("step 7 ✓ – resume signal sent", "batch_id", activeBatchID)
		}()
	} else {
		slog.Info("step 7: pause demonstration disabled (PAUSE_AT_BATCH=-1)")
	}

	// ── Wait for completion or interrupt ──────────────────────────────────────
	slog.Info("waiting for orchestrator to finish (Ctrl+C to interrupt)")
	if err := waitForOrchestratorCompletion(ctx, targetSM, activeBatchID); err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Warn("interrupted – orchestrator still running in background")
		} else {
			slog.Error("orchestration failed", "err", err)
		}
	} else {
		printFinalMetrics(ctx, rdb, activeBatchID)
	}

	// Graceful worker shutdown.
	slog.Info("shutting down workers...")
	workerShutdown()
	workerWg.Wait()
	slog.Info("all workers stopped")

	slog.Info("shutting down orchestrator continuation worker...")
	orchWorkerShutdown()
	orchWorkerWg.Wait()
	slog.Info("orchestrator continuation worker stopped")
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Queue worker pool (queue.Worker pattern from distributed_queue)
// ─────────────────────────────────────────────────────────────────────────────

// startWorkerPool creates and starts a worker pool using the queue.Worker API.
// This follows the same pattern as distributed_queue/main.go.
func startWorkerPool(
	ctx context.Context,
	cfg config,
	manager *repository.Manager,
	qc *queue.Client,
	baseExecutor *executor.BaseExecutor,
	orchestrator *batch.Orchestrator,
	orchRegistry *registry.OrchestratorRegistry,
) (*sync.WaitGroup, func()) {
	var wg sync.WaitGroup

	// Create execution context adapter to bridge executor with handler
	execAdapter := executor.NewExecutionContextAdapter(baseExecutor)

	// Create execution handler with executor context
	execHandler := handler.NewExecutionHandlerWithContext(manager, qc, execAdapter, orchestrator)

	// Build queue config for worker
	queueConfig := &queue.Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: cfg.redisAddr,
		},
		Concurrency: cfg.workerConcurrency,
		Queues: map[string]int{
			"default":            5,
			"target-workflow-v1": 5, // Target SM queue
			"source-workflow-v1": 2, // Source SM queue (lower priority)
		},
		RetryPolicy: &queue.RetryPolicy{
			MaxRetry: 3,
			Timeout:  10 * time.Minute,
		},
	}

	// Create worker with handler
	worker, err := queue.NewWorker(queueConfig, execHandler)
	if err != nil {
		slog.Error("failed to create worker", "err", err)
		return &wg, func() {}
	}

	// Start worker in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("worker starting", "concurrency", cfg.workerConcurrency)

		if err := worker.Run(); err != nil {
			slog.Error("worker error", "err", err)
		}

		slog.Info("worker stopped")
	}()

	// Return wait group and shutdown function
	shutdown := func() {
		slog.Info("initiating worker shutdown...")
		worker.Shutdown()
	}

	slog.Info("worker pool started", "concurrency", cfg.workerConcurrency)
	return &wg, shutdown
}

// Note: The processTask and fireMicroBatchHook functions have been removed.
// Task processing is now handled by the queue.ExecutionHandler which is
// registered with the Asynq server. The handler in pkg/handler/execution_handler.go
// automatically handles micro-batch hook signaling via the orchestrator registry.

// ─────────────────────────────────────────────────────────────────────────────
// Orchestrator Continuation Worker
// ─────────────────────────────────────────────────────────────────────────────

// startOrchestratorContinuationWorker starts a worker that monitors for orchestrator
// executions in RUNNING status (which means they were just resumed from a Message state)
// and continues executing them until they pause again or complete.
func startOrchestratorContinuationWorker(
	ctx context.Context,
	rdb *redis.Client,
	orchestrator *batch.Orchestrator,
	manager *repository.Manager,
	queueClient *queue.Client,
	smFactory func(context.Context, string, *repository.Manager) (batch.StateMachine, error),
) (*sync.WaitGroup, func()) {
	var wg sync.WaitGroup
	stopCh := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("orchestrator continuation worker starting")

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("orchestrator continuation worker stopping (context cancelled)")
				return
			case <-stopCh:
				slog.Info("orchestrator continuation worker stopping (shutdown signal)")
				return
			case <-ticker.C:
				// Check for orchestrators that need continuation
				if err := continueOrchestratorExecutions(ctx, orchestrator, manager, queueClient, smFactory); err != nil {
					slog.Error("orchestrator continuation error", "err", err)
				}
			}
		}
	}()

	shutdown := func() {
		close(stopCh)
	}

	slog.Info("orchestrator continuation worker started")
	return &wg, shutdown
}

// continueOrchestratorExecutions finds orchestrator executions in RUNNING status
// and continues their execution. These are orchestrators that were just resumed
// from a Message state and need to continue running.
//
// NOTE: This function is intentionally disabled because the orchestrator continuation
// is now handled automatically by the resumeOrchestrator method in orchestrator.go.
// When SignalMicroBatchComplete is called, it already resumes the orchestrator with
// proper handlers and executes it until it pauses or completes.
func continueOrchestratorExecutions(
	ctx context.Context,
	orchestrator *batch.Orchestrator,
	manager *repository.Manager,
	queueClient *queue.Client,
	smFactory func(context.Context, string, *repository.Manager) (batch.StateMachine, error),
) error {
	// Disabled - orchestrator continuation is handled by SignalMicroBatchComplete
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Source execution seeder (makes the example self-contained)
// ─────────────────────────────────────────────────────────────────────────────

// seedSourceExecutions creates N SUCCEEDED executions in the source state machine
// so ExecuteBatch has records to filter on.  In a real system these already exist.
func seedSourceExecutions(
	ctx context.Context,
	cfg config,
	manager *repository.Manager,
) ([]string, error) {
	slog.Info("seeding source executions", "count", cfg.sourceExecCount)

	sourceSM, err := persistent.New(
		[]byte(sourceWorkflowYAML),
		false,
		"source-workflow-v1",
		manager,
	)
	if err != nil {
		return nil, fmt.Errorf("create source SM: %w", err)
	}

	sourceExec := executor.NewBaseExecutor()
	sourceExec.RegisterGoFunction("source:ingest", handleSourceIngest)
	sourceExec.RegisterGoFunction("source:validate", handleSourceValidate)
	sourceSM.SetExecutor(sourceExec)

	if err := sourceSM.SaveDefinition(ctx); err != nil {
		return nil, fmt.Errorf("save source definition: %w", err)
	}

	ids := make([]string, 0, cfg.sourceExecCount)
	var mu sync.Mutex
	sem := make(chan struct{}, 20) // 20 concurrent seeders
	var wg sync.WaitGroup
	var seedErr error

	for i := 0; i < cfg.sourceExecCount; i++ {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer func() {
				<-sem
				wg.Done()
			}()
			input := map[string]interface{}{
				"record_id": fmt.Sprintf("record-%06d", idx),
				"source":    "nightly-export",
				"batch":     "2026-02-28",
			}
			execName := fmt.Sprintf("source-exec-%06d", idx)
			result, err := sourceSM.Execute(ctx, input,
				statemachine2.WithExecutionName(execName),
			)
			if err != nil {
				mu.Lock()
				seedErr = err
				mu.Unlock()
				return
			}
			mu.Lock()
			ids = append(ids, result.ID)
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if seedErr != nil {
		return nil, fmt.Errorf("seed worker error: %w", seedErr)
	}
	slog.Info("seeding complete", "seeded", len(ids))
	return ids, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Orchestrator completion polling
// ─────────────────────────────────────────────────────────────────────────────

// waitForOrchestratorCompletion polls the repository until the orchestrator
// execution for batchID reaches SUCCEEDED or FAILED, or ctx is cancelled.
// It prints a progress line every 10 seconds.
func waitForOrchestratorCompletion(
	ctx context.Context,
	targetSM *persistent.StateMachine,
	batchID string,
) error {
	if batchID == "" {
		return nil
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// List orchestrator executions for this batch.
			filter := &repository.ExecutionFilter{
				StateMachineID: batch.OrchestratorStateMachineID,
			}
			executions, err := targetSM.GetRepositoryManager().ListExecutions(ctx, filter)
			if err != nil {
				slog.Warn("poll: list executions failed", "err", err)
				continue
			}
			for _, exec := range executions {
				// Match by the batchID embedded in the execution name.
				if exec.Name == "" {
					continue
				}
				slog.Info("orchestrator status",
					"exec_id", exec.ExecutionID,
					"state", exec.CurrentState,
					"status", exec.Status,
				)
				switch exec.Status {
				case "SUCCEEDED":
					slog.Info("✓ orchestration SUCCEEDED", "batch_id", batchID)
					return nil
				case "FAILED":
					return fmt.Errorf("orchestration FAILED: batch_id=%s exec_id=%s", batchID, exec.ExecutionID)
				}
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Terminal metrics summary
// ─────────────────────────────────────────────────────────────────────────────

func printFinalMetrics(ctx context.Context, rdb *redis.Client, batchID string) {
	if batchID == "" {
		return
	}
	// Read cumulative counters from Redis directly.
	processed, _ := rdb.Get(ctx, fmt.Sprintf("metrics:%s:total_processed", batchID)).Int64()
	failed, _ := rdb.Get(ctx, fmt.Sprintf("metrics:%s:total_failed", batchID)).Int64()
	var rate float64
	if processed > 0 {
		rate = float64(failed) / float64(processed) * 100
	}
	fmt.Printf("\n┌─────────────────────────────────────────┐\n")
	fmt.Printf("│            Batch Final Metrics          │\n")
	fmt.Printf("├─────────────────────────────────────────┤\n")
	fmt.Printf("│  Batch ID   : %-26s│\n", truncate(batchID, 26))
	fmt.Printf("│  Processed  : %-26d│\n", processed)
	fmt.Printf("│  Failed     : %-26d│\n", failed)
	fmt.Printf("│  Failure %%  : %-25.2f%%│\n", rate)
	fmt.Printf("└─────────────────────────────────────────┘\n\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// ─────────────────────────────────────────────────────────────────────────────
// Task handlers (business logic)
// ─────────────────────────────────────────────────────────────────────────────

func handleSourceIngest(_ context.Context, input interface{}) (interface{}, error) {
	m, _ := input.(map[string]interface{})
	return map[string]interface{}{
		"record_id":   m["record_id"],
		"ingested_at": time.Now().UTC(),
		"status":      "ingested",
	}, nil
}

func handleSourceValidate(_ context.Context, input interface{}) (interface{}, error) {
	m, _ := input.(map[string]interface{})
	return map[string]interface{}{
		"record_id":    m["record_id"],
		"validated_at": time.Now().UTC(),
		"valid":        true,
	}, nil
}

func handleEnrich(_ context.Context, input interface{}) (interface{}, error) {
	m, _ := input.(map[string]interface{})
	// Simulate occasional transient errors that the Retry block will absorb.

	//if id, ok := m["record_id"].(string); ok && len(id) > 0 && id[len(id)-1] == '9' {
	//	// ~10 % of records end in '9' – simulate a retryable error.
	//	// On retry the input is the same so the second attempt succeeds.
	//	if _, already := m["_retry_ok"]; !already {
	//		return nil, fmt.Errorf("transient enrich error (will retry)")
	//	}
	//}

	return map[string]interface{}{
		"record_id":   m["record_id"],
		"enriched_at": time.Now().UTC(),
		"tags":        []string{"enriched", "ready"},
		"_retry_ok":   true,
	}, nil
}

func handleStore(_ context.Context, input interface{}) (interface{}, error) {
	m, _ := input.(map[string]interface{})
	return map[string]interface{}{
		"record_id": m["record_id"],
		"stored_at": time.Now().UTC(),
		"persisted": true,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Configuration
// ─────────────────────────────────────────────────────────────────────────────

type config struct {
	redisAddr         string
	postgresURL       string
	sourceExecCount   int
	microBatchSize    int
	workerConcurrency int
	pauseAtBatch      int
}

func loadConfig() config {
	return config{
		redisAddr:         envStr("REDIS_ADDR", "localhost:6379"),
		postgresURL:       envStr("POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"),
		sourceExecCount:   envInt("SOURCE_EXEC_COUNT", 10),
		microBatchSize:    envInt("MICRO_BATCH_SIZE", 5),
		workerConcurrency: envInt("WORKER_CONCURRENCY", 2),
		pauseAtBatch:      envInt("PAUSE_AT_BATCH", 2),
	}
}

func envStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

// ─────────────────────────────────────────────────────────────────────────────
// Infrastructure builders
// ─────────────────────────────────────────────────────────────────────────────

func buildRepository(ctx context.Context, cfg config) (*repository.Manager, error) {
	var repoCfg *repository.Config

	if cfg.postgresURL != "" {
		slog.Info("using PostgreSQL (GORM) repository", "url_set", true)
		repoCfg = &repository.Config{
			Strategy:      "gorm-postgres",
			ConnectionURL: cfg.postgresURL,
			Options: map[string]interface{}{
				"max_open_conns":    25,
				"max_idle_conns":    5,
				"conn_max_lifetime": 5 * time.Minute,
				"log_level":         "warn",
			},
		}
	} else {
		slog.Info("no POSTGRES_URL set – using in-memory repository (data is not persisted)")
		repoCfg = &repository.Config{Strategy: "memory"}
	}

	manager, err := repository.NewPersistenceManager(repoCfg)
	if err != nil {
		return nil, err
	}
	if err := manager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("initialize repository: %w", err)
	}
	return manager, nil
}

func buildQueueClient(cfg config) (*queue.Client, error) {
	// Asynq/Redis-backed queue client for distributed task execution
	// The framework's queue.Client wraps Asynq
	queueCfg := &queue.Config{
		RedisClientOpt: &asynq.RedisClientOpt{
			Addr: cfg.redisAddr,
		},
		Concurrency: cfg.workerConcurrency,
		Queues: map[string]int{
			"default": 10,
		},
	}
	return queue.NewClient(queueCfg)
}

// setupExecutor registers task handlers for both source and target workflows
func setupExecutor() *executor.BaseExecutor {
	exec := executor.NewBaseExecutor()

	// Source workflow handlers
	exec.RegisterGoFunction("source:ingest", handleSourceIngest)
	exec.RegisterGoFunction("source:validate", handleSourceValidate)

	// Target workflow handlers
	exec.RegisterGoFunction("target:enrich", handleEnrich)
	exec.RegisterGoFunction("target:store", handleStore)

	slog.Info("executor configured with task handlers")
	return exec
}

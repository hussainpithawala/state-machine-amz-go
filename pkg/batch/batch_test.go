package batch_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
	"github.com/redis/go-redis/v9"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newTestRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return client, func() { client.Close(); mr.Close() }
}

// ─── Barrier ──────────────────────────────────────────────────────────────────

func TestBarrier_ExactlyOneWinner(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	b, err := batch.NewBarrier(ctx, rdb)
	if err != nil {
		t.Fatalf("NewBarrier: %v", err)
	}

	const n = 100
	mbID := "test-batch:0"
	if err := b.Init(ctx, mbID, n); err != nil {
		t.Fatalf("Init: %v", err)
	}

	winners := 0
	for i := 0; i < n; i++ {
		res, err := b.Decrement(ctx, mbID)
		if err != nil {
			t.Fatalf("Decrement[%d]: %v", i, err)
		}
		if res.IsLastWorker {
			winners++
		}
	}
	if winners != 1 {
		t.Errorf("expected 1 winner, got %d", winners)
	}
}

func TestBarrier_Idempotent_Init(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	b, _ := batch.NewBarrier(ctx, rdb)
	const mbID = "test-batch:1"
	b.Init(ctx, mbID, 5)
	b.Init(ctx, mbID, 999) // second init must be no-op

	rem, _ := b.Remaining(ctx, mbID)
	if rem != 5 {
		t.Errorf("expected remaining=5, got %d", rem)
	}
}

func TestBarrier_DoubleFire_ReturnsSentinel(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	b, _ := batch.NewBarrier(ctx, rdb)
	const mbID = "test-batch:2"
	b.Init(ctx, mbID, 1)
	b.Decrement(ctx, mbID) // clears key

	res, err := b.Decrement(ctx, mbID)
	if err != nil {
		t.Fatalf("second decrement: %v", err)
	}
	if res.Remaining != -1 {
		t.Errorf("expected sentinel -1, got %d", res.Remaining)
	}
	if res.IsLastWorker {
		t.Error("double-fire must not claim IsLastWorker")
	}
}

// ─── Metrics ──────────────────────────────────────────────────────────────────

func TestMetrics_InstantaneousRate(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	m := batch.NewMetricsRecorder(rdb, 10)
	batchID := "m-batch"

	// 3 failures, 7 successes → 30 %
	for i := 0; i < 10; i++ {
		m.RecordOutcome(ctx, batch.TaskOutcome{
			BatchID: batchID, MicroBatchID: "x", TaskID: fmt.Sprintf("t%d", i),
			Success: i >= 3, CompletedAt: time.Now(),
		})
	}

	rate, _ := m.InstantaneousFailureRate(ctx, batchID)
	if rate < 0.29 || rate > 0.31 {
		t.Errorf("expected ~0.30, got %.4f", rate)
	}
}

func TestMetrics_SlidingWindowTrim(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	m := batch.NewMetricsRecorder(rdb, 5)
	batchID := "trim-batch"

	// 10 failures then 5 successes; window=5 → only successes remain
	for i := 0; i < 10; i++ {
		m.RecordOutcome(ctx, batch.TaskOutcome{BatchID: batchID, Success: false, CompletedAt: time.Now()})
	}
	for i := 0; i < 5; i++ {
		m.RecordOutcome(ctx, batch.TaskOutcome{BatchID: batchID, Success: true, CompletedAt: time.Now()})
	}

	rate, _ := m.InstantaneousFailureRate(ctx, batchID)
	if rate != 0 {
		t.Errorf("expected 0 after trim, got %.4f", rate)
	}
}

// ─── Evaluator ────────────────────────────────────────────────────────────────

func TestEvaluator_SevereFailure(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	m := batch.NewMetricsRecorder(rdb, 10)
	ev := batch.NewEvaluator(m)

	for i := 0; i < 10; i++ {
		m.RecordOutcome(ctx, batch.TaskOutcome{BatchID: "ev-batch", Success: i >= 6, CompletedAt: time.Now()})
	}

	policy := batch.FailurePolicy{SevereFailureThreshold: 0.5, SoftFailureThreshold: 0.2}
	res, _ := ev.Evaluate(ctx, "ev-batch", 0, policy)
	if !res.SevereFailure {
		t.Error("expected SevereFailure=true at 40% failure rate")
	}
}

func TestEvaluator_SoftPause(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	m := batch.NewMetricsRecorder(rdb, 10)
	ev := batch.NewEvaluator(m)

	// 30 % failure – above soft (20 %), below severe (50 %)
	for i := 0; i < 10; i++ {
		m.RecordOutcome(ctx, batch.TaskOutcome{BatchID: "pause-batch", Success: i >= 3, CompletedAt: time.Now()})
	}
	for i := 0; i < 3; i++ {
		rdb.Set(ctx, fmt.Sprintf("metrics:pause-batch:mb:%d:rate", i), 0.30, 0)
	}

	policy := batch.FailurePolicy{WindowM: 5, SevereFailureThreshold: 0.5, SoftFailureThreshold: 0.2}
	res, _ := ev.Evaluate(ctx, "pause-batch", 2, policy)
	if res.SevereFailure {
		t.Error("should not be SevereFailure at 30%")
	}
	if !res.BelowThreshold {
		t.Error("expected BelowThreshold=true when rolling avg 30% > soft 20%")
	}
}

func TestEvaluator_Healthy(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	m := batch.NewMetricsRecorder(rdb, 10)
	ev := batch.NewEvaluator(m)

	for i := 0; i < 10; i++ {
		m.RecordOutcome(ctx, batch.TaskOutcome{BatchID: "ok-batch", Success: true, CompletedAt: time.Now()})
	}

	policy := batch.FailurePolicy{SevereFailureThreshold: 0.5, SoftFailureThreshold: 0.2}
	res, _ := ev.Evaluate(ctx, "ok-batch", 0, policy)
	if res.SevereFailure || res.BelowThreshold {
		t.Errorf("healthy batch must not trigger pause/halt (severe=%v, below=%v)", res.SevereFailure, res.BelowThreshold)
	}
}

// ─── Resume Controller ────────────────────────────────────────────────────────

func TestResume_SignalConsumedOnce(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	rc := batch.NewResumeController(rdb)
	const batchID = "pause-op"

	// No signal yet
	res, _ := rc.Check(ctx, batchID)
	if res.ShouldResume {
		t.Error("expected ShouldResume=false before signal")
	}

	// Operator signals
	rc.Signal(ctx, batchID, "ops@co.com", "DB recovered")

	res, _ = rc.Check(ctx, batchID)
	if !res.ShouldResume {
		t.Error("expected ShouldResume=true after signal")
	}
	if res.ResumedBy != "ops@co.com" {
		t.Errorf("unexpected ResumedBy: %q", res.ResumedBy)
	}

	// Signal already consumed
	res, _ = rc.Check(ctx, batchID)
	if res.ShouldResume {
		t.Error("signal must be consumed after first check")
	}
}

// ─── MicroBatchMeta extraction ────────────────────────────────────────────────

func TestWorkerExtractMeta(t *testing.T) {
	meta := batch.MicroBatchMeta{
		BatchID:          "b1",
		MicroBatchID:     "b1:0",
		OrchestratorSMID: "micro-batch-orchestrator-v1",
	}

	// Simulate how the dispatcher embeds meta in task input
	// import s := "encoding/json"
	data, _ := json.Marshal(meta)
	taskInput := map[string]interface{}{
		batch.MicroBatchInputKey: string(data),
	}

	extracted, ok := batch.WorkerExtractMeta(taskInput)
	if !ok {
		t.Fatal("WorkerExtractMeta: expected ok=true")
	}
	if extracted.BatchID != meta.BatchID {
		t.Errorf("BatchID: want %q, got %q", meta.BatchID, extracted.BatchID)
	}
	if extracted.MicroBatchID != meta.MicroBatchID {
		t.Errorf("MicroBatchID: want %q, got %q", meta.MicroBatchID, extracted.MicroBatchID)
	}
}

func TestWorkerExtractMeta_NilInput(t *testing.T) {
	_, ok := batch.WorkerExtractMeta(nil)
	if ok {
		t.Error("expected ok=false for nil input")
	}
}

// ─── Integration: full micro-batch loop (no state machine; pure barrier logic) ─

func TestIntegration_MicroBatchBarrierCycle(t *testing.T) {
	rdb, cleanup := newTestRedis(t)
	defer cleanup()
	ctx := context.Background()

	barrier, _ := batch.NewBarrier(ctx, rdb)
	metrics := batch.NewMetricsRecorder(rdb, 50)
	ev := batch.NewEvaluator(metrics)

	const (
		batchID  = "int-batch"
		totalMBs = 3
		mbSize   = 20
	)

	for mb := 0; mb < totalMBs; mb++ {
		mbID := fmt.Sprintf("%s:%d", batchID, mb)
		barrier.Init(ctx, mbID, mbSize)

		var winners int
		for w := 0; w < mbSize; w++ {
			metrics.RecordOutcome(ctx, batch.TaskOutcome{
				BatchID: batchID, MicroBatchID: mbID,
				TaskID:  fmt.Sprintf("id-%04d", mb*mbSize+w),
				Success: true, CompletedAt: time.Now(),
			})
			res, _ := barrier.Decrement(ctx, mbID)
			if res.IsLastWorker {
				winners++
				metrics.SnapshotMicroBatchRate(ctx, batchID, mb)
			}
		}
		if winners != 1 {
			t.Errorf("mb%d: expected 1 winner, got %d", mb, winners)
		}

		policy := batch.FailurePolicy{SevereFailureThreshold: 0.5, SoftFailureThreshold: 0.2}
		eval, _ := ev.Evaluate(ctx, batchID, mb, policy)
		if eval.SevereFailure || eval.BelowThreshold {
			t.Errorf("mb%d: healthy batch triggered pause/halt", mb)
		}
	}

	processed, failed, _ := metrics.CumulativeCounters(ctx, batchID)
	if processed != int64(totalMBs*mbSize) {
		t.Errorf("expected %d processed, got %d", totalMBs*mbSize, processed)
	}
	if failed != 0 {
		t.Errorf("expected 0 failed, got %d", failed)
	}
}

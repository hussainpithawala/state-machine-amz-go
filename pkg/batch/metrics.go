package batch

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// MetricsRecorder records per-task outcomes into Redis and exposes the two
// statistical control measures used by the Evaluator.
//
//  1. Instantaneous failure rate  – fraction of "1"s in the last N entries of
//     a Redis List (ring-buffer trimmed to windowN).
//
//  2. Rolling average failure rate – mean of per-micro-batch rate snapshots
//     across the last M completed micro-batches, stored as individual keys.
type MetricsRecorder struct {
	rdb     *redis.Client
	windowN int
}

// NewMetricsRecorder returns a MetricsRecorder wired to the given Redis client.
func NewMetricsRecorder(rdb *redis.Client, windowN int) *MetricsRecorder {
	if windowN <= 0 {
		windowN = DefaultWindowN
	}
	return &MetricsRecorder{rdb: rdb, windowN: windowN}
}

// RecordOutcome appends the outcome (0=success, 1=failure) to the sliding
// window and increments the cumulative counters.  Safe for concurrent callers.
func (m *MetricsRecorder) RecordOutcome(ctx context.Context, o TaskOutcome) error {
	pipe := m.rdb.Pipeline()
	bit := "0"
	if !o.Success {
		bit = "1"
	}
	wk := keyMetricsWindow(o.BatchID)
	pipe.RPush(ctx, wk, bit)
	pipe.LTrim(ctx, wk, int64(-m.windowN), -1)
	pipe.Expire(ctx, wk, MetricsTTLSeconds)
	pipe.Incr(ctx, keyTotalProcessed(o.BatchID))
	if !o.Success {
		pipe.Incr(ctx, keyTotalFailed(o.BatchID))
	}
	_, err := pipe.Exec(ctx)
	return wrapErr(err, "RecordOutcome %s", o.BatchID)
}

// SnapshotMicroBatchRate computes the current instantaneous rate from the
// sliding window and stores it as the authoritative rate for this micro-batch.
// Called by the barrier winner immediately after detecting zero remaining.
func (m *MetricsRecorder) SnapshotMicroBatchRate(ctx context.Context, batchID string, mbIndex int) (float64, error) {
	entries, err := m.rdb.LRange(ctx, keyMetricsWindow(batchID), 0, -1).Result()
	if err != nil {
		return 0, wrapErr(err, "snapshot lrange %s", batchID)
	}
	rate := computeFailureRate(entries)
	if err := m.rdb.Set(ctx, keyMicroBatchRate(batchID, mbIndex), rate, MetricsTTLSeconds).Err(); err != nil {
		return 0, wrapErr(err, "snapshot set %s/%d", batchID, mbIndex)
	}
	return rate, nil
}

// InstantaneousFailureRate returns the failure rate over the last windowN tasks.
func (m *MetricsRecorder) InstantaneousFailureRate(ctx context.Context, batchID string) (float64, error) {
	entries, err := m.rdb.LRange(ctx, keyMetricsWindow(batchID), 0, -1).Result()
	if err != nil {
		return 0, wrapErr(err, "instant rate %s", batchID)
	}
	return computeFailureRate(entries), nil
}

// RollingAverageFailureRate returns the mean failure rate across the last M
// micro-batches ending at currentMBIndex.
func (m *MetricsRecorder) RollingAverageFailureRate(ctx context.Context, batchID string, currentMBIndex, windowM int) (float64, error) {
	if windowM <= 0 {
		windowM = DefaultWindowM
	}
	start := currentMBIndex - windowM + 1
	if start < 0 {
		start = 0
	}
	var total float64
	var count int
	for i := start; i <= currentMBIndex; i++ {
		v, err := m.rdb.Get(ctx, keyMicroBatchRate(batchID, i)).Float64()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return 0, wrapErr(err, "rolling rate %s/%d", batchID, i)
		}
		total += v
		count++
	}
	if count == 0 {
		return 0, nil
	}
	return total / float64(count), nil
}

// CumulativeCounters returns the all-time processed and failed task counts.
func (m *MetricsRecorder) CumulativeCounters(ctx context.Context, batchID string) (processed, failed int64, err error) {
	pipe := m.rdb.Pipeline()
	pc := pipe.Get(ctx, keyTotalProcessed(batchID))
	fc := pipe.Get(ctx, keyTotalFailed(batchID))
	if _, e := pipe.Exec(ctx); e != nil && e != redis.Nil {
		return 0, 0, wrapErr(e, "cumulative counters %s", batchID)
	}
	if v, e := pc.Int64(); e == nil {
		processed = v
	}
	if v, e := fc.Int64(); e == nil {
		failed = v
	}
	return processed, failed, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func computeFailureRate(entries []string) float64 {
	if len(entries) == 0 {
		return 0
	}
	var failures float64
	for _, e := range entries {
		v, _ := strconv.ParseFloat(e, 64)
		failures += v
	}
	return failures / float64(len(entries))
}

func wrapErr(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("batch/metrics: "+format+": %w", append(args, err)...)
}

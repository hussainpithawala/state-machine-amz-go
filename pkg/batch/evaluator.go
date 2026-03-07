package batch

import (
	"context"
	"fmt"
)

// Evaluator is the heart of the adaptive feedback loop.  It is called by the
// batch:evaluate task handler after every WaitForMicroBatchCompletion resolves.
//
// Decision matrix
// ───────────────
//
//	instantaneous rate ≥ SevereFailureThreshold  →  SevereFailure  (BatchFailed)
//	rolling M-batch avg ≥ SoftFailureThreshold   →  BelowThreshold (PauseBatch)
//	neither                                       →  continue       (DispatchMicroBatch)
type Evaluator struct {
	metrics *MetricsRecorder
}

// NewEvaluator returns an Evaluator backed by the given MetricsRecorder.
func NewEvaluator(metrics *MetricsRecorder) *Evaluator {
	return &Evaluator{metrics: metrics}
}

// Evaluate computes both statistical control measures and returns an
// EvaluationResult that drives the EvaluationDecision Choice state.
//
//	batchID        – the batch being evaluated
//	completedIndex – the 0-based micro-batch index that just finished
//	policy         – failure thresholds and window sizes
func (e *Evaluator) Evaluate(ctx context.Context, batchID string, completedIndex int, policy FailurePolicy) (EvaluationResult, error) {
	applyDefaults(&policy)

	instantRate, err := e.metrics.InstantaneousFailureRate(ctx, batchID)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("evaluator: instantaneous rate: %w", err)
	}

	rollingRate, err := e.metrics.RollingAverageFailureRate(ctx, batchID, completedIndex, policy.WindowM)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("evaluator: rolling avg rate: %w", err)
	}

	processed, failed, err := e.metrics.CumulativeCounters(ctx, batchID)
	if err != nil {
		return EvaluationResult{}, fmt.Errorf("evaluator: cumulative counters: %w", err)
	}

	severeFailure := instantRate >= policy.SevereFailureThreshold
	belowThreshold := !severeFailure && (rollingRate >= policy.SoftFailureThreshold)

	return EvaluationResult{
		SevereFailure:             severeFailure,
		BelowThreshold:            belowThreshold,
		InstantaneousFailureRate:  instantRate,
		RollingAverageFailureRate: rollingRate,
		TotalProcessed:            processed,
		TotalFailed:               failed,
	}, nil
}

func applyDefaults(p *FailurePolicy) {
	if p.WindowN <= 0 {
		p.WindowN = DefaultWindowN
	}
	if p.WindowM <= 0 {
		p.WindowM = DefaultWindowM
	}
	if p.SevereFailureThreshold <= 0 {
		p.SevereFailureThreshold = DefaultSevereFailureThreshold
	}
	if p.SoftFailureThreshold <= 0 {
		p.SoftFailureThreshold = DefaultSoftFailureThreshold
	}
}

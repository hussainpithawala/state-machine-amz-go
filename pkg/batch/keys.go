package batch

import "fmt"

// ──────────────────────────────────────────────────────────────────────────────
// Redis Key Schema
//
//   batch:<batchID>:ids                  LIST    – full ordered set of source execution IDs
//   batch:<batchID>:cursor               STRING  – INT offset into the IDs list
//   barrier:<microBatchID>               STRING  – INT decrement counter
//   metrics:<batchID>:window             LIST    – sliding window of "0"/"1" outcomes
//   metrics:<batchID>:mb:<idx>:rate      STRING  – FLOAT snapshot per micro-batch
//   metrics:<batchID>:total_processed    STRING  – INT cumulative processed count
//   metrics:<batchID>:total_failed       STRING  – INT cumulative failed count
//   resume:<batchID>                     STRING  – JSON ResumeSignal set by operator
// ──────────────────────────────────────────────────────────────────────────────

const (
	// DefaultMicroBatchSize is used when OrchestratorInput.MicroBatchSize == 0.
	DefaultMicroBatchSize = 1_000

	// DefaultWindowN is the instantaneous failure-rate window (individual tasks).
	DefaultWindowN = 500

	// DefaultWindowM is the rolling-average window (number of micro-batches).
	DefaultWindowM = 5

	// DefaultSevereFailureThreshold halts the batch immediately.
	DefaultSevereFailureThreshold = 0.5

	// DefaultSoftFailureThreshold pauses the batch for human review.
	DefaultSoftFailureThreshold = 0.2

	// BarrierTTLSeconds is how long a barrier key lives in Redis.
	BarrierTTLSeconds = 86_400 // 24 h

	// MetricsTTLSeconds is the lifetime of per-batch metric keys.
	MetricsTTLSeconds = 7 * 86_400 // 7 days

	// ResumeTTLSeconds is the lifetime of a resume signal key.
	ResumeTTLSeconds = 86_400 // 24 h

	// IDsListTTLSeconds is the lifetime of the stored IDs list in Redis.
	IDsListTTLSeconds = 7 * 86_400 // 7 days

	// MicroBatchCorrelationKey is the CorrelationKey used in the Message state.
	// Every WaitForMicroBatchCompletion pause is correlated on this key.
	MicroBatchCorrelationKey = "micro_batch_id"
)

func keyIDsList(batchID string) string {
	return fmt.Sprintf("batch:%s:ids", batchID)
}

func keyCursor(batchID string) string {
	return fmt.Sprintf("batch:%s:cursor", batchID)
}

func keyBarrier(microBatchID string) string {
	return fmt.Sprintf("barrier:%s", microBatchID)
}

func keyMetricsWindow(batchID string) string {
	return fmt.Sprintf("metrics:%s:window", batchID)
}

func keyMicroBatchRate(batchID string, mbIndex int) string {
	return fmt.Sprintf("metrics:%s:mb:%d:rate", batchID, mbIndex)
}

func keyResume(batchID string) string {
	return fmt.Sprintf("resume:%s", batchID)
}

func keyTotalProcessed(batchID string) string {
	return fmt.Sprintf("metrics:%s:total_processed", batchID)
}

func keyTotalFailed(batchID string) string {
	return fmt.Sprintf("metrics:%s:total_failed", batchID)
}

// microBatchID constructs the stable correlation value for a specific micro-batch.
// Format: "<batchID>:<index>"
func microBatchID(batchID string, index int) string {
	return fmt.Sprintf("%s:%d", batchID, index)
}

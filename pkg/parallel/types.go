package parallel

const (
	StatusWaiting   = "WAITING"
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"

	ParallelBranchInputKey = "__parallel_branch_meta__"
	ParallelCorrelationKey = "parallel_id"

	FailurePolicyWaitForAll = "WAIT_FOR_ALL"
	FailurePolicyFailFast   = "FAIL_FAST"
)

// ParallelBranchMeta is embedded in the queue task payload input so workers
// can identify tasks belonging to a parallel branch and drive the barrier on
// completion.
type ParallelBranchMeta struct {
	ParallelID      string `json:"parallel_id"`
	BranchIndex     int    `json:"branch_index"`
	ParentExecID    string `json:"parent_exec_id"`
	ParentSMID      string `json:"parent_sm_id"`
	ParentStateName string `json:"parent_state_name"`
	FailurePolicy   string `json:"failure_policy,omitempty"`
}

// WorkerExtractMeta extracts ParallelBranchMeta from a task input payload.
// Returns nil, false if the input does not contain parallel branch metadata.
func WorkerExtractMeta(input interface{}) (*ParallelBranchMeta, bool) {
	inputMap, ok := input.(map[string]interface{})
	if !ok {
		return nil, false
	}

	raw, exists := inputMap[ParallelBranchInputKey]
	if !exists {
		return nil, false
	}

	metaMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, false
	}

	meta := &ParallelBranchMeta{}

	if v, ok := metaMap["parallel_id"].(string); ok {
		meta.ParallelID = v
	}
	if v, ok := metaMap["branch_index"].(float64); ok {
		meta.BranchIndex = int(v)
	}
	if v, ok := metaMap["parent_exec_id"].(string); ok {
		meta.ParentExecID = v
	}
	if v, ok := metaMap["parent_sm_id"].(string); ok {
		meta.ParentSMID = v
	}
	if v, ok := metaMap["parent_state_name"].(string); ok {
		meta.ParentStateName = v
	}
	if v, ok := metaMap["failure_policy"].(string); ok {
		meta.FailurePolicy = v
	}

	if meta.ParallelID == "" {
		return nil, false
	}

	return meta, true
}

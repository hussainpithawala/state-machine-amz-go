package queue

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// ExecutionTaskAggregator implements asynq.GroupAggregator to combine
// individual execution tasks into a single batch task for processing.
//
// When tasks are enqueued with the same GroupID, Asynq holds them in Redis
// until a trigger condition is met (GroupMaxSize, GroupMaxDelay, or GroupGracePeriod).
// The aggregator then combines all tasks in the group into a single batch task
// that can be processed together, reducing overhead and enabling batch optimization.
type ExecutionTaskAggregator struct{}

// Aggregate takes a slice of tasks belonging to the same group and returns
// a single aggregated task containing all original payloads.
//
// The aggregated task uses TypeBatchTask ("statemachine:batch") as its type
// and contains a BatchTaskPayload with all individual task payloads.
func (a *ExecutionTaskAggregator) Aggregate(group string, tasks []*asynq.Task) *asynq.Task {
	if len(tasks) == 0 {
		return nil
	}

	// Extract payloads from all tasks
	batchPayload := &BatchTaskPayload{
		GroupID:      group,
		TaskCount:    len(tasks),
		OriginalType: TypeExecutionTask,
		Tasks:        make([]json.RawMessage, 0, len(tasks)),
	}

	for i, task := range tasks {
		// Preserve the original payload as-is
		batchPayload.Tasks = append(batchPayload.Tasks, task.Payload())

		// Extract metadata from first task for the aggregated task
		if i == 0 {
			var execPayload ExecutionTaskPayload
			if err := json.Unmarshal(task.Payload(), &execPayload); err == nil {
				batchPayload.StateMachineID = execPayload.StateMachineID
				batchPayload.SourceStateName = execPayload.SourceStateName
				batchPayload.InputTransformerName = execPayload.InputTransformerName
			}
		}
	}

	// Marshal the batch payload
	data, err := json.Marshal(batchPayload)
	if err != nil {
		// Fallback: return the first task if marshaling fails
		fmt.Printf("error: failed to marshal batch payload for group %s: %v\n", group, err)
		return tasks[0]
	}

	// Create the aggregated task with a unique task ID based on the group
	aggregatedTask := asynq.NewTask(
		TypeBatchTask,
		data,
		asynq.TaskID(fmt.Sprintf("batch-%s", group)),
	)

	return aggregatedTask
}

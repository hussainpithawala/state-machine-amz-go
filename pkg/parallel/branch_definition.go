package parallel

import (
	"encoding/json"
	"fmt"

	"github.com/hussainpithawala/state-machine-amz-go/internal/states"
)

// CreateBranchDefinition marshals a Branch into a valid ASL JSON state machine
// definition and generates a deterministic state machine ID.
// The returned JSON can be stored in the repository and loaded by queue workers
// via persistent.NewFromDefnId.
func CreateBranchDefinition(parentSMID, parallelStateName string, branchIndex int, branch states.Branch) (defnJSON []byte, smID string, err error) {
	smID = fmt.Sprintf("%s-parallel-%s-branch-%d", parentSMID, parallelStateName, branchIndex)

	defn := map[string]interface{}{
		"Name":    smID,
		"StartAt": branch.StartAt,
		"States":  branch.States,
	}

	if branch.Comment != "" {
		defn["Comment"] = branch.Comment
	}

	defnJSON, err = json.Marshal(defn)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal branch %d definition: %w", branchIndex, err)
	}

	return defnJSON, smID, nil
}

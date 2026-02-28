package batch

import _ "embed"

// OrchestratorDefinitionJSON returns the orchestrator state machine definition
// as a JSON byte slice.  The file is embedded at compile time via go:embed so
// there is no runtime file-path dependency.
//
// Call Orchestrator.EnsureDefinition(ctx, OrchestratorDefinitionJSON()) once
// at application startup to register the definition in the repository.

//go:embed workflows/microbatch_orchestrator.json
var orchestratorDefinitionBytes []byte

// OrchestratorDefinitionJSON returns the embedded JSON definition.
func OrchestratorDefinitionJSON() []byte {
	return orchestratorDefinitionBytes
}

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

// BulkOrchestratorDefinitionJSON returns the bulk orchestrator state machine definition
// as a JSON byte slice.  The file is embedded at compile time via go:embed so
// there is no runtime file-path dependency.
//
// Call Orchestrator.EnsureDefinition(ctx, BulkOrchestratorDefinitionJSON()) once
// at application startup to register the bulk definition in the repository.

//go:embed workflows/bulk_orchestrator.json
var bulkOrchestratorDefinitionBytes []byte

// BulkOrchestratorDefinitionJSON returns the embedded bulk orchestrator JSON definition.
func BulkOrchestratorDefinitionJSON() []byte {
	return bulkOrchestratorDefinitionBytes
}

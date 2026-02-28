// Package registry provides a thread-safe store that maps an orchestrator
// state-machine ID to the live *batch.Orchestrator instance.
//
// The queue worker needs to call orch.SignalMicroBatchComplete after every
// task, but it receives only the OrchestratorSMID (embedded in the task
// payload) – not a direct Go reference. The registry bridges that gap without
// requiring a global variable.
package registry

import (
	"fmt"
	"sync"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/batch"
)

// OrchestratorRegistry maps orchestrator state-machine IDs to live instances.
type OrchestratorRegistry struct {
	mu    sync.RWMutex
	store map[string]*batch.Orchestrator
}

// New returns an empty OrchestratorRegistry.
func New() *OrchestratorRegistry {
	return &OrchestratorRegistry{store: make(map[string]*batch.Orchestrator)}
}

// Register stores orch under id.  Overwrites any previous entry for that id.
func (r *OrchestratorRegistry) Register(id string, orch *batch.Orchestrator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[id] = orch
}

// Get returns the orchestrator registered under id, or an error if not found.
func (r *OrchestratorRegistry) Get(id string) (*batch.Orchestrator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	orch, ok := r.store[id]
	if !ok {
		return nil, fmt.Errorf("registry: no orchestrator registered for id %q", id)
	}
	return orch, nil
}

// Remove deletes the entry for id (call after orchestration completes to free memory).
func (r *OrchestratorRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.store, id)
}

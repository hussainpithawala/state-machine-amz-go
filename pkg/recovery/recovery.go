// pkg/recovery/recovery.go
package recovery

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/execution"
	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

// RecoveryStrategy defines the strategy for handling orphaned executions
type RecoveryStrategy string

const (
	// StrategyRetry retries the last failed state
	StrategyRetry RecoveryStrategy = "RETRY"
	// StrategySkip skips the last state and moves to the next (if known)
	StrategySkip RecoveryStrategy = "SKIP"
	// StrategyFail marks the execution as failed after max attempts
	StrategyFail RecoveryStrategy = "FAIL"
	// StrategyPause pauses the execution for manual intervention
	StrategyPause RecoveryStrategy = "PAUSE"
)

// RecoveryConfig holds configuration for the recovery manager
type RecoveryConfig struct {
	// Enabled enables/disables automatic recovery
	Enabled bool
	// ScanInterval is how often to scan for orphaned executions
	ScanInterval time.Duration
	// OrphanedThreshold is the duration after which a RUNNING execution is considered orphaned
	OrphanedThreshold time.Duration
	// DefaultRecoveryStrategy is the default strategy for recovery
	DefaultRecoveryStrategy RecoveryStrategy
	// DefaultMaxRecoveryAttempts is the default max recovery attempts
	DefaultMaxRecoveryAttempts int
	// StateMachineID filters recovery to specific state machine (empty = all)
	StateMachineID string
}

// DefaultRecoveryConfig returns a default recovery configuration
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		Enabled:                  true,
		ScanInterval:             30 * time.Second,
		OrphanedThreshold:        5 * time.Minute,
		DefaultRecoveryStrategy:  StrategyRetry,
		DefaultMaxRecoveryAttempts: 3,
	}
}

// RecoveryManager handles crash recovery for state machine executions
type RecoveryManager struct {
	repositoryManager *repository.Manager
	config            *RecoveryConfig
	stopChan          chan struct{}
	isRunning         bool
	mu                sync.Mutex
	recoveryFunc      RecoveryFunc
}

// RecoveryFunc is the function signature for executing recovery
type RecoveryFunc func(ctx context.Context, exec *execution.Execution) (*execution.Execution, error)

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(repositoryManager *repository.Manager, config *RecoveryConfig) *RecoveryManager {
	if config == nil {
		config = DefaultRecoveryConfig()
	}
	return &RecoveryManager{
		repositoryManager: repositoryManager,
		config:            config,
		stopChan:          make(chan struct{}),
	}
}

// OrphanedExecution represents an execution that appears to be orphaned
type OrphanedExecution struct {
	ExecutionID    string
	StateMachineID string
	CurrentState   string
	Status         string
	StartTime      time.Time
	LastUpdateTime time.Time
	Duration       time.Duration
	RecoveryMetadata *repository.RecoveryMetadata
}

// FindOrphanedExecutions finds executions that appear to be stuck in RUNNING state
func (rm *RecoveryManager) FindOrphanedExecutions(ctx context.Context) ([]*OrphanedExecution, error) {
	if !rm.config.Enabled {
		return []*OrphanedExecution{}, nil
	}

	// Build filter for RUNNING executions
	filter := &repository.ExecutionFilter{
		Status:       "RUNNING",
		StateMachineID: rm.config.StateMachineID,
	}

	executions, err := rm.repositoryManager.ListExecutions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}

	now := time.Now()
	var orphaned []*OrphanedExecution

	for _, exec := range executions {
		// Check if execution has been running longer than the threshold
		if exec.StartTime != nil {
			duration := now.Sub(*exec.StartTime)
			if duration > rm.config.OrphanedThreshold {
				orphaned = append(orphaned, &OrphanedExecution{
					ExecutionID:    exec.ExecutionID,
					StateMachineID: exec.StateMachineID,
					CurrentState:   exec.CurrentState,
					Status:         exec.Status,
					StartTime:      *exec.StartTime,
					LastUpdateTime: exec.UpdatedAt,
					Duration:       duration,
					RecoveryMetadata: exec.RecoveryMetadata,
				})
			}
		}
	}

	return orphaned, nil
}

// RecoverExecution attempts to recover a single orphaned execution
func (rm *RecoveryManager) RecoverExecution(ctx context.Context, orphaned *OrphanedExecution, recoveryFunc RecoveryFunc) error {
	// Get full execution record
	execRecord, err := rm.repositoryManager.GetExecution(ctx, orphaned.ExecutionID)
	if err != nil {
		return fmt.Errorf("failed to get execution: %w", err)
	}

	// Get state history to determine last successful state
	history, err := rm.repositoryManager.GetStateHistory(ctx, orphaned.ExecutionID)
	if err != nil {
		return fmt.Errorf("failed to get state history: %w", err)
	}

	// Create execution context from record
	execCtx := rm.reconstructExecution(execRecord, history)

	// Check if we can still recover
	if !execCtx.CanRecover() {
		log.Printf("Recovery: Execution %s has exceeded max recovery attempts (%d), marking as FAILED\n",
			orphaned.ExecutionID, execCtx.RecoveryMetadata.RecoveryAttemptCount)
		return rm.markExecutionFailed(ctx, execCtx, fmt.Errorf("max recovery attempts exceeded"))
	}

	// Mark recovery attempt
	execCtx.MarkRecoveryAttempt()

	// Apply recovery strategy
	switch rm.config.DefaultRecoveryStrategy {
	case StrategyRetry:
		return rm.recoverWithRetry(ctx, execCtx, recoveryFunc)
	case StrategySkip:
		return rm.recoverWithSkip(ctx, execCtx, recoveryFunc)
	case StrategyFail:
		return rm.markExecutionFailed(ctx, execCtx, fmt.Errorf("recovery strategy is FAIL"))
	case StrategyPause:
		return rm.markExecutionPaused(ctx, execCtx)
	default:
		return rm.recoverWithRetry(ctx, execCtx, recoveryFunc)
	}
}

// reconstructExecution rebuilds an Execution object from persisted records
func (rm *RecoveryManager) reconstructExecution(record *repository.ExecutionRecord, history []*repository.StateHistoryRecord) *execution.Execution {
	execCtx := &execution.Execution{
		StateMachineID:        record.StateMachineID,
		ID:                    record.ExecutionID,
		Name:                  record.Name,
		Status:                record.Status,
		StartTime:             *record.StartTime,
		Input:                 record.Input,
		Output:                record.Output,
		CurrentState:          record.CurrentState,
		HistorySequenceNumber: record.HistorySequenceNumber,
	}

	if record.EndTime != nil {
		execCtx.EndTime = *record.EndTime
	}

	if record.Error != "" {
		execCtx.Error = fmt.Errorf("%s", record.Error)
	}

	// Reconstruct state history
	for _, h := range history {
		stateHistory := execution.StateHistory{
			StateName:      h.StateName,
			StateType:      h.StateType,
			Status:         h.Status,
			Input:          h.Input,
			Output:         h.Output,
			StartTime:      *h.StartTime,
			RetryCount:     h.RetryCount,
			SequenceNumber: h.SequenceNumber,
		}
		if h.EndTime != nil {
			stateHistory.EndTime = *h.EndTime
		}
		if h.Error != "" {
			stateHistory.Error = fmt.Errorf("%s", h.Error)
		}
		execCtx.History = append(execCtx.History, stateHistory)
	}

	// Reconstruct recovery metadata
	if record.RecoveryMetadata != nil {
		execCtx.RecoveryMetadata = &execution.RecoveryMetadata{
			LastSuccessfulState:       record.RecoveryMetadata.LastSuccessfulState,
			LastSuccessfulStateOutput: record.RecoveryMetadata.LastSuccessfulStateOutput,
			RecoveryAttemptCount:      record.RecoveryMetadata.RecoveryAttemptCount,
			LastRecoveryAttemptAt:     record.RecoveryMetadata.LastRecoveryAttemptAt,
			MaxRecoveryAttempts:       record.RecoveryMetadata.MaxRecoveryAttempts,
			RecoveryStrategy:          record.RecoveryMetadata.RecoveryStrategy,
			CrashDetectedAt:           record.RecoveryMetadata.CrashDetectedAt,
		}
	} else {
		// Initialize with defaults
		execCtx.PrepareForRecovery(rm.config.DefaultMaxRecoveryAttempts, string(rm.config.DefaultRecoveryStrategy))
	}

	// Find last successful state from history
	lastSuccessfulState := rm.findLastSuccessfulState(history)
	if lastSuccessfulState != "" {
		execCtx.RecoveryMetadata.LastSuccessfulState = lastSuccessfulState
		// Get output of last successful state
		lastOutput := rm.getLastSuccessfulStateOutput(history, lastSuccessfulState)
		if lastOutput != nil {
			execCtx.RecoveryMetadata.LastSuccessfulStateOutput = lastOutput
		}
	}

	// Mark crash detected
	execCtx.MarkCrashDetected()

	return execCtx
}

// findLastSuccessfulState finds the last successfully completed state from history
func (rm *RecoveryManager) findLastSuccessfulState(history []*repository.StateHistoryRecord) string {
	var lastSuccessful string
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Status == "SUCCEEDED" {
			lastSuccessful = history[i].StateName
			break
		}
	}
	return lastSuccessful
}

// getLastSuccessfulStateOutput retrieves the output of the last successful state
func (rm *RecoveryManager) getLastSuccessfulStateOutput(history []*repository.StateHistoryRecord, stateName string) interface{} {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].StateName == stateName && history[i].Status == "SUCCEEDED" {
			return history[i].Output
		}
	}
	return nil
}

// recoverWithRetry retries from the last successful state
func (rm *RecoveryManager) recoverWithRetry(ctx context.Context, execCtx *execution.Execution, recoveryFunc RecoveryFunc) error {
	log.Printf("Recovery: Retrying execution %s from state %s (attempt %d)\n",
		execCtx.ID, execCtx.CurrentState, execCtx.RecoveryMetadata.RecoveryAttemptCount)

	// Use the output of the last successful state as input
	recoveryInput := execCtx.GetRecoveryInput()
	if recoveryInput != nil {
		execCtx.Input = recoveryInput
	}

	// Execute recovery
	result, err := recoveryFunc(ctx, execCtx)
	if err != nil {
		log.Printf("Recovery: Failed to recover execution %s: %v\n", execCtx.ID, err)
		return err
	}

	log.Printf("Recovery: Successfully recovered execution %s, status: %s\n", result.ID, result.Status)
	return nil
}

// recoverWithSkip attempts to skip the current state and move to the next
func (rm *RecoveryManager) recoverWithSkip(ctx context.Context, execCtx *execution.Execution, recoveryFunc RecoveryFunc) error {
	log.Printf("Recovery: Attempting to skip state %s in execution %s\n",
		execCtx.CurrentState, execCtx.ID)

	// For skip strategy, we attempt to continue from the current state
	// This is useful when the current state is known to be non-critical
	return rm.recoverWithRetry(ctx, execCtx, recoveryFunc)
}

// markExecutionFailed marks an execution as failed
func (rm *RecoveryManager) markExecutionFailed(ctx context.Context, execCtx *execution.Execution, err error) error {
	execCtx.Status = "FAILED"
	execCtx.Error = err
	execCtx.EndTime = time.Now()

	return rm.repositoryManager.SaveExecution(ctx, execCtx)
}

// markExecutionPaused marks an execution as paused for manual intervention
func (rm *RecoveryManager) markExecutionPaused(ctx context.Context, execCtx *execution.Execution) error {
	execCtx.Status = "PAUSED"
	execCtx.EndTime = time.Now()

	if err := rm.repositoryManager.SaveExecution(ctx, execCtx); err != nil {
		return fmt.Errorf("failed to save paused execution: %w", err)
	}

	log.Printf("Recovery: Execution %s marked as PAUSED for manual intervention\n", execCtx.ID)
	return nil
}

// StartBackgroundScanner starts a background goroutine that periodically scans for orphaned executions
func (rm *RecoveryManager) StartBackgroundScanner(recoveryFunc RecoveryFunc) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.isRunning {
		return fmt.Errorf("recovery scanner is already running")
	}

	if !rm.config.Enabled {
		return fmt.Errorf("recovery is disabled")
	}

	rm.isRunning = true
	rm.recoveryFunc = recoveryFunc

	go rm.backgroundScanner()

	log.Printf("Recovery: Started background scanner with interval %v\n", rm.config.ScanInterval)
	return nil
}

// StopBackgroundScanner stops the background scanner
func (rm *RecoveryManager) StopBackgroundScanner() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.isRunning {
		return nil
	}

	close(rm.stopChan)
	rm.isRunning = false

	log.Printf("Recovery: Stopped background scanner\n")
	return nil
}

// backgroundScanner is the background goroutine that scans for orphaned executions
func (rm *RecoveryManager) backgroundScanner() {
	ticker := time.NewTicker(rm.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.stopChan:
			return
		case <-ticker.C:
			rm.scanAndRecover()
		}
	}
}

// scanAndRecover performs a single scan and recovery cycle
func (rm *RecoveryManager) scanAndRecover() {
	ctx := context.Background()

	orphaned, err := rm.FindOrphanedExecutions(ctx)
	if err != nil {
		log.Printf("Recovery: Failed to find orphaned executions: %v\n", err)
		return
	}

	if len(orphaned) == 0 {
		return
	}

	log.Printf("Recovery: Found %d orphaned executions\n", len(orphaned))

	for _, orphan := range orphaned {
		if err := rm.RecoverExecution(ctx, orphan, rm.recoveryFunc); err != nil {
			log.Printf("Recovery: Failed to recover execution %s: %v\n", orphan.ExecutionID, err)
		}
	}
}

// IsRunning returns whether the background scanner is running
func (rm *RecoveryManager) IsRunning() bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.isRunning
}

// GetConfig returns the recovery configuration
func (rm *RecoveryManager) GetConfig() *RecoveryConfig {
	return rm.config
}

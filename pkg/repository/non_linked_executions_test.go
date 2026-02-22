//go:build integration
// +build integration

// pkg/repository/non_linked_executions_test.go
package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestListNonLinkedExecutions_Postgres tests the ListNonLinkedExecutions method with PostgreSQL
func TestListNonLinkedExecutions_Postgres(t *testing.T) {
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
	}

	config := Config{
		Strategy:      "postgres",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns": 10,
		},
	}

	repo, err := NewPostgresRepository(&config)
	require.NoError(t, err, "Failed to create PostgreSQL repository")
	defer repo.Close()

	err = repo.Initialize(context.Background())
	require.NoError(t, err, "Failed to initialize repository")

	ctx := context.Background()

	// Clean up before test
	_, _ = repo.db.ExecContext(ctx, "DELETE FROM linked_executions")
	_, _ = repo.db.ExecContext(ctx, "DELETE FROM state_history")
	_, _ = repo.db.ExecContext(ctx, "DELETE FROM executions")

	// Create test executions
	exec1 := createTestExecution("exec-1", "sm-1", "SUCCEEDED")
	exec2 := createTestExecution("exec-2", "sm-1", "SUCCEEDED")
	exec3 := createTestExecution("exec-3", "sm-1", "SUCCEEDED")

	if err := repo.SaveExecution(ctx, exec1); err != nil {
		t.Fatalf("Failed to save execution 1: %v", err)
	}
	if err := repo.SaveExecution(ctx, exec2); err != nil {
		t.Fatalf("Failed to save execution 2: %v", err)
	}
	if err := repo.SaveExecution(ctx, exec3); err != nil {
		t.Fatalf("Failed to save execution 3: %v", err)
	}

	// Create a linked execution (exec-1 -> exec-2)
	linkedExec := &LinkedExecutionRecord{
		ID:                     "link-1",
		SourceStateMachineID:   "sm-1",
		SourceExecutionID:      "exec-1",
		SourceStateName:        "ChainState",
		InputTransformerName:   "Transform1",
		TargetStateMachineName: "sm-2",
		TargetExecutionID:      "exec-2",
		CreatedAt:              time.Now(),
	}

	if err := repo.SaveLinkedExecution(ctx, linkedExec); err != nil {
		t.Fatalf("Failed to save linked execution: %v", err)
	}

	// Test: List non-linked executions
	filter := &LinkedExecutionFilter{}
	nonLinked, err := repo.ListNonLinkedExecutions(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list non-linked executions: %v", err)
	}

	// Should return exec-2 and exec-3 (not exec-1 since it's a source)
	if len(nonLinked) != 2 {
		t.Errorf("Expected 2 non-linked executions, got %d", len(nonLinked))
	}

	// Test with status filter: Find only SUCCEEDED executions without links
	filterWithStatus := &LinkedExecutionFilter{
		SourceExecutionStatus: "SUCCEEDED",
	}
	nonLinkedSuccess, err := repo.ListNonLinkedExecutions(ctx, filterWithStatus)
	if err != nil {
		t.Fatalf("Failed to list non-linked SUCCEEDED executions: %v", err)
	}
	if len(nonLinkedSuccess) != 2 {
		t.Errorf("Expected 2 non-linked SUCCEEDED executions, got %d", len(nonLinkedSuccess))
	}

	// Verify exec-1 is not in the list
	for _, exec := range nonLinked {
		if exec.ExecutionID == "exec-1" {
			t.Errorf("exec-1 should not be in non-linked executions list")
		}
	}

	// Verify exec-2 and exec-3 are in the list
	foundExec2 := false
	foundExec3 := false
	for _, exec := range nonLinked {
		if exec.ExecutionID == "exec-2" {
			foundExec2 = true
		}
		if exec.ExecutionID == "exec-3" {
			foundExec3 = true
		}
	}

	if !foundExec2 {
		t.Errorf("exec-2 should be in non-linked executions list")
	}
	if !foundExec3 {
		t.Errorf("exec-3 should be in non-linked executions list")
	}
}

// TestListNonLinkedExecutions_GormPostgres tests the ListNonLinkedExecutions method with GORM
func TestListNonLinkedExecutions_GormPostgres(t *testing.T) {
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable"
	}

	config := Config{
		Strategy:      "gorm",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns": 10,
		},
	}

	repo, err := NewGormPostgresRepository(&config)
	require.NoError(t, err, "Failed to create GORM repository")
	defer repo.Close()

	err = repo.Initialize(context.Background())
	require.NoError(t, err, "Failed to initialize repository")

	ctx := context.Background()

	// Clean up before test
	repo.db.Exec("DELETE FROM linked_executions")
	repo.db.Exec("DELETE FROM state_history")
	repo.db.Exec("DELETE FROM executions")

	// Create test executions
	exec1 := createTestExecution("exec-gorm-1", "sm-gorm-1", "SUCCEEDED")
	exec2 := createTestExecution("exec-gorm-2", "sm-gorm-1", "SUCCEEDED")
	exec3 := createTestExecution("exec-gorm-3", "sm-gorm-1", "SUCCEEDED")

	if err := repo.SaveExecution(ctx, exec1); err != nil {
		t.Fatalf("Failed to save execution 1: %v", err)
	}
	if err := repo.SaveExecution(ctx, exec2); err != nil {
		t.Fatalf("Failed to save execution 2: %v", err)
	}
	if err := repo.SaveExecution(ctx, exec3); err != nil {
		t.Fatalf("Failed to save execution 3: %v", err)
	}

	// Create a linked execution (exec-gorm-1 -> exec-gorm-2)
	linkedExec := &LinkedExecutionRecord{
		ID:                     "link-gorm-1",
		SourceStateMachineID:   "sm-gorm-1",
		SourceExecutionID:      "exec-gorm-1",
		SourceStateName:        "ChainState",
		InputTransformerName:   "Transform1",
		TargetStateMachineName: "sm-gorm-2",
		TargetExecutionID:      "exec-gorm-2",
		CreatedAt:              time.Now(),
	}

	if err := repo.SaveLinkedExecution(ctx, linkedExec); err != nil {
		t.Fatalf("Failed to save linked execution: %v", err)
	}

	// Test: List non-linked executions
	filter := &LinkedExecutionFilter{}
	nonLinked, err := repo.ListNonLinkedExecutions(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list non-linked executions: %v", err)
	}

	// Should return exec-gorm-2 and exec-gorm-3 (not exec-gorm-1 since it's a source)
	if len(nonLinked) != 2 {
		t.Errorf("Expected 2 non-linked executions, got %d", len(nonLinked))
	}

	// Test with transformer filter: Find executions without links using Transform1
	filterWithTransformer := &LinkedExecutionFilter{
		InputTransformerName: "Transform1",
	}
	nonLinkedTransformer, err := repo.ListNonLinkedExecutions(ctx, filterWithTransformer)
	if err != nil {
		t.Fatalf("Failed to list non-linked executions with transformer filter: %v", err)
	}
	// Should return all 3 executions since only exec-gorm-1 has a link with Transform1
	if len(nonLinkedTransformer) != 2 {
		t.Errorf("Expected 2 non-linked executions with Transform1 filter, got %d", len(nonLinkedTransformer))
	}

	// Verify exec-gorm-1 is not in the list
	for _, exec := range nonLinked {
		if exec.ExecutionID == "exec-gorm-1" {
			t.Errorf("exec-gorm-1 should not be in non-linked executions list")
		}
	}

	// Verify exec-gorm-2 and exec-gorm-3 are in the list
	foundExec2 := false
	foundExec3 := false
	for _, exec := range nonLinked {
		if exec.ExecutionID == "exec-gorm-2" {
			foundExec2 = true
		}
		if exec.ExecutionID == "exec-gorm-3" {
			foundExec3 = true
		}
	}

	if !foundExec2 {
		t.Errorf("exec-gorm-2 should be in non-linked executions list")
	}
	if !foundExec3 {
		t.Errorf("exec-gorm-3 should be in non-linked executions list")
	}
}

// Helper function to create test execution
func createTestExecution(execID, smID, status string) *ExecutionRecord {
	now := time.Now()
	return &ExecutionRecord{
		ExecutionID:    execID,
		StateMachineID: smID,
		Name:           "test-execution",
		Input:          map[string]interface{}{"test": "data"},
		Status:         status,
		StartTime:      &now,
		CurrentState:   "TestState",
		Metadata:       map[string]interface{}{},
	}
}

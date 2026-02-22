//go:build integration
// +build integration

package persistent

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
	statemachine2 "github.com/hussainpithawala/state-machine-amz-go/pkg/statemachine"
)

// TestLinkedExecutionCreation tests that linked execution records are created automatically
func TestLinkedExecutionCreation(t *testing.T) {
	ctx := context.Background()

	// Setup test database
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_linked_test?sslmode=disable"
	}

	config := &repository.Config{
		Strategy:      "gorm-postgres",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns": 5,
			"log_level":      "warn",
		},
	}

	manager, err := repository.NewPersistenceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Initialize schema
	err = manager.Initialize(ctx)
	require.NoError(t, err)

	// Clean up before test
	cleanupLinkedExecutionTest(t, manager)

	// Create source state machine
	sourceDefinition := []byte(`{
		"Comment": "Source state machine",
		"StartAt": "ProcessOrder",
		"States": {
			"ProcessOrder": {
				"Type": "Pass",
				"Result": {"orderId": "12345", "total": 100.50},
				"End": true
			}
		}
	}`)

	sourceSM, err := New(sourceDefinition, true, "sm-source-test", manager)
	require.NoError(t, err)

	// Save source state machine definition
	err = sourceSM.SaveDefinition(ctx)
	require.NoError(t, err)

	// Execute source state machine
	sourceExec, err := sourceSM.Execute(ctx, map[string]interface{}{"input": "data"})
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", sourceExec.Status)

	// Create target state machine
	targetDefinition := []byte(`{
		"Comment": "Target state machine",
		"StartAt": "ReceiveOrder",
		"States": {
			"ReceiveOrder": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	targetSM, err := New(targetDefinition, true, "sm-target-test", manager)
	require.NoError(t, err)

	// Save target state machine definition
	err = targetSM.SaveDefinition(ctx)
	require.NoError(t, err)

	// Execute target state machine with source execution ID
	targetExec, err := targetSM.Execute(ctx, nil,
		statemachine2.WithExecutionName("chained-execution"),
		statemachine2.WithSourceExecution(sourceExec.ID, "ProcessOrder"),
	)
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", targetExec.Status)

	// Verify LinkedExecution record was created
	time.Sleep(100 * time.Millisecond) // Give time for async operations

	// Query the linked_executions table directly
	repo := manager.GetRepository()
	if gormRepo, ok := repo.(*repository.GormPostgresRepository); ok {
		var linkedExecs []repository.LinkedExecutionModel
		err = gormRepo.GetDB().WithContext(ctx).
			Where("source_execution_id = ? AND target_execution_id = ?",
				sourceExec.ID, targetExec.ID).
			Find(&linkedExecs).Error
		require.NoError(t, err)
		require.Len(t, linkedExecs, 1, "Should have exactly one linked execution record")

		linkedExec := linkedExecs[0]
		assert.Equal(t, "sm-source-test", linkedExec.SourceStateMachineID)
		assert.Equal(t, sourceExec.ID, linkedExec.SourceExecutionID)
		assert.Equal(t, "ProcessOrder", linkedExec.SourceStateName)
		assert.Equal(t, "Target state machine", linkedExec.TargetStateMachineName)
		assert.Equal(t, targetExec.ID, linkedExec.TargetExecutionID)
	}
}

// TestLinkedExecutionWithTransformer tests linked execution with input transformer
func TestLinkedExecutionWithTransformer(t *testing.T) {
	ctx := context.Background()

	// Setup test database
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_linked_test?sslmode=disable"
	}

	config := &repository.Config{
		Strategy:      "gorm-postgres",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns": 5,
			"log_level":      "warn",
		},
	}

	manager, err := repository.NewPersistenceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Initialize schema
	err = manager.Initialize(ctx)
	require.NoError(t, err)

	// Clean up before test
	cleanupLinkedExecutionTest(t, manager)

	// Create source state machine
	sourceDefinition := []byte(`{
		"Comment": "Source state machine",
		"StartAt": "GenerateData",
		"States": {
			"GenerateData": {
				"Type": "Pass",
				"Result": {"value": 42},
				"End": true
			}
		}
	}`)

	sourceSM, err := New(sourceDefinition, true, "sm-source-transform", manager)
	require.NoError(t, err)

	// Execute source
	sourceExec, err := sourceSM.Execute(ctx, nil)
	require.NoError(t, err)

	// Create target state machine
	targetDefinition := []byte(`{
		"Comment": "Target state machine",
		"StartAt": "ProcessData",
		"States": {
			"ProcessData": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	targetSM, err := New(targetDefinition, true, "sm-target-transform", manager)
	require.NoError(t, err)

	// Execute with transformer
	transformer := func(input interface{}) (interface{}, error) {
		if inputMap, ok := input.(map[string]interface{}); ok {
			return map[string]interface{}{
				"transformed": inputMap["value"],
			}, nil
		}
		return input, nil
	}

	targetExec, err := targetSM.Execute(ctx, nil,
		statemachine2.WithSourceExecution(sourceExec.ID),
		statemachine2.WithInputTransformer(transformer),
	)
	require.NoError(t, err)
	require.Equal(t, "SUCCEEDED", targetExec.Status)

	// Verify LinkedExecution record has transformer info
	time.Sleep(100 * time.Millisecond)

	repo := manager.GetRepository()
	if gormRepo, ok := repo.(*repository.GormPostgresRepository); ok {
		var linkedExecs []repository.LinkedExecutionModel
		err = gormRepo.GetDB().WithContext(ctx).
			Where("target_execution_id = ?", targetExec.ID).
			Find(&linkedExecs).Error
		require.NoError(t, err)
		require.Len(t, linkedExecs, 1)

		linkedExec := linkedExecs[0]
		assert.Equal(t, "custom_transformer", linkedExec.InputTransformerName)
	}
}

// TestLinkedExecutionMultipleChains tests multiple executions chained from one source
func TestLinkedExecutionMultipleChains(t *testing.T) {
	ctx := context.Background()

	// Setup test database
	connURL := os.Getenv("POSTGRES_TEST_URL")
	if connURL == "" {
		connURL = "postgres://postgres:postgres@localhost:5432/statemachine_linked_test?sslmode=disable"
	}

	config := &repository.Config{
		Strategy:      "gorm-postgres",
		ConnectionURL: connURL,
		Options: map[string]interface{}{
			"max_open_conns": 5,
			"log_level":      "warn",
		},
	}

	manager, err := repository.NewPersistenceManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// Initialize schema
	err = manager.Initialize(ctx)
	require.NoError(t, err)

	// Clean up before test
	cleanupLinkedExecutionTest(t, manager)

	// Create source state machine
	sourceDefinition := []byte(`{
		"Comment": "Source state machine",
		"StartAt": "ProduceData",
		"States": {
			"ProduceData": {
				"Type": "Pass",
				"Result": {"data": "shared"},
				"End": true
			}
		}
	}`)

	sourceSM, err := New(sourceDefinition, true, "sm-source-multi", manager)
	require.NoError(t, err)

	// Execute source once
	sourceExec, err := sourceSM.Execute(ctx, nil)
	require.NoError(t, err)

	// Create target state machine
	targetDefinition := []byte(`{
		"Comment": "Target state machine",
		"StartAt": "ConsumeData",
		"States": {
			"ConsumeData": {
				"Type": "Pass",
				"End": true
			}
		}
	}`)

	targetSM, err := New(targetDefinition, true, "sm-target-multi", manager)
	require.NoError(t, err)

	// Execute target multiple times from the same source
	targetExecIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		targetExec, err := targetSM.Execute(ctx, nil,
			statemachine2.WithExecutionName(fmt.Sprintf("chained-%d", i)),
			statemachine2.WithSourceExecution(sourceExec.ID),
		)
		require.NoError(t, err)
		targetExecIDs[i] = targetExec.ID
	}

	// Verify multiple LinkedExecution records exist
	time.Sleep(100 * time.Millisecond)

	repo := manager.GetRepository()
	if gormRepo, ok := repo.(*repository.GormPostgresRepository); ok {
		var linkedExecs []repository.LinkedExecutionModel
		err = gormRepo.GetDB().WithContext(ctx).
			Where("source_execution_id = ?", sourceExec.ID).
			Find(&linkedExecs).Error
		require.NoError(t, err)
		require.Len(t, linkedExecs, 3, "Should have 3 linked execution records from same source")

		// Verify all have same source but different targets
		for i, linkedExec := range linkedExecs {
			assert.Equal(t, sourceExec.ID, linkedExec.SourceExecutionID)
			assert.Equal(t, targetExecIDs[i], linkedExec.TargetExecutionID)
		}
	}
}

// cleanupLinkedExecutionTest removes test data
func cleanupLinkedExecutionTest(t *testing.T, manager *repository.Manager) {
	repo := manager.GetRepository()
	if gormRepo, ok := repo.(*repository.GormPostgresRepository); ok {
		ctx := context.Background()
		_ = gormRepo.GetDB().WithContext(ctx).Exec("TRUNCATE TABLE linked_executions CASCADE").Error
		_ = gormRepo.GetDB().WithContext(ctx).Exec("TRUNCATE TABLE state_history CASCADE").Error
		_ = gormRepo.GetDB().WithContext(ctx).Exec("TRUNCATE TABLE executions CASCADE").Error
		_ = gormRepo.GetDB().WithContext(ctx).Exec("TRUNCATE TABLE state_machines CASCADE").Error
	}
}

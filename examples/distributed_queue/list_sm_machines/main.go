package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hussainpithawala/state-machine-amz-go/pkg/repository"
)

// Example demonstrating the ListStateMachines method
func main() {
	ctx := context.Background()

	// Setup repository
	repoManager, err := setupRepository(ctx)
	if err != nil {
		log.Fatalf("Failed to setup repository: %v", err)
	}
	defer repoManager.Close()

	fmt.Println("=== ListStateMachines Examples ===\n")
	// Example 1: List all state machines
	fmt.Println("1. List all state machines:")
	allStateMachines, err := repoManager.ListStateMachines(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to list state machines: %v", err)
	}
	printStateMachines(allStateMachines)

	// Example 2: Filter by name (partial match)
	fmt.Println("\n2. Filter by name (search for 'order'):")
	nameFilter := &repository.DefinitionFilter{
		Name: "order",
	}
	filteredByName, err := repoManager.ListStateMachines(ctx, nameFilter)
	if err != nil {
		log.Fatalf("Failed to list state machines: %v", err)
	}
	printStateMachines(filteredByName)

	// Example 3: Filter by specific ID
	if len(allStateMachines) > 0 {
		fmt.Printf("\n3. Filter by specific ID (%s):\n", allStateMachines[0].ID)
		idFilter := &repository.DefinitionFilter{
			StateMachineID: allStateMachines[0].ID,
		}
		filteredByID, err := repoManager.ListStateMachines(ctx, idFilter)
		if err != nil {
			log.Fatalf("Failed to list state machines: %v", err)
		}
		printStateMachines(filteredByID)
	}

	fmt.Println("\n=== Examples completed ===")
}

func printStateMachines(stateMachines []*repository.StateMachineRecord) {
	if len(stateMachines) == 0 {
		fmt.Println("  No state machines found")
		return
	}

	fmt.Printf("  Found %d state machine(s):\n", len(stateMachines))
	for i, sm := range stateMachines {
		fmt.Printf("\n  [%d] ID: %s\n", i+1, sm.ID)
		fmt.Printf("      Name: %s\n", sm.Name)
		fmt.Printf("      Description: %s\n", sm.Description)
		fmt.Printf("      Type: %s\n", sm.Type)
		fmt.Printf("      Version: %s\n", sm.Version)
		fmt.Printf("      Created: %s\n", sm.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("      Updated: %s\n", sm.UpdatedAt.Format("2006-01-02 15:04:05"))

		if sm.Metadata != nil && len(sm.Metadata) > 0 {
			metadataJSON, _ := json.MarshalIndent(sm.Metadata, "      ", "  ")
			fmt.Printf("      Metadata: %s\n", string(metadataJSON))
		}
	}
}

func setupRepository(ctx context.Context) (*repository.Manager, error) {
	// Configure persistence with GORM PostgreSQL
	config := &repository.Config{
		Strategy:      "gorm-postgres",
		ConnectionURL: "postgres://postgres:postgres@localhost:5432/statemachine_test?sslmode=disable",
		Options: map[string]interface{}{
			"max_open_conns": 25,
			"max_idle_conns": 5,
		},
	}

	// Create persistence manager
	manager, err := repository.NewPersistenceManager(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistence manager: %w", err)
	}

	// Initialize schema
	if err := manager.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize persistence: %w", err)
	}

	return manager, nil
}

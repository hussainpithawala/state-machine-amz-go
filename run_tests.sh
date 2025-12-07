#!/bin/bash
# File: run_tests.sh

set -e

echo "Running Pass State tests..."
go test -v ./internal/states -run "TestPassState"

echo ""
echo "Running all state tests..."
go test -v ./internal/states

echo ""
echo "Running validator tests..."
go test -v ./internal/validator

echo ""
echo "Running executor tests..."
go test -v ./pkg/executor

echo ""
echo "Running statemachine tests..."
go test -v ./pkg/statemachine

echo ""
echo "All tests completed successfully!"
#!/bin/bash

echo "Cleaning up..."
go clean

echo "Downloading dependencies..."
go mod download

echo "Tidying modules..."
go mod tidy

echo "Building..."
go build ./...

if [ $? -eq 0 ]; then
    echo "✅ Build successful!"

    echo "Building examples..."
    go build -o bin/sm-cli ./cmd/sm-cli
    go build -o bin/sm-test ./cmd/sm-test

    echo "✅ Examples built!"
else
    echo "❌ Build failed!"
    exit 1
fi
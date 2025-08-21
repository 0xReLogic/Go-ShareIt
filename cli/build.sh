#!/bin/bash
echo "Building Share-it CLI..."

# Create releases directory if it doesn't exist
mkdir -p releases

# Build for Windows (amd64)
echo "Building for Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -o releases/share-it-cli-windows-amd64.exe main.go

# Build for Linux (amd64)
echo "Building for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -o releases/share-it-cli-linux-amd64 main.go

# Build for macOS (amd64)
echo "Building for macOS (amd64)..."
GOOS=darwin GOARCH=amd64 go build -o releases/share-it-cli-darwin-amd64 main.go

echo "Build complete! Binaries are in the 'releases' directory."
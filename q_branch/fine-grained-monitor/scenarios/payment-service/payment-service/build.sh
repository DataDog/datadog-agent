#!/bin/bash
set -e

echo "Building rapid-http component..."

# Ensure GOPATH/bin is in PATH for go-installed binaries
export PATH="${PATH}:$(go env GOPATH)/bin"

# Generate Swagger documentation first so docs package exists for tidy
echo "Generating Swagger documentation..."
if ! command -v swag &> /dev/null; then
    echo "Installing swag..."
    go install github.com/swaggo/swag/cmd/swag@latest
fi
swag init -g main.go --output ./docs

# Pre-build: Run go mod tidy to ensure dependencies are correct
echo "Running go mod tidy..."
go mod tidy

# Format Go code
echo "Formatting Go code..."
go fmt ./...
# Use goimports for import organization if available
if command -v goimports &> /dev/null; then
    goimports -w . 2>/dev/null || true
fi

# Build Docker image
echo "Building Docker image..."
docker build -t test-rapid-http .

echo "✓ Build successful"






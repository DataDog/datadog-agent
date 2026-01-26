#!/bin/bash
# Smoke test for rapid-http component
# This runs BEFORE Docker to catch compilation errors early
set -e

echo "🔥 Running smoke tests for rapid-http..."

# Check if we have Go installed
if ! command -v go &> /dev/null; then
    echo "⚠️  Go not installed - skipping local compilation check"
    exit 0
fi

# Ensure GOPATH/bin is in PATH for go-installed binaries
export PATH="${PATH}:$(go env GOPATH)/bin"

# Check if main.go exists
if [ ! -f "main.go" ]; then
    echo "❌ main.go not found"
    exit 1
fi

# Check if go.mod exists
if [ ! -f "go.mod" ]; then
    echo "❌ go.mod not found"
    exit 1
fi

# Generate swagger docs if swag is available (needed for imports)
if command -v swag &> /dev/null; then
    echo "   Generating Swagger documentation..."
    swag init -g main.go --output ./docs 2>/dev/null || true
elif [ -d "docs" ]; then
    echo "   Swagger docs directory exists, skipping generation"
fi

# Run go mod tidy to check dependencies
echo "   Checking Go dependencies..."
go mod tidy 2>&1 || {
    echo "❌ go mod tidy failed"
    exit 1
}

# Try to compile (but don't build the full binary)
echo "   Checking compilation..."
go build -o /dev/null . 2>&1 || {
    echo "❌ Go compilation failed"
    exit 1
}

# Check for syntax errors in shell scripts
for script in build.sh test.sh; do
    if [ -f "$script" ]; then
        echo "   Checking $script syntax..."
        bash -n "$script" 2>&1 || {
            echo "❌ Syntax error in $script"
            exit 1
        }
    fi
done

echo "✓ Smoke tests passed for rapid-http"

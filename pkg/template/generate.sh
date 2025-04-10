#!/bin/bash

set -e

# This script generates the code for the template package.
# It takes the code from the Go standard library and applies the patches.

if ! command -v gopatch &> /dev/null; then
    echo "gopatch could not be found, installing it..."
    go install github.com/uber-go/gopatch@latest
fi

echo "Generating code for Go version $(go version)"

GOROOT=$(go env GOROOT)
if [ -z "$GOROOT" ]; then
    echo "Could not find Go's source code path"
    exit 1
fi

# Remove the previous code
rm -rf text html internal

# Copy the code from the Go standard library
cp -r "$GOROOT/src/text/template" text
cp -r "$GOROOT/src/html/template" html
mkdir internal
cp -r "$GOROOT/src/internal/fmtsort" internal/fmtsort

echo "Removing test files..."
# remove all test files as they don't pass, and some use more dependencies
find . -name "*_test.go" -delete
rm -rf ./*/testdata

echo "Applying patches..."
# remove the piece of code executing methods (which disables dead code elimination)
git apply no-method.patch
# change the imports to use the local package
gopatch -p imports.patch ./...
# remove a godebug variable
gopatch -p godebug.patch ./...

#!/bin/bash

echo "Building Go functions for $ARCHITECTURE architecture"
go_test_dirs=("metric" "log" "timeout" "trace" "appsec")
cd src
for go_dir in "${go_test_dirs[@]}"; do
    env GOOS=linux CGO_ENABLED=0 GOARCH=$ARCHITECTURE go build -ldflags="-s -w" -o bin/"$go_dir" go-tests/"$go_dir"/main.go
done

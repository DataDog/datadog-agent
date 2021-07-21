#!/bin/bash -e

output_file="cpu_plugin.so"

echo "Compiling plugin..."
go build -o "$output_file" -buildmode=plugin plugin.go

echo "Artifact: $output_file"
echo "Place this file in '<CURRENT_DIR>/go-native-plugins' directory before starting the agent"
echo "Compiling plugin: OK"

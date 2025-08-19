#!/bin/bash
### This script is used to run go test and scrub the output, the command can be used as follow:
### ./gotest-scrubbed.sh <packages comma separated> -- <go tests flags>
### Use GOTEST_COMMAND environment variable to override the default "go test -json" command
set -euo pipefail

# Use custom command if provided, otherwise default to "go test -json"
GOTEST_CMD="${GOTEST_COMMAND:-go test -json}"

$GOTEST_CMD "$1" "${@:3}" | 
sed -E 's/\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b/**************************\1/g' | # Scrub API keys
sed -E 's/\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b/************************************\1/g' # Scrub APP keys

#!/usr/bin/env bash
### This script is used to run go test and scrub the output, the command can be used as follow:
### ./gotest-scrubbed.sh <packages comma separated> -- <go tests flags>
### Use GOTEST_COMMAND environment variable to override the default "go test -json <packages>" command
set -euo pipefail

# Use custom command if provided, otherwise default to "go test -json <packages>"
GOTEST_CMD="${GOTEST_COMMAND:-go test -json $1}"

# Scrub API \ APP keys
$GOTEST_CMD "${@:3}" | sed --unbuffered -E \
  -e 's/\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b/**************************\1/g' \
  -e 's/\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b/************************************\1/g'

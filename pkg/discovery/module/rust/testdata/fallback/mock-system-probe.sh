#!/bin/bash
# Mock system-probe that echoes arguments to a marker file
# Usage: mock-system-probe.sh <marker_file> [args...]

if [ -z "$1" ]; then
  echo "Usage: $0 <marker_file> [args...]" >&2
  exit 1
fi

marker_file="$1"
shift

echo "system-probe called with args: " "$@" > "$marker_file"
exit 0

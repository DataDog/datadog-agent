#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Shared library - shellcheck can't follow dynamic paths but the file exists
# shellcheck disable=SC1091
source "$SCRIPT_DIR/../../lib/run-in-container.sh"

echo "Running tests..."
run_inv_in_container test "$@"
echo "Tests completed successfully"

#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Shared library - shellcheck can't follow dynamic paths but the file exists
# shellcheck disable=SC1091
source "$SCRIPT_DIR/../../lib/run-in-container.sh"

echo "Building Datadog Agent..."
run_inv_in_container agent.build "$@"
echo "Build completed successfully"

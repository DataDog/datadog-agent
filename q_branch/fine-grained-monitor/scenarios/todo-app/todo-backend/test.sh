#!/bin/sh
set -e

COMPONENT_NAME="${COMPONENT_NAME:-todo-backend}"

is_inside_container() {
    [ -f /.dockerenv ] || grep -q docker /proc/1/cgroup 2>/dev/null || grep -q kubepods /proc/1/cgroup 2>/dev/null
}

run_tests() {
    echo "Testing rapid-http component..."

    PORT="${PORT:-8081}"
    HOST="${HOST:-localhost}"

    echo "• Checking health endpoint..."
    curl -sf "http://${HOST}:${PORT}/health" | head -c 200 >/dev/null

    echo "• Listing swagger availability..."
    curl -sf "http://${HOST}:${PORT}/swagger/doc.json" >/dev/null
	
    # !!!! LLM NOTE: ADD SMOKE TESTS FOR THE RAPID-HTTP COMPONENT, AND REMOVE THIS NOTE.
    #
    # For now, just validate the build succeeds
    # In the future, this can be extended to:
    # - Run health checks
    # - Test API endpoints
    # - Run integration tests

    echo "✓ TESTS PASSED"
}

if is_inside_container; then
    run_tests
else
    echo "Running tests inside container '${COMPONENT_NAME}'..."
    SCRIPT_CONTENT=$(cat "$0")
    docker compose exec -T "${COMPONENT_NAME}" sh -c "$SCRIPT_CONTENT"
fi

#!/bin/sh
# Test script for load-generator component
# Validates that the load generator is properly testing application endpoints

set -e

COMPONENT_NAME="${COMPONENT_NAME:-load-generator}"

is_inside_container() {
    [ "${RUNNING_IN_CONTAINER:-}" = "true" ]
}

run_tests() {
    SCRIPT_DIR="${SCRIPT_DIR:-$(cd "$(dirname "${0}")" >/dev/null 2>&1 && pwd)}"

    BLUE='\033[0;34m'
    RED='\033[0;31m'
    NC='\033[0m'

    echo "🧪 Testing load-generator component"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    if ! command -v go >/dev/null 2>&1; then
        printf "%b\n" "${RED}❌ Go is not installed or not in PATH${NC}"
        echo "   Go is expected to be available in the container for tests"
        exit 1
    fi

    export STATS_TIMEOUT="${STATS_TIMEOUT:-2m}"
    export MIN_REQUESTS="${MIN_REQUESTS:-2}"
    export MIN_ENDPOINTS="${MIN_ENDPOINTS:-1}"

    LOAD_GEN_PORT="${LOAD_GEN_PORT:-8089}"
    export LOAD_GEN_PORT

    printf "%b\n" "${BLUE}Load generator port: ${LOAD_GEN_PORT}${NC}"
    echo ""
    printf "%b\n" "${BLUE}Running stats validation test...${NC}"
    echo ""

    if [ ! -d "$SCRIPT_DIR/stats-test" ]; then
        printf "%b\n" "${RED}❌ stats-test directory not found inside container (${SCRIPT_DIR}/stats-test)${NC}"
        exit 1
    fi

    (cd "$SCRIPT_DIR/stats-test" && go run .)
}

if is_inside_container; then
    # When running inside container, set SCRIPT_DIR to /app where files are copied
    SCRIPT_DIR="${SCRIPT_DIR:-/app}"
    run_tests
else
    echo "Running tests inside container '${COMPONENT_NAME}'..."
    # Execute the script that is baked into the image. This avoids inlining the
    # script body (which can cause host-side variable expansion and break paths).
    docker compose exec -T "${COMPONENT_NAME}" sh -c "SCRIPT_DIR=/app /app/test.sh"
fi
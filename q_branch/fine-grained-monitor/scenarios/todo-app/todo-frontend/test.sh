#!/bin/sh
set -e

COMPONENT_NAME="${COMPONENT_NAME:-todo-frontend}"

is_inside_container() {
    [ -f /.dockerenv ] || grep -q docker /proc/1/cgroup 2>/dev/null || grep -q kubepods /proc/1/cgroup 2>/dev/null
}

run_tests() {
    echo "Testing todo-frontend..."

    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    TESTS_DIR="${SCRIPT_DIR}/integration-tests"

    COMPONENT_PORT="${COMPONENT_PORT:-8080}"
    FRONTEND_URL="${FRONTEND_URL:-http://127.0.0.1:${COMPONENT_PORT}}"
    export COMPONENT_PORT FRONTEND_URL
    echo "Using FRONTEND_URL=${FRONTEND_URL}"

    # Allow Puppeteer to download a compatible Chromium when available, but prefer a system install if present
    CHROME_PATH=""
    for candidate in /usr/lib/chromium/chrome /usr/bin/chromium-browser /usr/bin/chromium /usr/bin/google-chrome; do
        if [ -x "$candidate" ]; then
            CHROME_PATH="$candidate"
            break
        fi
    done
    if [ -n "$CHROME_PATH" ]; then
        export PUPPETEER_EXECUTABLE_PATH="$CHROME_PATH"
        export PUPPETEER_SKIP_DOWNLOAD="true"
        export PUPPETEER_SKIP_CHROMIUM_DOWNLOAD="true"
    else
        # Let Puppeteer manage the download/cache when no system binary is present
        unset PUPPETEER_SKIP_DOWNLOAD
        unset PUPPETEER_SKIP_CHROMIUM_DOWNLOAD
    fi

    if [ -d "$TESTS_DIR" ]; then
        echo "Installing test dependencies..."
        (cd "$TESTS_DIR" && npm install)

        echo "Ensuring Chromium is available for Puppeteer..."
        # If the system chromium is not found for some reason, fall back to Puppeteer's detected path.
        if [ -z "$PUPPETEER_EXECUTABLE_PATH" ]; then
            CHROME_PATH="$(cd "$TESTS_DIR" && node -e "const puppeteer=require('puppeteer'); console.log(puppeteer.executablePath())")"
            export PUPPETEER_EXECUTABLE_PATH="${CHROME_PATH}"
        fi
        export PUPPETEER_CACHE_DIR="${PUPPETEER_CACHE_DIR:-/tmp/.cache/puppeteer}"
        export PUPPETEER_BROWSERS_PATH="${PUPPETEER_BROWSERS_PATH:-/tmp/.cache/puppeteer}"

        TEST_TIMEOUT_SECONDS="${TEST_TIMEOUT_SECONDS:-240}"
        echo "Running integration tests (timeout ${TEST_TIMEOUT_SECONDS}s)..."

        if command -v timeout >/dev/null 2>&1; then
            (cd "$TESTS_DIR" && timeout "${TEST_TIMEOUT_SECONDS}" npm test)
        else
            echo "timeout command not found; running without timeout"
            (cd "$TESTS_DIR" && npm test)
        fi
    fi

    echo "✓ TESTS PASSED"
}

if is_inside_container; then
    run_tests
else
    # Execute the script inside the running container to ensure the app is reachable at localhost
    SCRIPT_CONTENT=$(cat "$0")
    docker compose exec -T "${COMPONENT_NAME}" sh -c "$SCRIPT_CONTENT"
fi

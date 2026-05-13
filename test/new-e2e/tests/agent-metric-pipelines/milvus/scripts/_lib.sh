#!/usr/bin/env bash
# Shared helpers for the Milvus scenario scripts.
#
# - Locates the repo root and the milvus test directory (works from any cwd).
# - Authenticates to Datadog via dd-auth so DD_API_KEY (and DD_APP_KEY) are
#   in the environment, then maps them to the E2E_* names the framework
#   expects.
# - Computes the stack name used by the test so up/down/inspect agree.
#
# Source this file; don't run it directly.

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)
MILVUS_DIR=$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)
REPO_ROOT=$(cd -- "${MILVUS_DIR}/../../../../.." &>/dev/null && pwd)

# Stack name: stable per checkout so up/down/check operate on the same env.
# Override with MILVUS_STACK_NAME for ad-hoc runs.
STACK_NAME=${MILVUS_STACK_NAME:-milvus-dev}

# testID embedded in AD labels / DD_TAGS / agent host tags. Override with
# MILVUS_E2E_TEST_ID to keep it stable across redeploys.
TEST_ID=${MILVUS_E2E_TEST_ID:-${STACK_NAME//[^a-zA-Z0-9_-]/_}}

log()  { printf '\033[1;34m[milvus]\033[0m %s\n' "$*" >&2; }
fail() { printf '\033[1;31m[milvus]\033[0m %s\n' "$*" >&2; exit 1; }

# Authenticate with dd-auth and surface the resulting key under the name
# the e2e-framework runner reads (E2E_API_KEY). Idempotent — safe to call
# from every script.
ensure_dd_auth() {
    if ! command -v dd-auth >/dev/null 2>&1; then
        fail "dd-auth not found in PATH; install it or set DD_API_KEY manually"
    fi
    log "running dd-auth..."
    # dd-auth typically exports DD_API_KEY (and sometimes DD_APP_KEY) into the
    # current shell. We `eval` its output if it prints `export ...` lines, and
    # fall back to just running it (relying on side effects in the calling
    # shell or env file).
    local out
    out=$(dd-auth 2>&1) || fail "dd-auth failed: ${out}"
    if grep -qE '^export ' <<<"${out}"; then
        # shellcheck disable=SC2046
        eval "$(grep -E '^export ' <<<"${out}")"
    fi
    [[ -n "${DD_API_KEY:-}" ]] || fail "dd-auth ran but DD_API_KEY is not set"

    export E2E_API_KEY="${DD_API_KEY}"
    if [[ -n "${DD_APP_KEY:-}" ]]; then
        export E2E_APP_KEY="${DD_APP_KEY}"
    fi
    log "DD auth ok (E2E_API_KEY set, length=${#E2E_API_KEY})"
}

# Print the path used to invoke dda. Honors DDA if set.
dda_bin() { echo "${DDA:-dda}"; }

require_dda() {
    command -v "$(dda_bin)" >/dev/null 2>&1 \
        || fail "$(dda_bin) not found in PATH (see docs/public/how-to/test/e2e.md)"
}

# Retrieve a single Pulumi stack output value (e.g. host address). Returns
# empty string if the stack doesn't exist yet.
stack_output() {
    local key=$1
    (cd "${REPO_ROOT}/test/new-e2e" \
        && pulumi stack output -s "${STACK_NAME}" "${key}" 2>/dev/null) || true
}

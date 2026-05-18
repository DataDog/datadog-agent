#!/usr/bin/env bash
# Shared helpers for the Milvus scenario scripts.
#
# - Locates the repo root and the milvus test directory (works from any cwd).
# - Authenticates to Datadog via `dd-auth --output` so the API key, app key
#   and site are exported into the current shell, then re-exports them
#   under the names the e2e-framework runner expects (E2E_API_KEY,
#   E2E_APP_KEY, MILVUS_DD_SITE).
# - Computes the stack name used by the test so up/down/check agree.
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

# Authenticate with dd-auth and surface the credentials under the names the
# e2e-framework runner reads. Idempotent — safe to call from every script.
#
# dd-auth(1) is a no-arg-required CLI that prints KEY=VALUE pairs to stdout
# when invoked with `--output`:
#
#   $ dd-auth --output
#   DD_API_KEY=...
#   DD_APP_KEY=...
#   DD_SITE=datadoghq.com
#
# Honors `DD_AUTH_DOMAIN` (native dd-auth env var) for selecting a non-default
# org, e.g. `DD_AUTH_DOMAIN=dddev.datadoghq.com`.
ensure_dd_auth() {
    if ! command -v dd-auth >/dev/null 2>&1; then
        fail "dd-auth not found in PATH; install it or set DD_API_KEY/DD_SITE manually"
    fi

    log "running dd-auth --output${DD_AUTH_DOMAIN:+ (domain=${DD_AUTH_DOMAIN})}..."
    local creds
    creds=$(dd-auth --output) \
        || fail "dd-auth failed; try \`dd-auth -v --output\` to debug"

    # Export each KEY=VALUE line into our env.
    while IFS= read -r line; do
        [[ -z "${line}" || "${line}" == \#* ]] && continue
        [[ "${line}" == *=* ]] || continue
        export "${line?}"
    done <<<"${creds}"

    [[ -n "${DD_API_KEY:-}" ]] || fail "dd-auth output did not include DD_API_KEY"
    [[ -n "${DD_SITE:-}"    ]] || fail "dd-auth output did not include DD_SITE"

    # Map to the names the e2e-framework runner picks up.
    export E2E_API_KEY="${DD_API_KEY}"
    [[ -n "${DD_APP_KEY:-}" ]] && export E2E_APP_KEY="${DD_APP_KEY}"

    # The Milvus provisioner reads MILVUS_DD_SITE at deploy time so the agent
    # ships to the same org dd-auth authenticated against (otherwise it would
    # default to datadoghq.com regardless of the auth target).
    export MILVUS_DD_SITE="${DD_SITE}"

    log "DD auth ok (site=${DD_SITE}, api key length=${#DD_API_KEY})"
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

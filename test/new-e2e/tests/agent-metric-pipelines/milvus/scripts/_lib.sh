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

# Targeting the `agent-integrations-dev` AWS account is done by:
#
#  1. A first-class environment entry added to
#     test/e2e-framework/resources/aws/environmentDefaults.go:
#     agentIntegrationsDevDefault() (env name "aws/agent-integrations-dev").
#     That's where the VPC, subnets, SG, region and AWS profile live.
#     We rely on `test/new-e2e/go.mod`'s replace directive that points the
#     framework dependency at the in-tree `test/e2e-framework/`, so the
#     patch takes effect immediately with no version bump.
#
#  2. Setting `E2E_ENVIRONMENTS=aws/agent-integrations-dev` so the
#     framework looks up that entry instead of the default
#     `aws/agent-sandbox`.
#
#  3. A small E2E_STACK_PARAMS payload that forces *public* SSM-based
#     AMI resolution (the AMI IDs hard-coded in
#     test/e2e-framework/resources/aws/platforms.json are private to
#     agent-sandbox / agent-qa and unreadable from this account).
#
# State produced by ./scripts/bootstrap-integrations-dev.sh.
INTEGRATIONS_DEV_KEY_NAME="e2e-agent-integrations-dev-${USER}"
INTEGRATIONS_DEV_KEY_PRIVATE="${HOME}/.ssh/id_rsa_e2e_agent_integrations_dev_${USER}.pem"
INTEGRATIONS_DEV_KEY_PUBLIC="${INTEGRATIONS_DEV_KEY_PRIVATE%.pem}.pub"
INTEGRATIONS_DEV_PASSPHRASE_FILE="${HOME}/.config/dd-agent-milvus-lab/pulumi.passphrase"

# Make the test runner target agent-integrations-dev. Sources its keypair +
# pulumi passphrase from the bootstrap state and pushes Pulumi config
# overrides via E2E_STACK_PARAMS. No framework/dda patch required.
ensure_aws_setup_integrations_dev() {
    for f in "${INTEGRATIONS_DEV_KEY_PRIVATE}" "${INTEGRATIONS_DEV_KEY_PUBLIC}" "${INTEGRATIONS_DEV_PASSPHRASE_FILE}"; do
        [[ -f "${f}" ]] || fail "missing ${f} — run ./scripts/bootstrap-integrations-dev.sh first"
    done

    # If the user invoked us from inside an `aws-vault exec ... --` subshell,
    # the AWS profile's credential_process (`aws-vault export ...`) will
    # refuse to nest and fail with:
    #   aws-vault: error: exec: aws-vault sessions should be nested with care
    # Drop AWS_VAULT for child processes so credential_process can run
    # cleanly. The user's session cache stays in aws-vault, so the freshly
    # spawned `aws-vault export` will reuse it without re-prompting.
    if [[ -n "${AWS_VAULT:-}" ]]; then
        log "detected nested AWS_VAULT=${AWS_VAULT}; unsetting for child processes"
        unset AWS_VAULT
    fi

    export E2E_KEY_PAIR_NAME="${INTEGRATIONS_DEV_KEY_NAME}"
    export E2E_AWS_PRIVATE_KEY_PATH="${INTEGRATIONS_DEV_KEY_PRIVATE}"
    export E2E_AWS_PUBLIC_KEY_PATH="${INTEGRATIONS_DEV_KEY_PUBLIC}"
    export E2E_PULUMI_PASSWORD="$(<"${INTEGRATIONS_DEV_PASSPHRASE_FILE}")"

    # Select our framework environment entry.
    export E2E_ENVIRONMENTS="aws/agent-integrations-dev"

    # The only remaining Pulumi overrides we need are the AMI hints:
    # the hard-coded AMI IDs in test/e2e-framework/resources/aws/platforms.json
    # are private to agent-sandbox / agent-qa and unreadable here, so we
    # force the framework's os_resolver to take its `useLatestAMI` branch
    # (os_resolver.go:78) which queries the *public* SSM parameter
    # /aws/service/ecs/optimized-ami/amazon-linux-2/recommended/image_id.
    # The empty version slot in the descriptor is what triggers that path.
    export E2E_STACK_PARAMS=$(python3 - <<'PY'
import json
print(json.dumps({
    "ddinfra:osDescriptor":       "amazon-linux-ecs::x86_64",
    "ddinfra:osImageIDUseLatest": "true",
}))
PY
    )
    log "agent-integrations-dev env wired (E2E_ENVIRONMENTS=${E2E_ENVIRONMENTS})"
}

# Retrieve a single Pulumi stack output value (e.g. host address). Returns
# empty string if the stack doesn't exist yet.
stack_output() {
    local key=$1
    (cd "${REPO_ROOT}/test/new-e2e" \
        && pulumi stack output -s "${STACK_NAME}" "${key}" 2>/dev/null) || true
}

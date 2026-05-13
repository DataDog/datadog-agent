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

# AWS resource overrides for the `agent-integrations-dev` account.
#
# These IDs were discovered with `aws ec2 describe-*` against account
# 030537971304 on 2026-05-13 (account-admin role). They are intentionally
# hard-coded here rather than a real first-class `agentIntegrationsDev`
# entry in test/e2e-framework/resources/aws/environmentDefaults.go — that
# upstream change is option B in the design discussion; this is option A.
#
# Network layout:
#   VPC 'agent-integrations-dev' (10.11.208.112/28 + secondary /22)
#     ├─ public  10.11.220.x/26  → IGW (MapPublicIpOnLaunch=False)
#     ├─ private 10.11.221-223.x/24 → NAT for egress, TGW for VPN inbound
#     └─ transit 10.11.208.x/28  → Appgate VPN
# We place the EC2 in a private subnet so:
#   * inbound SSH works over Datadog VPN (TGW route 10.0.0.0/8)
#   * outbound (pulumi up plumbing, docker pulls, agent → intake) goes
#     through the NAT gateway.
#
# The 'common' SG (sg-03d583f7425a802f7) allows:
#   * all traffic from CIDRs 10.11.{192,193,194}.0/24 (corp VPN networks)
#   * all traffic from itself (intra-VPC)
#   * unrestricted egress
INTEGRATIONS_DEV_VPC=vpc-07e6913338cbe8fea
INTEGRATIONS_DEV_SUBNETS_JSON='[{"id":"subnet-0acb59fda8504f5bb","macos_compatible":false},{"id":"subnet-0d7bb3d71e68abcc2","macos_compatible":false},{"id":"subnet-0658d161c778e6168","macos_compatible":false}]'
INTEGRATIONS_DEV_SGS_JSON='["sg-03d583f7425a802f7"]'
INTEGRATIONS_DEV_AWS_PROFILE=exec-sso-agent-integrations-dev-account-admin

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

    # E2E_STACK_PARAMS is forwarded verbatim to `pulumi config set` by
    # tasks/new_e2e_tests.py, overriding the agentSandboxDefault() values
    # baked into environmentDefaults.go.  Values that are themselves JSON
    # (subnets, security groups) must be embedded as JSON strings, so we
    # let python do the encoding to avoid quoting headaches.
    #
    # IMPORTANT: keys must use Pulumi's `<namespace>:<key>` syntax. The
    # framework reads `aws/defaultVPCID` from the `ddinfra` namespace and
    # `region`/`profile` from the `aws` namespace, so the full Pulumi keys
    # are `ddinfra:aws/defaultVPCID` and `aws:profile`. Using the plain
    # `aws/profile` form silently no-ops (the auto.ConfigMap setter treats
    # the whole string as the namespace and an empty key).
    #
    # ddinfra:osDescriptor + ddinfra:osImageIDUseLatest force the framework
    # to look up the AMI via *public* SSM Parameter Store at deploy time
    # instead of using the hard-coded AMI IDs in
    # test/e2e-framework/resources/aws/platforms.json — those IDs are
    # private to the agent-sandbox / agent-qa accounts and would fail with
    # 'Not authorized for images: […]' here. The empty version slot in the
    # descriptor is required so resolveAmazonLinuxECSAMI() takes the
    # SSM path (os_resolver.go:78). The SSM parameter
    # /aws/service/ecs/optimized-ami/amazon-linux-2/recommended/image_id is
    # universally readable so this works in any account.
    E2E_STACK_PARAMS=$(
        AWS_PROFILE="${INTEGRATIONS_DEV_AWS_PROFILE}" \
        VPC_ID="${INTEGRATIONS_DEV_VPC}" \
        SUBNETS_JSON="${INTEGRATIONS_DEV_SUBNETS_JSON}" \
        SGS_JSON="${INTEGRATIONS_DEV_SGS_JSON}" \
        python3 - <<'PY'
import json, os
print(json.dumps({
    # `aws` namespace.
    "aws:profile":                                os.environ["AWS_PROFILE"],
    # `ddinfra` namespace. The AWS-related infra keys all live under
    # `ddinfra:aws/...` (see test/e2e-framework/resources/aws/environment.go).
    "ddinfra:aws/defaultVPCID":                   os.environ["VPC_ID"],
    "ddinfra:aws/defaultSubnets":                 os.environ["SUBNETS_JSON"],
    "ddinfra:aws/defaultSecurityGroups":          os.environ["SGS_JSON"],
    # No 'ec2InstanceRole' instance profile exists in this account.
    "ddinfra:aws/defaultInstanceProfile":         "",
    # No internal ECR mirror in this account; compose images pull from
    # public quay.io / docker.io.
    "ddinfra:aws/defaultInternalRegistry":        "",
    "ddinfra:aws/defaultInternalDockerhubMirror": "",
    # AMI resolution: force public SSM lookup for an Amazon-Linux-2 ECS
    # image (the platforms.json AMIs are private to agent-sandbox).
    "ddinfra:osDescriptor":                       "amazon-linux-ecs::x86_64",
    "ddinfra:osImageIDUseLatest":                 "true",
}))
PY
    )
    export E2E_STACK_PARAMS
    log "agent-integrations-dev overrides ready (profile=${INTEGRATIONS_DEV_AWS_PROFILE})"
}

# Retrieve a single Pulumi stack output value (e.g. host address). Returns
# empty string if the stack doesn't exist yet.
stack_output() {
    local key=$1
    (cd "${REPO_ROOT}/test/new-e2e" \
        && pulumi stack output -s "${STACK_NAME}" "${key}" 2>/dev/null) || true
}

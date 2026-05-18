#!/usr/bin/env bash
# Tear down the Milvus scenario stack provisioned by up.sh.
#
# Usage: ./scripts/down.sh

source "$(dirname -- "${BASH_SOURCE[0]}")/_lib.sh"
ensure_dd_auth   # pulumi destroy still hits AWS; keep credentials wired up

log "destroying stack=${STACK_NAME}"

cd "${REPO_ROOT}/test/new-e2e"

# Use pulumi directly so this works even if the test binary isn't around.
# `--yes` skips the confirmation prompt; `--non-interactive` is safe to use
# even when a TTY is attached.
if ! pulumi stack -s "${STACK_NAME}" --non-interactive >/dev/null 2>&1; then
    log "stack ${STACK_NAME} does not exist, nothing to do"
    exit 0
fi

pulumi destroy -s "${STACK_NAME}" --yes --non-interactive
pulumi stack rm -s "${STACK_NAME}" --yes --non-interactive

log "stack ${STACK_NAME} destroyed"

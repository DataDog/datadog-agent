#!/usr/bin/env bash
# End-to-end smoke test for the Milvus deployment.
#
#   1. up.sh        — provision the stack
#   2. wait         — for Milvus to come up and the traffic loop to start
#   3. check.sh     — verify containers, agent check, traffic, metrics
#   4. (optional) down.sh — tear down the stack on success
#
# By default the stack is LEFT RUNNING after a successful smoke test so you
# can inspect it. Pass --teardown to destroy it on success.

set -euo pipefail

TEARDOWN=0
for arg in "$@"; do
    case "${arg}" in
        --teardown|-t) TEARDOWN=1 ;;
        -h|--help)
            sed -n '2,12p' "$0" | sed 's/^# \?//'
            exit 0
            ;;
        *) echo "unknown arg: ${arg}" >&2; exit 2 ;;
    esac
done

source "$(dirname -- "${BASH_SOURCE[0]}")/_lib.sh"

log "===== 1/4 : up ====="
"${SCRIPT_DIR}/up.sh"

# Milvus standalone needs ~90s, pymilvus install + first iteration another
# ~60-120s. Be generous; check.sh will retry the slow assertions itself.
WAIT_SECS=${MILVUS_WAIT_SECS:-180}
log "===== 2/4 : waiting ${WAIT_SECS}s for milvus + traffic to warm up ====="
sleep "${WAIT_SECS}"

log "===== 3/4 : check ====="
if ! "${SCRIPT_DIR}/check.sh"; then
    fail "smoke check failed — leaving the stack up for inspection (./scripts/down.sh to remove)"
fi

if (( TEARDOWN == 1 )); then
    log "===== 4/4 : down ====="
    "${SCRIPT_DIR}/down.sh"
else
    log "===== 4/4 : skipping teardown (pass --teardown to destroy) ====="
fi

log "milvus scenario smoke test PASSED"

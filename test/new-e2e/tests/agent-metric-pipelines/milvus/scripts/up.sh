#!/usr/bin/env bash
# Deploy the Milvus scenario and leave it running.
#
# Usage: ./scripts/up.sh
#
# Honors:
#   MILVUS_STACK_NAME   pulumi stack name (default: milvus-dev)
#   MILVUS_E2E_TEST_ID  per-run tag stamped onto every Milvus metric
#                       (default: <stack name>)

source "$(dirname -- "${BASH_SOURCE[0]}")/_lib.sh"
require_dda
ensure_dd_auth

log "deploying stack=${STACK_NAME} test_id=${TEST_ID}"

cd "${REPO_ROOT}"

# E2E_DEV_MODE=true keeps the stack alive after the test returns.
# We point both E2E_STACK_NAME and MILVUS_E2E_TEST_ID at our chosen values
# so the test code uses them verbatim.
E2E_DEV_MODE=true \
E2E_STACK_NAME="${STACK_NAME}" \
MILVUS_E2E_TEST_ID="${TEST_ID}" \
"$(dda_bin)" inv new-e2e-tests.run \
    --targets=./tests/agent-metric-pipelines/milvus \
    --run='^TestMilvusEnv$'

log "stack ${STACK_NAME} provisioned"
log "  Datadog filter:  e2e_test_id:${TEST_ID}"
log "  Check status:    ./scripts/check.sh"
log "  Tear down:       ./scripts/down.sh"

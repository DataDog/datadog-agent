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
ensure_aws_setup_integrations_dev

log "deploying stack=${STACK_NAME} test_id=${TEST_ID}"

cd "${REPO_ROOT}"

# Translate the JSON in E2E_STACK_PARAMS into individual --configparams
# CLI args. `tasks/new_e2e_tests.py` only re-serializes E2E_STACK_PARAMS
# into the test subprocess's env when `parsed_params` is non-empty
# (i.e. when at least one --configparams was passed on the command line),
# so going through the CLI is the safest way to guarantee our overrides
# reach the Go test.
CONFIG_ARGS=()
while IFS= read -r line; do
    [[ -z "${line}" ]] && continue
    CONFIG_ARGS+=(--configparams "${line}")
done < <(python3 - <<PY
import json, os
for k, v in json.loads(os.environ["E2E_STACK_PARAMS"]).items():
    print(f"{k}={v}")
PY
)

# E2E_DEV_MODE=true keeps the stack alive after the test returns.
E2E_DEV_MODE=true \
E2E_STACK_NAME="${STACK_NAME}" \
MILVUS_E2E_TEST_ID="${TEST_ID}" \
"$(dda_bin)" inv new-e2e-tests.run \
    --targets=./tests/agent-metric-pipelines/milvus \
    --run='^TestMilvusEnv$' \
    "${CONFIG_ARGS[@]}"

log "stack ${STACK_NAME} provisioned"
log "  Datadog filter:  e2e_test_id:${TEST_ID}"
log "  Check status:    ./scripts/check.sh"
log "  Tear down:       ./scripts/down.sh"

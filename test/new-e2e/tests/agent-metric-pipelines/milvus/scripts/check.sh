#!/usr/bin/env bash
# Smoke-test the deployed Milvus scenario.
#
# Asserts:
#   1. Pulumi stack exists.
#   2. SSH to the VM works.
#   3. The five expected containers (agent + milvus stack + traffic) are running.
#   4. The Datadog Agent has scheduled the milvus check.
#   5. The traffic container is producing iterations.
#   6. The Milvus /metrics endpoint serves Prometheus output.
#
# Exit non-zero on the first failed check.

source "$(dirname -- "${BASH_SOURCE[0]}")/_lib.sh"
ensure_dd_auth   # not strictly required, but keeps env consistent

# --- 1. stack exists -------------------------------------------------------

cd "${REPO_ROOT}/test/new-e2e"
pulumi stack -s "${STACK_NAME}" --non-interactive >/dev/null 2>&1 \
    || fail "stack ${STACK_NAME} not found — did you run ./scripts/up.sh ?"

# --- 2. resolve VM address from Pulumi outputs -----------------------------

HOST_ADDRESS=$(stack_output dockervm-address)
[[ -n "${HOST_ADDRESS}" ]] \
    || HOST_ADDRESS=$(pulumi stack output -s "${STACK_NAME}" --json 2>/dev/null \
        | python3 -c "import json,sys; d=json.load(sys.stdin)
import re
def walk(o):
    if isinstance(o, dict):
        for k,v in o.items():
            if k == 'address' and isinstance(v, str): yield v
            else: yield from walk(v)
    elif isinstance(o, list):
        for i in o: yield from walk(i)
for a in walk(d):
    print(a); break")
[[ -n "${HOST_ADDRESS}" ]] || fail "could not locate VM address in pulumi outputs"
log "VM address: ${HOST_ADDRESS}"

SSH_KEY=${E2E_AWS_PRIVATE_KEY_PATH:-${HOME}/.ssh/id_rsa}
SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10)
[[ -f "${SSH_KEY}" ]] && SSH_OPTS+=(-i "${SSH_KEY}")
SSH_USER=${SSH_USER:-ubuntu}

ssh_run() {
    ssh "${SSH_OPTS[@]}" "${SSH_USER}@${HOST_ADDRESS}" "$@"
}

# --- 3. SSH works ----------------------------------------------------------

log "checking SSH..."
ssh_run true || fail "ssh to ${SSH_USER}@${HOST_ADDRESS} failed"

# --- 4. containers ---------------------------------------------------------

log "listing containers..."
CONTAINERS=$(ssh_run "sudo docker ps --format '{{.Names}}'") \
    || fail "docker ps failed on the VM"

echo "${CONTAINERS}" | sed 's/^/    /' >&2

for name in datadog-agent milvus-standalone milvus-etcd milvus-minio milvus-traffic; do
    grep -qx "${name}" <<<"${CONTAINERS}" \
        || fail "expected container ${name} not running"
done
log "all 5 containers running ✓"

# --- 5. milvus check scheduled --------------------------------------------

log "checking 'agent status' for milvus..."
if ssh_run "sudo docker exec datadog-agent agent status 2>/dev/null | grep -E '^\s*milvus' -A 5" \
    | tee /dev/stderr | grep -q "milvus"; then
    log "milvus integration loaded ✓"
else
    fail "milvus integration not present in agent status"
fi

# --- 6. traffic generator producing iterations ----------------------------

log "checking traffic generator logs..."
TRAFFIC_LOG=$(ssh_run "sudo docker logs --tail 20 milvus-traffic 2>&1") \
    || fail "could not fetch milvus-traffic logs"
echo "${TRAFFIC_LOG}" | sed 's/^/    /' >&2
grep -qE 'iteration=[0-9]+ ok' <<<"${TRAFFIC_LOG}" \
    || fail "traffic generator has not produced a successful iteration yet"
log "traffic generator producing iterations ✓"

# --- 7. milvus /metrics endpoint live -------------------------------------

log "fetching milvus /metrics..."
METRICS_HEAD=$(ssh_run "sudo docker exec datadog-agent curl -fsS http://milvus:9091/metrics 2>/dev/null | head -5") \
    || fail "agent could not reach milvus:9091/metrics"
[[ -n "${METRICS_HEAD}" ]] || fail "milvus /metrics returned empty"
echo "${METRICS_HEAD}" | sed 's/^/    /' >&2
log "milvus metrics endpoint live ✓"

log "all smoke checks passed"
log "next: open Datadog and filter by e2e_test_id:${TEST_ID}"

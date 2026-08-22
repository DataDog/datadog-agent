#!/usr/bin/env bash
# Runs the standalone local-only Remote Queries proof:
# fakeintake -> standalone OS private-action-runner process -> com.datadoghq.remotequeries.execute
# -> real local AgentSecure gRPC RemoteQueryExecuteStream over Agent IPC TLS/auth
# -> loaded Postgres check -> fixture-table proof query -> fakeintake publish.
# The HTTP execute endpoint remains as a dev preflight for local evidence only.
#
# Defaults assume the remote-queries-poc worktree layout and reuse the
# integrations-core Postgres integration test compose fixture. Override
# AGENT_REPO, INTEGRATIONS_CORE, TMP_ROOT, CMD_PORT, POSTGRES_COMPOSE_FILE,
# POSTGRES_COMPOSE_PROJECT, POSTGRES_IMAGE, RQ_REMOTE_QUERY, RQ_POSTGRES_*, or
# AGENT_PYTHON_VERSION / AGENT_PYTHON_ABI if needed. This proof intentionally follows the repository's
# fakeintake/OPMS precedent and sets DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true
# for the standalone-process tracer bullet. Signed task verification is postponed
# to backend/AP/RC work.

set -euo pipefail

AGENT_REPO=${AGENT_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
INTEGRATIONS_CORE=${INTEGRATIONS_CORE:-/home/bits/dd/tasks/remote-queries-poc/worktrees/integrations-core}
TMP_ROOT=${TMP_ROOT:-/tmp/rq-standalone-par-agent-postgres}
CMD_PORT=${CMD_PORT:-55003}
POSTGRES_COMPOSE_FILE=${POSTGRES_COMPOSE_FILE:-$INTEGRATIONS_CORE/postgres/tests/compose/docker-compose.yaml}
POSTGRES_COMPOSE_PROJECT=${POSTGRES_COMPOSE_PROJECT:-rq-standalone-par-agent-postgres-$$}
POSTGRES_IMAGE=${POSTGRES_IMAGE:-13-alpine}
POSTGRES_LOCALE=${POSTGRES_LOCALE:-UTF8}
RQ_REMOTE_QUERY_WAS_SET=0
if [[ -n "${RQ_REMOTE_QUERY+x}" ]]; then
  RQ_REMOTE_QUERY_WAS_SET=1
fi
RQ_REMOTE_QUERY=${RQ_REMOTE_QUERY:-}
RQ_REMOTE_OPERATION_WAS_SET=0
if [[ -n "${RQ_REMOTE_OPERATION+x}" ]]; then
  RQ_REMOTE_OPERATION_WAS_SET=1
fi
RQ_REMOTE_OPERATION=${RQ_REMOTE_OPERATION:-}
RQ_REMOTE_FORMAT_WAS_SET=0
if [[ -n "${RQ_REMOTE_FORMAT+x}" ]]; then
  RQ_REMOTE_FORMAT_WAS_SET=1
fi
RQ_REMOTE_FORMAT=${RQ_REMOTE_FORMAT:-}
RQ_POSTGRES_HOST=${RQ_POSTGRES_HOST:-localhost}
RQ_POSTGRES_PORT=${RQ_POSTGRES_PORT:-5432}
RQ_POSTGRES_DBNAME=${RQ_POSTGRES_DBNAME:-datadog_test}
RQ_POSTGRES_USERNAME=${RQ_POSTGRES_USERNAME:-bob}
RQ_POSTGRES_PASSWORD=${RQ_POSTGRES_PASSWORD:-bob}
PIP_PLATFORM=${PIP_PLATFORM:-manylinux2014_x86_64}
AGENT_PYTHON_VERSION=${AGENT_PYTHON_VERSION:-}
AGENT_PYTHON_ABI=${AGENT_PYTHON_ABI:-}

AGENT_PID=""
POSTGRES_COMPOSE_STARTED=0
PROOF_CASE_NAME=""
CASE_RESULTS_DIR=""

PROOF_CASE_NAMES=(
  "seed"
  "fixture-city"
  "copy-fixture-city"
  "copy-binary-payload"
  "payload-1mib"
  "payload-2mib"
  "payload-4mib"
  "payload-8mib"
  "payload-16mib"
  "payload-32mib"
)

PROOF_CASE_QUERIES=(
  "SELECT 1 AS value"
  "SELECT city, country FROM cities ORDER BY city"
  "SELECT city, country FROM cities ORDER BY city"
  "SELECT decode('00ff80', 'hex') AS payload"
  "SELECT repeat('x', 1048576) AS payload"
  "SELECT repeat('x', 2097152) AS payload"
  "SELECT repeat('x', 4194304) AS payload"
  "SELECT repeat('x', 8388608) AS payload"
  "SELECT repeat('x', 16777216) AS payload"
  "SELECT repeat('x', 33554432) AS payload"
)

log() {
  printf '\n[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"
}

cleanup_agent() {
  if [[ -n "${AGENT_PID:-}" ]] && kill -0 "$AGENT_PID" 2>/dev/null; then
    kill "$AGENT_PID" 2>/dev/null || true
    wait "$AGENT_PID" 2>/dev/null || true
  fi
  AGENT_PID=""
}

docker_compose() {
  docker compose -f "$POSTGRES_COMPOSE_FILE" -p "$POSTGRES_COMPOSE_PROJECT" "$@"
}

cleanup_all() {
  cleanup_agent
  if [[ "$POSTGRES_COMPOSE_STARTED" == "1" ]]; then
    docker_compose down --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup_all EXIT

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

python_minor_from_version() {
  local version=$1
  awk -F. '{print $1 "." $2}' <<<"$version"
}

abi_from_python_minor() {
  local minor=$1
  printf 'cp%s\n' "${minor//./}"
}

write_agent_config() {
  cat > "$TMP_ROOT/datadog.yaml" <<YAML
api_key: '00000000000000000000000000000000'
site: datadoghq.com
dd_url: http://127.0.0.1:9
hostname: rq-standalone-local-proof
cmd_host: 127.0.0.1
cmd_port: $CMD_PORT
auth_token_file_path: $TMP_ROOT/run/auth_token
ipc_cert_file_path: $TMP_ROOT/run/ipc_cert.pem
confd_path: $TMP_ROOT/conf.d
additional_checksd: $TMP_ROOT/checks.d
remote_queries.match_check.enabled: true
remote_queries.execute.enabled: true
remote_queries.execute.enable_query_allowlist: false
log_level: debug
log_file: $TMP_ROOT/agent.log
python_lazy_loading: false
YAML
}

detect_agent_python() {
  if [[ -n "$AGENT_PYTHON_ABI" ]]; then
    if [[ -z "$AGENT_PYTHON_VERSION" ]]; then
      AGENT_PYTHON_VERSION="${AGENT_PYTHON_ABI#cp}"
      AGENT_PYTHON_VERSION="${AGENT_PYTHON_VERSION:0:1}.${AGENT_PYTHON_VERSION:1}"
    fi
    log "Using overridden Agent Python ABI: $AGENT_PYTHON_ABI (version hint: $AGENT_PYTHON_VERSION)"
    return
  fi

  if [[ -n "$AGENT_PYTHON_VERSION" ]]; then
    local minor
    minor=$(python_minor_from_version "$AGENT_PYTHON_VERSION")
    AGENT_PYTHON_VERSION="$minor"
    AGENT_PYTHON_ABI=$(abi_from_python_minor "$minor")
    log "Using overridden Agent Python version: $AGENT_PYTHON_VERSION ($AGENT_PYTHON_ABI)"
    return
  fi

  log "Detecting Python runtime from the built Agent binary"
  : > "$TMP_ROOT/agent.log"
  "$AGENT_REPO/bin/agent/agent" run -c "$TMP_ROOT" \
    > "$TMP_ROOT/python-detect-stdout.log" 2> "$TMP_ROOT/python-detect-stderr.log" &
  AGENT_PID=$!

  local detected=""
  for _ in $(seq 1 80); do
    detected=$(grep -aoE '"pythonV":"[0-9]+\.[0-9]+(\.[0-9]+)?' "$TMP_ROOT/agent.log" 2>/dev/null | head -1 | cut -d: -f2 | tr -d '"' || true)
    if [[ -n "$detected" ]]; then
      break
    fi
    if ! kill -0 "$AGENT_PID" 2>/dev/null; then
      break
    fi
    sleep 0.5
  done
  cleanup_agent

  if [[ -z "$detected" && -f "$AGENT_REPO/omnibus/config/software/python3.rb" ]]; then
    detected=$(sed -nE 's/^default_version "([0-9]+\.[0-9]+)(\.[0-9]+)?"/\1/p' "$AGENT_REPO/omnibus/config/software/python3.rb" | head -1)
    if [[ -n "$detected" ]]; then
      log "Could not detect runtime Python from Agent logs; falling back to source config: $detected"
    fi
  fi

  if [[ -z "$detected" ]]; then
    echo "Unable to detect Agent Python version. Set AGENT_PYTHON_VERSION=3.12 or AGENT_PYTHON_ABI=cp312." >&2
    tail -80 "$TMP_ROOT/python-detect-stderr.log" >&2 || true
    exit 1
  fi

  AGENT_PYTHON_VERSION=$(python_minor_from_version "$detected")
  AGENT_PYTHON_ABI=$(abi_from_python_minor "$AGENT_PYTHON_VERSION")
  log "Detected Agent Python runtime: $detected; installing wheels for $AGENT_PYTHON_VERSION ($AGENT_PYTHON_ABI)"
}

install_python_deps() {
  local py_digits
  py_digits=${AGENT_PYTHON_VERSION//./}

  log "Installing temporary Python deps into $TMP_ROOT/pydeps for $AGENT_PYTHON_ABI on $PIP_PLATFORM"
  python3 -m pip install --quiet --target "$TMP_ROOT/pydeps" \
    --only-binary=:all: --platform "$PIP_PLATFORM" \
    --implementation cp --python-version "$py_digits" --abi "$AGENT_PYTHON_ABI" \
    'psycopg[binary,pool]' cachetools packaging semver 'pydantic<3' python-dateutil mmh3

  for dep in "$TMP_ROOT"/pydeps/*; do
    local base
    base=$(basename "$dep")
    [[ -e "$TMP_ROOT/checks.d/$base" ]] || ln -s "$dep" "$TMP_ROOT/checks.d/$base"
  done
}

setup_tmp_tree() {
  rm -rf "$TMP_ROOT"
  mkdir -p "$TMP_ROOT/conf.d/postgres.d" "$TMP_ROOT/run" "$TMP_ROOT/checks.d/datadog_checks" "$TMP_ROOT/pydeps" "$TMP_ROOT/results"

  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/base" "$TMP_ROOT/checks.d/datadog_checks/base"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/checks" "$TMP_ROOT/checks.d/datadog_checks/checks"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/config.py" "$TMP_ROOT/checks.d/datadog_checks/config.py"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/errors.py" "$TMP_ROOT/checks.d/datadog_checks/errors.py"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/log.py" "$TMP_ROOT/checks.d/datadog_checks/log.py"
  ln -s "$INTEGRATIONS_CORE/postgres/datadog_checks/postgres" "$TMP_ROOT/checks.d/datadog_checks/postgres"
  cat > "$TMP_ROOT/checks.d/datadog_checks/__init__.py" <<'PY'
__path__ = __import__('pkgutil').extend_path(__path__, __name__)
PY

  write_agent_config
  detect_agent_python
  install_python_deps
}

postgres_is_ready() {
  PGPASSWORD="$RQ_POSTGRES_PASSWORD" psql \
    -h "$RQ_POSTGRES_HOST" \
    -p "$RQ_POSTGRES_PORT" \
    -U "$RQ_POSTGRES_USERNAME" \
    -d "$RQ_POSTGRES_DBNAME" \
    -c 'select 1' >/dev/null 2>&1
}

postgres_target_is_default_fixture() {
  [[ "$RQ_POSTGRES_HOST" == "localhost" && \
    "$RQ_POSTGRES_PORT" == "5432" && \
    "$RQ_POSTGRES_DBNAME" == "datadog_test" && \
    "$RQ_POSTGRES_USERNAME" == "bob" && \
    "$RQ_POSTGRES_PASSWORD" == "bob" ]]
}

start_postgres_fixture() {
  if postgres_is_ready; then
    log "Using existing compatible Postgres fixture at $RQ_POSTGRES_HOST:$RQ_POSTGRES_PORT/$RQ_POSTGRES_DBNAME"
    return
  fi

  if ! postgres_target_is_default_fixture; then
    echo "Overridden RQ_POSTGRES_* target is not reachable; refusing to start the default compose fixture for a different target." >&2
    exit 1
  fi

  [[ -f "$POSTGRES_COMPOSE_FILE" ]] || {
    echo "Postgres compose fixture not found: $POSTGRES_COMPOSE_FILE" >&2
    exit 1
  }
  docker compose version >/dev/null 2>&1 || {
    echo "docker compose is required to start the integrations-core Postgres fixture" >&2
    exit 1
  }

  log "Starting integrations-core Postgres fixture project=$POSTGRES_COMPOSE_PROJECT image=postgres:$POSTGRES_IMAGE"
  export POSTGRES_IMAGE POSTGRES_LOCALE
  docker_compose down --remove-orphans >/dev/null 2>&1 || true
  docker_compose up -d postgres
  POSTGRES_COMPOSE_STARTED=1

  for _ in $(seq 1 120); do
    if postgres_is_ready && docker_compose exec -T postgres test -e /tmp/container_ready.txt >/dev/null 2>&1; then
      log "Integrations-core Postgres fixture is ready at $RQ_POSTGRES_HOST:$RQ_POSTGRES_PORT/$RQ_POSTGRES_DBNAME"
      return
    fi
    sleep 0.5
  done

  docker_compose ps >&2 || true
  docker_compose logs --tail=80 postgres >&2 || true
  echo "Integrations-core Postgres fixture did not become ready" >&2
  exit 1
}

write_postgres_config() {
  cat > "$TMP_ROOT/conf.d/postgres.d/conf.yaml" <<YAML
init_config: {}
instances:
  - host: $RQ_POSTGRES_HOST
    port: $RQ_POSTGRES_PORT
    dbname: $RQ_POSTGRES_DBNAME
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
YAML
}

remote_query_limits_json() {
  RQ_REMOTE_QUERY="$RQ_REMOTE_QUERY" python3 - <<'PY'
import json
import os
import re

query = os.environ["RQ_REMOTE_QUERY"]
max_rows = 2 if query == "SELECT city, country FROM cities ORDER BY city" else 1
max_bytes = 4 * 1024
timeout_ms = 5000
match = re.fullmatch(r"SELECT repeat\('x', ([0-9]+)\) AS payload", query)
if match:
    payload_bytes = int(match.group(1))
    max_bytes = payload_bytes + 1024 * 1024
    timeout_ms = 60000
print(json.dumps({"maxRows": max_rows, "maxBytes": max_bytes, "timeoutMs": timeout_ms}))
PY
}

summarize_json_file() {
  local input=$1
  local output=$2
  python3 - "$input" "$output" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    body = json.load(f)

def summarize(value):
    if isinstance(value, dict):
        return {key: summarize(nested) for key, nested in value.items()}
    if isinstance(value, list):
        return [summarize(nested) for nested in value]
    if isinstance(value, str) and len(value) > 4096:
        return f"<{len(value)} bytes>"
    return value

summary = summarize(body)
with open(sys.argv[2], "w", encoding="utf-8") as f:
    json.dump(summary, f, indent=2, sort_keys=True)
    f.write("\n")
print(json.dumps(summary, sort_keys=True))
PY
}

start_agent_and_wait_for_postgres_check() {
  : > "$TMP_ROOT/agent.log"
  PYTHONPATH="$TMP_ROOT/checks.d:$TMP_ROOT/pydeps" \
    "$AGENT_REPO/bin/agent/agent" run -c "$TMP_ROOT" \
    > "$TMP_ROOT/live-stdout.log" 2> "$TMP_ROOT/live-stderr.log" &
  AGENT_PID=$!

  for _ in $(seq 1 80); do
    if grep -q "successfully loaded check 'postgres'" "$TMP_ROOT/agent.log" && [[ -f "$TMP_ROOT/run/auth_token" && -f "$TMP_ROOT/run/ipc_cert.pem" ]]; then
      log "Agent loaded the Postgres check and exposed IPC artifacts"
      grep -n "successfully loaded check 'postgres'\|Scheduling check postgres" "$TMP_ROOT/agent.log" | tail -20
      return
    fi
    if ! kill -0 "$AGENT_PID" 2>/dev/null; then
      echo "Agent exited early; stderr follows:" >&2
      tail -80 "$TMP_ROOT/live-stderr.log" >&2 || true
      exit 1
    fi
    sleep 0.5
  done

  echo "Timed out waiting for loaded Postgres check" >&2
  grep -ni 'postgres\|remote_queries\|ModuleNotFound\|ImportError\|unable to load check\|AttributeError' "$TMP_ROOT/agent.log" >&2 || true
  exit 1
}

call_agent_execute_preflight() {
  local payload
  local limits
  limits=$(remote_query_limits_json)
  payload=$(RQ_POSTGRES_HOST="$RQ_POSTGRES_HOST" RQ_POSTGRES_PORT="$RQ_POSTGRES_PORT" RQ_POSTGRES_DBNAME="$RQ_POSTGRES_DBNAME" RQ_REMOTE_QUERY="$RQ_REMOTE_QUERY" RQ_LIMITS_JSON="$limits" python3 - <<'PY'
import json
import os
print(json.dumps({
    "integration": "postgres",
    "target": {
        "host": os.environ["RQ_POSTGRES_HOST"],
        "port": int(os.environ["RQ_POSTGRES_PORT"]),
        "dbname": os.environ["RQ_POSTGRES_DBNAME"],
    },
    "query": os.environ["RQ_REMOTE_QUERY"],
    "limits": json.loads(os.environ["RQ_LIMITS_JSON"]),
}))
PY
)
  local token
  token=$(cat "$TMP_ROOT/run/auth_token")

  log "[$PROOF_CASE_NAME] Preflight real Agent IPC HTTP execute endpoint is disabled; Remote Queries execution requires AgentSecure COPY streaming"
  local body_file="$CASE_RESULTS_DIR/agent-execute-preflight.raw-body"
  local status_file="$CASE_RESULTS_DIR/agent-execute-preflight.status"
  local summary_file="$CASE_RESULTS_DIR/agent-execute-preflight.summary.json"
  local status
  status=$(curl -sS -k -o "$body_file" -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    "https://127.0.0.1:${CMD_PORT}/agent/remote-queries/execute")
  printf 'agent_execute_http_status=%s\n' "$status" | tee "$status_file"
  if [[ -s "$body_file" ]]; then
    summarize_json_file "$body_file" "$summary_file"
  fi

  if [[ "$status" != "400" ]]; then
    echo "FAIL: expected Agent execute preflight HTTP 400 for disabled inline execution, got $status" >&2
    exit 1
  fi
  rm -f "$body_file"
}

run_standalone_go_proof() {
  log "[$PROOF_CASE_NAME] Running standalone PAR process -> real AgentSecure gRPC streaming IPC -> Postgres -> fakeintake proof test"
  (
    cd "$AGENT_REPO"
    RQ_STANDALONE_PROOF=1 \
    RQ_STANDALONE_PAR_BIN="$AGENT_REPO/bin/privateactionrunner/privateactionrunner" \
    RQ_STANDALONE_AGENT_PID="$AGENT_PID" \
    RQ_STANDALONE_AGENT_CMD_PORT="$CMD_PORT" \
    RQ_STANDALONE_AGENT_AUTH_TOKEN_FILE="$TMP_ROOT/run/auth_token" \
    RQ_STANDALONE_AGENT_IPC_CERT_FILE="$TMP_ROOT/run/ipc_cert.pem" \
    RQ_STANDALONE_EVIDENCE_FILE="$CASE_RESULTS_DIR/standalone-proof-evidence.txt" \
    RQ_POSTGRES_HOST="$RQ_POSTGRES_HOST" \
    RQ_POSTGRES_PORT="$RQ_POSTGRES_PORT" \
    RQ_POSTGRES_DBNAME="$RQ_POSTGRES_DBNAME" \
    RQ_REMOTE_QUERY="$RQ_REMOTE_QUERY" \
    RQ_REMOTE_OPERATION="copy_stream" \
    RQ_REMOTE_FORMAT="${RQ_REMOTE_FORMAT:-csv}" \
    dda inv test --targets=./pkg/privateactionrunner/bundles/remotequeries \
      --extra-args='-run TestRemoteQueriesActionRunsThroughStandalonePARProcessWithRealAgentIPC -count=1 -v'
  ) | tee "$CASE_RESULTS_DIR/standalone-proof-test.log"
}

run_proof_case() {
  PROOF_CASE_NAME=$1
  RQ_REMOTE_QUERY=$2
  if [[ "$RQ_REMOTE_OPERATION_WAS_SET" != "1" ]]; then
    RQ_REMOTE_OPERATION=copy_stream
  fi
  if [[ "$RQ_REMOTE_FORMAT_WAS_SET" != "1" ]]; then
    RQ_REMOTE_FORMAT=csv
    if [[ "$PROOF_CASE_NAME" == "copy-binary-payload" ]]; then
      RQ_REMOTE_FORMAT=binary
    fi
  fi
  CASE_RESULTS_DIR="$TMP_ROOT/results/$PROOF_CASE_NAME"
  mkdir -p "$CASE_RESULTS_DIR"
  printf '%s\n' "$RQ_REMOTE_QUERY" > "$CASE_RESULTS_DIR/query.sql"

  log "[$PROOF_CASE_NAME] Starting proof case: $RQ_REMOTE_QUERY"
  call_agent_execute_preflight
  run_standalone_go_proof
  printf 'case=%s status=passed\n' "$PROOF_CASE_NAME" | tee "$CASE_RESULTS_DIR/status.txt"
}

run_proof_cases() {
  if [[ "$RQ_REMOTE_QUERY_WAS_SET" == "1" ]]; then
    run_proof_case "single" "$RQ_REMOTE_QUERY"
    return
  fi

  local idx
  for idx in "${!PROOF_CASE_NAMES[@]}"; do
    run_proof_case "${PROOF_CASE_NAMES[$idx]}" "${PROOF_CASE_QUERIES[$idx]}"
  done
}

main() {
  require_cmd docker
  require_cmd psql
  require_cmd python3
  require_cmd curl
  require_cmd dda

  [[ -x "$AGENT_REPO/bin/agent/agent" ]] || {
    echo "Agent binary not found/executable: $AGENT_REPO/bin/agent/agent" >&2
    echo "Build it first with: dda inv agent.build --build-exclude=systemd" >&2
    exit 1
  }
  [[ -x "$AGENT_REPO/bin/privateactionrunner/privateactionrunner" ]] || {
    echo "Private Action Runner binary not found/executable: $AGENT_REPO/bin/privateactionrunner/privateactionrunner" >&2
    echo "Build it first with: dda inv privateactionrunner.build" >&2
    exit 1
  }
  [[ -d "$INTEGRATIONS_CORE/postgres/datadog_checks/postgres" ]] || {
    echo "Postgres integration not found under: $INTEGRATIONS_CORE" >&2
    exit 1
  }

  log "Preparing temporary harness at $TMP_ROOT"
  setup_tmp_tree
  start_postgres_fixture
  write_postgres_config
  start_agent_and_wait_for_postgres_check
  run_proof_cases

  log "Sanitized standalone proof evidence"
  find "$TMP_ROOT/results" -name 'standalone-proof-evidence.txt' -print -exec cat {} \;

  log "Done. Sanitized artifacts left in $TMP_ROOT/results"
  log "Key evidence: per-case preflight summaries, fakeintake enqueue/dequeue/publish, standalone PAR PID, and real AgentSecure IPC evidence are under $TMP_ROOT/results/<case>/"
}

main "$@"

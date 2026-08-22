#!/usr/bin/env bash
# Runs the fused local-only Remote Queries proof:
# fakeintake -> live WorkflowRunner PAR loop -> com.datadoghq.remotequeries.execute
# -> real local AgentSecure gRPC RemoteQueryExecute over Agent IPC TLS/auth
# -> loaded Postgres check -> fixture-table proof query -> fakeintake publish.
# The HTTP execute endpoint remains as a dev preflight for local evidence only.
#
# Defaults assume the remote-queries-poc worktree layout and reuse the
# integrations-core Postgres integration test compose fixture. Override
# AGENT_REPO, INTEGRATIONS_CORE, TMP_ROOT, CMD_PORT, POSTGRES_COMPOSE_FILE,
# POSTGRES_COMPOSE_PROJECT, POSTGRES_IMAGE, RQ_REMOTE_QUERY, RQ_POSTGRES_*, or
# AGENT_PYTHON_VERSION / AGENT_PYTHON_ABI if needed. The proof is local-only and intentionally sets
# DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true inside the Go proof test.

set -euo pipefail

AGENT_REPO=${AGENT_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
INTEGRATIONS_CORE=${INTEGRATIONS_CORE:-/home/bits/dd/tasks/remote-queries-poc/worktrees/integrations-core}
TMP_ROOT=${TMP_ROOT:-/tmp/rq-fused-local-par-agent-postgres}
CMD_PORT=${CMD_PORT:-55003}
POSTGRES_COMPOSE_FILE=${POSTGRES_COMPOSE_FILE:-$INTEGRATIONS_CORE/postgres/tests/compose/docker-compose.yaml}
POSTGRES_COMPOSE_PROJECT=${POSTGRES_COMPOSE_PROJECT:-rq-fused-local-par-agent-postgres-$$}
POSTGRES_IMAGE=${POSTGRES_IMAGE:-13-alpine}
POSTGRES_LOCALE=${POSTGRES_LOCALE:-UTF8}
RQ_REMOTE_QUERY=${RQ_REMOTE_QUERY:-SELECT city, country FROM cities ORDER BY city}
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
hostname: rq-fused-local-proof
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
  payload=$(RQ_POSTGRES_HOST="$RQ_POSTGRES_HOST" RQ_POSTGRES_PORT="$RQ_POSTGRES_PORT" RQ_POSTGRES_DBNAME="$RQ_POSTGRES_DBNAME" RQ_REMOTE_QUERY="$RQ_REMOTE_QUERY" python3 - <<'PY'
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
    "limits": {"maxRows": 2 if os.environ["RQ_REMOTE_QUERY"] == "SELECT city, country FROM cities ORDER BY city" else 1, "maxBytes": 1024, "timeoutMs": 1000},
}))
PY
)
  local token
  token=$(cat "$TMP_ROOT/run/auth_token")

  log "Preflight real Agent IPC HTTP execute endpoint is disabled; Remote Queries execution requires AgentSecure COPY streaming"
  local status
  status=$(curl -sS -k -o "$TMP_ROOT/results/agent-execute-preflight.body" -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    "https://127.0.0.1:${CMD_PORT}/agent/remote-queries/execute")
  printf 'agent_execute_http_status=%s\n' "$status" | tee "$TMP_ROOT/results/agent-execute-preflight.status"
  cat "$TMP_ROOT/results/agent-execute-preflight.body"
  printf '\n'

  if [[ "$status" != "400" ]]; then
    echo "FAIL: expected Agent execute preflight HTTP 400 for disabled inline execution, got $status" >&2
    exit 1
  fi
  if grep -Eq 'password|token|secret' "$TMP_ROOT/results/agent-execute-preflight.body"; then
    echo "FAIL: Agent execute preflight response contained credential-shaped text" >&2
    exit 1
  fi
}

run_fused_go_proof() {
  log "Running fused PAR -> real AgentSecure gRPC IPC -> Postgres -> fakeintake proof test"
  (
    cd "$AGENT_REPO"
    RQ_FUSED_PROOF=1 \
    RQ_FUSED_AGENT_CMD_PORT="$CMD_PORT" \
    RQ_FUSED_AGENT_AUTH_TOKEN_FILE="$TMP_ROOT/run/auth_token" \
    RQ_FUSED_AGENT_IPC_CERT_FILE="$TMP_ROOT/run/ipc_cert.pem" \
    RQ_FUSED_EVIDENCE_FILE="$TMP_ROOT/results/fused-proof-evidence.txt" \
    RQ_POSTGRES_HOST="$RQ_POSTGRES_HOST" \
    RQ_POSTGRES_PORT="$RQ_POSTGRES_PORT" \
    RQ_POSTGRES_DBNAME="$RQ_POSTGRES_DBNAME" \
    RQ_REMOTE_QUERY="$RQ_REMOTE_QUERY" \
    dda inv test --targets=./pkg/privateactionrunner/bundles/remotequeries \
      --extra-args='-run TestRemoteQueriesActionRunsThroughLivePARLoopWithRealAgentIPC -count=1 -v'
  ) | tee "$TMP_ROOT/results/fused-proof-test.log"
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
  [[ -d "$INTEGRATIONS_CORE/postgres/datadog_checks/postgres" ]] || {
    echo "Postgres integration not found under: $INTEGRATIONS_CORE" >&2
    exit 1
  }

  log "Preparing temporary harness at $TMP_ROOT"
  setup_tmp_tree
  start_postgres_fixture
  write_postgres_config
  start_agent_and_wait_for_postgres_check
  call_agent_execute_preflight
  run_fused_go_proof

  log "Sanitized fused proof evidence"
  cat "$TMP_ROOT/results/fused-proof-evidence.txt"

  log "Done. Sanitized artifacts left in $TMP_ROOT"
  log "Key evidence: fakeintake enqueue/dequeue/publish and real AgentSecure IPC evidence are in $TMP_ROOT/results/fused-proof-evidence.txt"
}

main "$@"

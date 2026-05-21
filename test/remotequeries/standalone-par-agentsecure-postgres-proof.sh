#!/usr/bin/env bash
# Runs the standalone local-only Remote Queries proof:
# fakeintake -> standalone OS private-action-runner process -> com.datadoghq.remotequeries.execute
# -> real local AgentSecure gRPC RemoteQueryExecute over Agent IPC TLS/auth
# -> loaded Postgres check -> SELECT 1 AS value -> fakeintake publish.
# The HTTP execute endpoint remains as a dev preflight for local evidence only.
#
# Defaults assume the remote-queries-poc worktree layout. Override AGENT_REPO,
# INTEGRATIONS_CORE, TMP_ROOT, CMD_PORT, POSTGRES_IMAGE, or AGENT_PYTHON_VERSION /
# AGENT_PYTHON_ABI if needed. This proof intentionally follows the repository's
# fakeintake/OPMS precedent and sets DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true
# for the standalone-process tracer bullet. Signed task verification is postponed
# to backend/AP/RC work.

set -euo pipefail

AGENT_REPO=${AGENT_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
INTEGRATIONS_CORE=${INTEGRATIONS_CORE:-/home/bits/dd/tasks/remote-queries-poc/worktrees/integrations-core}
TMP_ROOT=${TMP_ROOT:-/tmp/rq-standalone-par-agent-postgres}
CMD_PORT=${CMD_PORT:-55003}
POSTGRES_IMAGE=${POSTGRES_IMAGE:-postgres:11-alpine}
POSTGRES_CONTAINER=${POSTGRES_CONTAINER:-rq-standalone-par-agent-postgres-$$}
PIP_PLATFORM=${PIP_PLATFORM:-manylinux2014_x86_64}
AGENT_PYTHON_VERSION=${AGENT_PYTHON_VERSION:-}
AGENT_PYTHON_ABI=${AGENT_PYTHON_ABI:-}

AGENT_PID=""
POSTGRES_STARTED=0

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

cleanup_all() {
  cleanup_agent
  if [[ "$POSTGRES_STARTED" == "1" ]]; then
    docker rm -f "$POSTGRES_CONTAINER" >/dev/null 2>&1 || true
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
log_level: debug
log_file: $TMP_ROOT/agent.log
python_lazy_loading: false
telemetry.enabled: false
inventories_enabled: false
process_config.enabled: 'false'
logs_enabled: false
apm_config.enabled: false
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

start_postgres_fixture() {
  if psql 'postgresql://postgres@localhost:5432/postgres' -c 'select 1' >/dev/null 2>&1; then
    log "Using existing local Postgres on localhost:5432"
    return
  fi

  local published_containers proof_containers other_containers
  published_containers=$(docker ps --filter publish=5432 --format '{{.Names}}' || true)
  if [[ -n "$published_containers" ]]; then
    proof_containers=$(grep -E '^rq-standalone-par-agent-postgres-' <<<"$published_containers" || true)
    other_containers=$(grep -Ev '^rq-standalone-par-agent-postgres-' <<<"$published_containers" || true)

    if [[ -n "$proof_containers" ]]; then
      log "Removing stale proof Postgres container(s) bound to localhost:5432 but not accepting psql"
      xargs -r docker rm -f <<<"$proof_containers" >/dev/null
    fi
    if [[ -n "$other_containers" ]]; then
      echo "Port 5432 is published by non-proof Docker container(s), and psql select 1 failed:" >&2
      sed 's/^/  /' <<<"$other_containers" >&2
      echo "Refusing to remove unrelated containers. Stop them or point this proof at a reachable local Postgres." >&2
      exit 1
    fi
  fi

  log "Starting disposable Postgres fixture $POSTGRES_CONTAINER from $POSTGRES_IMAGE"
  if ! docker run --rm --name "$POSTGRES_CONTAINER" \
    -e POSTGRES_HOST_AUTH_METHOD=trust \
    -p 5432:5432 \
    -d "$POSTGRES_IMAGE" >/dev/null; then
    echo "Failed to start disposable Postgres fixture on port 5432." >&2
    echo "If another local Postgres is intended, ensure this succeeds: psql postgresql://postgres@localhost:5432/postgres -c 'select 1'" >&2
    exit 1
  fi
  POSTGRES_STARTED=1

  for _ in $(seq 1 60); do
    if psql 'postgresql://postgres@localhost:5432/postgres' -c 'select 1' >/dev/null 2>&1; then
      log "Postgres fixture is ready"
      return
    fi
    sleep 0.5
  done

  docker logs "$POSTGRES_CONTAINER" | tail -80 >&2 || true
  echo "Postgres fixture did not become ready" >&2
  exit 1
}

write_postgres_config() {
  cat > "$TMP_ROOT/conf.d/postgres.d/conf.yaml" <<'YAML'
init_config: {}
instances:
  - host: localhost
    port: 5432
    dbname: postgres
    username: postgres
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
  local payload='{"integration":"postgres","target":{"host":"localhost","port":5432,"dbname":"postgres"},"query":"SELECT 1 AS value","limits":{"maxRows":1,"maxBytes":1024,"timeoutMs":1000}}'
  local token
  token=$(cat "$TMP_ROOT/run/auth_token")

  log "Preflight real Agent IPC HTTP execute endpoint (dev evidence only)"
  local status
  status=$(curl -sS -k -o "$TMP_ROOT/results/agent-execute-preflight.body" -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    "https://127.0.0.1:${CMD_PORT}/agent/remote-queries/execute")
  printf 'agent_execute_http_status=%s\n' "$status" | tee "$TMP_ROOT/results/agent-execute-preflight.status"
  cat "$TMP_ROOT/results/agent-execute-preflight.body"
  printf '\n'

  if [[ "$status" != "200" ]]; then
    echo "FAIL: expected Agent execute preflight HTTP 200, got $status" >&2
    exit 1
  fi
  if ! grep -Eq '"status"[[:space:]]*:[[:space:]]*"SUCCEEDED".*"value"[[:space:]]*:[[:space:]]*1|"value"[[:space:]]*:[[:space:]]*1.*"status"[[:space:]]*:[[:space:]]*"SUCCEEDED"' "$TMP_ROOT/results/agent-execute-preflight.body"; then
    echo "FAIL: Agent execute preflight response did not contain SUCCEEDED row value=1" >&2
    exit 1
  fi
  if grep -Eq 'password|token|secret' "$TMP_ROOT/results/agent-execute-preflight.body"; then
    echo "FAIL: Agent execute preflight response contained credential-shaped text" >&2
    exit 1
  fi
}

run_standalone_go_proof() {
  log "Running standalone PAR process -> real AgentSecure gRPC IPC -> Postgres -> fakeintake proof test"
  (
    cd "$AGENT_REPO"
    RQ_STANDALONE_PROOF=1 \
    RQ_STANDALONE_PAR_BIN="$AGENT_REPO/bin/privateactionrunner/privateactionrunner" \
    RQ_STANDALONE_AGENT_PID="$AGENT_PID" \
    RQ_STANDALONE_AGENT_CMD_PORT="$CMD_PORT" \
    RQ_STANDALONE_AGENT_AUTH_TOKEN_FILE="$TMP_ROOT/run/auth_token" \
    RQ_STANDALONE_AGENT_IPC_CERT_FILE="$TMP_ROOT/run/ipc_cert.pem" \
    RQ_STANDALONE_EVIDENCE_FILE="$TMP_ROOT/results/standalone-proof-evidence.txt" \
    dda inv test --targets=./pkg/privateactionrunner/bundles/remotequeries \
      --extra-args='-run TestRemoteQueriesActionRunsThroughStandalonePARProcessWithRealAgentIPC -count=1 -v'
  ) | tee "$TMP_ROOT/results/standalone-proof-test.log"
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
  call_agent_execute_preflight
  run_standalone_go_proof

  log "Sanitized standalone proof evidence"
  cat "$TMP_ROOT/results/standalone-proof-evidence.txt"

  log "Done. Sanitized artifacts left in $TMP_ROOT"
  log "Key evidence: fakeintake enqueue/dequeue/publish, standalone PAR PID, and real AgentSecure IPC evidence are in $TMP_ROOT/results/standalone-proof-evidence.txt"
}

main "$@"

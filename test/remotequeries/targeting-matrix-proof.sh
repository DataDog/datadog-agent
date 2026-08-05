#!/usr/bin/env bash
# Local Remote Queries targeting matrix proof.
#
# Topology:
#   - two local Agent processes with distinct hostnames and command API ports;
#   - four Postgres containers split across two Docker networks;
#   - two databases per Postgres container, each seeded with remote_query_identity.
#
# This harness proves the Agent's local Remote Queries match bridge routes by the
# full {host, port, dbname} tuple. It intentionally remains local and explicit:
# PAR registration/routing is documented as a follow-on assumption unless the
# optional standalone/fused PAR proof is run separately for a selected target.

set -euo pipefail

AGENT_REPO=${AGENT_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
INTEGRATIONS_CORE=${INTEGRATIONS_CORE:-/home/bits/dd/tasks/remote-queries-poc/worktrees/integrations-core}
TMP_ROOT=${TMP_ROOT:-/tmp/rq-targeting-matrix-proof}
POSTGRES_COMPOSE_FILE=${POSTGRES_COMPOSE_FILE:-$AGENT_REPO/test/remotequeries/targeting-matrix-compose.yaml}
POSTGRES_COMPOSE_PROJECT=${POSTGRES_COMPOSE_PROJECT:-rq-targeting-matrix-proof-$$}
POSTGRES_IMAGE=${POSTGRES_IMAGE:-13-alpine}
PIP_PLATFORM=${PIP_PLATFORM:-manylinux2014_x86_64}
AGENT_PYTHON_VERSION=${AGENT_PYTHON_VERSION:-}
AGENT_PYTHON_ABI=${AGENT_PYTHON_ABI:-}
AGENT_A_CMD_PORT=${AGENT_A_CMD_PORT:-55103}
AGENT_B_CMD_PORT=${AGENT_B_CMD_PORT:-55104}
RQ_POSTGRES_USERNAME=${RQ_POSTGRES_USERNAME:-bob}
RQ_POSTGRES_PASSWORD=${RQ_POSTGRES_PASSWORD:-bob}

POSTGRES_COMPOSE_STARTED=0
AGENT_A_PID=""
AGENT_B_PID=""

MATRIX_CASES=(
  "a rq-proof-agent-a localhost 15432 postgres_a1_db1"
  "a rq-proof-agent-a localhost 15432 postgres_a1_db2"
  "a rq-proof-agent-a localhost 15433 postgres_a2_db1"
  "a rq-proof-agent-a localhost 15433 postgres_a2_db2"
  "b rq-proof-agent-b localhost 25432 postgres_b1_db1"
  "b rq-proof-agent-b localhost 25432 postgres_b1_db2"
  "b rq-proof-agent-b localhost 25433 postgres_b2_db1"
  "b rq-proof-agent-b localhost 25433 postgres_b2_db2"
)

log() {
  printf '\n[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

docker_compose() {
  POSTGRES_IMAGE="$POSTGRES_IMAGE" docker compose -f "$POSTGRES_COMPOSE_FILE" -p "$POSTGRES_COMPOSE_PROJECT" "$@"
}

cleanup_agent_pid() {
  local pid=$1
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

cleanup_all() {
  cleanup_agent_pid "$AGENT_A_PID"
  cleanup_agent_pid "$AGENT_B_PID"
  if [[ "$POSTGRES_COMPOSE_STARTED" == "1" ]]; then
    docker_compose down --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup_all EXIT

python_minor_from_version() {
  local version=$1
  awk -F. '{print $1 "." $2}' <<<"$version"
}

abi_from_python_minor() {
  local minor=$1
  printf 'cp%s\n' "${minor//./}"
}

write_minimal_agent_config_for_python_detect() {
  mkdir -p "$TMP_ROOT/python-detect/run"
  cat > "$TMP_ROOT/python-detect/datadog.yaml" <<YAML
api_key: '00000000000000000000000000000000'
dd_url: http://127.0.0.1:9
hostname: rq-targeting-python-detect
cmd_host: 127.0.0.1
cmd_port: 55199
auth_token_file_path: $TMP_ROOT/python-detect/run/auth_token
ipc_cert_file_path: $TMP_ROOT/python-detect/run/ipc_cert.pem
log_file: $TMP_ROOT/python-detect/agent.log
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

  log "Detecting Python runtime from built Agent binary"
  write_minimal_agent_config_for_python_detect
  "$AGENT_REPO/bin/agent/agent" run -c "$TMP_ROOT/python-detect" \
    > "$TMP_ROOT/python-detect/stdout.log" 2> "$TMP_ROOT/python-detect/stderr.log" &
  local detect_pid=$!

  local detected=""
  for _ in $(seq 1 80); do
    detected=$(grep -aoE '"pythonV":"[0-9]+\.[0-9]+(\.[0-9]+)?' "$TMP_ROOT/python-detect/agent.log" 2>/dev/null | head -1 | cut -d: -f2 | tr -d '"' || true)
    if [[ -n "$detected" ]]; then
      break
    fi
    if ! kill -0 "$detect_pid" 2>/dev/null; then
      break
    fi
    sleep 0.5
  done
  cleanup_agent_pid "$detect_pid"

  if [[ -z "$detected" && -f "$AGENT_REPO/omnibus/config/software/python3.rb" ]]; then
    detected=$(sed -nE 's/^default_version "([0-9]+\.[0-9]+)(\.[0-9]+)?"/\1/p' "$AGENT_REPO/omnibus/config/software/python3.rb" | head -1)
  fi
  if [[ -z "$detected" ]]; then
    echo "Unable to detect Agent Python version. Set AGENT_PYTHON_VERSION=3.12 or AGENT_PYTHON_ABI=cp312." >&2
    tail -80 "$TMP_ROOT/python-detect/stderr.log" >&2 || true
    exit 1
  fi

  AGENT_PYTHON_VERSION=$(python_minor_from_version "$detected")
  AGENT_PYTHON_ABI=$(abi_from_python_minor "$AGENT_PYTHON_VERSION")
  log "Detected Agent Python runtime: $detected; installing wheels for $AGENT_PYTHON_VERSION ($AGENT_PYTHON_ABI)"
}

install_python_deps_for() {
  local python_version=$1 python_abi=$2 py_digits
  py_digits=${python_version//./}

  log "Installing temporary Python deps into $TMP_ROOT/pydeps for $python_abi"
  python3 -m pip install --quiet --upgrade --target "$TMP_ROOT/pydeps" \
    --only-binary=:all: --platform "$PIP_PLATFORM" \
    --implementation cp --python-version "$py_digits" --abi "$python_abi" \
    'psycopg[binary,pool]' cachetools packaging semver 'pydantic<3' python-dateutil mmh3 lazy_loader PyYAML
}

install_python_deps() {
  # Install any compatibility wheel set first and the detected Agent ABI last so
  # ABI-specific extension modules (for example pydantic_core) match the Agent's
  # embedded Python runtime when packages share the same target directory.
  if [[ "$AGENT_PYTHON_ABI" != "cp312" ]]; then
    install_python_deps_for "3.12" "cp312"
  fi
  install_python_deps_for "$AGENT_PYTHON_VERSION" "$AGENT_PYTHON_ABI"
}

setup_checksd() {
  mkdir -p "$TMP_ROOT/checks.d/datadog_checks" "$TMP_ROOT/pydeps"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/base" "$TMP_ROOT/checks.d/datadog_checks/base"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/checks" "$TMP_ROOT/checks.d/datadog_checks/checks"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/config.py" "$TMP_ROOT/checks.d/datadog_checks/config.py"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/errors.py" "$TMP_ROOT/checks.d/datadog_checks/errors.py"
  ln -s "$INTEGRATIONS_CORE/datadog_checks_base/datadog_checks/log.py" "$TMP_ROOT/checks.d/datadog_checks/log.py"
  ln -s "$INTEGRATIONS_CORE/postgres/datadog_checks/postgres" "$TMP_ROOT/checks.d/datadog_checks/postgres"
  cat > "$TMP_ROOT/checks.d/datadog_checks/__init__.py" <<'PY'
__path__ = __import__('pkgutil').extend_path(__path__, __name__)
PY
}

write_agent_config() {
  local side=$1 hostname=$2 cmd_port=$3
  local root="$TMP_ROOT/agent_$side"
  mkdir -p "$root/conf.d/postgres.d" "$root/run" "$root/results"
  cat > "$root/datadog.yaml" <<YAML
api_key: '00000000000000000000000000000000'
site: datadoghq.com
dd_url: http://127.0.0.1:9
hostname: $hostname
cmd_host: 127.0.0.1
cmd_port: $cmd_port
auth_token_file_path: $root/run/auth_token
ipc_cert_file_path: $root/run/ipc_cert.pem
confd_path: $root/conf.d
additional_checksd: $TMP_ROOT/checks.d
remote_queries.match_check.enabled: true
remote_queries.execute.enabled: true
remote_queries.execute.enable_query_allowlist: false
log_level: debug
log_file: $root/agent.log
python_lazy_loading: false
YAML
}

write_postgres_instances() {
  local side=$1 prefix1 prefix2 port1 port2
  local root="$TMP_ROOT/agent_$side"
  if [[ "$side" == "a" ]]; then
    prefix1=postgres_a1; prefix2=postgres_a2; port1=15432; port2=15433
  else
    prefix1=postgres_b1; prefix2=postgres_b2; port1=25432; port2=25433
  fi

  cat > "$root/conf.d/postgres.d/conf.yaml" <<YAML
init_config: {}
instances:
  - host: localhost
    port: $port1
    dbname: ${prefix1}_db1
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
    tags:
      - rq_database_instance:${prefix1}_db1
    database_identifier:
      template: \$rq_database_instance
  - host: localhost
    port: $port1
    dbname: ${prefix1}_db2
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
    tags:
      - rq_database_instance:${prefix1}_db2
    database_identifier:
      template: \$rq_database_instance
  - host: localhost
    port: $port2
    dbname: ${prefix2}_db1
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
    tags:
      - rq_database_instance:${prefix2}_db1
    database_identifier:
      template: \$rq_database_instance
  - host: localhost
    port: $port2
    dbname: ${prefix2}_db2
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
    tags:
      - rq_database_instance:${prefix2}_db2
    database_identifier:
      template: \$rq_database_instance
YAML
}

setup_tmp_tree() {
  rm -rf "$TMP_ROOT"
  mkdir -p "$TMP_ROOT/results"
  setup_checksd
  detect_agent_python
  install_python_deps
  # Re-link deps after pip populated pydeps.
  for dep in "$TMP_ROOT"/pydeps/*; do
    local base
    base=$(basename "$dep")
    [[ -e "$TMP_ROOT/checks.d/$base" ]] || ln -s "$dep" "$TMP_ROOT/checks.d/$base"
  done
  write_agent_config a rq-proof-agent-a "$AGENT_A_CMD_PORT"
  write_agent_config b rq-proof-agent-b "$AGENT_B_CMD_PORT"
  write_postgres_instances a
  write_postgres_instances b
}

start_postgres_fixture() {
  log "Starting Postgres targeting matrix fixture project=$POSTGRES_COMPOSE_PROJECT"
  docker_compose down --remove-orphans >/dev/null 2>&1 || true
  docker_compose up -d
  POSTGRES_COMPOSE_STARTED=1

  for _ in $(seq 1 120); do
    if docker_compose ps --format json 2>/dev/null | python3 -c 'import json,sys; rows=[json.loads(l) for l in sys.stdin if l.strip()]; sys.exit(0 if rows and all(r.get("Health") in ("healthy", "") for r in rows) else 1)' 2>/dev/null; then
      log "Postgres matrix fixture is healthy"
      return
    fi
    sleep 1
  done
  docker_compose ps >&2 || true
  docker_compose logs --tail=120 >&2 || true
  echo "Postgres targeting matrix fixture did not become healthy" >&2
  exit 1
}

start_agent() {
  local side=$1
  local root="$TMP_ROOT/agent_$side"
  : > "$root/agent.log"
  PYTHONPATH="$TMP_ROOT/checks.d:$TMP_ROOT/pydeps" \
    "$AGENT_REPO/bin/agent/agent" run -c "$root" \
    > "$root/stdout.log" 2> "$root/stderr.log" &
  local pid=$!
  if [[ "$side" == "a" ]]; then
    AGENT_A_PID=$pid
  else
    AGENT_B_PID=$pid
  fi

  for _ in $(seq 1 100); do
    local loaded_count
    loaded_count=$(grep -c "successfully loaded check 'postgres'" "$root/agent.log" 2>/dev/null || true)
    if [[ "$loaded_count" -ge 4 && -f "$root/run/auth_token" && -f "$root/run/ipc_cert.pem" ]]; then
      log "Agent $side loaded $loaded_count Postgres checks and exposed IPC artifacts"
      return
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "Agent $side exited early; stderr follows:" >&2
      tail -100 "$root/stderr.log" >&2 || true
      exit 1
    fi
    sleep 0.5
  done

  echo "Timed out waiting for Agent $side to load four Postgres checks" >&2
  grep -ni 'postgres\|remote_queries\|ModuleNotFound\|ImportError\|unable to load check\|AttributeError' "$root/agent.log" >&2 || true
  exit 1
}

start_agents() {
  start_agent a
  start_agent b
}

agent_cmd_port() {
  if [[ "$1" == "a" ]]; then
    echo "$AGENT_A_CMD_PORT"
  else
    echo "$AGENT_B_CMD_PORT"
  fi
}

call_match_check() {
  local side=$1 host=$2 port=$3 dbname=$4 out_prefix=$5
  local root="$TMP_ROOT/agent_$side" token payload status
  token=$(cat "$root/run/auth_token")
  payload=$(HOST="$host" PORT="$port" DBNAME="$dbname" python3 - <<'PY'
import json, os
print(json.dumps({
    "integration": "postgres",
    "target": {"host": os.environ["HOST"], "port": int(os.environ["PORT"]), "dbname": os.environ["DBNAME"]},
}))
PY
)
  status=$(curl -sS -k -o "$out_prefix.body" -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    "https://127.0.0.1:$(agent_cmd_port "$side")/agent/remote-queries/match-check")
  printf '%s' "$status" > "$out_prefix.status"
}

call_match_check_dbi() {
  local side=$1 database_instance=$2 out_prefix=$3
  local root="$TMP_ROOT/agent_$side" token payload status
  token=$(cat "$root/run/auth_token")
  payload=$(DATABASE_INSTANCE="$database_instance" python3 - <<'PY'
import json, os
print(json.dumps({
    "integration": "postgres",
    "target": {"database_instance": os.environ["DATABASE_INSTANCE"]},
}))
PY
)
  status=$(curl -sS -k -o "$out_prefix.body" -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    "https://127.0.0.1:$(agent_cmd_port "$side")/agent/remote-queries/match-check")
  printf '%s' "$status" > "$out_prefix.status"
}

assert_match_status() {
  local out_prefix=$1 expected_http=$2 expected_status=$3 label=$4
  local actual_http actual_status
  actual_http=$(cat "$out_prefix.status")
  actual_status=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("status"))' "$out_prefix.body")
  if [[ "$actual_http" != "$expected_http" || "$actual_status" != "$expected_status" ]]; then
    echo "FAIL $label: expected HTTP $expected_http/$expected_status, got HTTP $actual_http/$actual_status" >&2
    cat "$out_prefix.body" >&2
    exit 1
  fi
}

run_positive_match_matrix() {
  log "Checking all positive Agent-local match cases"
  : > "$TMP_ROOT/results/positive-match-cases.tsv"
  local i=0
  for row in "${MATRIX_CASES[@]}"; do
    read -r side expected_agent host port dbname <<<"$row"
    local out="$TMP_ROOT/results/positive-$i-$side-$dbname"
    call_match_check "$side" "$host" "$port" "$dbname" "$out"
    assert_match_status "$out" 200 ok "positive $side $host:$port/$dbname"
    printf '%s\t%s\t%s\t%s\t%s\tOK\n' "$side" "$expected_agent" "$host" "$port" "$dbname" >> "$TMP_ROOT/results/positive-match-cases.tsv"
    i=$((i + 1))
  done
}

run_positive_dbi_match_matrix() {
  log "Checking positive Agent-local database_instance match cases"
  : > "$TMP_ROOT/results/positive-dbi-match-cases.tsv"
  local i=0
  for row in "${MATRIX_CASES[@]}"; do
    read -r side expected_agent _host _port dbname <<<"$row"
    local out="$TMP_ROOT/results/positive-dbi-$i-$side-$dbname"
    call_match_check_dbi "$side" "$dbname" "$out"
    assert_match_status "$out" 200 ok "positive database_instance $side $dbname"
    printf '%s\t%s\t%s\tOK\n' "$side" "$expected_agent" "$dbname" >> "$TMP_ROOT/results/positive-dbi-match-cases.tsv"
    i=$((i + 1))
  done
}

run_negative_dbi_match_matrix() {
  log "Checking negative fail-closed database_instance match cases: wrong Agent and invented identifier"
  : > "$TMP_ROOT/results/negative-dbi-match-cases.tsv"
  local i=0
  for row in "${MATRIX_CASES[@]}"; do
    read -r side _expected_agent _host _port dbname <<<"$row"
    local wrong_side out
    if [[ "$side" == "a" ]]; then wrong_side=b; else wrong_side=a; fi

    out="$TMP_ROOT/results/negative-dbi-$i-wrong-agent-$wrong_side-$dbname"
    call_match_check_dbi "$wrong_side" "$dbname" "$out"
    assert_match_status "$out" 404 target_not_found "negative database_instance wrong-agent $wrong_side $dbname"
    printf 'wrong-agent\t%s\t%s\ttarget_not_found\n' "$wrong_side" "$dbname" >> "$TMP_ROOT/results/negative-dbi-match-cases.tsv"
    i=$((i + 1))
  done

  out="$TMP_ROOT/results/negative-dbi-invented"
  call_match_check_dbi a "rq-proof-invented-db-instance" "$out"
  assert_match_status "$out" 404 target_not_found "negative invented database_instance"
  printf 'invented\ta\trq-proof-invented-db-instance\ttarget_not_found\n' >> "$TMP_ROOT/results/negative-dbi-match-cases.tsv"
}

run_negative_match_matrix() {
  log "Checking negative fail-closed match cases: wrong Agent, wrong port, wrong dbname"
  : > "$TMP_ROOT/results/negative-match-cases.tsv"
  local i=0
  for row in "${MATRIX_CASES[@]}"; do
    read -r side _hostlabel host port dbname <<<"$row"
    local wrong_side wrong_port wrong_db prefix out
    if [[ "$side" == "a" ]]; then wrong_side=b; else wrong_side=a; fi

    out="$TMP_ROOT/results/negative-$i-wrong-agent-$wrong_side-$dbname"
    call_match_check "$wrong_side" "$host" "$port" "$dbname" "$out"
    assert_match_status "$out" 404 target_not_found "negative wrong-agent $wrong_side $host:$port/$dbname"
    printf 'wrong-agent\t%s\t%s\t%s\t%s\ttarget_not_found\n' "$wrong_side" "$host" "$port" "$dbname" >> "$TMP_ROOT/results/negative-match-cases.tsv"

    wrong_port=$((port + 1000))
    out="$TMP_ROOT/results/negative-$i-wrong-port-$side-$dbname"
    call_match_check "$side" "$host" "$wrong_port" "$dbname" "$out"
    assert_match_status "$out" 404 target_not_found "negative wrong-port $side $host:$wrong_port/$dbname"
    printf 'wrong-port\t%s\t%s\t%s\t%s\ttarget_not_found\n' "$side" "$host" "$wrong_port" "$dbname" >> "$TMP_ROOT/results/negative-match-cases.tsv"

    prefix=${dbname%_db[12]}
    wrong_db="${prefix}_db_missing"
    out="$TMP_ROOT/results/negative-$i-wrong-dbname-$side-$wrong_db"
    call_match_check "$side" "$host" "$port" "$wrong_db" "$out"
    assert_match_status "$out" 404 target_not_found "negative wrong-dbname $side $host:$port/$wrong_db"
    printf 'wrong-dbname\t%s\t%s\t%s\t%s\ttarget_not_found\n' "$side" "$host" "$port" "$wrong_db" >> "$TMP_ROOT/results/negative-match-cases.tsv"
    i=$((i + 1))
  done
}

assert_identity_rows() {
  log "Querying remote_query_identity in all eight databases via psql fixture smoke"
  : > "$TMP_ROOT/results/identity-rows.tsv"
  for row in "${MATRIX_CASES[@]}"; do
    read -r _side expected_agent host port dbname <<<"$row"
    local expected_marker actual
    expected_marker="${expected_agent}|${host}|${port}|${dbname}"
    actual=$(PGPASSWORD="$RQ_POSTGRES_PASSWORD" psql -X -A -t \
      -h "$host" -p "$port" -U "$RQ_POSTGRES_USERNAME" -d "$dbname" \
      -F $'\t' \
      -c "SELECT expected_agent_hostname, expected_postgres_host, expected_postgres_port, expected_dbname, marker FROM remote_query_identity")
    local expected
    expected=$(printf '%s\t%s\t%s\t%s\t%s' "$expected_agent" "$host" "$port" "$dbname" "$expected_marker")
    if [[ "$actual" != "$expected" ]]; then
      echo "FAIL identity $host:$port/$dbname" >&2
      echo "expected: $expected" >&2
      echo "actual:   $actual" >&2
      exit 1
    fi
    printf '%s\n' "$actual" >> "$TMP_ROOT/results/identity-rows.tsv"
  done
}

write_summary() {
  cat > "$TMP_ROOT/results/README.txt" <<TXT
Remote Queries targeting matrix proof artifacts

Topology:
- Agent A: hostname rq-proof-agent-a, cmd port $AGENT_A_CMD_PORT, configured for localhost:15432 postgres_a1_db{1,2} and localhost:15433 postgres_a2_db{1,2}
- Agent B: hostname rq-proof-agent-b, cmd port $AGENT_B_CMD_PORT, configured for localhost:25432 postgres_b1_db{1,2} and localhost:25433 postgres_b2_db{1,2}
- Docker networks: ${POSTGRES_COMPOSE_PROJECT}_agent_a and ${POSTGRES_COMPOSE_PROJECT}_agent_b

Covered:
- positives: host/port/dbname selectors in positive-match-cases.tsv; database_instance selectors in positive-dbi-match-cases.tsv
- negatives: wrong Agent, wrong port, wrong dbname for each valid tuple in negative-match-cases.tsv; database_instance fail-closed cases in negative-dbi-match-cases.tsv
- fixture identity rows: see identity-rows.tsv

Note: this harness validates Agent-local match targeting and fixture identity. It does not register two production PAR runners/connections.
TXT
}

main() {
  require_cmd docker
  require_cmd psql
  require_cmd python3
  require_cmd curl

  [[ -x "$AGENT_REPO/bin/agent/agent" ]] || {
    echo "Agent binary not found/executable: $AGENT_REPO/bin/agent/agent" >&2
    echo "Build it first with: dda inv agent.build --build-exclude=systemd" >&2
    exit 1
  }
  [[ -d "$INTEGRATIONS_CORE/postgres/datadog_checks/postgres" ]] || {
    echo "Postgres integration not found under: $INTEGRATIONS_CORE" >&2
    exit 1
  }

  log "Preparing targeting matrix harness at $TMP_ROOT"
  setup_tmp_tree
  start_postgres_fixture
  assert_identity_rows
  start_agents
  run_positive_match_matrix
  run_positive_dbi_match_matrix
  run_negative_match_matrix
  run_negative_dbi_match_matrix
  write_summary

  log "Done. Sanitized results are under $TMP_ROOT/results"
}

main "$@"

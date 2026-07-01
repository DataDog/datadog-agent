#!/usr/bin/env bash
# AP/PAR/Agent Remote Queries targeting matrix proof.
#
# Starts the same two-Agent/four-Postgres matrix as targeting-matrix-proof.sh,
# then starts one self-enrolled private-action-runner per Agent. The script
# discovers the two AP RemoteAction connection IDs and executes the AP action
# against every valid target tuple. The allowed fixture query returns a unique
# marker from each database's cities table, proving the AP request reached the
# exact DB. An invented DB target is asserted to fail closed.

set -euo pipefail

AGENT_REPO=${AGENT_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
INTEGRATIONS_CORE=${INTEGRATIONS_CORE:-/home/bits/dd/tasks/remote-queries-poc/worktrees/integrations-core}
TMP_ROOT=${TMP_ROOT:-/tmp/rq-ap-targeting-matrix-proof}
POSTGRES_COMPOSE_FILE=${POSTGRES_COMPOSE_FILE:-$AGENT_REPO/test/remotequeries/targeting-matrix-compose.yaml}
POSTGRES_COMPOSE_PROJECT=${POSTGRES_COMPOSE_PROJECT:-rq-ap-targeting-matrix-proof-$$}
POSTGRES_IMAGE=${POSTGRES_IMAGE:-13-alpine}
PIP_PLATFORM=${PIP_PLATFORM:-manylinux2014_x86_64}
AGENT_PYTHON_VERSION=${AGENT_PYTHON_VERSION:-}
AGENT_PYTHON_ABI=${AGENT_PYTHON_ABI:-}
AGENT_A_CMD_PORT=${AGENT_A_CMD_PORT:-55203}
AGENT_B_CMD_PORT=${AGENT_B_CMD_PORT:-55204}
RQ_POSTGRES_USERNAME=${RQ_POSTGRES_USERNAME:-bob}
RQ_POSTGRES_PASSWORD=${RQ_POSTGRES_PASSWORD:-bob}
FQN=${FQN:-com.datadoghq.remoteaction.queries.execute}
RQ_REMOTE_QUERY=${RQ_REMOTE_QUERY:-SELECT city, country FROM cities ORDER BY city}
RQ_POLL_INTERVAL=${RQ_POLL_INTERVAL:-1}
RQ_MAX_POLLS=${RQ_MAX_POLLS:-45}
DD_SITE=${DD_SITE:-datad0g.com}
KEEP_ALIVE=${KEEP_ALIVE:-0}

POSTGRES_COMPOSE_STARTED=0
AGENT_A_PID=""
AGENT_B_PID=""
PAR_A_PID=""
PAR_B_PID=""
CONNECTION_A=""
CONNECTION_B=""

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

log() { printf '\n[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" | tee -a "$TMP_ROOT/results/harness.log"; }
require_cmd() { command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }; }

usage() {
  cat <<'TXT'
Usage: ap-targeting-matrix-proof.sh [--keep-alive]

Options:
  --keep-alive   Set up the Agent/PAR/Postgres/AP connection matrix, write
                 connection/PID metadata under $TMP_ROOT/results, print a
                 KEEPALIVE_READY marker, and block until SIGINT/SIGTERM.
                 Equivalent: KEEP_ALIVE=1 ap-targeting-matrix-proof.sh
TXT
}

parse_args() {
  while (($#)); do
    case "$1" in
      --keep-alive)
        KEEP_ALIVE=1
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "unknown argument: $1" >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done
}

docker_compose() { POSTGRES_IMAGE="$POSTGRES_IMAGE" docker compose -f "$POSTGRES_COMPOSE_FILE" -p "$POSTGRES_COMPOSE_PROJECT" "$@"; }

cleanup_pid() {
  local pid=$1
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}

cleanup_all() {
  cleanup_pid "$PAR_A_PID"
  cleanup_pid "$PAR_B_PID"
  cleanup_pid "$AGENT_A_PID"
  cleanup_pid "$AGENT_B_PID"
  if [[ "$POSTGRES_COMPOSE_STARTED" == "1" ]]; then
    docker_compose down --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup_all EXIT

python_minor_from_version() { awk -F. '{print $1 "." $2}' <<<"$1"; }
abi_from_python_minor() { printf 'cp%s\n' "${1//./}"; }

write_minimal_agent_config_for_python_detect() {
  mkdir -p "$TMP_ROOT/python-detect/run"
  cat > "$TMP_ROOT/python-detect/datadog.yaml" <<YAML
api_key: '00000000000000000000000000000000'
dd_url: http://127.0.0.1:9
hostname: rq-ap-targeting-python-detect
cmd_host: 127.0.0.1
cmd_port: 55299
auth_token_file_path: $TMP_ROOT/python-detect/run/auth_token
ipc_cert_file_path: $TMP_ROOT/python-detect/run/ipc_cert.pem
log_file: $TMP_ROOT/python-detect/agent.log
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
    log "Using overridden Agent Python ABI: $AGENT_PYTHON_ABI"
    return
  fi
  if [[ -n "$AGENT_PYTHON_VERSION" ]]; then
    AGENT_PYTHON_VERSION=$(python_minor_from_version "$AGENT_PYTHON_VERSION")
    AGENT_PYTHON_ABI=$(abi_from_python_minor "$AGENT_PYTHON_VERSION")
    log "Using overridden Agent Python version: $AGENT_PYTHON_VERSION ($AGENT_PYTHON_ABI)"
    return
  fi

  log "Detecting Python runtime from built Agent binary"
  write_minimal_agent_config_for_python_detect
  "$AGENT_REPO/bin/agent/agent" run -c "$TMP_ROOT/python-detect" > "$TMP_ROOT/python-detect/stdout.log" 2> "$TMP_ROOT/python-detect/stderr.log" &
  local detect_pid=$! detected=""
  for _ in $(seq 1 80); do
    detected=$(grep -aoE '"pythonV":"[0-9]+\.[0-9]+(\.[0-9]+)?' "$TMP_ROOT/python-detect/agent.log" 2>/dev/null | head -1 | cut -d: -f2 | tr -d '"' || true)
    [[ -n "$detected" ]] && break
    kill -0 "$detect_pid" 2>/dev/null || break
    sleep 0.5
  done
  cleanup_pid "$detect_pid"
  if [[ -z "$detected" && -f "$AGENT_REPO/omnibus/config/software/python3.rb" ]]; then
    detected=$(sed -nE 's/^default_version "([0-9]+\.[0-9]+)(\.[0-9]+)?"/\1/p' "$AGENT_REPO/omnibus/config/software/python3.rb" | head -1)
  fi
  [[ -n "$detected" ]] || { echo "Unable to detect Agent Python version. Set AGENT_PYTHON_VERSION or AGENT_PYTHON_ABI." >&2; exit 1; }
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
  install_python_deps_for "$AGENT_PYTHON_VERSION" "$AGENT_PYTHON_ABI"
  if [[ "$AGENT_PYTHON_ABI" != "cp312" ]]; then
    install_python_deps_for "3.12" "cp312"
  fi
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

link_pydeps() {
  for dep in "$TMP_ROOT"/pydeps/*; do
    local base
    base=$(basename "$dep")
    [[ -e "$TMP_ROOT/checks.d/$base" ]] || ln -s "$dep" "$TMP_ROOT/checks.d/$base"
  done
}

write_agent_config() {
  local side=$1 hostname=$2 cmd_port=$3
  local root="$TMP_ROOT/agent_$side"
  mkdir -p "$root/conf.d/postgres.d" "$root/run" "$root/results"
  cat > "$root/datadog.yaml" <<YAML
api_key: "$DD_API_KEY"
app_key: "$DD_APP_KEY"
site: datad0g.com
dd_url: https://app.datad0g.com
hostname: $hostname
cmd_host: 127.0.0.1
cmd_port: $cmd_port
auth_token_file_path: $root/run/auth_token
ipc_cert_file_path: $root/run/ipc_cert.pem
confd_path: $root/conf.d
additional_checksd: $TMP_ROOT/checks.d
remote_queries.match_check.enabled: true
remote_queries.execute.enabled: true
log_level: debug
log_file: $root/agent.log
python_lazy_loading: false
telemetry.enabled: false
inventories_enabled: false
process_config.enabled: 'false'
logs_enabled: false
apm_config.enabled: false
YAML
}

write_postgres_instances() {
  local side=$1 prefix1 prefix2 port1 port2
  local root="$TMP_ROOT/agent_$side"
  if [[ "$side" == "a" ]]; then prefix1=postgres_a1; prefix2=postgres_a2; port1=15432; port2=15433; else prefix1=postgres_b1; prefix2=postgres_b2; port1=25432; port2=25433; fi
  cat > "$root/conf.d/postgres.d/conf.yaml" <<YAML
init_config: {}
instances:
  - host: localhost
    port: $port1
    dbname: ${prefix1}_db1
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
  - host: localhost
    port: $port1
    dbname: ${prefix1}_db2
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
  - host: localhost
    port: $port2
    dbname: ${prefix2}_db1
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
  - host: localhost
    port: $port2
    dbname: ${prefix2}_db2
    username: $RQ_POSTGRES_USERNAME
    password: $RQ_POSTGRES_PASSWORD
YAML
}

write_par_config() {
  local side=$1 hostname=$2 cmd_port=$3
  local root="$TMP_ROOT/agent_$side" par_root="$TMP_ROOT/par_$side"
  mkdir -p "$par_root"
  cat > "$par_root/datadog.yaml" <<YAML
api_key: "$DD_API_KEY"
app_key: "$DD_APP_KEY"
site: datad0g.com
hostname: $hostname
cmd_host: 127.0.0.1
cmd_port: $cmd_port
auth_token_file_path: $root/run/auth_token
ipc_cert_file_path: $root/run/ipc_cert.pem
log_level: debug
telemetry.enabled: false
inventories_enabled: false
process_config.enabled: 'false'
logs_enabled: false
apm_config.enabled: false
private_action_runner:
  enabled: true
  self_enroll: true
  log_file: $par_root/private-action-runner.log
  default_actions_enabled: false
  actions_allowlist:
    - $FQN
  task_concurrency: 1
  task_timeout_seconds: 120
YAML
}

setup_tmp_tree() {
  rm -rf "$TMP_ROOT"
  mkdir -p "$TMP_ROOT/results"
  setup_checksd
  detect_agent_python
  install_python_deps
  link_pydeps
  write_agent_config a rq-proof-agent-a "$AGENT_A_CMD_PORT"
  write_agent_config b rq-proof-agent-b "$AGENT_B_CMD_PORT"
  write_postgres_instances a
  write_postgres_instances b
  write_par_config a rq-proof-agent-a "$AGENT_A_CMD_PORT"
  write_par_config b rq-proof-agent-b "$AGENT_B_CMD_PORT"
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
  local side=$1 pid loaded_count
  local root="$TMP_ROOT/agent_$side"
  : > "$root/agent.log"
  PYTHONPATH="$TMP_ROOT/checks.d:$TMP_ROOT/pydeps" "$AGENT_REPO/bin/agent/agent" run -c "$root" > "$root/stdout.log" 2> "$root/stderr.log" &
  pid=$!
  if [[ "$side" == "a" ]]; then AGENT_A_PID=$pid; else AGENT_B_PID=$pid; fi
  for _ in $(seq 1 120); do
    loaded_count=$(grep -c "successfully loaded check 'postgres'" "$root/agent.log" 2>/dev/null || true)
    if [[ "$loaded_count" -ge 4 && -f "$root/run/auth_token" && -f "$root/run/ipc_cert.pem" ]]; then
      log "Agent $side loaded $loaded_count Postgres checks and exposed IPC artifacts"
      return
    fi
    kill -0 "$pid" 2>/dev/null || { echo "Agent $side exited early" >&2; tail -100 "$root/stderr.log" >&2 || true; exit 1; }
    sleep 0.5
  done
  echo "Timed out waiting for Agent $side" >&2
  grep -ni 'postgres\|ModuleNotFound\|unable to load' "$root/agent.log" >&2 || true
  exit 1
}

start_par() {
  local side=$1 pid
  local par_root="$TMP_ROOT/par_$side"
  DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true "$AGENT_REPO/bin/privateactionrunner/privateactionrunner" run -c "$par_root" > "$par_root/stdout.log" 2> "$par_root/stderr.log" &
  pid=$!
  if [[ "$side" == "a" ]]; then PAR_A_PID=$pid; else PAR_B_PID=$pid; fi
  for _ in $(seq 1 120); do
    if grep -q "Self-enrollment successful" "$par_root/private-action-runner.log" 2>/dev/null; then
      log "PAR $side self-enrollment successful"
      return
    fi
    kill -0 "$pid" 2>/dev/null || { echo "PAR $side exited early" >&2; tail -100 "$par_root/stderr.log" >&2 || true; tail -100 "$par_root/private-action-runner.log" >&2 || true; exit 1; }
    sleep 0.5
  done
  echo "Timed out waiting for PAR $side self-enrollment" >&2
  tail -120 "$par_root/private-action-runner.log" >&2 || true
  exit 1
}

api_get_connections() {
  local out=$1 status
  status=$(curl -sS -o "$out" -w '%{http_code}' \
    -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" \
    "https://api.$DD_SITE/api/v2/actions/connections?page%5Bsize%5D=100&page%5Bnumber%5D=0&include=private_actions_runner&filter%5Bintegration%5D=REMOTE_ACTION")
  [[ "$status" == "200" ]] || { echo "connection list HTTP $status" >&2; cat "$out" >&2; return 1; }
}

api_get_runners() {
  local out=$1 status
  status=$(curl -sS -o "$out" -w '%{http_code}' \
    -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" \
    "https://api.$DD_SITE/api/v2/on_prem_runners?page%5Bsize%5D=100&page%5Bnumber%5D=0")
  [[ "$status" == "200" ]] || { echo "runner list HTTP $status" >&2; cat "$out" >&2; return 1; }
}

api_get_execution_groups() {
  local out=$1 status
  status=$(curl -sS -o "$out" -w '%{http_code}' \
    -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" \
    "https://api.$DD_SITE/api/v2/actions/execution-groups?page%5Bsize%5D=100&page%5Bnumber%5D=0")
  [[ "$status" == "200" ]] || { echo "execution-group list HTTP $status" >&2; cat "$out" >&2; return 1; }
}

api_delete_resource() {
  local path=$1 out=$2 status
  status=$(curl -sS -o "$out" -w '%{http_code}' \
    -X DELETE \
    -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" \
    "https://api.$DD_SITE$path")
  [[ "$status" == "200" || "$status" == "202" || "$status" == "204" ]] || { echo "delete $path HTTP $status" >&2; cat "$out" >&2; return 1; }
}

delete_temp_execution_group_if_present() {
  local out="$TMP_ROOT/results/precleanup-execution-groups.json" ids id
  log "Deleting stale rq-ap-staging-proof-eg execution group if present"
  api_get_execution_groups "$out"
  mapfile -t ids < <(jq -r '.data[] | select(.attributes.name == "rq-ap-staging-proof-eg") | .id' "$out")
  for id in "${ids[@]}"; do
    [[ -n "$id" && "$id" != "null" ]] || continue
    api_delete_resource "/api/v2/actions/execution-groups/$id" "$TMP_ROOT/results/precleanup-execution-group-$id.json"
    log "deleted stale execution group $id"
  done
}

cleanup_ap_resources_for_hostname() {
  local hostname=$1 out ids id
  log "Deleting stale AP RemoteAction resources for hostname=$hostname before proof"
  out="$TMP_ROOT/results/precleanup-connections-$hostname.json"
  api_get_connections "$out"
  mapfile -t ids < <(jq -r --arg host "$hostname" '.data[] | select((.attributes.tags // []) | index("hostname:" + $host)) | .id' "$out")
  for id in "${ids[@]}"; do
    [[ -n "$id" && "$id" != "null" ]] || continue
    api_delete_resource "/api/v2/actions/connections/$id" "$TMP_ROOT/results/precleanup-connection-$id.json"
    log "deleted stale connection $id for hostname=$hostname"
  done

  out="$TMP_ROOT/results/precleanup-runners-$hostname.json"
  api_get_runners "$out"
  mapfile -t ids < <(jq -r --arg host "$hostname" '.data[] | select((.attributes.agentHostname // .attributes.agent_hostname // .attributes.hostname // .attributes.name // "") | contains($host)) | .id' "$out")
  for id in "${ids[@]}"; do
    [[ -n "$id" && "$id" != "null" ]] || continue
    api_delete_resource "/api/v2/on_prem_runners/$id" "$TMP_ROOT/results/precleanup-runner-$id.json"
    log "deleted stale runner $id for hostname=$hostname"
  done
}

resolve_connection_for_hostname() {
  local hostname=$1 matches
  local out="$TMP_ROOT/results/connections-$hostname.json"
  for _ in $(seq 1 60); do
    api_get_connections "$out"
    jq --arg host "$hostname" '[.data[] | select((.attributes.tags // []) | index("hostname:" + $host)) | {id, name:.attributes.name, tags:.attributes.tags, runner:.relationships.private_actions_runner.data.id}]' "$out" > "$TMP_ROOT/results/connection-matches-$hostname.json"
    matches=$(jq 'length' "$TMP_ROOT/results/connection-matches-$hostname.json")
    if [[ "$matches" == "1" ]]; then
      jq -r '.[0].id' "$TMP_ROOT/results/connection-matches-$hostname.json"
      return
    fi
    sleep 2
  done
  echo "expected exactly one AP RemoteAction connection for hostname=$hostname" >&2
  cat "$TMP_ROOT/results/connection-matches-$hostname.json" >&2 || true
  exit 1
}

start_agents_and_pars() {
  start_agent a
  start_agent b
  start_par a
  start_par b
  sleep 8
  CONNECTION_A=$(resolve_connection_for_hostname rq-proof-agent-a)
  CONNECTION_B=$(resolve_connection_for_hostname rq-proof-agent-b)
  log "AP connection for agent a: $CONNECTION_A"
  log "AP connection for agent b: $CONNECTION_B"
  printf 'a\t%s\n' "$CONNECTION_A" > "$TMP_ROOT/results/connections.tsv"
  printf 'b\t%s\n' "$CONNECTION_B" >> "$TMP_ROOT/results/connections.tsv"
}

connection_for_side() { if [[ "$1" == "a" ]]; then echo "$CONNECTION_A"; else echo "$CONNECTION_B"; fi; }
expected_data_for() { printf '%s|%s|%s|%s,%s\n' "$1" "$2" "$3" "$4" "$1"; }

ap_execute_case() {
  local label=$1 side=$2 expected_agent=$3 host=$4 port=$5 dbname=$6 expect_success=$7
  local cid run_dir status task_id st action_status final_json data expected
  cid=$(connection_for_side "$side")
  run_dir="$TMP_ROOT/results/ap-$label"
  mkdir -p "$run_dir"
  jq -n --arg query "$RQ_REMOTE_QUERY" --arg host "$host" --arg dbname "$dbname" --argjson port "$port" \
    '{integration:"postgres",operation:"copy_stream",format:"csv",target:{host:$host,port:$port,dbname:$dbname},query:$query,copyLimits:{chunkBytes:262144,timeoutMs:5000,maxBytes:4096,maxRowBytes:4096}}' > "$run_dir/input.json"
  jq -n --arg fqn "$FQN" --arg cid "$cid" --slurpfile inputs "$run_dir/input.json" \
    '{data:{type:"action_execution",attributes:{fqn:$fqn,connection_id:$cid,inputs:$inputs[0]}}}' > "$run_dir/execute.req.json"

  status=$(curl -sS -o "$run_dir/execute.json" -w '%{http_code}' \
    -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" -H 'Content-Type: application/json' \
    --data @"$run_dir/execute.req.json" "https://api.$DD_SITE/api/unstable/actions/execute")
  echo "$status" > "$run_dir/execute.status"
  if [[ "$status" != "200" && "$status" != "202" ]]; then
    [[ "$expect_success" == "0" ]] && return 0
    echo "AP execute $label HTTP $status" >&2; cat "$run_dir/execute.json" >&2; exit 1
  fi
  task_id=$(jq -r '.data.id // empty' "$run_dir/execute.json")
  [[ -n "$task_id" ]] || { echo "AP execute $label missing task id" >&2; cat "$run_dir/execute.json" >&2; exit 1; }
  for i in $(seq 1 "$RQ_MAX_POLLS"); do
    sleep "$RQ_POLL_INTERVAL"
    status=$(curl -sS -o "$run_dir/poll.$i.json" -w '%{http_code}' \
      -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" \
      "https://api.$DD_SITE/api/unstable/actions/execute/$task_id")
    echo "$status" > "$run_dir/poll.$i.status"
    st=$(jq -r '.data.attributes.status // empty' "$run_dir/poll.$i.json" 2>/dev/null || true)
    [[ "$st" == "FAILED" || "$st" == "SUCCEEDED" || "$st" == "COMPLETED" ]] && break
  done
  final_json=$(find "$run_dir" -maxdepth 1 -name 'poll.*.json' -print 2>/dev/null | sort -V | tail -1)
  [[ -n "$final_json" ]] || { echo "AP execute $label had no poll result" >&2; exit 1; }
  jq '{task_file: input_filename, status:.data.attributes.status, output:(.data.attributes.output // .data.attributes.result // .data.attributes.outputs // .data.attributes)}' "$final_json" > "$run_dir/final-summary.json"
  data=$(jq -r '.data.attributes.output.data // .data.attributes.result.data // .data.attributes.outputs.data // empty' "$final_json")
  printf '%s' "$data" > "$run_dir/final-data.txt"
  st=$(jq -r '.data.attributes.status // empty' "$final_json")
  action_status=$(jq -r '.data.attributes.output.status // .data.attributes.result.status // .data.attributes.outputs.status // empty' "$final_json")

  if [[ "$expect_success" == "1" ]]; then
    expected=$(expected_data_for "$expected_agent" "$host" "$port" "$dbname")
    if [[ "$st" != "SUCCEEDED" || "$action_status" != "SUCCEEDED" || "$data" != "$expected" ]]; then
      echo "AP positive $label failed: platform_status=$st action_status=$action_status" >&2
      echo "expected data: $expected" >&2
      echo "actual data:   $data" >&2
      cat "$run_dir/final-summary.json" >&2
      exit 1
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$side" "$cid" "$host" "$port" "$dbname" "$data" >> "$TMP_ROOT/results/ap-positive-hits.tsv"
  else
    if [[ "$st" == "SUCCEEDED" && "$action_status" == "SUCCEEDED" ]]; then
      echo "AP negative $label unexpectedly succeeded" >&2
      cat "$run_dir/final-summary.json" >&2
      exit 1
    fi
    printf '%s\t%s\t%s\t%s\t%s\t%s/%s\n' "$side" "$cid" "$host" "$port" "$dbname" "$st" "$action_status" >> "$TMP_ROOT/results/ap-negative-cases.tsv"
  fi
}

run_ap_matrix() {
  log "Running AP positive matrix until all eight DB identity strings are hit"
  : > "$TMP_ROOT/results/ap-positive-hits.tsv"
  local i=0
  for row in "${MATRIX_CASES[@]}"; do
    read -r side expected_agent host port dbname <<<"$row"
    ap_execute_case "positive-$i-$side-$dbname" "$side" "$expected_agent" "$host" "$port" "$dbname" 1
    i=$((i + 1))
  done
  local hit_count
  hit_count=$(cut -f5 "$TMP_ROOT/results/ap-positive-hits.tsv" | sort -u | wc -l | tr -d ' ')
  [[ "$hit_count" == "8" ]] || { echo "expected to hit 8 unique DBs, hit $hit_count" >&2; cat "$TMP_ROOT/results/ap-positive-hits.tsv" >&2; exit 1; }

  log "Running AP invented DB fail-closed check"
  : > "$TMP_ROOT/results/ap-negative-cases.tsv"
  ap_execute_case "negative-invented-db" a rq-proof-agent-a localhost 15432 postgres_a1_db_invented 0
}

write_summary() {
  cat > "$TMP_ROOT/results/README.txt" <<TXT
AP Remote Queries targeting matrix proof artifacts

Connections:
$(cat "$TMP_ROOT/results/connections.tsv")

Positive AP hits: ap-positive-hits.tsv
Invented DB negative: ap-negative-cases.tsv

Each positive AP execution called /api/unstable/actions/execute with an explicit RemoteAction connection_id and a target {host, port, dbname}. The returned CSV is the database-specific marker seeded in the fixture's cities table.
TXT
}

write_keepalive_metadata() {
  cat > "$TMP_ROOT/results/keepalive-connections.tsv" <<TXT
a	$CONNECTION_A
b	$CONNECTION_B
TXT
  cat > "$TMP_ROOT/results/keepalive-pids.tsv" <<TXT
harness	$$
agent_a	$AGENT_A_PID
agent_b	$AGENT_B_PID
par_a	$PAR_A_PID
par_b	$PAR_B_PID
TXT
}

keep_alive_until_interrupted() {
  write_keepalive_metadata
  printf 'KEEPALIVE_READY tmp_root=%s connection_a=%s connection_b=%s\n' "$TMP_ROOT" "$CONNECTION_A" "$CONNECTION_B"
  log "Keep-alive mode is ready; waiting for SIGINT/SIGTERM"
  trap 'log "Keep-alive mode received interrupt; exiting for cleanup"; exit 0' INT TERM
  while true; do
    sleep 3600 &
    wait "$!"
  done
}

main() {
  parse_args "$@"
  require_cmd docker; require_cmd psql; require_cmd python3; require_cmd curl; require_cmd jq
  : "${DD_API_KEY:?DD_API_KEY missing}"
  : "${DD_APP_KEY:?DD_APP_KEY missing}"
  : "${DD_APPLICATION_KEY:?DD_APPLICATION_KEY missing}"
  [[ -x "$AGENT_REPO/bin/agent/agent" ]] || { echo "missing Agent binary: $AGENT_REPO/bin/agent/agent" >&2; exit 1; }
  [[ -x "$AGENT_REPO/bin/privateactionrunner/privateactionrunner" ]] || { echo "missing PAR binary: $AGENT_REPO/bin/privateactionrunner/privateactionrunner" >&2; exit 1; }
  [[ -d "$INTEGRATIONS_CORE/postgres/datadog_checks/postgres" ]] || { echo "Postgres integration not found under: $INTEGRATIONS_CORE" >&2; exit 1; }

  mkdir -p "$TMP_ROOT/results"
  log "Preparing AP targeting matrix harness at $TMP_ROOT"
  delete_temp_execution_group_if_present
  cleanup_ap_resources_for_hostname rq-proof-agent-a
  cleanup_ap_resources_for_hostname rq-proof-agent-b
  setup_tmp_tree
  start_postgres_fixture
  start_agents_and_pars
  if [[ "$KEEP_ALIVE" == "1" ]]; then
    keep_alive_until_interrupted
  else
    run_ap_matrix
    write_summary
    log "Done. Sanitized AP targeting results are under $TMP_ROOT/results"
  fi
}

main "$@"

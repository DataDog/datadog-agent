#!/usr/bin/env bash
# AP/PAR/Agent Remote Queries targeting matrix proof.
#
# Starts the same two-Agent/four-Postgres matrix as targeting-matrix-proof.sh,
# then starts one self-enrolled private-action-runner per Agent. The script
# discovers the two AP RemoteAction connection IDs and executes the AP action
# against every valid target tuple. The default identity query returns a unique
# marker from each database's remote_query_identity table, proving the AP request
# reached the exact DB. An invented DB target is asserted to fail closed.

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
RQ_REMOTE_QUERY=${RQ_REMOTE_QUERY:-SELECT current_database() AS current_db, expected_agent_hostname, expected_postgres_host, expected_postgres_port, expected_dbname, marker FROM remote_query_identity}
RQ_POLL_INTERVAL=${RQ_POLL_INTERVAL:-1}
RQ_MAX_POLLS=${RQ_MAX_POLLS:-45}
DD_SITE=${DD_SITE:-datad0g.com}
RQ_HARNESS_RESOLVER_ORG_UUID=${RQ_HARNESS_RESOLVER_ORG_UUID:-e8a99904-55b4-11f1-87f4-9699bf5698e1}
KEEP_ALIVE=${KEEP_ALIVE:-0}
RQ_HARNESS_TMUX_REPLACE=${RQ_HARNESS_TMUX_REPLACE:-0}
RQ_HARNESS_TMUX_CHILD=${RQ_HARNESS_TMUX_CHILD:-0}
RQ_HARNESS_TMUX_WINDOW_NAME=${RQ_HARNESS_TMUX_WINDOW_NAME:-agent-harness}
RQ_HARNESS_TMUX_SESSION=${RQ_HARNESS_TMUX_SESSION:-rq-agent-harness}
RQ_HARNESS_TMUX_READY_TIMEOUT=${RQ_HARNESS_TMUX_READY_TIMEOUT:-900}

POSTGRES_COMPOSE_STARTED=0
AGENT_A_PID=""
AGENT_B_PID=""
PAR_A_PID=""
PAR_B_PID=""
CONNECTION_A=""
CONNECTION_B=""
TMUX_WINDOW_ID=""
TMUX_SUPERVISOR_PANE=""
TMUX_AGENT_A_PANE=""
TMUX_AGENT_B_PANE=""
TMUX_PAR_A_PANE=""
TMUX_PAR_B_PANE=""
TMUX_POSTGRES_A1_PANE=""
TMUX_POSTGRES_A2_PANE=""
TMUX_POSTGRES_B1_PANE=""
TMUX_POSTGRES_B2_PANE=""
LAST_TMUX_PANE=""

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
Usage: ap-targeting-matrix-proof.sh [--keep-alive] [--replace-tmux-window]

Options:
  --keep-alive            Set up the Agent/PAR/Postgres/AP connection matrix, write
                          connection/PID/tmux metadata under $TMP_ROOT/results,
                          print a KEEPALIVE_READY marker, and supervise the
                          long-lived processes in a visible tmux window named
                          agent-harness. Equivalent: KEEP_ALIVE=1.
  --replace-tmux-window   If a live agent-harness tmux window already exists,
                          kill it before starting. Equivalent:
                          RQ_HARNESS_TMUX_REPLACE=1.
TXT
}

parse_args() {
  while (($#)); do
    case "$1" in
      --keep-alive)
        KEEP_ALIVE=1
        ;;
      --replace-tmux-window)
        RQ_HARNESS_TMUX_REPLACE=1
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

tmux_env_args() {
  local env_name
  for env_name in \
    AGENT_REPO INTEGRATIONS_CORE TMP_ROOT POSTGRES_COMPOSE_FILE POSTGRES_COMPOSE_PROJECT POSTGRES_IMAGE \
    PIP_PLATFORM AGENT_PYTHON_VERSION AGENT_PYTHON_ABI AGENT_A_CMD_PORT AGENT_B_CMD_PORT \
    RQ_POSTGRES_USERNAME RQ_POSTGRES_PASSWORD FQN RQ_REMOTE_QUERY RQ_POLL_INTERVAL RQ_MAX_POLLS \
    DD_SITE RQ_HARNESS_RESOLVER_ORG_UUID; do
    printf '%s\0' -e "${env_name}=${!env_name-}"
  done
  # Use harness-specific names for credentials so an existing tmux session
  # environment (for example DD_SITE=datadoghq.eu) cannot override this run.
  printf '%s\0' -e "RQ_HARNESS_DD_API_KEY=${DD_API_KEY-}"
  printf '%s\0' -e "RQ_HARNESS_DD_APP_KEY=${DD_APP_KEY-}"
  printf '%s\0' -e "RQ_HARNESS_DD_APPLICATION_KEY=${DD_APPLICATION_KEY-}"
  printf '%s\0' -e "RQ_HARNESS_DD_SITE=${DD_SITE-}"
}

tmux_window_ids_by_name() {
  tmux list-windows -a -F '#{window_id} #{window_name}' 2>/dev/null | awk -v name="$RQ_HARNESS_TMUX_WINDOW_NAME" '$2 == name {print $1}'
}

kill_existing_tmux_window_if_requested() {
  local window_id
  local -a existing_windows
  mapfile -t existing_windows < <(tmux_window_ids_by_name)
  if ((${#existing_windows[@]} == 0)); then
    return
  fi
  if [[ "$RQ_HARNESS_TMUX_REPLACE" != "1" ]]; then
    printf 'tmux window named %s already exists: %s\n' "$RQ_HARNESS_TMUX_WINDOW_NAME" "${existing_windows[*]}" >&2
    printf 'Re-run with --replace-tmux-window or RQ_HARNESS_TMUX_REPLACE=1 after confirming it is safe to replace.\n' >&2
    exit 1
  fi
  for window_id in "${existing_windows[@]}"; do
    tmux kill-window -t "$window_id" 2>/dev/null || true
  done
}

current_tmux_session() {
  if [[ -n "${TMUX_PANE:-}" ]]; then
    tmux display-message -p -t "$TMUX_PANE" '#S' 2>/dev/null || true
  fi
}

create_tmux_supervisor_window() {
  local session window_id command
  local -a env_args=()
  while IFS= read -r -d '' item; do
    env_args+=("$item")
  done < <(tmux_env_args)

  # Variables expand in the tmux child shell, not in this parent process.
  # shellcheck disable=SC2016
  command='export DD_API_KEY="$RQ_HARNESS_DD_API_KEY" DD_APP_KEY="$RQ_HARNESS_DD_APP_KEY" DD_APPLICATION_KEY="$RQ_HARNESS_DD_APPLICATION_KEY" DD_SITE="$RQ_HARNESS_DD_SITE"; exec "$AGENT_REPO/test/remotequeries/ap-targeting-matrix-proof.sh" --keep-alive'
  session=$(current_tmux_session)
  if [[ -n "$session" ]]; then
    window_id=$(tmux new-window -d -P -F '#{window_id}' -t "$session:" -n "$RQ_HARNESS_TMUX_WINDOW_NAME" -c "$AGENT_REPO" \
      "${env_args[@]}" -e RQ_HARNESS_TMUX_CHILD=1 "bash -lc '$command'")
  elif tmux has-session -t "$RQ_HARNESS_TMUX_SESSION" 2>/dev/null; then
    window_id=$(tmux new-window -d -P -F '#{window_id}' -t "$RQ_HARNESS_TMUX_SESSION:" -n "$RQ_HARNESS_TMUX_WINDOW_NAME" -c "$AGENT_REPO" \
      "${env_args[@]}" -e RQ_HARNESS_TMUX_CHILD=1 "bash -lc '$command'")
  else
    window_id=$(tmux new-session -d -P -F '#{window_id}' -s "$RQ_HARNESS_TMUX_SESSION" -n "$RQ_HARNESS_TMUX_WINDOW_NAME" -c "$AGENT_REPO" \
      "${env_args[@]}" -e RQ_HARNESS_TMUX_CHILD=1 "bash -lc '$command'")
  fi
  tmux set-option -w -t "$window_id" remain-on-exit on >/dev/null
  tmux set-option -w -t "$window_id" pane-border-status top >/dev/null
  tmux set-option -w -t "$window_id" pane-border-format '#{pane_index}: #{pane_title}' >/dev/null
  printf '%s\n' "$window_id"
}

wait_for_tmux_keepalive_ready() {
  local window_id=$1 ready_file="$TMP_ROOT/results/keepalive-ready" pane_dead
  for _ in $(seq 1 "$RQ_HARNESS_TMUX_READY_TIMEOUT"); do
    if [[ -f "$ready_file" ]]; then
      cat "$ready_file"
      return 0
    fi
    pane_dead=$(tmux display-message -p -t "$window_id" '#{pane_dead}' 2>/dev/null || echo 1)
    if [[ "$pane_dead" == "1" ]]; then
      echo "tmux supervisor pane exited before KEEPALIVE_READY" >&2
      [[ -f "$TMP_ROOT/results/harness.log" ]] && tail -120 "$TMP_ROOT/results/harness.log" >&2
      return 1
    fi
    sleep 1
  done
  echo "timed out waiting for KEEPALIVE_READY from tmux window $window_id" >&2
  [[ -f "$TMP_ROOT/results/harness.log" ]] && tail -120 "$TMP_ROOT/results/harness.log" >&2
  return 1
}

launch_keepalive_tmux_supervisor() {
  require_cmd tmux
  mkdir -p "$TMP_ROOT/results"
  rm -f "$TMP_ROOT/results/keepalive-ready"
  kill_existing_tmux_window_if_requested
  local window_id
  window_id=$(create_tmux_supervisor_window)
  printf 'Started tmux keep-alive supervisor window %s named %s\n' "$window_id" "$RQ_HARNESS_TMUX_WINDOW_NAME"
  wait_for_tmux_keepalive_ready "$window_id"
}

tmux_pane_dead() {
  local pane=$1
  [[ "$(tmux display-message -p -t "$pane" '#{pane_dead}' 2>/dev/null || echo 1)" == "1" ]]
}

init_child_tmux_context() {
  require_cmd tmux
  TMUX_SUPERVISOR_PANE=${TMUX_PANE:-}
  [[ -n "$TMUX_SUPERVISOR_PANE" ]] || { echo "RQ_HARNESS_TMUX_CHILD=1 requires running inside a tmux pane" >&2; exit 1; }
  TMUX_WINDOW_ID=$(tmux display-message -p -t "$TMUX_SUPERVISOR_PANE" '#{window_id}')
  tmux rename-window -t "$TMUX_WINDOW_ID" "$RQ_HARNESS_TMUX_WINDOW_NAME"
  tmux select-pane -t "$TMUX_SUPERVISOR_PANE" -T 'supervisor / harness log'
  tmux set-option -w -t "$TMUX_WINDOW_ID" remain-on-exit on >/dev/null
  tmux set-option -w -t "$TMUX_WINDOW_ID" pane-border-status top >/dev/null
  tmux set-option -w -t "$TMUX_WINDOW_ID" pane-border-format '#{pane_index}: #{pane_title}' >/dev/null
}

new_tmux_process_pane() {
  local role=$1 title=$2 command=$3 pane_id
  local -a env_args=()
  while IFS= read -r -d '' item; do
    env_args+=("$item")
  done < <(tmux_env_args)
  pane_id=$(tmux split-window -d -P -F '#{pane_id}' -t "$TMUX_WINDOW_ID" -c "$AGENT_REPO" "${env_args[@]}" "bash -lc 'export DD_API_KEY=\"\$RQ_HARNESS_DD_API_KEY\" DD_APP_KEY=\"\$RQ_HARNESS_DD_APP_KEY\" DD_APPLICATION_KEY=\"\$RQ_HARNESS_DD_APPLICATION_KEY\" DD_SITE=\"\$RQ_HARNESS_DD_SITE\"; $command'")
  tmux select-pane -t "$pane_id" -T "$title"
  tmux select-layout -t "$TMUX_WINDOW_ID" tiled >/dev/null
  LAST_TMUX_PANE=$pane_id
  case "$role" in
    agent_a) TMUX_AGENT_A_PANE=$pane_id ;;
    agent_b) TMUX_AGENT_B_PANE=$pane_id ;;
    par_a) TMUX_PAR_A_PANE=$pane_id ;;
    par_b) TMUX_PAR_B_PANE=$pane_id ;;
    postgres_a1) TMUX_POSTGRES_A1_PANE=$pane_id ;;
    postgres_a2) TMUX_POSTGRES_A2_PANE=$pane_id ;;
    postgres_b1) TMUX_POSTGRES_B1_PANE=$pane_id ;;
    postgres_b2) TMUX_POSTGRES_B2_PANE=$pane_id ;;
  esac
  printf '%s\n' "$pane_id"
}

start_one_postgres_pane() {
  local role=$1 service=$2 title=$3 command
  # Variables expand in the tmux status pane; service is inserted here.
  # shellcheck disable=SC2016
  command='while true; do clear; date -u; echo "Postgres service: '$service'"; echo "Compose project: $POSTGRES_COMPOSE_PROJECT"; POSTGRES_IMAGE="$POSTGRES_IMAGE" docker compose -f "$POSTGRES_COMPOSE_FILE" -p "$POSTGRES_COMPOSE_PROJECT" ps '$service'; echo; POSTGRES_IMAGE="$POSTGRES_IMAGE" docker compose -f "$POSTGRES_COMPOSE_FILE" -p "$POSTGRES_COMPOSE_PROJECT" logs --tail=30 --no-color '$service'; sleep 10; done'
  new_tmux_process_pane "$role" "$title" "$command" >/dev/null
}

start_postgres_status_panes() {
  if [[ "$KEEP_ALIVE" != "1" || "$RQ_HARNESS_TMUX_CHILD" != "1" ]]; then
    return
  fi
  start_one_postgres_pane postgres_a1 postgres_a1 'Postgres A1 - :15432'
  start_one_postgres_pane postgres_a2 postgres_a2 'Postgres A2 - :15433'
  start_one_postgres_pane postgres_b1 postgres_b1 'Postgres B1 - :25432'
  start_one_postgres_pane postgres_b2 postgres_b2 'Postgres B2 - :25433'
}

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
remote_queries.execute.enable_query_allowlist: false
log_level: debug
log_file: $root/agent.log
python_lazy_loading: false
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
  start_postgres_status_panes
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
  local side=$1 pid loaded_count pane command
  local root="$TMP_ROOT/agent_$side"
  : > "$root/agent.log"
  if [[ "$KEEP_ALIVE" == "1" && "$RQ_HARNESS_TMUX_CHILD" == "1" ]]; then
    # AGENT_REPO/TMP_ROOT expand in the tmux pane; side is inserted here.
    # shellcheck disable=SC2016
    command='echo "Agent '$side': $AGENT_REPO/bin/agent/agent run -c $TMP_ROOT/agent_'$side'"; export PYTHONPATH="$TMP_ROOT/checks.d:$TMP_ROOT/pydeps"; exec "$AGENT_REPO/bin/agent/agent" run -c "$TMP_ROOT/agent_'$side'"'
    if [[ "$side" == "a" ]]; then
      new_tmux_process_pane "agent_$side" 'Agent A - rq-proof-agent-a' "$command" >/dev/null
    else
      new_tmux_process_pane "agent_$side" 'Agent B - rq-proof-agent-b' "$command" >/dev/null
    fi
    pane=$LAST_TMUX_PANE
    pid=$(tmux display-message -p -t "$pane" '#{pane_pid}')
  else
    PYTHONPATH="$TMP_ROOT/checks.d:$TMP_ROOT/pydeps" "$AGENT_REPO/bin/agent/agent" run -c "$root" > "$root/stdout.log" 2> "$root/stderr.log" &
    pid=$!
  fi
  if [[ "$side" == "a" ]]; then AGENT_A_PID=$pid; else AGENT_B_PID=$pid; fi
  for _ in $(seq 1 120); do
    loaded_count=$(grep -c "successfully loaded check 'postgres'" "$root/agent.log" 2>/dev/null || true)
    if [[ "$loaded_count" -ge 4 && -f "$root/run/auth_token" && -f "$root/run/ipc_cert.pem" ]]; then
      log "Agent $side loaded $loaded_count Postgres checks and exposed IPC artifacts"
      return
    fi
    if [[ "$KEEP_ALIVE" == "1" && "$RQ_HARNESS_TMUX_CHILD" == "1" ]]; then
      tmux_pane_dead "$pane" && { echo "Agent $side exited early" >&2; exit 1; }
    else
      kill -0 "$pid" 2>/dev/null || { echo "Agent $side exited early" >&2; tail -100 "$root/stderr.log" >&2 || true; exit 1; }
    fi
    sleep 0.5
  done
  echo "Timed out waiting for Agent $side" >&2
  grep -ni 'postgres\|ModuleNotFound\|unable to load' "$root/agent.log" >&2 || true
  exit 1
}

start_par() {
  local side=$1 pid pane command
  local par_root="$TMP_ROOT/par_$side"
  if [[ "$KEEP_ALIVE" == "1" && "$RQ_HARNESS_TMUX_CHILD" == "1" ]]; then
    # AGENT_REPO/TMP_ROOT expand in the tmux pane; side is inserted here.
    # shellcheck disable=SC2016
    command='echo "PAR '$side': $AGENT_REPO/bin/privateactionrunner/privateactionrunner run -c $TMP_ROOT/par_'$side'"; export DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true; exec "$AGENT_REPO/bin/privateactionrunner/privateactionrunner" run -c "$TMP_ROOT/par_'$side'"'
    if [[ "$side" == "a" ]]; then
      new_tmux_process_pane "par_$side" 'PAR A - private-action-runner for Agent A' "$command" >/dev/null
    else
      new_tmux_process_pane "par_$side" 'PAR B - private-action-runner for Agent B' "$command" >/dev/null
    fi
    pane=$LAST_TMUX_PANE
    pid=$(tmux display-message -p -t "$pane" '#{pane_pid}')
  else
    DD_INTERNAL_PAR_SKIP_TASK_VERIFICATION=true "$AGENT_REPO/bin/privateactionrunner/privateactionrunner" run -c "$par_root" > "$par_root/stdout.log" 2> "$par_root/stderr.log" &
    pid=$!
  fi
  if [[ "$side" == "a" ]]; then PAR_A_PID=$pid; else PAR_B_PID=$pid; fi
  for _ in $(seq 1 120); do
    if grep -q "Self-enrollment successful" "$par_root/private-action-runner.log" 2>/dev/null; then
      log "PAR $side self-enrollment successful"
      return
    fi
    if [[ "$KEEP_ALIVE" == "1" && "$RQ_HARNESS_TMUX_CHILD" == "1" ]]; then
      tmux_pane_dead "$pane" && { echo "PAR $side exited early" >&2; tail -100 "$par_root/private-action-runner.log" >&2 || true; exit 1; }
    else
      kill -0 "$pid" 2>/dev/null || { echo "PAR $side exited early" >&2; tail -100 "$par_root/stderr.log" >&2 || true; tail -100 "$par_root/private-action-runner.log" >&2 || true; exit 1; }
    fi
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

api_get_restriction_policy() {
  local connection_id=$1 out=$2 status
  status=$(curl -sS -o "$out" -w '%{http_code}' \
    -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" \
    "https://api.$DD_SITE/api/v2/restriction_policy/connection%3A$connection_id")
  [[ "$status" == "200" ]] || { echo "restriction policy GET for connection $connection_id HTTP $status" >&2; cat "$out" >&2; return 1; }
}

api_post_restriction_policy() {
  local connection_id=$1 body=$2 out=$3 status
  status=$(curl -sS -o "$out" -w '%{http_code}' \
    -X POST \
    -H "DD-API-KEY: $DD_API_KEY" -H "DD-APPLICATION-KEY: $DD_APPLICATION_KEY" -H 'Content-Type: application/json' \
    --data @"$body" \
    "https://api.$DD_SITE/api/v2/restriction_policy/connection%3A$connection_id")
  echo "$status" > "$out.status"
  [[ "$status" == "200" || "$status" == "201" || "$status" == "204" ]] || { echo "restriction policy POST for connection $connection_id HTTP $status" >&2; cat "$out" >&2; return 1; }
}

grants_only_org_resolver() {
  local policy_json=$1 principal=$2
  jq -e --arg principal "$principal" '
    [.data.attributes.bindings[]? | .relation as $relation | (.principals // [])[] | select(. == $principal) | $relation] as $relations
    | ($relations | length) == 1 and $relations[0] == "resolver"
  ' "$policy_json" >/dev/null
}

grant_org_resolver_on_connection() {
  local connection_id=$1
  local principal="org:$RQ_HARNESS_RESOLVER_ORG_UUID"
  local prefix="$TMP_ROOT/results/restriction-policy-connection-$connection_id"
  local before="$prefix-before.json" update="$prefix-update.json" update_response="$prefix-update-response.json" after="$prefix-after.json"

  log "granting org resolver on AP connection $connection_id for $principal"
  api_get_restriction_policy "$connection_id" "$before"

  if grants_only_org_resolver "$before" "$principal"; then
    jq '.' "$before" > "$update"
    log "org resolver already present on AP connection $connection_id for $principal"
  else
    jq --arg principal "$principal" '
      def has_principal($p): ((.principals // []) | index($p)) != null;
      (.data.attributes.bindings // []) as $current
      | [range(0; $current | length) as $i | select($current[$i] | has_principal($principal)) | $i] as $principal_idxs
      | .data.attributes.bindings = (
          if ($principal_idxs | length) == 0 then
            $current + [{relation:"resolver", principals:[$principal]}]
          elif ($principal_idxs | length) == 1 and (($current[$principal_idxs[0]].principals // []) | length) == 1 then
            $current | .[$principal_idxs[0]].relation = "resolver"
          else
            [
              $current[]
              | if has_principal($principal) then
                  .principals = ((.principals // []) - [$principal])
                  | select(((.principals // []) | length) > 0)
                else
                  .
                end
            ] as $bindings
            | ($bindings | map(.relation == "resolver") | index(true)) as $resolver_idx
            | if $resolver_idx == null then
                $bindings + [{relation:"resolver", principals:[$principal]}]
              else
                $bindings
                | .[$resolver_idx].principals = ((.[$resolver_idx].principals // []) + [$principal])
              end
          end
        )
    ' "$before" > "$update"
    api_post_restriction_policy "$connection_id" "$update" "$update_response"
  fi

  api_get_restriction_policy "$connection_id" "$after"
  grants_only_org_resolver "$after" "$principal" || { echo "restriction policy for connection $connection_id does not grant only resolver to $principal" >&2; cat "$after" >&2; exit 1; }
  log "verified org resolver on AP connection $connection_id for $principal"
}

delete_temp_execution_group_if_present() {
  local out="$TMP_ROOT/results/precleanup-execution-groups.json" ids id
  log "Deleting stale rq-ap-staging-proof-eg execution group if present"
  if ! api_get_execution_groups "$out"; then
    log "Skipping stale execution-group cleanup because execution groups could not be listed"
    return
  fi
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
  grant_org_resolver_on_connection "$CONNECTION_A"
  grant_org_resolver_on_connection "$CONNECTION_B"
  printf 'a\t%s\n' "$CONNECTION_A" > "$TMP_ROOT/results/connections.tsv"
  printf 'b\t%s\n' "$CONNECTION_B" >> "$TMP_ROOT/results/connections.tsv"
}

connection_for_side() { if [[ "$1" == "a" ]]; then echo "$CONNECTION_A"; else echo "$CONNECTION_B"; fi; }
expected_data_for() { printf '%s,%s,%s,%s,%s,%s|%s|%s|%s\n' "$4" "$1" "$2" "$3" "$4" "$1" "$2" "$3" "$4"; }

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

Each positive AP execution called /api/unstable/actions/execute with an explicit RemoteAction connection_id and a target {host, port, dbname}. The returned CSV is the database-specific identity row seeded in the fixture's remote_query_identity table.
TXT
}

write_keepalive_tmux_metadata() {
  local role pane window_name window_id pane_id pane_title pane_pid pane_current_command
  printf 'role\twindow_name\twindow_id\tpane_id\tpane_title\tpane_pid\tpane_current_command\n' > "$TMP_ROOT/results/keepalive-tmux.tsv"
  for role in supervisor agent_a agent_b par_a par_b postgres_a1 postgres_a2 postgres_b1 postgres_b2; do
    case "$role" in
      supervisor) pane=$TMUX_SUPERVISOR_PANE ;;
      agent_a) pane=$TMUX_AGENT_A_PANE ;;
      agent_b) pane=$TMUX_AGENT_B_PANE ;;
      par_a) pane=$TMUX_PAR_A_PANE ;;
      par_b) pane=$TMUX_PAR_B_PANE ;;
      postgres_a1) pane=$TMUX_POSTGRES_A1_PANE ;;
      postgres_a2) pane=$TMUX_POSTGRES_A2_PANE ;;
      postgres_b1) pane=$TMUX_POSTGRES_B1_PANE ;;
      postgres_b2) pane=$TMUX_POSTGRES_B2_PANE ;;
    esac
    [[ -n "$pane" ]] || continue
    window_name=$(tmux display-message -p -t "$pane" '#{window_name}')
    window_id=$(tmux display-message -p -t "$pane" '#{window_id}')
    pane_id=$(tmux display-message -p -t "$pane" '#{pane_id}')
    pane_title=$(tmux display-message -p -t "$pane" '#{pane_title}')
    pane_pid=$(tmux display-message -p -t "$pane" '#{pane_pid}')
    pane_current_command=$(tmux display-message -p -t "$pane" '#{pane_current_command}')
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$role" "$window_name" "$window_id" "$pane_id" "$pane_title" "$pane_pid" "$pane_current_command" >> "$TMP_ROOT/results/keepalive-tmux.tsv"
  done
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
  if [[ "$RQ_HARNESS_TMUX_CHILD" == "1" ]]; then
    write_keepalive_tmux_metadata
  fi
}

keep_alive_until_interrupted() {
  local ready_line
  write_keepalive_metadata
  ready_line=$(printf 'KEEPALIVE_READY tmp_root=%s connection_a=%s connection_b=%s tmux_window=%s\n' "$TMP_ROOT" "$CONNECTION_A" "$CONNECTION_B" "$TMUX_WINDOW_ID")
  printf '%s\n' "$ready_line" | tee "$TMP_ROOT/results/keepalive-ready"
  log "Keep-alive mode is ready in tmux window ${TMUX_WINDOW_ID:-unknown}; waiting for SIGINT/SIGTERM"
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

  if [[ "$KEEP_ALIVE" == "1" && "$RQ_HARNESS_TMUX_CHILD" != "1" ]]; then
    launch_keepalive_tmux_supervisor
    exit 0
  fi
  if [[ "$KEEP_ALIVE" == "1" && "$RQ_HARNESS_TMUX_CHILD" == "1" ]]; then
    init_child_tmux_context
  fi

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

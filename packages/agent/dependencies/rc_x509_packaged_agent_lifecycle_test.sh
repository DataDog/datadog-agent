#!/usr/bin/env bash

set -euo pipefail

archive="$1"
if [[ "$archive" != /* ]]; then
    archive="$PWD/$archive"
fi

stage="$TEST_TMPDIR/rc-x509-agent-lifecycle"
mkdir -p "$stage"
/usr/bin/tar -xf "$archive" -C "$stage"

agent="$stage/bin/agent/agent"
if [[ ! -x "$agent" ]]; then
    echo "missing executable staged production Agent: $agent" >&2
    exit 1
fi

python3_bin="$(command -v python3)"
cmd_port="$($python3_bin -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
config_dir="$stage/config"
log_file="$stage/agent.log"
stdout_file="$stage/agent.stdout.log"
pid_file="$stage/agent.pid"
auth_token_file="$config_dir/auth_token"
mkdir -p "$config_dir"

cat > "$config_dir/datadog.yaml" <<EOF
api_key: 00000000000000000000000000000000
hostname: rc-x509-packaged-agent-smoke
log_level: info
log_to_console: false
log_file: '$log_file'
auth_token_file_path: '$auth_token_file'
ipc_address: 127.0.0.1
cmd_port: $cmd_port
dd_url: http://127.0.0.1:1
proxy:
  http: http://127.0.0.1:1
  https: http://127.0.0.1:1
  no_proxy:
    - 127.0.0.1
    - localhost
enable_payloads:
  events: false
  json_to_v1_intake: false
  series: false
  service_checks: false
  sketches: false
agent_telemetry:
  enabled: false
use_dogstatsd: false
logs_enabled: false
apm_config:
  enabled: false
process_config:
  process_collection:
    enabled: false
  container_collection:
    enabled: false
inventories_enabled: false
inventories_diagnostics_enabled: false
enable_metadata_collection: false
remote_configuration:
  enabled: true
  rc_dd_url: http://127.0.0.1:1
  no_tls: true
  x509:
    enabled: true
    debug_ping_enabled: true
EOF

agent_pid=""
cleanup() {
    if [[ -n "$agent_pid" ]] && kill -0 "$agent_pid" 2>/dev/null; then
        kill "$agent_pid" 2>/dev/null || true
        sleep 1
        kill -9 "$agent_pid" 2>/dev/null || true
    fi
    rm -f "$auth_token_file"
}
trap cleanup EXIT

env \
    -u CI_IDENTITIES_GITLAB_ID_TOKEN \
    -u BUILDBARN_ID_TOKEN \
    PATH="/usr/sbin:/usr/bin:/sbin:/bin:$PATH" \
    DD_API_KEY=00000000000000000000000000000000 \
    DD_REMOTE_CONFIGURATION_API_KEY=00000000000000000000000000000000 \
    HTTP_PROXY=http://127.0.0.1:1 \
    HTTPS_PROXY=http://127.0.0.1:1 \
    NO_PROXY=127.0.0.1,localhost \
    "$agent" -c "$config_dir" run --pidfile "$pid_file" >"$stdout_file" 2>&1 &
agent_pid=$!

started=false
for _ in $(seq 1 60); do
    if grep -Fq 'remote config x509 client started' "$log_file" 2>/dev/null; then
        started=true
        break
    fi
    if ! kill -0 "$agent_pid" 2>/dev/null; then
        break
    fi
    sleep 1
done
if [[ "$started" != true ]]; then
    echo "packaged Agent did not report that the x509 component started" >&2
    tail -n 300 "$log_file" "$stdout_file" 2>/dev/null >&2 || true
    exit 1
fi

stopped=false
for _ in $(seq 1 30); do
    if "$agent" -c "$config_dir" stop >/dev/null 2>&1; then
        stopped=true
        break
    fi
    if ! kill -0 "$agent_pid" 2>/dev/null; then
        break
    fi
    sleep 1
done
if [[ "$stopped" != true ]]; then
    echo "could not request a graceful packaged-Agent shutdown" >&2
    tail -n 300 "$log_file" "$stdout_file" 2>/dev/null >&2 || true
    exit 1
fi

for _ in $(seq 1 30); do
    if ! kill -0 "$agent_pid" 2>/dev/null; then
        break
    fi
    sleep 1
done
if kill -0 "$agent_pid" 2>/dev/null; then
    echo "packaged Agent did not exit after the graceful stop request" >&2
    tail -n 300 "$log_file" "$stdout_file" 2>/dev/null >&2 || true
    exit 1
fi

agent_status=0
wait "$agent_pid" || agent_status=$?
agent_pid=""
if [[ "$agent_status" -ne 0 ]]; then
    echo "packaged Agent exited with status $agent_status" >&2
    tail -n 300 "$log_file" "$stdout_file" 2>/dev/null >&2 || true
    exit 1
fi
if ! grep -Fq 'remote config x509 client stopped' "$log_file"; then
    echo "packaged Agent exited without reporting a clean x509 component stop" >&2
    tail -n 300 "$log_file" "$stdout_file" 2>/dev/null >&2 || true
    exit 1
fi

echo 'Packaged Agent initialized and cleanly stopped the x509 component'

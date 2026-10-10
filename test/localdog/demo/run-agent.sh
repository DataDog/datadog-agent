#!/usr/bin/env bash
# Start (or restart) a Datadog Agent container that ships everything to the
# localdog server instead of datadoghq.com.
#
# Env overrides:
#   LOCALDOG_URL      localdog intake URL         (default http://localhost:8282)
#   LOG_DIR           dir containing app.log      (default /instance_storage/localdog-demo/logs)
#   CONFD_DIR         generated agent conf.d dir  (default <LOG_DIR>/../conf.d)
#   AGENT_IMAGE       agent image                 (default gcr.io/datadoghq/agent:7)
#   AGENT_CONTAINER   container name              (default localdog-agent)
#   DD_HOSTNAME       reported hostname           (default localdog-laptop)
#   NETWORK_MODE      "host" (default) or "bridge" (fallback, see below)
#   EXTRA_DOCKER_ARGS extra args passed verbatim to `docker run`
set -euo pipefail
# shellcheck source=common.sh
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

NETWORK_MODE="${NETWORK_MODE:-host}"

# --- intake URL as seen from inside the container -------------------------
# With --network host (Linux, or Docker Desktop >= 4.34 with "Enable host
# networking" turned on in Settings > Resources > Network), "localhost" inside
# the container is the host, so LOCALDOG_URL can be used as-is.
# In bridge mode (older Docker Desktop on macOS), the host is reachable as
# host.docker.internal, and the app must reach the agent through published
# ports 8125/udp and 8126/tcp instead.
INTAKE_URL="${LOCALDOG_URL}"
if [[ "${NETWORK_MODE}" != "host" ]]; then
  INTAKE_URL="$(echo "${LOCALDOG_URL}" | sed -E 's#//(localhost|127\.0\.0\.1)([:/]|$)#//host.docker.internal\2#')"
fi
INTAKE_URL="${INTAKE_URL%/}"
# host:port form, for the logs-style ("*.logs_dd_url") endpoints.
INTAKE_HOSTPORT="$(echo "${INTAKE_URL}" | sed -E 's#^[a-z]+://##; s#/.*$##')"
case "${INTAKE_URL}" in https://*) LOGS_NO_SSL=false ;; *) LOGS_NO_SSL=true ;; esac

mkdir -p "${LOG_DIR}" "${CONFD_DIR}/demo-node.d"
touch "${LOG_DIR}/app.log"

# --- custom log integration for the demo app -------------------------------
# LOG_DIR is mounted at the same path inside the container, so the path matches.
cat > "${CONFD_DIR}/demo-node.d/conf.yaml" <<YAML
logs:
  - type: file
    path: ${LOG_DIR}/*.log
    service: demo-node
    source: nodejs
    tags:
      - env:local
      - team:localdog
YAML

# --- environment ------------------------------------------------------------
envs=(
  DD_API_KEY="${DD_API_KEY}"
  DD_HOSTNAME="${DD_HOSTNAME}"
  DD_ENV=local
  DD_SITE=localdog.invalid           # never resolves; every endpoint below overrides it
  DD_LOG_LEVEL="${DD_LOG_LEVEL:-info}"

  # Core intake (metrics, service checks, events, metadata, ...)
  DD_DD_URL="${INTAKE_URL}"
  DD_SKIP_SSL_VALIDATION=true

  # Logs
  DD_LOGS_ENABLED=true
  DD_LOGS_CONFIG_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_LOGS_CONFIG_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_LOGS_CONFIG_FORCE_USE_HTTP=true
  DD_LOGS_CONFIG_USE_COMPRESSION=true
  DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=false

  # APM
  DD_APM_ENABLED=true
  DD_APM_NON_LOCAL_TRAFFIC=true
  DD_APM_DD_URL="${INTAKE_URL}"
  DD_APM_PROFILING_DD_URL="${INTAKE_URL}/api/v2/profile"
  # The trace-agent's instrumentation-telemetry proxy (/telemetry/proxy) always
  # forwards with an https:// scheme (pkg/trace/api/telemetry.go), regardless
  # of the scheme in apm_config.telemetry.dd_url, so it cannot reach a
  # plain-http localdog. Disabled by default; set APM_TELEMETRY=true if
  # localdog serves TLS.
  DD_APM_TELEMETRY_ENABLED="${APM_TELEMETRY:-false}"
  DD_APM_TELEMETRY_DD_URL="${INTAKE_URL}"
  DD_APM_DEBUGGER_DD_URL="${INTAKE_URL}/api/v2/logs"
  DD_APM_DEBUGGER_DIAGNOSTICS_DD_URL="${INTAKE_URL}/api/v2/debugger"
  DD_APM_SYMDB_DD_URL="${INTAKE_URL}"
  DD_APM_RECEIVER_PORT=8126

  # DogStatsD
  DD_DOGSTATSD_NON_LOCAL_TRAFFIC=true
  DD_DOGSTATSD_PORT=8125

  # Processes / containers
  DD_PROCESS_CONFIG_PROCESS_DD_URL="${INTAKE_URL}"
  DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED=true
  DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED=true
  DD_ORCHESTRATOR_EXPLORER_ORCHESTRATOR_DD_URL="${INTAKE_URL}"

  # Event-platform forwarders (same list fakeintake uses in
  # test/e2e-framework/components/datadog/agentparams/params.go:withIntakeHostname)
  DD_DATABASE_MONITORING_METRICS_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_DATABASE_MONITORING_METRICS_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_DATABASE_MONITORING_ACTIVITY_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_DATABASE_MONITORING_ACTIVITY_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_DATABASE_MONITORING_SAMPLES_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_DATABASE_MONITORING_SAMPLES_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_NETWORK_DEVICES_METADATA_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_NETWORK_DEVICES_METADATA_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_NETWORK_DEVICES_SNMP_TRAPS_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_NETWORK_DEVICES_SNMP_TRAPS_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_NETWORK_DEVICES_NETFLOW_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_NETWORK_DEVICES_NETFLOW_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_NETWORK_PATH_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_NETWORK_PATH_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_NETWORK_DEVICES_CONFIG_MANAGEMENT_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_NETWORK_DEVICES_CONFIG_MANAGEMENT_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_SYNTHETICS_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_SYNTHETICS_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_CONTAINER_LIFECYCLE_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_CONTAINER_LIFECYCLE_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_CONTAINER_IMAGE_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_CONTAINER_IMAGE_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_SBOM_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_SBOM_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_SDS_RESULT_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_SDS_RESULT_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_SERVICE_DISCOVERY_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_SERVICE_DISCOVERY_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_CONFIG_FILES_DISCOVERY_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_CONFIG_FILES_DISCOVERY_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_SOFTWARE_INVENTORY_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_SOFTWARE_INVENTORY_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_DATA_STREAMS_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_DATA_STREAMS_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_EVENT_MANAGEMENT_FORWARDER_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_EVENT_MANAGEMENT_FORWARDER_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_COMPLIANCE_CONFIG_ENDPOINTS_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_COMPLIANCE_CONFIG_ENDPOINTS_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_RUNTIME_SECURITY_CONFIG_ENDPOINTS_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_RUNTIME_SECURITY_CONFIG_ENDPOINTS_LOGS_NO_SSL="${LOGS_NO_SSL}"

  # Agent self-telemetry: off, but pointed at localdog in case it is re-enabled.
  DD_AGENT_TELEMETRY_ENABLED=false
  DD_AGENT_TELEMETRY_LOGS_DD_URL="${INTAKE_HOSTPORT}"
  DD_AGENT_TELEMETRY_LOGS_NO_SSL="${LOGS_NO_SSL}"
  DD_AGENT_TELEMETRY_USE_COMPRESSION=false

  # Things that would otherwise call out to Datadog-owned endpoints.
  DD_REMOTE_CONFIGURATION_ENABLED=false
  DD_REMOTE_UPDATES=false
)

# --- mounts (only add what exists so it works on Linux and macOS) ----------
mounts=(
  -v "${LOG_DIR}:${LOG_DIR}:ro"
  -v "${CONFD_DIR}/demo-node.d:/etc/datadog-agent/conf.d/demo-node.d:ro"
)
add_mount() { # src dst [opts]
  if [[ -e "$1" ]]; then mounts+=(-v "$1:$2${3:+:$3}"); else echo "note: $1 not found, skipping mount" >&2; fi
}
add_mount /var/run/docker.sock /var/run/docker.sock ro
if [[ "$(uname -s)" == "Linux" ]]; then
  add_mount /proc /host/proc ro
  add_mount /sys/fs/cgroup /host/sys/fs/cgroup ro
  add_mount /etc/passwd /etc/passwd ro
fi
# On macOS Docker Desktop /proc and /sys/fs/cgroup belong to the Linux VM; the
# agent image already sees the VM's /proc, so we skip the host mounts there.

net_args=()
if [[ "${NETWORK_MODE}" == "host" ]]; then
  net_args=(--network host --pid host)
else
  # Bridge fallback: publish the agent ports so the app on the host can send
  # traces (8126/tcp) and DogStatsD (8125/udp) to localhost.
  net_args=(-p 8126:8126/tcp -p 8125:8125/udp --add-host host.docker.internal:host-gateway)
fi

env_args=()
for e in "${envs[@]}"; do env_args+=(-e "$e"); done

if docker ps -a --format '{{.Names}}' | grep -qx "${AGENT_CONTAINER}"; then
  echo "Removing existing container ${AGENT_CONTAINER}..."
  docker rm -f "${AGENT_CONTAINER}" >/dev/null
fi

echo "Starting ${AGENT_CONTAINER} (${AGENT_IMAGE}) -> ${INTAKE_URL} [network=${NETWORK_MODE}]"
# shellcheck disable=SC2086
docker run -d --name "${AGENT_CONTAINER}" \
  --restart unless-stopped \
  --cgroupns host \
  "${net_args[@]}" \
  "${env_args[@]}" \
  "${mounts[@]}" \
  ${EXTRA_DOCKER_ARGS:-} \
  "${AGENT_IMAGE}" >/dev/null

cat <<MSG
Agent container '${AGENT_CONTAINER}' started.
  intake:     ${INTAKE_URL} (logs: ${INTAKE_HOSTPORT}, no_ssl=${LOGS_NO_SSL})
  APM:        localhost:8126   DogStatsD: localhost:8125/udp
  tailing:    ${LOG_DIR}/*.log  (conf: ${CONFD_DIR}/demo-node.d/conf.yaml)

  docker logs -f ${AGENT_CONTAINER}
  docker exec ${AGENT_CONTAINER} agent status
  $(dirname "$0")/stop-agent.sh
MSG

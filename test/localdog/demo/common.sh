# Shared defaults for the localdog demo scripts. Source, don't execute.
# shellcheck shell=bash

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Base directory for runtime state (logs, generated agent conf.d, pid file).
# Defaults to /instance_storage/localdog-demo on the workspace; falls back to
# ~/.localdog-demo where /instance_storage does not exist (e.g. macOS).
if [[ -z "${LOCALDOG_DEMO_HOME:-}" ]]; then
  if [[ -d /instance_storage && -w /instance_storage ]]; then
    LOCALDOG_DEMO_HOME=/instance_storage/localdog-demo
  else
    LOCALDOG_DEMO_HOME="${HOME}/.localdog-demo"
  fi
fi
LOG_DIR="${LOG_DIR:-${LOCALDOG_DEMO_HOME}/logs}"
CONFD_DIR="${CONFD_DIR:-${LOCALDOG_DEMO_HOME}/conf.d}"
PID_FILE="${PID_FILE:-${LOCALDOG_DEMO_HOME}/demo-node.pid}"

LOCALDOG_URL="${LOCALDOG_URL:-http://localhost:8282}"
AGENT_CONTAINER="${AGENT_CONTAINER:-localdog-agent}"
AGENT_IMAGE="${AGENT_IMAGE:-gcr.io/datadoghq/agent:7}"
DD_HOSTNAME="${DD_HOSTNAME:-localdog-laptop}"
DD_API_KEY="${DD_API_KEY:-00000000000000000000000000000000}"

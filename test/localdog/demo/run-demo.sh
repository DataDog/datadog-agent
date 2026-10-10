#!/usr/bin/env bash
# Start the demo-node app in the background, writing JSON logs to LOG_DIR/app.log.
#
# Env overrides: LOG_DIR, PORT (3000), LOADGEN (1), LOADGEN_RPS (4), ERROR_RATE (0.05),
#                DD_TRACE_AGENT_URL (http://localhost:8126), DD_DOGSTATSD_HOST/PORT.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

APP_DIR="${DEMO_DIR}/node-app"
PORT="${PORT:-3000}"
NODE_BIN="${NODE_BIN:-$(command -v node)}"

mkdir -p "${LOG_DIR}" "$(dirname "${PID_FILE}")"

if [[ -f "${PID_FILE}" ]] && kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
  echo "demo-node already running (pid $(cat "${PID_FILE}")). Run ./stop-demo.sh first." >&2
  exit 1
fi

if [[ ! -d "${APP_DIR}/node_modules" ]]; then
  echo "Installing node dependencies..."
  (cd "${APP_DIR}" && command npm install --no-fund --no-audit)
fi

export DD_SERVICE=demo-node DD_ENV=local DD_VERSION=1.0.0
export DD_LOGS_INJECTION=true DD_RUNTIME_METRICS_ENABLED=true
export DD_TRACE_AGENT_URL="${DD_TRACE_AGENT_URL:-http://localhost:8126}"
export DD_DOGSTATSD_HOST="${DD_DOGSTATSD_HOST:-localhost}" DD_DOGSTATSD_PORT="${DD_DOGSTATSD_PORT:-8125}"
export DD_REMOTE_CONFIGURATION_ENABLED=false DD_INSTRUMENTATION_TELEMETRY_ENABLED="${DD_INSTRUMENTATION_TELEMETRY_ENABLED:-false}"
export DD_HOSTNAME
export LOG_FILE="${LOG_DIR}/app.log" PORT LOADGEN="${LOADGEN:-1}"

cd "${APP_DIR}"
nohup "${NODE_BIN}" --require ./src/tracer.js src/index.js >"${LOG_DIR}/demo-node.stdout" 2>&1 &
echo $! >"${PID_FILE}"
sleep 1
if ! kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
  echo "demo-node failed to start; see ${LOG_DIR}/demo-node.stdout" >&2
  tail -20 "${LOG_DIR}/demo-node.stdout" >&2
  exit 1
fi

cat <<MSG
demo-node started (pid $(cat "${PID_FILE}")) on http://localhost:${PORT}
  app logs (JSON, tailed by the agent): ${LOG_FILE}
  stdout/stderr:                        ${LOG_DIR}/demo-node.stdout
  traces  -> ${DD_TRACE_AGENT_URL}
  metrics -> ${DD_DOGSTATSD_HOST}:${DD_DOGSTATSD_PORT}/udp
  load generator: $([[ "${LOADGEN}" == "0" ]] && echo off || echo "on (~${LOADGEN_RPS:-4} req/s)")

Try:
  curl localhost:${PORT}/api/users
  curl localhost:${PORT}/api/users/3
  curl -XPOST -H 'content-type: application/json' -d '{"quantity":2}' localhost:${PORT}/api/orders
  curl localhost:${PORT}/api/slow
  curl localhost:${PORT}/api/error
  tail -f ${LOG_FILE}

Make sure the agent is running (./run-agent.sh) and localdog is listening on ${LOCALDOG_URL}.
Stop with: $(dirname "$0")/stop-demo.sh
MSG

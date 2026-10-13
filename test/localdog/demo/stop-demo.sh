#!/usr/bin/env bash
# Stop the demo-node app started by run-demo.sh.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
if [[ -f "${PID_FILE}" ]] && kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
  kill "$(cat "${PID_FILE}")" && echo "Stopped demo-node (pid $(cat "${PID_FILE}"))."
else
  echo "demo-node is not running."
fi
rm -f "${PID_FILE}"

#!/usr/bin/env bash
# Stop and remove the localdog agent container.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
if docker ps -a --format '{{.Names}}' | grep -qx "${AGENT_CONTAINER}"; then
  docker rm -f "${AGENT_CONTAINER}" >/dev/null && echo "Removed ${AGENT_CONTAINER}."
else
  echo "${AGENT_CONTAINER} is not running."
fi

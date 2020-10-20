#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

# ${DATADOG_AGENT_IMAGE} is provided by the CI
test "${DATADOG_AGENT_IMAGE}" || {
    echo "DATADOG_AGENT_IMAGE envvar needs to be set" >&2
    exit 2
}

echo "DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"

cd "$(dirname "$0")"

# TODO run all workflows ?

./argo template create ../../argo-workflows/agent/templates/*.yaml
./argo submit ../../argo-workflows/agent/workflow.yaml -w --parameter datadog-agent-image-repository="${DATADOG_AGENT_IMAGE%:*}" --parameter datadog-agent-image-tag="${DATADOG_AGENT_IMAGE#*:}" || :
# we are waiting for the end of the workflow but we don't care about its return code
exit 0

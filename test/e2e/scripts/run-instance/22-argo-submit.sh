#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

# ${DATADOG_AGENT_IMAGE} and ${DATADOG_CLUSTER_AGENT_IMAGE} are provided by the CI
if [[ -z ${DATADOG_AGENT_IMAGE:+x} ]] || [[ -z ${DATADOG_CLUSTER_AGENT_IMAGE:+x} ]]; then
    echo "DATADOG_AGENT_IMAGE and DATADOG_CLUSTER_AGENT_IMAGE environment variables need to be set" >&2
    exit 2
fi

echo "DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
echo "DATADOG_CLUSTER_AGENT_IMAGE=${DATADOG_CLUSTER_AGENT_IMAGE}"

cd "$(dirname "$0")"

if [[ -n ${DOCKER_REGISTRY_URL+x} ]] && [[ -n ${DOCKER_REGISTRY_LOGIN+x} ]] && [[ -n ${DOCKER_REGISTRY_PWD+x} ]]; then
    oldstate=$(shopt -po xtrace ||:); set +x  # Do not log credentials
    kubectl create secret docker-registry docker-registry --docker-server="$DOCKER_REGISTRY_URL" --docker-username="$DOCKER_REGISTRY_LOGIN" --docker-password="$DOCKER_REGISTRY_PWD"
    eval "$oldstate"
fi

# TODO run all workflows ?

./argo template create ../../argo-workflows/templates/*.yaml
./argo submit ../../argo-workflows/workflow.yaml --wait \
       --parameter datadog-agent-image-repository="${DATADOG_AGENT_IMAGE%:*}" \
       --parameter datadog-agent-image-tag="${DATADOG_AGENT_IMAGE#*:}" \
       --parameter datadog-cluster-agent-image-repository="${DATADOG_CLUSTER_AGENT_IMAGE%:*}" \
       --parameter datadog-cluster-agent-image-tag="${DATADOG_CLUSTER_AGENT_IMAGE#*:}" || :
# we are waiting for the end of the workflow but we don't care about its return code
exit 0

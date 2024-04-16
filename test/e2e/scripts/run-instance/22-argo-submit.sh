#!/bin/bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo

# ${DATADOG_AGENT_IMAGE} and ${DATADOG_CLUSTER_AGENT_IMAGE} are provided by the CI
if [[ -z ${DATADOG_AGENT_IMAGE:+x} ]] || [[ -z ${DATADOG_CLUSTER_AGENT_IMAGE:+x} ]]; then
    echo "DATADOG_AGENT_IMAGE and DATADOG_CLUSTER_AGENT_IMAGE environment variables need to be set" >&2
    exit 2
fi

ARGO_WORKFLOW=${ARGO_WORKFLOW:-''}

echo "DATADOG_AGENT_IMAGE=${DATADOG_AGENT_IMAGE}"
echo "DATADOG_CLUSTER_AGENT_IMAGE=${DATADOG_CLUSTER_AGENT_IMAGE}"
echo "ARGO_WORKFLOW=${ARGO_WORKFLOW}"

cd "$(dirname "$0")"

if [[ -n ${DOCKER_REGISTRY_URL+x} ]] && [[ -n ${DOCKER_REGISTRY_LOGIN+x} ]] && [[ -n ${DOCKER_REGISTRY_PWD+x} ]]; then
    oldstate=$(shopt -po xtrace ||:); set +x  # Do not log credentials
    kubectl create secret docker-registry docker-registry --docker-server="$DOCKER_REGISTRY_URL" --docker-username="$DOCKER_REGISTRY_LOGIN" --docker-password="$DOCKER_REGISTRY_PWD"
    eval "$oldstate"
fi

argo_submit_cws_cspm() {
    DATADOG_AGENT_SITE=${DATADOG_AGENT_SITE:-""}

    oldstate=$(shopt -po xtrace ||:); set +x  # Do not log credentials

    if [[ -z ${DATADOG_AGENT_API_KEY:+x} ]] || [[ -z ${DATADOG_AGENT_APP_KEY:+x} ]]; then
        echo "DATADOG_AGENT_API_KEY, DATADOG_AGENT_APP_KEY environment variables need to be set" >&2
        exit 2
    fi

    kubectl create secret generic dd-keys \
        --from-literal=DD_API_KEY="${DATADOG_AGENT_API_KEY}" \
        --from-literal=DD_APP_KEY="${DATADOG_AGENT_APP_KEY}" \
        --from-literal=DD_DDDEV_API_KEY="${DD_API_KEY}"

    eval "$oldstate"

    ./argo template create ../../argo-workflows/templates/*.yaml
    ./argo submit ../../argo-workflows/$1 --wait \
        --parameter datadog-agent-image-repository="${DATADOG_AGENT_IMAGE%:*}" \
        --parameter datadog-agent-image-tag="${DATADOG_AGENT_IMAGE#*:}" \
        --parameter datadog-cluster-agent-image-repository="${DATADOG_CLUSTER_AGENT_IMAGE%:*}" \
        --parameter datadog-cluster-agent-image-tag="${DATADOG_CLUSTER_AGENT_IMAGE#*:}" \
        --parameter datadog-agent-site="${DATADOG_AGENT_SITE#*:}" \
        --parameter ci_commit_short_sha="${CI_COMMIT_SHORT_SHA:-unknown}" \
        --parameter ci_pipeline_id="${CI_PIPELINE_ID:-unknown}" \
        --parameter ci_job_id="${CI_JOB_ID:-unknown}" || :
}

case "$ARGO_WORKFLOW" in
    "cws")
        argo_submit_cws_cspm cws-workflow.yaml
        ;;
    "cspm")
        argo_submit_cws_cspm cspm-workflow.yaml
        ;;
    *)
        kubectl create secret generic dd-keys  \
            --from-literal=DD_API_KEY=123er \
            --from-literal=DD_APP_KEY=123er1 \
            --from-literal=DD_DDDEV_API_KEY="${DD_API_KEY}"

        ./argo template create ../../argo-workflows/templates/*.yaml
        ./argo submit "../../argo-workflows/${ARGO_WORKFLOW}-workflow.yaml" --wait \
            --parameter datadog-agent-image-repository="${DATADOG_AGENT_IMAGE%:*}" \
            --parameter datadog-agent-image-tag="${DATADOG_AGENT_IMAGE#*:}" \
            --parameter datadog-cluster-agent-image-repository="${DATADOG_CLUSTER_AGENT_IMAGE%:*}" \
            --parameter datadog-cluster-agent-image-tag="${DATADOG_CLUSTER_AGENT_IMAGE#*:}" \
            --parameter ci_commit_short_sha="${CI_COMMIT_SHORT_SHA:-unknown}" \
            --parameter ci_pipeline_id="${CI_PIPELINE_ID:-unknown}" \
            --parameter ci_job_id="${CI_JOB_ID:-unknown}" || :
        ;;
esac

# we are waiting for the end of the workflow but we don't care about its return code
exit 0

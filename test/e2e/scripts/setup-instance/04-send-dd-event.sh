#!/bin/bash
set -euo pipefail

EXIT_CODE=$1

if [[ -n ${CI_JOB_URL+x} ]] && [[ -n ${DD_API_KEY+x} ]]; then
    if [[ $EXIT_CODE -eq 0 ]]; then
        succeeded_or_failed='succeeded :-D'
        alert_type=success
    else
        succeeded_or_failed='failed ;-('
        alert_type=error
    fi

    oldstate=$(shopt -po xtrace ||:); set +x  # Do not log credentials
    curl -s -X POST "https://api.datadoghq.com/api/v1/events?api_key=${DD_API_KEY}" \
         -H "Content-Type: application/json" \
         -d @- <<EOF
{
  "title": "datadog-agent e2e tests result",
  "text": "E2e tests ${succeeded_or_failed}\ndatadog-agent image: ${DATADOG_AGENT_IMAGE}\ndatadog-cluster-agent image: ${DATADOG_CLUSTER_AGENT_IMAGE}\nCommit: ${CI_COMMIT_SHORT_SHA}\n\nGitLab job: ${CI_JOB_URL}\nArgo UI: http://${MACHINE}",
  "alert_type": "${alert_type}",
  "tags": [
    "app:agent-e2e-tests",
    "argo-workflow:$ARGO_WORKFLOW",
    "datadog-agent-image:$DATADOG_AGENT_IMAGE",
    "datadog-cluster-agent-image:$DATADOG_CLUSTER_AGENT_IMAGE"
  ]
}
EOF
    eval "$oldstate"
fi

exit "$EXIT_CODE"

#!/bin/bash
set -euo pipefail

EXIT_CODE=$1

if [[ -n ${CI_JOB_URL+x} ]] && [[ -n ${DD_API_KEY+x} ]]; then
    if [[ $EXIT_CODE -eq 0 ]]; then
        event="{
\"title\": \"e2e tests result\",
\"text\": \"E2e tests succeeded:\n${CI_JOB_URL}\",
\"alert_type\": \"success\",
\"tags\": [
  \"app:agent-e2e-tests\",
  \"datadog-agent-image:$DATADOG_AGENT_IMAGE\",
  \"datadog-cluster-agent-image:$DATADOG_CLUSTER_AGENT_IMAGE\"
]
}"
    else
        event="{
\"title\": \"e2e tests result\",
\"text\": \"E2e tests failed:\n${CI_JOB_URL}\",
\"alert_type\": \"error\",
\"tags\": [
  \"app:agent-e2e-tests\",
  \"datadog-agent-image:$DATADOG_AGENT_IMAGE\",
  \"datadog-cluster-agent-image:$DATADOG_CLUSTER_AGENT_IMAGE\"
]
}"
    fi
    oldstate=$(shopt -po xtrace); set +x  # Do not log credentials
    curl -s -X POST "https://api.datadoghq.com/api/v1/events?api_key=${DD_API_KEY}" \
         -H "Content-Type: application/json" \
         -d "$event"
    eval "$oldstate"
fi

exit "$EXIT_CODE"

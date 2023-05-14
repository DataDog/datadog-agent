#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"

if ! locale -k LC_CTYPE | grep -qi 'charmap="utf-\+8"'; then
    no_utf8_opt='--no-utf8'
fi

mkdir data

for workflow in $(./argo list -o name | grep -v 'No workflows found'); do
    JSON_CRD_FILE=data/$workflow.json
    JUNIT_XML_FILE=data/$workflow-junit.xml
    ./argo get ${no_utf8_opt:-} "$workflow" --output json > $JSON_CRD_FILE
    docker run -v $PWD/data:/data:z argo-to-junit-helper:local /$JSON_CRD_FILE /$JUNIT_XML_FILE
    DATADOG_API_KEY=$DD_API_KEY datadog-ci junit upload --service agent-e2e-tests $JUNIT_XML_FILE
done

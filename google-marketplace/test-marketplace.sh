#!/bin/bash

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Missing version to build/test"
    exit 1
fi

REGISTRY=gcr.io/datadog-sandbox/datadog
FULL_TAG=$1

gcrane cp "datadog/operator:$FULL_TAG" "$REGISTRY/datadog-operator:$FULL_TAG"
docker build --pull --no-cache --build-arg TAG="$FULL_TAG" --tag "$REGISTRY/deployer:$FULL_TAG" . && docker push "$REGISTRY/deployer:$FULL_TAG"
mpdev verify --deployer=$REGISTRY/deployer:"$FULL_TAG" --parameters='{"name": "datadog", "namespace": "datadog-agent", "datadog.credentials.apiKey": "00000000000000000000000000000000"}'

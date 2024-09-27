#!/bin/bash

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Missing version to build/test"
    exit 1
fi

REGISTRY=gcr.io/datadog-sandbox/datadog
FULL_TAG=$1

echo "Running gcrane cp"
gcrane cp "datadog/operator:$FULL_TAG" "$REGISTRY/datadog-operator:$FULL_TAG"
echo "Running docker build"
docker build --pull --platform linux/amd64 --no-cache --build-arg TAG="$FULL_TAG" --tag "$REGISTRY/deployer:$FULL_TAG" . && docker push "$REGISTRY/deployer:$FULL_TAG"
# Note: do not use "createAgent": true, as when resources are cleaned up mpdev will orphan the DatadogAgent
echo "Running mpdev verify"
EXTRA_DOCKER_PARAMS=--platform=linux/amd64 mpdev verify --deployer=$REGISTRY/deployer:"$FULL_TAG"

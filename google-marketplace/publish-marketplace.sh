#!/bin/bash

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Missing version to publish"
    exit 1
fi

REGISTRY=gcr.io/datadog-public/datadog
FULL_TAG=$1
SHORT_TAG=$(echo "$FULL_TAG" | cut -d '.' -f 1-2)

echo "### Copying operator '$FULL_TAG/$SHORT_TAG' from DockerHub to '$REGISTRY/operator'"

gcrane cp "datadog/operator:$FULL_TAG" "$REGISTRY/datadog-operator:$FULL_TAG"
gcrane cp "datadog/operator:$FULL_TAG" "$REGISTRY/datadog-operator:$SHORT_TAG"

echo "### Publishing Deployer"

APP_VERSION=$(yq eval '.spec.descriptor.version' chart/datadog-mp/templates/application.yaml)
if [ "$APP_VERSION" != "$FULL_TAG" ];
then
  echo "### Input version: $FULL_TAG does not match '.spec.descriptor.version' from chart/datadog-mp/templates/application.yaml ($APP_VERSION). Please update this file"
  exit 1
fi

docker build --pull --platform linux/amd64 --no-cache --build-arg TAG="$FULL_TAG" --tag "$REGISTRY/deployer:$FULL_TAG" . && docker push "$REGISTRY/deployer:$FULL_TAG"
docker tag "$REGISTRY/deployer:$FULL_TAG" "$REGISTRY/deployer:$SHORT_TAG" && docker push "$REGISTRY/deployer:$SHORT_TAG"

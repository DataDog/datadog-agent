#!/bin/bash

# Build discovery service images and load them into the Kind cluster.
# Run this on the KindVM EC2 host after provisioning.
# Usage: ./build-and-load.sh <version> [kind-cluster-name]

set -euo pipefail

VERSION=${1:-v0.0.7}
KIND_CLUSTER=${2:-discovery-local}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for img in discovery-python-svc discovery-python-instrumented \
           discovery-node-json-server discovery-node-instrumented \
           discovery-rails; do
  echo "Building ghcr.io/datadog/apps-${img}:${VERSION}..."
  docker build -t "ghcr.io/datadog/apps-${img}:${VERSION}" "${SCRIPT_DIR}/${img}/"
  echo "Loading into Kind cluster ${KIND_CLUSTER}..."
  kind load docker-image "ghcr.io/datadog/apps-${img}:${VERSION}" --name "${KIND_CLUSTER}"
done

echo "All images built and loaded."

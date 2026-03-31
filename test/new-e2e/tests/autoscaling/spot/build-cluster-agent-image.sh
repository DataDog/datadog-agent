#!/bin/bash
# Builds the cluster-agent image for Linux inside a devenv container.
# macOS binaries won't run in kind, so the build must happen inside Docker.
#
# Usage: ./build-cluster-agent-image.sh [image] [arch]
#   image  Docker image name (default: ${USER}/cluster-agent:test)
#   arch   arm64 or amd64 (default: arm64)
set -euo pipefail

CLUSTER_AGENT_IMAGE="${1:-${USER}/cluster-agent:test}"
DEVENV_ARCH="${2:-arm64}"
REPO_DIR="$(cd "$(dirname "$0")/../../../../.." && pwd)"

DEVENV_IMAGE="registry.ddbuild.io/ci/datadog-agent-devenv:1-${DEVENV_ARCH}"

echo "Building ${CLUSTER_AGENT_IMAGE} (linux/${DEVENV_ARCH}) from ${REPO_DIR}"

exec docker run --rm \
    --platform "linux/${DEVENV_ARCH}" \
    --cap-add=SYS_PTRACE \
    --security-opt seccomp=unconfined \
    -w "/workspaces/datadog-agent" \
    -v "${REPO_DIR}:/workspaces/datadog-agent" \
    -v "/var/run/docker.sock:/var/run/docker.sock" \
    -v "${GOPATH:-${HOME}/go}/pkg/mod:/home/datadog/go/pkg/mod" \
    -v "${HOME}/.ssh:/home/datadog/.ssh" \
    --user datadog \
    "${DEVENV_IMAGE}" \
    bash -xc "
        git config --global --add safe.directory /workspaces/datadog-agent && \
        dda inv -e cluster-agent.build && \
        dda inv -e cws-instrumentation.build && \
        dda inv -e cluster-agent.image-build --arch ${DEVENV_ARCH} -t ${CLUSTER_AGENT_IMAGE}
    "

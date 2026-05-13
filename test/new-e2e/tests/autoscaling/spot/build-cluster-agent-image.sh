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

# The Docker socket GID inside the Colima VM differs from the host GID
# (typically 991 for the docker group in the VM vs 0 on macOS). Detect it
# so --group-add can give the datadog user access to the socket.
DOCKER_GID=$(docker run --rm --platform "linux/${DEVENV_ARCH}" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    "${DEVENV_IMAGE}" stat -c '%g' /var/run/docker.sock 2>/dev/null || true)

GROUP_FLAGS=()
if [ -n "${DOCKER_GID}" ]; then
    GROUP_FLAGS=(--group-add "${DOCKER_GID}")
fi

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
    "${GROUP_FLAGS[@]}" \
    "${DEVENV_IMAGE}" \
    bash -xc "
        git config --global --add safe.directory /workspaces/datadog-agent && \
        dda inv -e cluster-agent.build && \
        dda inv -e cws-instrumentation.build && \
        dda inv -e cluster-agent.image-build --arch ${DEVENV_ARCH} -t ${CLUSTER_AGENT_IMAGE}
    "

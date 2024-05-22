#!/usr/bin/env bash

# Docker login:
# aws-vault exec sso-build-stable-developer -- \
#   aws ecr get-login-password --region us-east-1 | \
#   docker login --username AWS --password-stdin 486234852809.dkr.ecr.us-east-1.amazonaws.com

DATADOG_AGENT_BUILDIMAGE_FAMILY="deb_arm64"
DATADOG_AGENT_BUILDIMAGES_SUFFIX=""
DATADOG_AGENT_BUILDIMAGES="v34377294-680442b3"
DATADOG_REPO_FOLDER="${HOME}/dd/datadog-agent"

docker build \
       --build-arg DATADOG_AGENT_BUILDIMAGE_FAMILY="${DATADOG_AGENT_BUILDIMAGE_FAMILY}" \
       --build-arg DATADOG_AGENT_BUILDIMAGE_SUFFIX="${DATADOG_AGENT_BUILDIMAGE_SUFFIX}" \
       --build-arg DATADOG_AGENT_BUILDIMAGES="${DATADOG_AGENT_BUILDIMAGES}" \
       -t omnibus-bazel \
       -f ~/dd/omnibust/Dockerfile.omnibus-bazel \
       ~/dd/omnibust

mkdir -p "/tmp/bazel"

RELEASE_VERSION=nightly-a7
AGENT_MAJOR_VERSION=7
PYTHON_RUNTIMES=3
OMNIBUS_BASE_DIR=/omnibus
GO_MOD_CACHE_PATH="/go/pkg/mod"
OMNIBUS_GIT_CACHE_DIR="/tmp/omnibus-git-cache"
docker run -t \
       --name omnibus-bazel-c \
       --rm \
       -w /datadog-agent \
       -v ${DATADOG_REPO_FOLDER}:/datadog-agent \
       -v /tmp/bazel/:/tmp/bazel \
       --mount source=build_gomod,target="${GO_MOD_CACHE_PATH}" \
       --mount source=build_omnibus_git_cache,target="${OMNIBUS_GIT_CACHE_DIR}" \
       -e AGENT_MAJOR_VERSION="${AGENT_MAJOR_VERSION}" \
       -e PYTHON_RUNTIMES="${PYTHON_RUNTIMES}" \
       -e PACKAGE_ARCH=arm64 \
       -e DESTINATION_DEB="datadog-agent_7_arm64.deb" \
       -e DESTINATION_DBG_DEB="datadog-agent-dbg_7_arm64.deb" \
       -e DD_PKG_ARCH=arm64 \
       -e RELEASE_VERSION="${RELEASE_VERSION}" \
       -e OMNIBUS_GIT_CACHE_DIR=${OMNIBUS_GIT_CACHE_DIR=} \
       -e ENABLE_BAZEL=true \
       omnibus-bazel \
       inv -e omnibus.build --release-version "$RELEASE_VERSION" --major-version "$AGENT_MAJOR_VERSION" --python-runtimes "$PYTHON_RUNTIMES" --base-dir "${OMNIBUS_BASE_DIR}" --go-mod-cache="${GO_MOD_CACHE_PATH}" \
       | tee ~/dd/omnibust/omnibus.$(date +"%Y%m%d_%H%M%S").log

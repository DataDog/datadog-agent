# syntax=docker/dockerfile:1
ARG BASE_IMAGE=datadog/agent-dev:nightly-main-py3-jmx
FROM ${BASE_IMAGE}

# REPO_PATH must match the host path where the agent was built — the
# agent binary's RUNPATH and the bazel-built rtloader's RPATH are
# absolute paths inside the source checkout. Mirroring the path inside
# the image lets the dynamic linker resolve everything without relinking.
ARG REPO_PATH=/home/vagrant/go/src/github.com/DataDog/datadog-agent

COPY bin/agent/agent /opt/datadog-agent/bin/agent/agent

RUN mkdir -p ${REPO_PATH}/dev/lib ${REPO_PATH}/dev/embedded
COPY dev/lib/ ${REPO_PATH}/dev/lib/
COPY dev/embedded/ ${REPO_PATH}/dev/embedded/

# Alt PoC: no run_discover/runDiscover symbols needed (trial-mode check flow).
# Sanity check: verify the rtloader is present and executable.
RUN test -f ${REPO_PATH}/dev/lib/libdatadog-agent-rtloader.so \
    && test -f ${REPO_PATH}/dev/lib/libdatadog-agent-three.so

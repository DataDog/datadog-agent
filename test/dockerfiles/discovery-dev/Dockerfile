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

# Sanity check: fail the build early if the new symbols are missing.
# Use grep -a (treat binary as text) since nm/binutils are not in the base image.
RUN grep -aq runDiscover ${REPO_PATH}/dev/lib/libdatadog-agent-three.so \
    && grep -aq run_discover ${REPO_PATH}/dev/lib/libdatadog-agent-rtloader.so

ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
ARG BASE_IMAGE_REGISTRY=docker.io
ARG AGENT_BASE_IMAGE_TAG

# ------------------------------
# Build the Full Host Profiler
# ------------------------------
FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS builder
ARG BASE_IMAGE_UBUNTU_VERSION
ARG BASE_IMAGE_UBUNTU_NAME

# Set environment variables
ARG DD_GIT_COMMIT_SHA
ENV DEBIAN_FRONTEND=noninteractive

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH=/go


# Set the working directory
WORKDIR /workspace
# Copy from the CI build context, no-op when doing a stand alone build
COPY full-host* /workspace
COPY host-profiler* /workspace

# Update and install necessary packages
RUN \
        mkdir -p /workspace/datadog-agent/bin/full-host-profiler/dist && \
        cp /workspace/full-host-profiler /workspace/datadog-agent/bin/full-host-profiler/ && \
        cp /workspace/host-profiler-config.yaml /workspace/datadog-agent/bin/full-host-profiler/dist/host-profiler-config.yaml

# ------------------------------
# Inject the full host profiler in the base image
# ------------------------------

FROM $AGENT_BASE_IMAGE_TAG AS release

LABEL org.opencontainers.image.title="DDOT Ebpf"
# Copy the built full host profiler from the builder stage
COPY --from=builder /workspace/datadog-agent/bin/full-host-profiler/full-host-profiler /opt/datadog-agent/embedded/bin/full-host-profiler
COPY --from=builder /workspace/datadog-agent/bin/full-host-profiler/dist/host-profiler-config.yaml /etc/datadog-agent/host-profiler-config.yaml
# Find all directories and files in /etc with world-writable permissions and remove write permissions for group and others
RUN find /etc -type d,f -perm -o+w -print0 | xargs -r -0 chmod g-w,o-w
# Host profiler requires objcopy
RUN apt-get update && apt-get install -y binutils

ENTRYPOINT ["/opt/datadog-agent/embedded/bin/full-host-profiler"]
CMD ["run", "--config", "/etc/datadog-agent/host-profiler-config.yaml"]

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
COPY host-profiler* /workspace

# Update and install necessary packages
RUN \
        mkdir -p /workspace/datadog-agent/bin/host-profiler/dist && \
        cp /workspace/host-profiler /workspace/datadog-agent/bin/host-profiler/ && \
        cp /workspace/host-profiler-config.yaml /workspace/datadog-agent/bin/host-profiler/dist/host-profiler-config.yaml

# ------------------------------
# Inject the full host profiler in the base image
# ------------------------------

FROM $AGENT_BASE_IMAGE_TAG AS release
ARG CI

LABEL org.opencontainers.image.title="DDOT Ebpf"
# Copy the built full host profiler from the builder stage
COPY --from=builder /workspace/datadog-agent/bin/host-profiler/host-profiler /opt/datadog-agent/embedded/bin/host-profiler
COPY --from=builder /workspace/datadog-agent/bin/host-profiler/dist/host-profiler-config.yaml /etc/datadog-agent/host-profiler-config.yaml
COPY seccomp-profile.json /etc/dd-host-profiler/seccomp.json
COPY logging-seccomp.json /etc/dd-host-profiler/logging-seccomp.json
# Find all directories and files in /etc with world-writable permissions and remove write permissions for group and others
RUN find /etc -type d,f -perm -o+w -print0 | xargs -r -0 chmod g-w,o-w
# APT mirror resilience in CI: rewrite deb822 ubuntu.sources URIs: to an ordered
# multi-URI list (>= 24.04) and the legacy one-line sources.list via mirror+file
# (< 24.04), using Canonical's global endpoint first with Leaseweb as a fallback,
# plus a short Acquire timeout.
RUN if [ "$CI" = "true" ]; then \
  printf 'Acquire::Retries "1";\nAcquire::http::Timeout "10";\nAcquire::https::Timeout "10";\n' > /etc/apt/apt.conf.d/99datadog-mirror-resilience && \
  printf 'http://archive.ubuntu.com/ubuntu\nhttp://mirror.leaseweb.net/ubuntu\n' > /etc/apt/mirrorlist.main && \
  printf 'http://ports.ubuntu.com/ubuntu-ports\nhttp://mirror.leaseweb.net/ubuntu-ports\n' > /etc/apt/mirrorlist.ports && \
  if [ -f /etc/apt/sources.list ]; then \
    sed -i -e 's#http://archive.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
           -e 's#http://security.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
           -e 's#http://ports.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' /etc/apt/sources.list; \
  fi && \
  if [ -f /etc/apt/sources.list.d/ubuntu.sources ]; then \
    sed -i -E \
      -e 's#^(URIs:[[:space:]]+)https?://[a-z0-9.-]*archive\.ubuntu\.com/ubuntu/?[[:space:]]*$#\1http://archive.ubuntu.com/ubuntu http://mirror.leaseweb.net/ubuntu#' \
      -e 's#^(URIs:[[:space:]]+)https?://security\.ubuntu\.com/ubuntu/?[[:space:]]*$#\1http://archive.ubuntu.com/ubuntu http://mirror.leaseweb.net/ubuntu#' \
      -e 's#^(URIs:[[:space:]]+)https?://[a-z0-9.-]*ports\.ubuntu\.com/ubuntu-ports/?[[:space:]]*$#\1http://ports.ubuntu.com/ubuntu-ports http://mirror.leaseweb.net/ubuntu-ports#' \
      /etc/apt/sources.list.d/ubuntu.sources; \
  fi; \
  fi
# Host profiler requires objcopy; libcap2-bin provides setcap for file capabilities
RUN apt-get update && \
    apt-get install -y --no-install-recommends binutils libcap2-bin && \
    rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*
# Set file capabilities so the profiler has a populated permitted set when running as non-root.
RUN setcap cap_bpf,cap_perfmon,cap_sys_ptrace,cap_sys_resource,cap_dac_read_search,cap_syslog,cap_checkpoint_restore,cap_ipc_lock=ep \
    /opt/datadog-agent/embedded/bin/host-profiler
# backward-compatibility symlink as some existing Helm charts / operator may reference the old binary name
RUN ln -s /opt/datadog-agent/embedded/bin/host-profiler /opt/datadog-agent/embedded/bin/full-host-profiler

ENV PATH="/opt/datadog-agent/embedded/bin:${PATH}" \
    DOCKER_DD_AGENT=true

ENTRYPOINT ["host-profiler"]
CMD ["run", "--config", "/etc/datadog-agent/host-profiler-config.yaml"]

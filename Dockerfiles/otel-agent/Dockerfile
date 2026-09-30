ARG BASE_IMAGE_UBUNTU_VERSION=26.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
ARG BASE_IMAGE_REGISTRY=docker.io
ARG AGENT_BASE_IMAGE_TAG

# ------------------------------
# Build the OTel agent
# ------------------------------
FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION AS builder
ARG BASE_IMAGE_UBUNTU_VERSION
ARG BASE_IMAGE_UBUNTU_NAME

# Set environment variables
ARG CI
ARG DD_GIT_COMMIT_SHA
ENV DEBIAN_FRONTEND=noninteractive

ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH=/go


# Set the working directory
WORKDIR /workspace
# Copy from the CI build context, no-op when doing a stand alone build
COPY otel-* /workspace

# APT mirror resilience in CI:
# Ubuntu EC2 AMIs point apt at a single regional mirror (us-east-1) with no real
# failover. When that mirror is slow-but-reachable, apt would stall until the CI
# job timeout: the `mirror` method only falls back on a fully-unreachable mirror,
# and Acquire::Retries retries the SAME URI. The base image version is
# configurable (BASE_IMAGE_UBUNTU_VERSION), so we handle both source formats:
#   - deb822 ubuntu.sources (>= 24.04): rewrite the URIs: fields to an ordered
#     multi-URI list so apt fails over to the next mirror on error.
#   - legacy one-line sources.list (< 24.04): the format can't list multiple URIs
#     per entry, so use mirror+file with a peer mirrorlist (fallback on an
#     unreachable mirror).
# In both cases Canonical's global archive/ports.ubuntu.com endpoint is used first
# with mirror.leaseweb.net as a fallback, and a short Acquire
# timeout + Retries 1 turns a slow mirror into a fast error. Related:
# test/new-e2e/tests/installer/host/host.go (ConfigureAptMirrors).
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

# Update and install necessary packages
# TODO: For now we have to clone the whole repository and checkout to a specific commit / ref / branch then recompile the otel-agent
# Cloning the whole repo is necessary because `get_version` in the build script runs git describe --tags --candidates=50...
RUN \
    if [ ! -f /workspace/otel-agent ]; then \
        apt-get update && \
        apt-get install -y --no-install-recommends \
        curl \
        ca-certificates \
        software-properties-common \
        build-essential \
        git \
        python3 \
        python3-pip \
        python3-venv && \
        git clone --filter=blob:none https://github.com/DataDog/datadog-agent.git datadog-agent && \
        cd datadog-agent && \
        git checkout $DD_GIT_COMMIT_SHA && \
        mkdir -p $GOPATH && \
        GO_VERSION=$(cat .go-version) && \
        ARCH=$(dpkg --print-architecture) && \
        curl -OL https://golang.org/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz && \
        tar -C /usr/local -xzf go${GO_VERSION}.linux-${ARCH}.tar.gz && \
        rm go${GO_VERSION}.linux-${ARCH}.tar.gz && \
        python3 -m venv venv && \
        . venv/bin/activate && \
        pip install --upgrade pip && \
        pip install dda && \
        dda self dep sync -f legacy-tasks && \
        invoke collector.generate && \
        invoke otel-agent.build; \
    else \
        mkdir -p /workspace/datadog-agent/bin/otel-agent/dist && \
        cp /workspace/otel-agent /workspace/datadog-agent/bin/otel-agent/ && \
        cp /workspace/otel-config.yaml /workspace/datadog-agent/bin/otel-agent/dist/otel-config.yaml; \
    fi

# ------------------------------
# Inject the otel-agent in the base image
# ------------------------------

FROM $AGENT_BASE_IMAGE_TAG AS release

LABEL org.opencontainers.image.title="Otel Agent"
# Copy the built OTel agent from the builder stage
COPY --from=builder /workspace/datadog-agent/bin/otel-agent/otel-agent /opt/datadog-agent/embedded/bin/otel-agent
COPY --from=builder /workspace/datadog-agent/bin/otel-agent/dist/otel-config.yaml /etc/datadog-agent/otel-config.yaml
# Find all directories and files in /etc with world-writable permissions and remove write permissions for group and others
RUN find /etc -type d,f -perm -o+w -print0 | xargs -r -0 chmod g-w,o-w

ENV DD_OTELCOLLECTOR_ENABLED=true
ENV DD_OTEL_STANDALONE=true

ENTRYPOINT ["otel-agent"]
CMD ["run", "--config", "file:/etc/datadog-agent/otel-config.yaml"]

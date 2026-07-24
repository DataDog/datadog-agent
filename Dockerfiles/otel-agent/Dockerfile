ARG BASE_IMAGE_UBUNTU_VERSION=24.04
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

# NOTE about APT mirrorlists:
# It seems that this feature could use some improvement. If you just get mirrorlist
# from mirrors.ubuntu.com/mirrors.txt, it might contain faulty mirrors that either
# cause `apt update` to fail with exit code 100 or make it hang on `0% [Working]`
# indefinitely. Therefore we create a mirrorlist with the 2 mirrors that we know
# should be reliable enough in combination and also well maintained.
RUN if [ "$CI" = "true" ]; then \
  echo "http://us-east-1.ec2.archive.ubuntu.com/ubuntu\tpriority:1\nhttp://archive.ubuntu.com/ubuntu" > /etc/apt/mirrorlist.main && \
  echo "http://us-east-1.ec2.ports.ubuntu.com/ubuntu-ports\tpriority:1\nhttp://ports.ubuntu.com/ubuntu-ports" > /etc/apt/mirrorlist.ports && \
  for f in /etc/apt/sources.list /etc/apt/sources.list.d/ubuntu.sources; do \
    if [ -f "$f" ]; then \
      sed -i -e 's#http://archive.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
             -e 's#http://security.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.main#g' \
             -e 's#http://ports.ubuntu.com\S*#mirror+file:/etc/apt/mirrorlist.ports#g' "$f"; \
    fi; \
  done; \
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

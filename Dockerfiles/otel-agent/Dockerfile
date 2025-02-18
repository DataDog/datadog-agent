# Use the Ubuntu Slim AMD64 base image
FROM ubuntu:24.04 AS builder

# Set environment variables
ARG DD_GIT_COMMIT_SHA
ENV DEBIAN_FRONTEND=noninteractive

# Set the working directory
WORKDIR /workspace

# Update and install necessary packages
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    software-properties-common \
    build-essential \
    git \
    python3 \
    python3-pip \
    python3-venv \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# TODO: For now we have to clone the whole repository and checkout to a specific commit / ref / branch then recompile the otel-agent
# Cloning the whole repo is necessary because `get_version` in the build script runs git describe --tags --candidates=50...
# We should eventually just fetch the binary from s3 or directly the package but alpine doesn't have a service manager
RUN git clone --filter=blob:none https://github.com/DataDog/datadog-agent.git datadog-agent && \
    cd datadog-agent && \
    git checkout $DD_GIT_COMMIT_SHA


RUN ls /workspace/

# Set the working directory to the source code
WORKDIR /workspace/datadog-agent

# Install Go based on architecture
RUN GO_VERSION=$(cat .go-version) && \
    ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "amd64" ]; then \
    GO_ARCH="linux-amd64"; \
    elif [ "$ARCH" = "arm64" ]; then \
    GO_ARCH="linux-arm64"; \
    else \
    echo "Unsupported architecture: $ARCH" && exit 1; \
    fi && \
    curl -OL https://golang.org/dl/go${GO_VERSION}.$GO_ARCH.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.$GO_ARCH.tar.gz && \
    rm go${GO_VERSION}.$GO_ARCH.tar.gz

# Set up Go environment
ENV PATH="/usr/local/go/bin:${PATH}"
RUN mkdir /go
ENV GOPATH=/go

# Verify installations
RUN go version && \
    python3 --version && \
    curl --version

# Create and activate virtual environment, then install requirements
RUN python3 -m venv venv && \
    . venv/bin/activate && \
    pip install --upgrade pip && \
    pip install -r requirements.txt

# TODO confirm if we can remove this
# Copy the manifest file
# COPY manifest.yaml /workspace/datadog-agent/comp/otelcol/collector-contrib/impl/manifest.yaml

# Generate the files
RUN . venv/bin/activate && invoke collector.generate

# Build the OTel agent
RUN . venv/bin/activate && invoke otel-agent.build

# The base image could evolve in the future. Notably, using alpine could work as well but would
# mean that we have no utilities in the image. It is not convenient for debugging purposes.
FROM alpine:3.21.3

# Install gcompat to get glibc compatibility
RUN apk add gcompat=1.1.0-r4

# Copy the built OTel agent from the builder stage
COPY --from=builder /workspace/datadog-agent/bin/otel-agent/otel-agent /opt/datadog-agent/embedded/bin/otel-agent
# TODO: confirm if this should be copied to otel-config.yaml.example
COPY --from=builder /workspace/datadog-agent/bin/otel-agent/dist/otel-config.yaml /etc/datadog-agent/otel-config.yaml

ENV PATH="/opt/datadog-agent/embedded/bin:${PATH}"

# TODO: Check the content of the image (world writtable files, etc...)

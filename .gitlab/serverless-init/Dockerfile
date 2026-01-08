# syntax = docker/dockerfile:experimental
#
# Builds serverless-init container image (standard, Debian-based) from datadog-agent source
# This Dockerfile runs from the root of the datadog-agent repository
#
# Uses golang:1.25.5 to match the version in datadog-agent's go.work file
# When go.work is updated, this Dockerfile should be updated to match

# Build stage - compile the binary
FROM golang:1.25.5 as builder
ARG EXTENSION_VERSION
ARG AGENT_VERSION
ARG CMD_PATH
ARG BUILD_TAGS

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source files
COPY . .

# Build the binary
WORKDIR /build/${CMD_PATH}

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    if [ -z "$AGENT_VERSION" ]; then \
        go build -ldflags="-w \
        -X github.com/DataDog/datadog-agent/pkg/serverless/tags.currentExtensionVersion=$EXTENSION_VERSION" \
        -tags "${BUILD_TAGS}" -o datadog-agent; \
    else \
        go build -ldflags="-w \
        -X github.com/DataDog/datadog-agent/pkg/serverless/tags.currentExtensionVersion=$EXTENSION_VERSION \
        -X github.com/DataDog/datadog-agent/pkg/version.agentVersionDefault=$AGENT_VERSION" \
        -tags "${BUILD_TAGS}" -o datadog-agent; \
    fi

RUN go tool nm datadog-agent | grep -w 'github.com/DataDog/datadog-agent/pkg/version.agentVersionDefault' || \
    (echo "agentVersionDefault variable doesn't exist" && exit 1)

# Final image - distroless with shell and datadog-init
FROM gcr.io/distroless/cc-debian12
COPY --from=busybox:1.37-uclibc /bin/sh /bin/sh

COPY --from=builder /build/${CMD_PATH}/datadog-agent /datadog-init
COPY ./.gitlab/serverless-init/serverless_init_dotnet.sh /dotnet.sh

ENTRYPOINT ["/datadog-init"]

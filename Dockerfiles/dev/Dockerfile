# syntax=docker/dockerfile:1
#
# Docker dev image: builds the local agent binary using the same build environment
# as `dda env dev run -- dda inv -- agent.build`, then layers it on top of the
# official agent image so Python, JMX, and all embedded deps remain intact.
#
# The source tree is bind-mounted during the build RUN (not COPY'd), so it never
# appears as an image layer. Only the compiled artifacts are kept.
#
# Usage (BuildKit required):
#   docker buildx build -t datadog/agent-dev:local -f Dockerfiles/dev/Dockerfile .
#
# Optional build args:
#   --build-arg AGENT_IMAGE=gcr.io/datadoghq/agent:7.x.y  (default: latest)
#   --build-arg BUILD_EXTRA_ARGS=""  extra flags forwarded to agent.build

ARG AGENT_IMAGE=gcr.io/datadoghq/agent:latest

########################################################################
# Stage 1: Official agent image — source of embedded Python/libs
########################################################################
FROM ${AGENT_IMAGE} AS agent-base

########################################################################
# Stage 2: Build stage — same image used by `dda env dev run`
########################################################################
FROM datadog/agent-dev-env-linux AS builder

ARG BUILD_EXTRA_ARGS=""

# Copy the agent's embedded Python and native libs to their final install
# path so the binary's RPATH is correct without post-build patching.
COPY --from=agent-base /opt/datadog-agent/embedded/bin     /opt/datadog-agent/embedded/bin
COPY --from=agent-base /opt/datadog-agent/embedded/lib     /opt/datadog-agent/embedded/lib
COPY --from=agent-base /opt/datadog-agent/embedded/include /opt/datadog-agent/embedded/include

# Ensure patchelf is available to fix RPATH after build
RUN apt-get update -q && apt-get install -y --no-install-recommends patchelf && rm -rf /var/lib/apt/lists/*

# Mount the source tree read-write (overlay — host files are never modified).
# Build artifacts are written outside the mount to /build-output so they
# persist after the bind mount is released.
# The build context includes real git metadata (HEAD, refs, packed-refs) so
# `git rev-parse` / `git describe` work without needing the full object store.
RUN --mount=type=bind,source=.,target=/root/repos/datadog-agent,rw \
    mkdir -p /build-output && \
    cd /root/repos/datadog-agent && \
    LD_LIBRARY_PATH=/opt/datadog-agent/embedded/lib \
    dda inv agent.build \
        --build-exclude=systemd \
        --cmake-options="-DPython3_ROOT_DIR=/opt/datadog-agent/embedded -DPython3_FIND_STRATEGY=LOCATION" \
        ${BUILD_EXTRA_ARGS} && \
    cp bin/agent/agent /build-output/agent && \
    # Patch the RPATH so the binary finds rtloader in the official agent image's lib dir.
    # The rtloader .so files are NOT replaced — the ones already in the image are used.
    patchelf --set-rpath /opt/datadog-agent/embedded/lib /build-output/agent

########################################################################
# Stage 3: Final image — official agent runtime + locally built binary
########################################################################
FROM ${AGENT_IMAGE}

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get clean && \
    apt-get -o Acquire::Retries=4 update && \
    apt-get install -y --no-install-recommends bash-completion less vim && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Replace only the agent binary. The rtloader .so files already in the official
# image are kept — the binary's RPATH was patched to find them at runtime.
COPY --from=builder /build-output/agent \
    /opt/datadog-agent/bin/agent/agent

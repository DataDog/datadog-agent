ARG BASE_IMAGE_REGISTRY=docker.io

FROM ${BASE_IMAGE_REGISTRY}/debian:bookworm-slim AS release

LABEL org.opencontainers.image.title="Datadog Pipeline Recorder"
LABEL org.opencontainers.image.description="Sidecar that records Agent pipeline signals to Vortex columnar files"
LABEL maintainer="Datadog <package@datadoghq.com>"

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --chmod=0755 pipelinerecorder /usr/local/bin/pipelinerecorder

RUN mkdir -p /data/signals /var/run/pipelinesink && \
    chown -R 1000:1000 /data/signals /var/run/pipelinesink

USER 1000

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 \
    CMD test -S "${RECORDER_SOCKET_PATH:-/var/run/pipelinesink/pipeline.sock}" || exit 1

ENTRYPOINT ["/usr/local/bin/pipelinerecorder"]

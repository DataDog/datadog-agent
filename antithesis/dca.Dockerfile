# Antithesis-ready Datadog Cluster Agent image.
#
# This is a separate Dockerfile from Dockerfiles/cluster-agent/Dockerfile because the
# release Dockerfile only assembles a runtime image from externally-built CI artifacts
# (COPY --from=artifacts) — it has no from-source build stage to build against here.
#
# Build context MUST be the repo root: docker build -f antithesis/dca.Dockerfile .

ARG BASE_IMAGE_GOLANG_VERSION=1.26.5
ARG BASE_IMAGE_UBUNTU_VERSION=24.04

# ------------------------------
# Stage 1: build
# ------------------------------
FROM golang:${BASE_IMAGE_GOLANG_VERSION} AS builder
ENV DEBIAN_FRONTEND=noninteractive
ENV NO_COLOR=1
RUN apt-get update && apt-get install -y --no-install-recommends gcc git && \
    rm -rf /var/lib/apt/lists/*

# CGO is mandatory for the Antithesis SDK to actually report anything at runtime
ENV CGO_ENABLED=1

WORKDIR /src
COPY . .

# Same build tags as tasks/build_tags.bzl CLUSTER_AGENT_TAGS + `antithesis`
RUN go build \
      -tags "clusterchecks,datadog.no_waf,kubeapiserver,orchestrator,zlib,zstd,ec2,cel,antithesis" \
      -o /out/datadog-cluster-agent \
      ./cmd/cluster-agent

# ------------------------------
# Stage 2: runtime
# ------------------------------
FROM ubuntu:${BASE_IMAGE_UBUNTU_VERSION} AS runtime
ENV DEBIAN_FRONTEND=noninteractive
ENV NO_COLOR=1
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates adduser && \
    rm -rf /var/lib/apt/lists/*

COPY Dockerfiles/cluster-agent/conf.d /etc/datadog-agent/conf.d
COPY Dockerfiles/cluster-agent/datadog-cluster.yaml /etc/datadog-agent/datadog-cluster.yaml
COPY Dockerfiles/cluster-agent/install_info /etc/datadog-agent/install_info
COPY Dockerfiles/cluster-agent/entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh

COPY --from=builder /out/datadog-cluster-agent /opt/datadog-agent/bin/datadog-cluster-agent/datadog-cluster-agent
RUN chmod +x /opt/datadog-agent/bin/datadog-cluster-agent/datadog-cluster-agent && \
    ln -s /opt/datadog-agent/bin/datadog-cluster-agent/datadog-cluster-agent /opt/datadog-agent/bin/agent

COPY antithesis/setup-complete.sh /antithesis/setup-complete.sh
RUN chmod +x /antithesis/setup-complete.sh

RUN adduser --system --no-create-home --disabled-password --ingroup root dd-agent && \
    mkdir -p /var/log/datadog/ /conf.d /opt/datadog-agent/run && \
    chown -R dd-agent:root /etc/datadog-agent/ /var/log/datadog/ /conf.d /opt/datadog-agent/run

ENTRYPOINT ["/entrypoint.sh"]
CMD ["datadog-cluster-agent", "start"]

# Antithesis-ready Datadog Cluster Agent image.
#
# This is a separate Dockerfile from Dockerfiles/cluster-agent/Dockerfile because the
# release Dockerfile only assembles a runtime image from externally-built CI artifacts
# (COPY --from=artifacts) — it has no from-source build stage to build against here.
#
# Build context MUST be the repo root: docker build -f antithesis/dca.Dockerfile .
#
# NO Antithesis Go instrumentor / coverage instrumentation / static assertion
# cataloging here — deliberately. `antithesis-go-instrumentor`'s default mode loads
# and type-checks every package under the module root (go/packages.Load over the
# whole tree, not just cmd/cluster-agent's dependency closure), and this repo has a
# handful of unrelated packages elsewhere in the module (missing generated rtloader
# CGO headers, a stale benchmark, some build-tag-excluded network test utilities)
# that fail that load and kill cataloging for the entire binary — see
# antithesis/scratchbook/deployment-topology.md "Instrumentation scope risk" for the
# investigation. `blt/antithesis-harness` (PR #51515, logs-agent Antithesis work in
# this same repo) hit the identical problem and settled on the same answer: skip the
# instrumentor, link the Antithesis SDK directly, and call assert.* by hand at the
# sites that matter. That is what this image does — the SDK is a real dependency
# (see go.mod) and cmd/cluster-agent/subcommands/start/command.go calls
# assert.Reachable(...) as the bootstrap property. Coverage-guided fuzzing and the
# pre-run "assertion never reached" catalog are the accepted cost; every assert.*
# call still fires and reports normally at runtime.
#
# The actual compile is a PLAIN `go build` (dda inv cluster-agent.build's own
# go_build() step is exactly this — CGO_ENABLED=1, the same build tags, no Bazel:
# Bazel is only used by `dda inv tidy` for dependency/BUILD-file bookkeeping, not by
# the build itself). Static config/assets are the already-checked-in
# Dockerfiles/cluster-agent files, not regenerated — fine for a test harness, not a
# distributable release artifact.

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

# CGO is mandatory for the Antithesis SDK to actually report anything at runtime —
# never set CGO_ENABLED=0 anywhere in this build path.
ENV CGO_ENABLED=1

WORKDIR /src
COPY . .

# Same build tags as tasks/build_tags.bzl CLUSTER_AGENT_TAGS.
RUN go build \
      -tags "clusterchecks,datadog.no_waf,kubeapiserver,orchestrator,zlib,zstd,ec2,cel" \
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

# entrypoint.sh puts /opt/datadog-agent/bin/datadog-cluster-agent/ (a directory) on
# PATH, matching the local dev build layout (bin/datadog-cluster-agent/datadog-cluster-agent)
# — the binary must be nested one level deep, not copied flat to that path.
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

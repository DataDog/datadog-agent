# PAR dual-process container image.
#
# Builds both binaries from source using the repo as build context:
#   par-control  — Rust control plane (always-on, ~1-2 MB RSS)
#   par-executor — Go execution plane (spawned on-demand by par-control)
#
# Usage (build from repo root):
#   docker build -f Dockerfiles/par-dual/Dockerfile -t par-dual:local .
#
# See Dockerfiles/par-dual/docker-compose.yml for local testing.

# ── Stage 1: Build par-control (Rust) ────────────────────────────────────
FROM rust:1.91-slim-bookworm AS rust-builder

WORKDIR /build

# Cache Cargo registry between rebuilds.
COPY Cargo.toml Cargo.lock ./
COPY comp/core/log/rust/           comp/core/log/rust/
COPY pkg/discovery/module/rust/    pkg/discovery/module/rust/
COPY pkg/procmgr/rust/             pkg/procmgr/rust/
COPY pkg/privateactionrunner/controlplane/rust/ \
     pkg/privateactionrunner/controlplane/rust/

RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libssl-dev && rm -rf /var/lib/apt/lists/*

RUN cargo build --release -p par-control

# ── Stage 2: Build par-executor (Go) ─────────────────────────────────────
FROM golang:1.24-bookworm AS go-builder

WORKDIR /build
# Copy only what's needed to build par-executor (avoid transferring the
# whole repo — Go modules are fetched from the network).
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
      -o /out/par-executor \
      ./cmd/par-executor/

# ── Stage 3: Runtime image ────────────────────────────────────────────────
FROM debian:bookworm-slim

# ca-certificates is required for HTTPS calls to api.datadoghq.com (OPMS).
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates && rm -rf /var/lib/apt/lists/*

# Install both binaries in the same directory — par-control discovers
# par-executor as a sibling (same directory) by default.
COPY --from=rust-builder /build/target/release/par-control  /opt/datadog-agent/embedded/bin/par-control
COPY --from=go-builder   /out/par-executor                  /opt/datadog-agent/embedded/bin/par-executor

RUN chmod 755 /opt/datadog-agent/embedded/bin/par-control \
              /opt/datadog-agent/embedded/bin/par-executor

ENV PATH="/opt/datadog-agent/embedded/bin:${PATH}"

# Config is mounted at runtime via volume or ConfigMap.
# Minimal required keys in /etc/datadog-agent/datadog.yaml:
#   private_action_runner:
#     urn: "urn:dd:apps:on-prem-runner:..."
#     private_key: "<base64url-encoded-JWK>"
VOLUME ["/etc/datadog-agent"]

# par-control is the entry point; it spawns par-executor on demand.
# --executor-binary defaults to the sibling par-executor binary.
ENTRYPOINT ["par-control", "run", "--config", "/etc/datadog-agent/datadog.yaml"]

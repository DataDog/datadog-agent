# =============================================================================
# cargo-chef multi-stage build for fast rebuilds
# =============================================================================
# Stage 1 (planner): Analyze dependencies and create recipe.json
# Stage 2 (cacher): Build dependencies only (cached unless Cargo.toml changes)
# Stage 3 (builder): Build application code (fast, deps already built)
# Stage 4 (runtime): Minimal final image
# =============================================================================

FROM rust:1.90-bookworm AS chef
RUN cargo install cargo-chef
WORKDIR /build

# -----------------------------------------------------------------------------
# Stage 1: Planner - create recipe.json from Cargo.toml/Cargo.lock
# -----------------------------------------------------------------------------
FROM chef AS planner
COPY . .
RUN cargo chef prepare --recipe-path recipe.json

# -----------------------------------------------------------------------------
# Stage 2: Cacher - build dependencies only (this layer is cached)
# -----------------------------------------------------------------------------
FROM chef AS cacher

# Install build dependencies (protoc needed for datadog-protos)
RUN apt-get update && apt-get install -y \
    pkg-config \
    libssl-dev \
    protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

COPY --from=planner /build/recipe.json recipe.json
RUN cargo chef cook --release --recipe-path recipe.json

# -----------------------------------------------------------------------------
# Stage 3: Builder - compile application code (deps already built)
# -----------------------------------------------------------------------------
FROM cacher AS builder
COPY . .
RUN cargo build --release

# -----------------------------------------------------------------------------
# Stage 4: Runtime - minimal final image
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/target/release/fine-grained-monitor /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/fine-grained-monitor"]

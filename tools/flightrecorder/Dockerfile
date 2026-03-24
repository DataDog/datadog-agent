# Stage 1: build
FROM rust:latest AS builder

# Install build dependencies (g++ and clang for native deps).
RUN apt-get update && apt-get install -y --no-install-recommends \
    g++ \
    clang \
    libclang-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy Cargo manifests for dependency layer caching.
COPY tools/flightrecorder/Cargo.toml tools/flightrecorder/Cargo.lock* ./

# Copy generated FlatBuffers code for the stub layer (needed at compile time).
COPY tools/flightrecorder/src/generated ./src/generated/

# Pre-fetch and compile dependencies with a stub binary so the heavy dep
# compilation is cached as a separate layer.
RUN mkdir -p src benches \
    && touch src/lib.rs \
    && echo 'fn main(){}' > src/main.rs \
    && echo 'fn main(){}' > src/reader.rs \
    && echo 'fn main(){}' > src/validate.rs \
    && echo 'fn main(){}' > benches/transport_bench.rs \
    && echo 'fn main(){}' > benches/framing_bench.rs \
    && echo 'fn main(){}' > benches/writer_bench.rs \
    && echo 'fn main(){}' > benches/compression_bench.rs \
    && cargo build --release \
    && rm src/main.rs src/reader.rs src/validate.rs benches/*

# Copy real source and rebuild (only changed files recompile).
COPY tools/flightrecorder/src ./src/
COPY tools/flightrecorder/benches ./benches/
RUN touch src/main.rs src/reader.rs src/validate.rs && cargo build --release

# Stage 2: minimal runtime image
FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/target/release/flightrecorder /usr/local/bin/flightrecorder
COPY --from=builder /build/target/release/validate /usr/local/bin/validate

HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 \
    CMD test -S "${RECORDER_SOCKET_PATH:-/var/run/flightrecorder/pipeline.sock}" || exit 1

ENTRYPOINT ["/usr/local/bin/flightrecorder"]

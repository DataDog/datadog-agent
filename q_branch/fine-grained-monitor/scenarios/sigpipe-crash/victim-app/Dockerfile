# Build stage 1: Build Rust library
FROM rust:1.83-bookworm AS rust-builder

WORKDIR /rust
COPY metrics/Cargo.toml metrics/Cargo.toml
COPY metrics/src metrics/src

WORKDIR /rust/metrics
RUN cargo build --release

# Build stage 2: Build Go binary with CGO
FROM golang:1.22-bookworm AS go-builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy Rust library from previous stage
COPY --from=rust-builder /rust/metrics/target/release/libmetrics.so /usr/local/lib/
COPY --from=rust-builder /rust/metrics/target/release/libmetrics.a /usr/local/lib/

# Copy Go source
COPY go.mod ./
COPY main.go ./

# Build with CGO enabled, statically link the Rust library
RUN CGO_ENABLED=1 \
    CGO_LDFLAGS="-L/usr/local/lib -lmetrics -lpthread -ldl -static" \
    go build -o victim-app .

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy the Rust shared library
COPY --from=rust-builder /rust/metrics/target/release/libmetrics.so /usr/local/lib/

# Update library cache
RUN ldconfig

# Copy Go binary
COPY --from=go-builder /app/victim-app /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/victim-app"]

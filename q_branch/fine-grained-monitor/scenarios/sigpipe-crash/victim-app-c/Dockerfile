# Build stage: Build C library and Go binary
FROM golang:1.22-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy C source and build shared library
COPY metrics.c metrics.h ./
RUN gcc -shared -fPIC -pthread -o libmetrics.so metrics.c

# Copy Go source
COPY go.mod main.go ./

# Build Go binary with CGO
RUN CGO_ENABLED=1 \
    CGO_CFLAGS="-I." \
    CGO_LDFLAGS="-L. -lmetrics -lpthread" \
    go build -o victim-app .

# Runtime stage
FROM debian:bookworm-slim

# Copy the C shared library
COPY --from=builder /app/libmetrics.so /usr/local/lib/

# Update library cache
RUN ldconfig

# Copy Go binary
COPY --from=builder /app/victim-app /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/victim-app"]

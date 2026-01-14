# Build stage
FROM golang:1.22-bookworm AS builder

WORKDIR /app
COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 go build -o uds-server .

# Runtime stage
FROM debian:bookworm-slim

COPY --from=builder /app/uds-server /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/uds-server"]

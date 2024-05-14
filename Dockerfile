FROM --platform=amd64 golang:1.22.3-bookworm as builder
WORKDIR /app
COPY . .
RUN go build -tags=otlp -o otel-agent ./cmd/otel-agent

FROM --platform=amd64  datadog/agent:7.54.0-rc.3
COPY --from=builder /app/otel-agent /otel-agent

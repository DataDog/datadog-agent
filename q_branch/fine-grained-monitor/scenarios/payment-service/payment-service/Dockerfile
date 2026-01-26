# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies (needed for Kafka library)
RUN apk add --no-cache git gcc musl-dev librdkafka-dev pkgconf curl

WORKDIR /app

# Install swag for generating Swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@latest

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate Swagger documentation
RUN swag init -g main.go --output ./docs || echo "Warning: Swagger generation failed, continuing..."


# Update go.mod after swagger generation
RUN go mod tidy

# Build the application (CGO_ENABLED=1 required for Kafka library, use dynamic librdkafka)
RUN CGO_ENABLED=1 GOOS=linux go build -tags dynamic -a -o main .

# Final stage
FROM alpine:latest

# Install ca-certificates and runtime utilities
RUN apk --no-cache add ca-certificates wget librdkafka bash curl stress-ng

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Copy Swagger docs (if generated)
COPY --from=builder /app/docs ./docs

# Expose port
EXPOSE 8081

# Run the application
CMD ["./main"]

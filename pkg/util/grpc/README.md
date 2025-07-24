# gRPC Metrics System

This package provides comprehensive gRPC metrics collection for the Datadog Agent. It includes both client-side and server-side interceptors that automatically collect metrics for every gRPC call.

## Features

- **Request Count**: Tracks total number of gRPC requests processed
- **Error Count**: Tracks total number of gRPC errors encountered with detailed error codes
- **Request Duration**: Histogram of gRPC request latencies
- **Payload Size**: Histogram of payload sizes for both requests and responses
- **Active Requests**: Gauge tracking currently active (in-flight) requests
- **Enabled by Default**: No configuration required - metrics are collected automatically
- **Configurable**: Enable/disable specific metric types to control overhead

## Metrics Schema

| Metric Name                     | Type      | Description                                     | Tags                                      |
| ------------------------------- | --------- | ----------------------------------------------- | ----------------------------------------- |
| `grpc.request_count`            | Counter   | Total number of gRPC requests processed         | `method`, `service`, `peer`, `status`     |
| `grpc.error_count`              | Counter   | Total number of gRPC errors encountered         | `method`, `service`, `peer`, `error_code` |
| `grpc.request_duration_seconds` | Histogram | Distribution of gRPC request latencies          | `method`, `service`, `peer`               |
| `grpc.payload_size_bytes`       | Histogram | Distribution of payload sizes for gRPC calls    | `method`, `service`, `peer`, `direction`  |
| `grpc.active_requests`          | Gauge     | Number of currently active (in-flight) requests | `method`, `service`, `peer`               |

## Configuration

The gRPC metrics system is **enabled by default** and requires no configuration. All metrics are automatically collected when interceptors are added to gRPC servers and clients.

## Usage

### Server-Side Integration

To add metrics to an existing gRPC server:

```go
import (
    "google.golang.org/grpc"
    grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// Option 1: Use helper function
server := grpc.NewServer(grpcutil.ServerOptionsWithMetrics(
    grpc.Creds(credentials.NewTLS(tlsConfig)),
    grpc.MaxRecvMsgSize(maxMessageSize),
)...)

// Option 2: Use convenience function
server := grpcutil.NewServerWithMetrics(
    grpc.Creds(credentials.NewTLS(tlsConfig)),
    grpc.MaxRecvMsgSize(maxMessageSize),
)
```

### Client-Side Integration

To add metrics to gRPC clients:

```go
import (
    "google.golang.org/grpc"
    grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// Use helper function
conn, err := grpc.Dial("localhost:50051",
    grpcutil.ClientOptionsWithMetrics(
        grpc.WithTransportCredentials(creds),
    )...)
```

### Manual Integration

For more control, you can add interceptors manually:

```go
import (
    "google.golang.org/grpc"
    grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

server := grpc.NewServer(
    grpc.UnaryInterceptor(grpcutil.UnaryServerInterceptor()),
    grpc.StreamInterceptor(grpcutil.StreamServerInterceptor()),
    // ... other options
)
```

## Tag Definitions

### Core Tags

- `method`: Full gRPC method name (e.g., `/datadog.agent.AgentService/GetStatus`)
- `service`: Service portion of the method (e.g., `datadog.agent.AgentService`)
- `peer`: Client or server address (e.g., `127.0.0.1:50051`)

### Status Tags (for `grpc.request_count`)

- `status`: gRPC status code classification
  - Possible values: `OK`, `UNAVAILABLE`, `DEADLINE_EXCEEDED`, `RESOURCE_EXHAUSTED`, `INVALID_ARGUMENT`, `UNKNOWN`, etc.

### Error Tags (for `grpc.error_count`)

- `error_code`: Specific error or failure code
  - Includes gRPC statuses plus transport errors: `UNAVAILABLE`, `DEADLINE_EXCEEDED`, `connection_error`, `timeout_error`, `close_send_error`, etc.

### Direction Tags (for `grpc.payload_size_bytes`)

- `direction`: Indicates whether the size is for the request or response payload
  - Possible values: `request`, `response`

## Performance Considerations

- The metrics system has minimal impact on RPC performance
- Payload size tracking may have a small performance impact on large messages
- Active requests tracking adds minimal overhead
- Detailed error tracking is lightweight and only executed on errors
- All metrics are collected automatically with no configuration required

## Integration Examples

### Core Agent gRPC Server

```go
// In comp/api/grpcserver/impl-agent/grpc.go
func (s *server) BuildServer() http.Handler {
    authInterceptor := grpcutil.StaticAuthInterceptor(s.IPC.GetAuthToken())

    maxMessageSize := s.configComp.GetInt("cluster_agent.cluster_tagger.grpc_max_message_size")

    opts := []googleGrpc.ServerOption{
        googleGrpc.Creds(credentials.NewTLS(s.IPC.GetTLSServerConfig())),
        googleGrpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(authInterceptor)),
        googleGrpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(authInterceptor)),
        googleGrpc.MaxRecvMsgSize(maxMessageSize),
        googleGrpc.MaxSendMsgSize(maxMessageSize),
    }

    // Add metrics interceptors
    opts = grpcutil.ServerOptionsWithMetrics(opts...)

    grpcServer := googleGrpc.NewServer(opts...)
    // ... rest of server setup
}
```

### Cluster Agent gRPC Server

```go
// In cmd/cluster-agent/api/server.go
authInterceptor := grpcutil.AuthInterceptor(func(token string) (interface{}, error) {
    // ... auth logic
})

maxMessageSize := cfg.GetInt("cluster_agent.cluster_tagger.grpc_max_message_size")
opts := []grpc.ServerOption{
    grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(authInterceptor)),
    grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(authInterceptor)),
    grpc.MaxSendMsgSize(maxMessageSize),
    grpc.MaxRecvMsgSize(maxMessageSize),
}

// Add metrics interceptors
opts = grpcutil.ServerOptionsWithMetrics(opts...)

grpcSrv := grpc.NewServer(opts...)
```

## Monitoring and Alerting

With these metrics, you can create dashboards and alerts for:

- **Latency Issues**: Monitor `grpc.request_duration_seconds` percentiles
- **Error Rates**: Alert on high `grpc.error_count` rates
- **Throughput**: Track `grpc.request_count` rates
- **Payload Sizes**: Monitor `grpc.payload_size_bytes` for large messages
- **Active Connections**: Monitor `grpc.active_requests` for connection pools

## Troubleshooting

### Metrics Not Appearing

1. Verify the interceptors are properly added to your gRPC server/client
2. Check agent logs for any gRPC metric errors
3. Ensure the telemetry system is properly initialized

### Performance Issues

1. The system is optimized for minimal performance impact
2. All metrics are lightweight and designed for production use
3. Monitor the overall impact in your environment if needed

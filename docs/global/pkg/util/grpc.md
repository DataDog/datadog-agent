> **TL;DR:** Provides helpers for creating TLS-secured gRPC clients and servers within the Datadog Agent, including bearer-token authentication interceptors, mTLS enforcement, telemetry metrics, and a logger bridge.

# pkg/util/grpc

## Purpose

`pkg/util/grpc` provides helpers for setting up gRPC clients and servers within the Datadog Agent. It covers:

- **Client creation** — typed factory functions that open a TLS-secured `grpc.ClientConn` to a running agent process (TCP or vsock).
- **Authentication** — bearer-token interceptors for both client and server sides.
- **mTLS** — server interceptors that verify the client presented a TLS certificate.
- **Metrics** — telemetry interceptors that record request count, latency, payload size, and error counts for every RPC.
- **Timeout helper** — a deadline wrapper compatible with gRPC error codes.
- **Logging** — a bridge that redirects gRPC's internal logger to the Datadog logger.
- **Sub-package `context`** — typed context keys used to pass auth token info through the request context.

## Key elements

### Key functions

#### Client creation (`agent_client.go`)

| Function | Returns | Description |
|----------|---------|-------------|
| `GetDDAgentClient(ctx, ipcAddress, cmdPort, tlsConfig, opts...)` | `pb.AgentClient` | Creates a gRPC client for the main agent's unary API. Blocking by default (uses `grpc.WithBlock` + exponential backoff). |
| `GetDDAgentSecureClient(ctx, ipcAddress, cmdPort, tlsConfig, opts...)` | `pb.AgentSecureClient` | Same, but returns the secure (streaming) client. |

Both functions use TLS credentials derived from the supplied `*tls.Config`. When `vsock_addr` is set in the agent config, they substitute a vsock dialer so the same code works in VM/hypervisor environments.

Passing `cmdPort = "-1"` disables the client entirely (returns an error immediately).

#### Authentication (`auth.go`)

| Symbol | Description |
|--------|-------------|
| `AuthInterceptor(verifier verifierFunc) grpc_auth.AuthFunc` | Server-side interceptor factory. Extracts a `Bearer` token from request headers and passes it to the caller-supplied verifier. Stores the returned token info in the context under `grpccontext.ContextKeyTokenInfoID`. |
| `StaticAuthInterceptor(token string) grpc_auth.AuthFunc` | Convenience wrapper around `AuthInterceptor` that does a constant-time comparison against a fixed token. |
| `NewBearerTokenAuth(token string) credentials.PerRPCCredentials` | Client-side per-RPC credentials that inject `Authorization: Bearer <token>` into every call. Requires TLS. |

#### mTLS (`cert.go`)

| Symbol | Description |
|--------|-------------|
| `RequireClientCert` | Unary server interceptor. Returns `Unauthenticated` if the client did not present a TLS certificate. |
| `RequireClientCertStream` | Stream server interceptor variant. |

These interceptors complement `tls.VerifyClientCertIfGiven` in the TLS configuration: the TLS handshake validates the certificate's signature; the interceptors enforce that one was actually provided.

#### Server setup (`server.go`)

| Function | Description |
|----------|-------------|
| `NewServerWithMetrics(opts...)` | Creates a `*grpc.Server` with metrics interceptors pre-installed. |
| `ServerOptionsWithMetrics(opts...)` | Returns server options with metrics unary + stream interceptors prepended. |
| `ServerOptionsWithMetricsAndAuth(authUnary, authStream, opts...)` | Returns options with both metrics and auth interceptors combined. The metrics interceptor runs first, then auth, then the handler. |
| `CombinedUnaryServerInterceptor(authInterceptor)` | Composes a single unary interceptor: metrics -> auth -> handler. |
| `CombinedStreamServerInterceptor(authInterceptor)` | Same for streaming. |
| `ClientOptionsWithMetrics(opts...)` | Returns dial options with client-side metrics interceptors prepended. |

#### Metrics interceptors (`interceptors.go`, `metrics.go`)

The interceptors record four Prometheus/telemetry metrics under the `grpc` namespace:

| Metric | Type | Labels |
|--------|------|--------|
| `grpc.request_count` | Counter | `service_method`, `peer`, `status` |
| `grpc.error_count` | Counter | `service_method`, `peer`, `error_code` |
| `grpc.request_duration_seconds` | Histogram | `service_method`, `peer` |
| `grpc.payload_size_bytes` | Histogram | `service_method`, `peer`, `direction` (`request`/`response`) |

Payload size is tracked only for messages that implement `Size() int` (all protobuf-generated types do).

#### Timeout helper (`timeout.go`)

```go
func DoWithTimeout(f func() error, d time.Duration) error
```

Runs `f` in a goroutine and returns its error. If `d` elapses first it returns a gRPC `DeadlineExceeded` status error.

#### Logger (`logger.go`)

```go
func NewLogger() grpclog.LoggerV2
```

Creates a `grpclog.LoggerV2` that forwards gRPC-internal log output to the Datadog logger. The log level is controlled by the standard `GRPC_GO_LOG_SEVERITY_LEVEL` and `GRPC_GO_LOG_VERBOSITY_LEVEL` environment variables. The default level is `error`.

### Key types

#### `pkg/util/grpc/context`

Two typed context keys to avoid collisions in the request context:

| Key | Used for |
|-----|---------|
| `ConnContextKey` | HTTP connection object (set in `http.Server.ConnContext`). |
| `ContextKeyTokenInfoID` | Token info stored by `AuthInterceptor` after a successful verification. |

## Usage

### Open a client connection to the main agent

```go
import grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"

client, err := grpcutil.GetDDAgentClient(ctx, ipcAddress, cmdPort, tlsConfig)
if err != nil {
    return err
}
resp, err := client.GetHostname(ctx, &pb.HostnameRequest{})
```

### Create a server with metrics + auth

```go
import grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"

authUnary  := grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(token))
authStream := grpc_auth.StreamServerInterceptor(grpcutil.StaticAuthInterceptor(token))

srv := grpc.NewServer(grpcutil.ServerOptionsWithMetricsAndAuth(authUnary, authStream)...)
```

### Add bearer token to outgoing calls

```go
opts := []grpc.DialOption{
    grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(token)),
}
conn, err := grpc.DialContext(ctx, target, opts...)
```

### Redirect gRPC logs

```go
import (
    grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
    "google.golang.org/grpc/grpclog"
)

grpclog.SetLoggerV2(grpcutil.NewLogger())
```

### Where this package is used

- `comp/core/tagger/impl-remote/` — remote tagger gRPC client
- `comp/core/workloadmeta/collectors/internal/remote/` — remote workloadmeta collectors
- `comp/core/remoteagentregistry/` — remote agent registry client/server
- `pkg/config/remote/client/` — Remote Configuration gRPC client
- `pkg/process/checks/` — process check host-info gRPC call
- `pkg/security/utils/grpc/` — security agent gRPC utilities
- `comp/trace/config/` — trace-agent hostname resolution

## Cross-references

| Topic | See also |
|-------|----------|
| IPC bearer token and mTLS certificate lifecycle; `GetTLSServerConfig` / `GetTLSClientConfig` consumed by gRPC server setup | [`comp/core/ipc`](../../comp/core/ipc.md) |
| Agent gRPC server construction (`AgentServer`, `AgentSecureServer`) wired on top of this package's interceptors | [`comp/api/grpcserver`](../../comp/api/grpcserver.md) |
| HTTP + gRPC server lifecycle, CMD and IPC listener management | [`comp/api/api`](../../comp/api/api.md) |
| Raw auth token and IPC cert generation primitives used as credentials for gRPC connections | [`pkg/api`](../../pkg/api.md) |
| Security agent gRPC event/command clients that connect to system-probe | [`pkg/security/agent`](../security/agent.md) |
| Remote Configuration gRPC client (`NewGRPCClient`) used by sub-processes | [`pkg/config/remote`](../../pkg/config/remote.md) |
| HTTP transport helpers used alongside gRPC for REST-style IPC calls | [`pkg/util/http`](http.md) |

### IPC server construction flow

The agent's internal gRPC server is assembled in layers:

1. `comp/core/ipc` generates the TLS certificate and bearer token on disk.
2. `comp/api/grpcserver/impl-agent` calls `grpcutil.ServerOptionsWithMetricsAndAuth` (this package) with TLS credentials from `ipc.GetTLSServerConfig()` and the `StaticAuthInterceptor` token from `ipc.GetAuthToken()`.
3. `comp/api/api` receives the resulting `http.Handler` from `grpcserver.BuildServer()` and multiplexes it with the HTTP API on a single TCP listener via `helpers.NewMuxedGRPCServer`.

Sub-processes (trace-agent, security-agent, system-probe RC client) call `GetDDAgentClient` or `GetDDAgentSecureClient` with the TLS config and auth token provided by their own `comp/core/ipc` instance to establish the inbound gRPC connection.

### Security agent event stream

`pkg/security/agent` uses `RuntimeSecurityEventClient` (backed by protobuf definitions in `proto/api`) to open a streaming gRPC connection to system-probe. The bearer token credentials are injected via `grpcutil.NewBearerTokenAuth` and the vsock dialer path is selected when `vsock_addr` is set — the same mechanism used by `GetDDAgentClient`.

### Metrics interceptors and `pkg/telemetry`

The four gRPC metrics (`grpc.request_count`, `grpc.error_count`, `grpc.request_duration_seconds`, `grpc.payload_size_bytes`) are registered using `pkg/telemetry` constructors, so they appear in the shared Prometheus registry and are served at the agent's `/telemetry` endpoint. The interceptors in `interceptors.go` / `metrics.go` call `telemetry.NewCounter` and `telemetry.NewHistogram` at package-init time — no fx wiring is needed for these metrics. See [`pkg/telemetry`](../telemetry.md) for the Counter/Histogram API and [`comp/core/telemetry`](../../comp/core/telemetry.md) for how the `/telemetry` endpoint is composed.

### Relationship to `pkg/api`

`pkg/api/security` (token creation/loading) and `pkg/api/security/cert` (IPC certificate generation) are the lowest layer: they perform the raw file I/O. `comp/core/ipc` wraps those primitives into a lifecycle-managed component and exposes `GetAuthToken()` and `GetTLSServerConfig()`. This package (`pkg/util/grpc`) consumes those outputs: `StaticAuthInterceptor(ipc.GetAuthToken())` for the server side and `NewBearerTokenAuth(token)` / `grpc.WithTransportCredentials(credentials.NewTLS(ipc.GetTLSClientConfig()))` for the client side. See [`pkg/api`](../../pkg/api.md) for the raw primitives.

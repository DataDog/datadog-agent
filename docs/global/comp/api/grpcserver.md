# comp/api/grpcserver

**Team:** agent-runtimes

## Purpose

`comp/api/grpcserver` encapsulates the construction of the agent's internal gRPC server. Its single responsibility is to build an `http.Handler` (backed by a `google.golang.org/grpc.Server`) that is then multiplexed with the regular HTTP API inside a single TCP listener via `comp/api/grpcserver/helpers.NewMuxedGRPCServer`.

Separating server construction from the HTTP API component (`comp/api/api`) keeps the gRPC service registrations and their dependencies out of the generic API layer and makes it straightforward to swap or disable the gRPC server without touching anything else.

Two fx flavors are provided:

| Package | Behavior |
|---|---|
| `comp/api/grpcserver/fx-agent` | Full agent implementation: registers `AgentServer` and `AgentSecureServer` with TLS and auth interceptors |
| `comp/api/grpcserver/fx-none` | No-op implementation: `BuildServer()` returns `nil`, used when no gRPC endpoint is needed |

## Key Elements

### Interface (`comp/api/grpcserver/def/component.go`)

```go
type Component interface {
    BuildServer() http.Handler
}
```

`BuildServer` is called once by `comp/api/api` when it starts the CMD listener. It returns a fully configured gRPC server that can handle both TLS and h2c (unencrypted HTTP/2) connections. Returning `nil` (as the `none` implementation does) signals to the caller that no gRPC handler should be registered.

### Agent implementation (`comp/api/grpcserver/impl-agent`)

`BuildServer` creates a `google.golang.org/grpc.Server` with:
- TLS credentials from `comp/core/ipc` (`s.IPC.GetTLSServerConfig()`)
- Mutual TLS client certificate verification (unless `vsock_addr` is set)
- Unary and streaming interceptors for metrics and auth (`grpcutil.ServerOptionsWithMetricsAndAuth`)
- Configurable max message size via `cluster_agent.cluster_tagger.grpc_max_message_size`

Two gRPC services are registered on the server:

| Service | Proto | Purpose |
|---|---|---|
| `AgentServer` | `pbgo/core` | Unauthenticated service; exposes `GetHostname` |
| `AgentSecureServer` | `pbgo/core` | Authenticated service; exposes tagger streaming, workloadmeta streaming, DogStatsD capture, remote config, autodiscovery config streaming, workload filter evaluation, and remote agent registry |

### Dependencies of the agent implementation

```go
type Requires struct {
    DogstatsdServer     dogstatsdServer.Component
    Capture             replay.Component
    PidMap              pidmap.Component
    SecretResolver      secrets.Component
    RcService           option.Option[rcservice.Component]
    RcServiceMRF        option.Option[rcservicemrf.Component]
    IPC                 ipc.Component
    Tagger              tagger.Component
    TagProcessor        option.Option[tagger.Processor]
    Cfg                 config.Component
    AutoConfig          autodiscovery.Component
    Workloadfilter      workloadfilter.Component
    WorkloadMeta        workloadmeta.Component
    Collector           option.Option[collector.Component]
    RemoteAgentRegistry remoteagentregistry.Component
    Telemetry           telemetry.Component
    Hostname            hostnameinterface.Component
    ConfigStream        configstream.Component
}
```

All optional dependencies (`option.Option[...]`) are handled with safe defaults: if a service is absent, the corresponding gRPC method returns `codes.Unimplemented`.

### Muxing helper (`comp/api/grpcserver/helpers/grpc.go`)

```go
func NewMuxedGRPCServer(
    addr string,
    tlsConfig *tls.Config,
    grpcServer http.Handler,
    httpHandler http.Handler,
    timeout time.Duration,
) *http.Server
```

This utility (used by `comp/api/api`) creates an `http.Server` that inspects the `Content-Type` header to route `application/grpc` traffic to the gRPC handler and everything else to the HTTP handler. When `tlsConfig` is nil it wraps with `h2c` to support clients that do not negotiate HTTP/2 via TLS (used over vsock).

### Mock (`comp/api/grpcserver/mock/mock.go`)

A mock is provided for tests that wire `comp/api/api` without needing real gRPC services.

## Usage

### Wiring the full agent gRPC server

```go
import grpcAgentfx "github.com/DataDog/datadog-agent/comp/api/grpcserver/fx-agent"

// In a cmd's fx app:
grpcAgentfx.Module()
```

### Wiring the no-op server

```go
import grpcNonefx "github.com/DataDog/datadog-agent/comp/api/grpcserver/fx-none"

grpcNonefx.Module()
```

### How `comp/api/api` uses the component

`comp/api/api/apiimpl/server_cmd.go` calls `BuildServer()` and passes the result to `helpers.NewMuxedGRPCServer`, which multiplexes gRPC and HTTP traffic over the same listener:

```go
grpcServer := server.grpcComponent.BuildServer()
// grpcServer may be nil if using fx-none; NewMuxedGRPCServer handles that
```

### Where it is used

- `cmd/agent/subcommands/run/command.go` — main agent binary uses `fx-agent`
- `cmd/cluster-agent/api/server.go` — cluster-agent wires its own grpc server directly (does not use this component)
- `comp/api/api/apiimpl/api.go` — depends on `grpc.Component`; calls `BuildServer` when starting the CMD listener
- `pkg/network/usm/tests` — test setup
- `cmd/agent/subcommands/jmx/command.go` — JMX sub-command wires a grpc server for JMX metric collection

## Related components

| Component / Package | Relationship |
|---|---|
| [`comp/api/api`](api.md) | The HTTP API server component that consumes `grpcserver`. It calls `BuildServer()` at startup and passes the result to `helpers.NewMuxedGRPCServer` so that gRPC and HTTP traffic are multiplexed over the same CMD-server TCP listener. `comp/api/api` owns the listener lifecycle; `comp/api/grpcserver` owns the gRPC service registration. |
| [`comp/core/ipc`](../core/ipc.md) | Provides the TLS credentials and bearer token used to secure the gRPC server. `impl-agent` calls `ipc.GetTLSServerConfig()` for the server-side TLS config and `ipc.GetAuthToken()` for the `StaticAuthInterceptor`. Mutual-TLS verification (unless vsock is in use) is enforced by the `RequireClientCert` interceptors from `pkg/util/grpc`. |
| [`pkg/util/grpc`](../../pkg/util/grpc.md) | Low-level gRPC helper library. `impl-agent` passes `grpcutil.ServerOptionsWithMetricsAndAuth` to `grpc.NewServer`, which installs metrics interceptors (request count, latency, payload size) and bearer-token auth in a single call. All telemetry metrics emitted by the gRPC server originate from this package. |
| [`comp/remote-config/rcservice`](../remote-config/rcservice.md) | Injected as `option.Option[rcservice.Component]` into `impl-agent`. The `AgentSecureServer` routes `ClientGetConfigs`, `ConfigGetState`, `ConfigResetState`, and `CreateConfigSubscription` RPCs to it. When the option is absent (RC disabled), those methods return `codes.Unimplemented`. |

### Server assembly flow

```
comp/core/ipc
  └─ GetTLSServerConfig() → TLS credentials
  └─ GetAuthToken()       → bearer token

pkg/util/grpc
  └─ ServerOptionsWithMetricsAndAuth(authUnary, authStream)
        │
        ▼
comp/api/grpcserver/impl-agent (BuildServer)
  └─ grpc.NewServer(opts...)
  └─ pb.RegisterAgentServer(srv, ...)         ← GetHostname (unauthenticated)
  └─ pb.RegisterAgentSecureServer(srv, ...)   ← tagger, workloadmeta, remote-config, …
        │  http.Handler wrapping grpc.Server
        ▼
comp/api/api (CMD listener)
  └─ helpers.NewMuxedGRPCServer(addr, tlsCfg, grpcHandler, httpHandler)
        │  Content-Type: application/grpc → grpcHandler
        │  everything else               → httpHandler (HTTP mux)
        ▼
single TCP listener (cmd_port)
```

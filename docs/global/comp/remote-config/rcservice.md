# comp/remote-config/rcservice

**Team:** remote-config

## Purpose

`comp/remote-config/rcservice` is the server-side Remote Configuration (RC) service component. It runs inside the core agent (and related binaries) to poll the Datadog backend for configuration updates and distribute those updates to any client that requests them — tracers, sub-agents, and other processes that subscribe over gRPC.

The component is **optional**: if `remote_configuration.enabled` is `false` in the agent configuration, the constructor returns `option.None[rcservice.Component]` and nothing is started. This lets callers depend on `option.Option[rcservice.Component]` and handle the absent case gracefully.

## Key Elements

### Interface (`comp/remote-config/rcservice/component.go`)

```go
type Component interface {
    ClientGetConfigs(ctx context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
    ConfigGetState() (*pbgo.GetStateConfigResponse, error)
    ConfigResetState() (*pbgo.ResetStateConfigResponse, error)
    CreateConfigSubscription(stream pbgo.AgentSecure_CreateConfigSubscriptionServer) error
}
```

| Method | Purpose |
|---|---|
| `ClientGetConfigs` | Polling endpoint called by tracers and agents to get the latest RC payloads |
| `ConfigGetState` | Returns the current state of the configuration and director repos from the local uptane store |
| `ConfigResetState` | Clears the local uptane store and reinitializes the client (used to force a clean re-sync) |
| `CreateConfigSubscription` | Opens a streaming gRPC subscription for push-based config delivery |

### Params (`component.go`)

```go
type Params struct {
    Options []service.Option
}
```

Callers can inject extra `service.Option` values at wiring time via `rcservice.Params`. If the `*rcservice.Params` dependency is absent (marked `optional:"true"`), the service still starts with only the defaults derived from the agent configuration.

### fx module (`rcserviceimpl/rcservice.go`)

```go
func Module() fxutil.Module {
    return fxutil.Component(
        fx.Provide(newRemoteConfigServiceOptional),
    )
}
```

The internal constructor (`newRemoteConfigServiceOptional`) reads keys like `remote_configuration.api_key`, `remote_configuration.rc_dd_url`, `remote_configuration.refresh_interval`, `remote_configuration.clients.ttl_seconds`, etc. and builds the underlying `pkg/config/remote/service.Service`. Lifecycle hooks call `configService.Start()` on `OnStart` and `configService.Stop()` on `OnStop`.

A startup failure reason (e.g., invalid API key) is exported as an `expvar` under `remoteConfigStartup.startupFailureReason` for observability.

### Dependencies

The component requires:
- `comp/core/config` — agent configuration
- `comp/core/hostname` — agent hostname reported to the backend
- `comp/core/log` — structured logging
- `comp/remote-config/rctelemetryreporter` — sends RC-specific telemetry to the backend
- `*rcservice.Params` (optional) — caller-supplied extra options

### Related component

`comp/remote-config/rcservicemrf` follows the same shape and is used when multi-region failover (MRF) is enabled. The gRPC server exposes its methods under `ClientGetConfigsHA` and `GetConfigStateHA`.

## Usage

### Wiring the component into an fx app

```go
// In a cmd's fx app (e.g., cmd/agent):
rcserviceimpl.Module()
rctelemetryreporterimpl.Module()
```

If the binary needs to pass extra options (e.g., to restrict which config products are served):

```go
fx.Supply(&rcservice.Params{
    Options: []service.Option{
        service.WithProducts(rdata.ProductAPMTracing, rdata.ProductLiveDebugging),
    },
})
```

### Consuming the component from another component

Because the component is optional, always unwrap the `option.Option`:

```go
type MyDeps struct {
    fx.In
    RcService option.Option[rcservice.Component]
}

func (d MyDeps) handleRequest(ctx context.Context, req *pbgo.ClientGetConfigsRequest) {
    svc, ok := d.RcService.Get()
    if !ok {
        return // RC disabled
    }
    resp, err := svc.ClientGetConfigs(ctx, req)
    // ...
}
```

### Where it is used

- `comp/api/grpcserver/impl-agent` — exposes all four interface methods over the `AgentSecure` gRPC service, routing `ClientGetConfigs`, `GetConfigState`, `ResetConfigState`, and `CreateConfigSubscription` calls from external clients
- `comp/updater/updater/updaterimpl` — the Datadog Updater wires `rcservice` to receive RC-based install/update directives
- `cmd/installer/subcommands/daemon/run.go` — the installer daemon supplies custom `rcservice.Params` to scope the products served
- `cmd/cluster-agent/subcommands/start/command.go` — the cluster-agent starts the RC service for cluster-wide config distribution

## Related packages

| Package | Relationship |
|---------|-------------|
| [`comp/remote-config/rcclient`](rcclient.md) | The in-process RC client component. `rcclient` connects to `rcservice` over the gRPC IPC channel and wraps a `pkg/config/remote/client.Client`. It handles built-in products (`AGENT_CONFIG`, `AGENT_TASK`, `AGENT_FAILOVER`) and dispatches updates to other components via fx-group listeners. `rcservice` is the server-side gateway; `rcclient` is the consumer running inside the same agent process. |
| [`pkg/config/remote`](../../pkg/config/remote.md) | The underlying implementation. `rcserviceimpl` wraps `pkg/config/remote/service.CoreAgentService`, which polls the Datadog backend, verifies payloads with `uptane.CoreAgentClient` (BoltDB store), and serves `ClientGetConfigs` / `CreateConfigSubscription` to downstream gRPC clients. All Uptane TUF verification logic lives in `pkg/config/remote/uptane`. |
| [`pkg/remoteconfig/state`](../../pkg/remoteconfig.md) | Client-side TUF state machine consumed by `pkg/config/remote/client.Client` inside `rcclient`. `rcservice` itself relies on the Uptane client in `pkg/config/remote/uptane` rather than `state.Repository` directly, but all product constants (`ProductAPMSampling`, `ProductCWSDD`, etc.) referenced in `service.Option` calls originate from this package. |
| [`comp/api/grpcserver`](../api/grpcserver.md) | The gRPC server component that exposes `rcservice`'s methods over the network. `grpcserver/impl-agent` takes `option.Option[rcservice.Component]` as a dependency and routes `AgentSecureServer` RPC calls to it. When `rcservice` is absent, the corresponding gRPC methods return `codes.Unimplemented`. |
| [`comp/core/ipc`](../core/ipc.md) | Provides the bearer-token and mTLS credentials used to secure the gRPC IPC channel that downstream clients (including `rcclient`) use to reach `rcservice`. The gRPC server set up by `grpcserver/impl-agent` calls `ipc.GetTLSServerConfig()` and `ipc.GetAuthToken()` to configure TLS and the `StaticAuthInterceptor`. |

### Data-flow overview

```
Datadog RC backend (HTTPS)
        │
        ▼
pkg/config/remote/service.CoreAgentService   ← polls backend, Uptane verification (BoltDB)
        │  gRPC IPC (mTLS + bearer token)
        ▼
comp/remote-config/rcservice                 ← fx wrapper (optional); lifecycle Start/Stop
        │  exposed via AgentSecureServer
        ▼
comp/api/grpcserver/impl-agent               ← routes ClientGetConfigs / CreateConfigSubscription
        │
        ├─► comp/remote-config/rcclient      ← in-process consumer (AGENT_CONFIG, AGENT_TASK, …)
        ├─► trace-agent (client.NewGRPCClient)
        ├─► system-probe (CreateConfigSubscription streaming)
        └─► security-agent (client.NewGRPCClient)
```

> **TL;DR:** Manages the lifecycle of the agent's CMD HTTP API server (CLI-to-agent communication with bearer-token auth) and IPC API server (inter-process config access with mutual TLS), with all routes contributed via an fx endpoint group.

# comp/api/api — Agent HTTP API Server Component

**Team:** agent-runtimes
**Import path (interface):** `github.com/DataDog/datadog-agent/comp/api/api/def`
**Import path (implementation):** `github.com/DataDog/datadog-agent/comp/api/api/apiimpl`
**Importers:** ~40 packages

## Purpose

`comp/api/api` manages the lifecycle of the two HTTP servers that the core agent exposes locally:

- **CMD API server** — handles CLI-to-agent communication (flare, status, checks, diagnose, …). Accepts bearer-token authentication and optionally multiplexes gRPC traffic on the same port.
- **IPC API server** — exposes the `/config/v1/` endpoint that other agent processes (system-probe, process-agent, …) use to query runtime configuration values. Requires mutual TLS (`RequireAndVerifyClientCert`), so only processes that hold the IPC certificate can call it.

Both servers start and stop via fx lifecycle hooks. All endpoints are added through the `AgentEndpointProvider` fx group, so any component can contribute routes without directly depending on the server implementation.

## Package layout

| Package | Role |
|---|---|
| `comp/api/api/def` | `Component` interface, `EndpointProvider`/`AgentEndpointProvider` types, `AuthorizedSet`, `AuthorizedConfigPathsCore` |
| `comp/api/api/apiimpl` | Implementation: `apiServer` struct, fx `Module()`, `MockModule()` |
| `apiimpl/internal/agent` | Routes registered under `/agent/` (health, JMX status, component status/configs, install-info) |
| `apiimpl/internal/config` | `/config/v1/` endpoint with allowlist-based config key access |
| `apiimpl/listener` | Platform-specific `net.Listener` construction (Unix socket on Linux/macOS, TCP on other platforms) |
| `apiimpl/observability` | Telemetry middleware and response-logging middleware |
| `comp/api/api/utils` | gRPC helpers and streaming utilities shared by callers |

## Key Elements

### Key interfaces

## Component interface

```go
type Component interface {
    CMDServerAddress() net.Addr  // address of the CMD API server after start
    IPCServerAddress() net.Addr  // address of the IPC API server after start (nil if disabled)
}
```

Both methods return `nil` until the fx `OnStart` hook completes.

### Key types

## Registering endpoints — `AgentEndpointProvider`

Components contribute routes to the CMD API server by returning an `AgentEndpointProvider` from their constructor:

```go
// In your component's provides struct:
type provides struct {
    fx.Out
    Endpoint api.AgentEndpointProvider
}

// In the constructor:
return provides{
    Endpoint: api.NewAgentEndpointProvider(myHandlerFunc, "/my-route", "GET"),
}
```

`NewAgentEndpointProvider` accepts `(handler http.HandlerFunc, route string, methods ...string)`. All registered providers are collected by fx via the `"agent_endpoint"` group and wired into the router automatically when the CMD server starts.

All requests to the CMD server pass through `ipc.HTTPMiddleware`, which validates the bearer token — endpoints added via `AgentEndpointProvider` do not need to implement authentication themselves.

### Key functions

## IPC server and the config allowlist

The IPC server exposes `/config/v1/{path}` (GET a single key) and `/config/v1/` (GET all allowed keys). Access is restricted to the keys listed in `AuthorizedConfigPathsCore` (defined in `def/component.go`). Authorization is prefix-aware: if `logs_config.additional_endpoints` is in the set, any path starting with `logs_config.additional_endpoints.` is also permitted.

To expose additional keys from a non-core agent (e.g. security-agent), components can call `GetConfigEndpointMuxCore` or construct a separate `configEndpoint` with a custom `AuthorizedSet`.

### Configuration and build flags

## Security model

| Server | Authentication | Transport |
|---|---|---|
| CMD API | Bearer token (validated by `ipc.HTTPMiddleware`) | mTLS optional |
| IPC API | Mutual TLS (`RequireAndVerifyClientCert`) | mTLS mandatory |

The telemetry middleware tags metrics with `"mTLS"` or `"token"` depending on whether the client presented the IPC certificate.

## fx wiring

```go
// In the agent run command:
apiimpl.Module(),           // provides api.Component
grpcAgentfx.Module(),       // provides grpc.Component (multiplexed on CMD server)
commonendpoints.Module(),   // contributes AgentEndpointProvider entries
```

Dependencies injected by fx: `config.Component`, `ipc.Component`, `telemetry.Component`, `grpc.Component`, and all `[]api.EndpointProvider` from the `"agent_endpoint"` group.

## Mock

`apiimpl.MockModule()` provides a no-op implementation (`mockAPIServer`) that returns `nil` for both address methods. Use it in unit tests for components that depend on `api.Component` but do not need real HTTP servers:

```go
fxutil.Test[MyComponent](t, fx.Options(
    apiimpl.MockModule(),
    fx.Provide(newMyComponent),
))
```

## Usage patterns

**Contributing a new endpoint from a component:**

```go
// flare component example (comp/core/flare/flare.go)
return provides{
    Comp:     f,
    Endpoint: api.NewAgentEndpointProvider(f.createAndReturnFlarePath, "/flare", "POST"),
}
```

**Reading the CMD server address after startup:**

```go
func NewMyComp(apiComp api.Component) MyComp {
    // safe to call after OnStart
    addr := apiComp.CMDServerAddress()
}
```

## Related components

The API server ties together several orthogonal concerns. The following table
shows how each neighbouring component fits in.

| Component | Doc | Relationship |
|---|---|---|
| `comp/core/ipc` | [../core/ipc.md](../core/ipc.md) | Owns the bearer token and IPC TLS cert/key pair. The API server uses `ipc.HTTPMiddleware` to protect every CMD endpoint and mTLS for the IPC server. `ipc.GetTLSServerConfig()` is passed to the CMD listener. CLI commands use `ipc.GetClient()` to call the agent over the same transport. |
| `comp/core/status` | [../core/status.md](../core/status.md) | Registers `GET /status`, `GET /{component}/status`, and `GET /status/sections` as `AgentEndpointProvider` values. Each agent component contributes a `status.Provider` (or `status.HeaderProvider`) via the `"status"` fx group; the status component assembles them. |
| `comp/core/flare` | [../core/flare.md](../core/flare.md) | Registers `POST /flare` as an `AgentEndpointProvider`. The CLI's `datadog-agent flare` subcommand calls this endpoint (via `ipc.GetClient()`) to trigger archive creation inside the running daemon. |
| `comp/api/grpcserver` | [grpcserver.md](grpcserver.md) | Builds the gRPC `http.Handler` that is multiplexed with the regular HTTP handler inside the CMD server via `helpers.NewMuxedGRPCServer`. `Content-Type: application/grpc` traffic is routed to the gRPC handler; everything else goes to the HTTP mux. Swap `fx-agent` for `fx-none` to disable gRPC entirely. |
| `pkg/api` | [../../pkg/api.md](../../pkg/api.md) | Provides the low-level primitives that `comp/core/ipc` builds on: `security.FetchOrCreateAuthToken`, `cert.FetchOrCreateIPCCert`, `util.TokenValidator`, and the `/version` endpoint handler. The IPC component calls `cert.FetchOrCreateIPCCert` at startup; the API server uses `util.TokenValidator` (via `ipc.HTTPMiddleware`) to authenticate Bearer tokens. |

### How an endpoint travels from component to CMD server

```
MyComponent.constructor()
  └── return provides{
          Endpoint: api.NewAgentEndpointProvider(myHandlerFunc, "/my-route", "GET"),
      }
        │  fx group "agent_endpoint"
        ▼
apiimpl.apiServer (fx.In []api.EndpointProvider)
        │  router.HandleFunc("/my-route", ipc.HTTPMiddleware(myHandlerFunc))
        ▼
CMD server listener (TCP / Unix socket)
        │  bearer-token validation (ipc.HTTPMiddleware)
        ▼
myHandlerFunc called with authenticated *http.Request
```

### CMD server vs IPC server at a glance

```
CMD API server
  ├── authentication: Bearer token (ipc.HTTPMiddleware)
  ├── transport: Unix socket (Linux/macOS) or TCP
  ├── gRPC: multiplexed via helpers.NewMuxedGRPCServer (if grpcserver != none)
  └── routes contributed via AgentEndpointProvider fx group

IPC API server
  ├── authentication: mutual TLS (RequireAndVerifyClientCert)
  ├── transport: Unix socket (Linux/macOS) or TCP
  └── fixed routes: GET /config/v1/{path}, GET /config/v1/
                    (allowlist: AuthorizedConfigPathsCore)
```

### Adding a new endpoint — quick reference

1. In your component's constructor `provides` struct add:
   ```go
   Endpoint api.AgentEndpointProvider `group:"agent_endpoint"`
   ```
2. Assign it with:
   ```go
   Endpoint: api.NewAgentEndpointProvider(myHandlerFunc, "/my-route", "GET"),
   ```
3. No authentication code is needed — `ipc.HTTPMiddleware` validates the
   bearer token for all routes automatically.
4. For tests, use `apiimpl.MockModule()` to avoid starting real HTTP servers.

## Key dependents

- `comp/core/flare` — registers `POST /flare`; see [flare.md](../core/flare.md)
- `comp/core/settings/settingsimpl` — registers `GET|POST /config`
- `comp/core/status/statusimpl` — registers `GET /status`; see [status.md](../core/status.md)
- `comp/dogstatsd/server` — registers `/dogstatsd-stats` endpoint; see [dogstatsd/server.md](../dogstatsd/server.md)
- `comp/metadata/*` — several metadata components register inventory endpoints
- `cmd/agent/subcommands/run` — wires `apiimpl.Module()` into the main agent fx app

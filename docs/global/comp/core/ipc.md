# comp/core/ipc — Inter-Process Communication Component

**Team:** agent-runtimes
**Import path (interface):** `github.com/DataDog/datadog-agent/comp/core/ipc/def`
**Import path (fx wiring):** `github.com/DataDog/datadog-agent/comp/core/ipc/fx`

## Purpose

The IPC component owns the lifecycle of the authentication artifacts used when
agent processes communicate with each other: the bearer token (`auth_token`)
and the mutual-TLS certificate/key pair (`ipc_cert`). It exposes:

- Pre-configured TLS configs for both HTTP clients and HTTP servers.
- An HTTP middleware that validates the bearer token on every incoming request.
- A ready-to-use `HTTPClient` that adds the bearer token and correct TLS to
  every outgoing request.

Any agent process that calls another agent process's HTTP API (e.g., the CLI
calling the core agent, the process-agent calling the core agent) goes through
this component.

## Key elements

### Component interface (`def/component.go`)

```go
type Component interface {
    GetAuthToken() string
    GetTLSClientConfig() *tls.Config
    GetTLSServerConfig() *tls.Config
    HTTPMiddleware(next http.Handler) http.Handler
    GetClient() HTTPClient
}
```

`GetTLSClientConfig` and `GetTLSServerConfig` both return a clone of the
internal config, so callers can mutate the copy freely.

`HTTPMiddleware` wraps an `http.Handler` and rejects requests whose
`Authorization: Bearer <token>` header does not match the stored auth token.

### HTTPClient interface (`def/component.go`)

`HTTPClient` is the interface callers use to talk to other agent processes.
All methods automatically attach the bearer token and TLS configuration.

Key methods:

| Method | Description |
|---|---|
| `Get(url, ...opts)` | Authenticated GET request, returns body bytes |
| `Post(url, contentType, body, ...opts)` | Authenticated POST, returns body bytes |
| `PostChunk(url, contentType, body, onChunk, ...opts)` | Streaming POST — calls `onChunk` for each received chunk |
| `PostForm(url, data, ...opts)` | POST with form-encoded body |
| `Do(req, ...opts)` | Low-level: send any pre-built `*http.Request` |
| `NewIPCEndpoint(path)` | Creates a reusable `Endpoint` pointed at a path on the local agent |

`RequestOption` functions (`WithTimeout`, `WithContext`, `WithValues`,
`WithCloseConnection`) are provided in `comp/core/ipc/httphelpers` and passed
as variadic arguments.

### Endpoint interface (`def/component.go`)

A lightweight wrapper for a single pre-configured path:

```go
type Endpoint interface {
    DoGet(options ...RequestOption) ([]byte, error)
}
```

Obtained via `HTTPClient.NewIPCEndpoint(path)`. It reads `cmd_host` and
`cmd_port` from configuration to build the target URL.

### Transport details (`httphelpers/client.go`)

The underlying HTTP transport supports two connection modes, selected at
construction time:

- **Unix domain socket** — the default; connects to `agent_ipc.socket_path`.
  The scheme `https+unix://` is used internally.
- **vsock** — used when `vsock_addr` is configured (VM guest/host scenarios).

### fx module options (`fx/fx.go`)

Three constructors are available, each as its own fx module:

| Module function | Constructor used | When to use |
|---|---|---|
| `fx.ModuleReadWrite()` | `NewReadWriteComponent` | Long-running daemon processes. Reads existing artifacts or creates them. |
| `fx.ModuleReadOnly()` | `NewReadOnlyComponent` | One-shot CLI commands. Reads existing artifacts; fails if absent. |
| `fx.ModuleInsecure()` | `NewInsecureComponent` | Diagnostics / flare generation only. Always succeeds; falls back to an empty TLS config when artifacts are missing. Do not use in production paths. |

The `Provides` struct returned by the implementation exposes both
`ipc.Component` and `ipc.HTTPClient` as separate fx values, so components can
depend on just the client without taking the full component.

### Mock (`mock/mock.go`, build tag `test`)

`mock.New(t)` creates an `IPCMock` backed by a hard-coded test certificate.
It also provides `NewMockServer(handler)`, which starts an `httptest.Server`
with the IPC TLS config and automatically sets `cmd_host`/`cmd_port` in the
test configuration so that `NewIPCEndpoint` resolves correctly.

## Usage

### Server side — protecting an HTTP handler

```go
// In an HTTP server component that already depends on ipc.Component:
mux.Handle("/my/endpoint", ipcComp.HTTPMiddleware(myHandler))
```

### Client side — calling another agent process

```go
// Preferred: obtain the endpoint once and reuse it.
endpoint, err := ipcComp.GetClient().NewIPCEndpoint("/agent/status")
if err != nil { ... }

body, err := endpoint.DoGet(httphelpers.WithTimeout(5 * time.Second))
```

For one-off requests:

```go
body, err := ipcComp.GetClient().Get(
    "https://localhost:5001/agent/hostname",
    httphelpers.WithContext(ctx),
)
```

### In tests

```go
func TestMyHandler(t *testing.T) {
    ipcMock := ipcmock.New(t)
    ts := ipcMock.NewMockServer(myHandler)
    // ts.URL is automatically wired into cmd_host/cmd_port
    body, err := ipcMock.GetClient().Get(ts.URL + "/my/endpoint")
    ...
}
```

### Where it is wired in the agent

- `cmd/agent/subcommands/run/command.go` — core agent daemon uses `ModuleReadWrite`.
- `cmd/process-agent`, `cmd/security-agent`, `cmd/otel-agent` — daemon
  processes likewise use `ModuleReadWrite`.
- CLI subcommands (`flare`, `diagnose`, `status`, …) use `ModuleReadOnly` or
  `ModuleInsecure` depending on whether they can tolerate missing artifacts.
- [`comp/core/hostname/remotehostnameimpl`](hostname.md) — uses `ipc.Component` to obtain a
  TLS config for the gRPC connection back to the core agent.

## Relationship to pkg/api and comp/api/api

`comp/core/ipc` builds on lower-level primitives and integrates with the HTTP API layer:

| Layer | Package | Role |
|---|---|---|
| Token & cert generation | `pkg/api/security` | `FetchOrCreateAuthToken`, `cert.FetchOrCreateIPCCert` — raw file I/O |
| Token validation middleware | `pkg/api/util` | `TokenValidator` — constant-time bearer token check |
| IPC lifecycle management | `comp/core/ipc` | Wraps `pkg/api/security` and `pkg/api/security/cert`; provides `HTTPMiddleware`, `GetClient()`, TLS configs as an fx component |
| HTTP servers | `comp/api/api` | Starts the CMD API server (bearer token auth) and the IPC API server (mutual TLS) using `ipc.HTTPMiddleware` and `ipc.GetTLSServerConfig()` |

See [`pkg/api`](../../pkg/api.md) for the raw auth token and cert primitives, and [`comp/api/api`](../api/api.md) for how the two HTTP servers are composed.

### Config keys read

`comp/core/ipc` reads the following keys from [`comp/core/config`](config.md):

| Config key | Effect |
|---|---|
| `auth_token_file_path` | Location of the bearer token file on disk |
| `cmd_host` / `cmd_port` | Address the CMD API server listens on; used by `NewIPCEndpoint` to build target URLs |
| `agent_ipc.socket_path` | Unix domain socket path for the IPC HTTP transport (default on Linux/macOS) |
| `vsock_addr` | vSock address for VM guest/host scenarios |

### fxutil integration

In unit tests, use `ipcmock.New(t)` to get a test-certificate-backed mock and `NewMockServer` to spin up a real `httptest.Server` with matching TLS. For fx-based tests inject the mock via `fx.Provide`:

```go
fxutil.Test[MyComp](t, fx.Options(
    fx.Provide(func(t *testing.T) ipcdef.Component { return ipcmock.New(t) }),
    mycomp.Module(),
))
```

See [`pkg/util/fxutil`](../../pkg/util/fxutil.md) for the full test helper API.

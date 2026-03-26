> **TL;DR:** `comp/core/configsync` keeps satellite agent processes (process-agent, security-agent, system-probe, etc.) in sync with the core agent by periodically polling the core agent's `/config/v1/` IPC endpoint and applying returned values to the local config store.

# comp/core/configsync — Configuration Synchronization Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/configsync`
**Team:** agent-configuration
**Importers:** ~8 packages

## Purpose

`comp/core/configsync` is used by satellite agent processes (process-agent, security-agent, system-probe, trace-agent, otel-agent) to keep their local configuration in sync with the core agent. These processes do not own a `datadog.yaml` themselves; instead they periodically poll the core agent's `/config/v1/` IPC endpoint and apply any values it returns to their own `config.Component` with source `SourceLocalConfigProcess`.

This means that runtime configuration changes made to the core agent (e.g. via `datadog-agent config set`) automatically propagate to all satellite processes without restarting them.

## Package layout

| Package | Role |
|---|---|
| `comp/core/configsync` (root) | `Component` interface (empty — side-effect component) |
| `comp/core/configsync/configsyncimpl` | `configSync` struct, polling loop, `Module()`, `Params` |

## Key elements

### Key interfaces

#### Component interface

```go
type Component interface{}
```

The component has no public methods. Its behavior is entirely driven by its fx lifecycle hooks:

- **Optional init-sync** — if `Params.OnInitSync` is true, the component polls the core agent at construction time and retries until it succeeds or `OnInitSyncTimeout` expires (fails the entire fx application on timeout).
- **On `OnStart`** — begins a background goroutine that polls the core agent at `agent_ipc.config_refresh_interval` second intervals.
- **On `OnStop`** — cancels the goroutine's context, stopping the loop.

### Key types

#### Params

```go
type Params struct {
    Timeout           time.Duration // per-request timeout for each call to the core agent
    OnInitSync        bool          // block at init until first successful sync
    OnInitSyncTimeout time.Duration // how long to retry before failing init
}

func NewParams(syncTimeout, syncOnInit, syncOnInitTimeout) Params
func NewDefaultParams() Params  // all zero values — async-only, no init sync
```

### Configuration and build flags

#### Configuration keys

| Key | Description |
|---|---|
| `agent_ipc.config_refresh_interval` | Polling interval in seconds; set to `<=0` to disable configsync |
| `agent_ipc.host` | Core agent IPC host (used when `use_socket` is false) |
| `agent_ipc.port` | Core agent IPC port |
| `agent_ipc.use_socket` | Use a Unix socket instead of TCP (scheme `https+unix`) |

If `config_refresh_interval` is 0 or negative, or the port is invalid, the component disables itself and logs a message.

### Key functions

#### fx wiring

```go
configsyncimpl.Module(configsyncimpl.NewDefaultParams()),
```

`Module()` uses `fx.Invoke(func(_ configsync.Component) {})` internally to force instantiation, because the component has no consumers that depend on it by type. Simply including the module in the fx application is sufficient to start the sync loop.

The component depends on `config.Component`, `log.Component`, and `ipc.HTTPClient` (from `comp/core/ipc`).

### Interaction with comp/core/config

`configsync` applies values it receives from the core agent using the `SourceLocalConfigProcess` source. This source has lower priority than CLI overrides and fleet policies, but higher priority than defaults and environment variables. Satellite processes should therefore still supply their own `config.Params` (pointing to the same `datadog.yaml` copy or a stub file), because `configsync` only overlays values on top of the existing store — it does not replace it. See [`comp/core/config`](config.md) for the full source-priority hierarchy.

### Interaction with comp/core/ipc

`configsync` uses the `ipc.HTTPClient` (from [`comp/core/ipc`](ipc.md)) to contact the core agent's `/config/v1/` endpoint. The client automatically attaches the bearer token and the correct TLS configuration. The core agent must be running and reachable at `agent_ipc.host:agent_ipc.port` (or the socket path when `agent_ipc.use_socket` is true) for the sync to succeed.

## Usage across the codebase

- **`cmd/process-agent`** — `configsyncimpl.Module(configsyncimpl.NewDefaultParams())` — async sync
- **`cmd/system-probe`** — included in the run command, async sync
- **`cmd/security-agent`** — included in the start command, async sync
- **`cmd/trace-agent`** — async sync
- **`cmd/otel-agent`** — async sync
- **`cmd/host-profiler`** — async sync

Processes that must not start until they have a valid config use `OnInitSync: true` with an appropriate timeout. Most satellite processes use the default (async) mode and degrade gracefully if the core agent is not yet available.

## Related components

| Component | Relationship |
|---|---|
| [`comp/core/config`](config.md) | Provides the `config.Component` that `configsync` writes into; also owns `agent_ipc.config_refresh_interval` and other `agent_ipc.*` keys read by `configsync`. |
| [`comp/core/ipc`](ipc.md) | Provides `ipc.HTTPClient` used to poll the core agent's `/config/v1/` endpoint. Handles bearer token auth and mTLS transparently. |

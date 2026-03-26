> **TL;DR:** `comp/core/healthprobe` starts a lightweight, unauthenticated HTTP server exposing `/live`, `/ready`, and `/startup` endpoints so that Kubernetes and other orchestrators can poll agent health without going through the authenticated CMD API.

# comp/core/healthprobe — Health Probe HTTP Server Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/healthprobe`
**Team:** agent-runtimes
**Importers:** ~8 packages

## Purpose

`comp/core/healthprobe` starts a lightweight HTTP server that exposes liveness, readiness, and startup health endpoints. Orchestrators (Kubernetes, ECS, etc.) poll these endpoints to determine whether the agent process is alive and ready to serve traffic. The server is separate from the main agent API, runs on a dedicated port, and intentionally has no authentication.

The server reads health status from `pkg/status/health`, which aggregates registrations from across the agent. When any registered health check is unhealthy, the endpoint returns HTTP 500 and logs the offending check names. An optional goroutine-stack dump can be triggered on unhealthy responses to aid debugging.

## Package layout

| Package | Role |
|---|---|
| `comp/core/healthprobe/def` | `Component` interface, `Options` struct |
| `comp/core/healthprobe/impl` | `healthprobe` struct, HTTP server construction, `NewComponent` |
| `comp/core/healthprobe/fx` | `Module()` — wires `NewComponent` into fx |

## Key elements

### Key interfaces

#### Component interface

```go
type Component interface{}
```

The component has no public methods. Its value is in its side effect: it starts and stops a health HTTP server as part of the fx lifecycle.

### Key types

#### Options

`Options` must be supplied to fx before including `Module()`:

```go
type Options struct {
    Port           int   // TCP port to listen on; set to <=0 to disable the server
    LogsGoroutines bool  // if true, dump all goroutine stacks when a check is unhealthy
}
```

### Key functions

#### HTTP endpoints

All endpoints return a JSON object with `Healthy` and `Unhealthy` arrays (field names from `pkg/status/health.Status`).

| Path | Health source |
|---|---|
| `/live` | `health.GetLiveNonBlocking()` — checks registered as "live" |
| `/ready` | `health.GetReadyNonBlocking()` — checks registered as "ready" |
| `/startup` | `health.GetStartupNonBlocking()` — checks registered as "startup" |
| `/*` (default) | Same as `/live` — backward compatibility |

A 200 response means all registered checks in that category are healthy. A 500 response means at least one is unhealthy; the body still contains the full status object.

### Configuration and build flags

#### fx wiring

```go
fx.Provide(func(config config.Component) healthprobe.Options {
    return healthprobe.Options{
        Port:           config.GetInt("health_port"),
        LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
    }
}),
healthprobefx.Module(),
```

The `fx` module uses `fxutil.ProvideOptional` so the component is gracefully absent when the port is 0 or not configured. The server is registered with the fx lifecycle and shuts down cleanly with a 1-second timeout on `OnStop`.

#### Configuration keys

| Key | Default | Description |
|---|---|---|
| `health_port` | `0` (disabled) | Port the health probe listens on |
| `log_all_goroutines_when_unhealthy` | `false` | Dump goroutine stacks on unhealthy response |

## Usage across the codebase

Every long-running agent binary includes this component:

- **`cmd/agent`** — main agent, port from `health_port`
- **`cmd/system-probe`** — uses `system_probe_config.health_port`
- **`cmd/dogstatsd`** — standalone DogStatsD daemon
- **`cmd/cluster-agent`** and **`cmd/cluster-agent-cloudfoundry`** — cluster agent
- **`cmd/serverless-init`** — serverless agent init process

## Relationship to pkg/status/health and comp/core/status

`comp/core/healthprobe` and `comp/core/status` serve distinct purposes and should not be confused:

| Aspect | `comp/core/healthprobe` | `comp/core/status` |
|---|---|---|
| Audience | External orchestrators (Kubernetes, ECS) | Operators / CLI users |
| Transport | Dedicated unauthenticated HTTP port | Agent CMD API (authenticated) |
| Data source | `pkg/status/health` catalogs | All registered `status.Provider` implementations |
| Output format | JSON `{"Healthy":[...], "Unhealthy":[...]}` | JSON / text / HTML status page |

`pkg/status/health` is the shared catalog that feeds the healthprobe's `/live`, `/ready`, and `/startup` endpoints. Any component that registers via `health.RegisterLiveness`, `health.RegisterReadiness`, or `health.RegisterStartup` will be reflected in healthprobe responses. The same catalog is also exposed through the CMD API at `/agent/status/health` (served by `comp/core/status`).

`comp/core/status` additionally aggregates `status.Provider` sections (collector stats, forwarder queues, JMX state, etc.) which are richer human-facing data not surfaced by the health probe.

## Related components and packages

| Component / Package | Doc | Relationship |
|---|---|---|
| `pkg/status/health` | [../../pkg/status/status.md](../../pkg/status/status.md) | The health catalog that the probe server queries. Components call `health.RegisterLiveness` / `health.RegisterReadiness` / `health.RegisterStartup` and drain the returned `Handle.C` channel to signal they are healthy. The probe calls `GetLiveNonBlocking()`, `GetReadyNonBlocking()`, and `GetStartupNonBlocking()` to assemble each HTTP response. |
| `comp/core/status` | [status.md](status.md) | Sibling component that exposes the same `pkg/status/health` catalog at `/agent/status/health` on the CMD API, alongside richer status sections. Also registers a flare provider that adds `status.log` to every flare. |

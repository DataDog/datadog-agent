> **TL;DR:** `expvarserver` starts a localhost-only HTTP server exposing Go's standard `/debug/vars` endpoint so in-process counters and memory statistics can be inspected without a full Datadog pipeline.

# comp/agent/expvarserver

**Package:** `github.com/DataDog/datadog-agent/comp/agent/expvarserver`
**Team:** agent-runtimes

## Purpose

`expvarserver` starts a lightweight HTTP server on localhost that exposes Go's standard `expvar` debug endpoint (`/debug/vars`). This endpoint publishes in-process counters and gauges registered via the `expvar` package — memory statistics, goroutine counts, and any custom metrics components register. It is useful for local debugging and profiling without requiring a full Datadog pipeline.

The server is intentionally bound to `127.0.0.1` only, so it is not reachable from outside the host.

## Key Elements

### Key interfaces

```go
// def/component.go
type Component interface{}
```

Like `autoexit`, the interface carries no callable methods. All behaviour is the side effect of starting and stopping the HTTP server as part of the fx lifecycle.

### Key functions

`NewComponent` registers two lifecycle hooks:

- **`OnStart`**: creates `http.Server` bound to `127.0.0.1:<expvar_port>` using `http.DefaultServeMux` (which `expvar` registers its handler on automatically via its `init()` function), then launches it in a goroutine.
- **`OnStop`**: calls `server.Shutdown(context.Background())` for a clean teardown.

The `expvar` package is imported as a blank import in the agent `run` command to ensure its `/debug/vars` handler is registered:

```go
_ "expvar"
```

`expvarserverfx.Module()` wires `NewComponent` into the fx graph.

### Configuration and build flags

| Key | Type | Description |
|-----|------|-------------|
| `expvar_port` | string | Port for the expvar HTTP server (e.g. `"5000"`) |

## Usage

The component is included in `comp/agent/bundle.go`:

```go
// comp/agent/bundle.go
expvarserverfx.Module()
```

Consumers declare a dependency to force instantiation:

```go
fx.Invoke(func(_ expvarserver.Component) {})
```

Active in:

- **`cmd/agent`** — main agent (`run` and Windows service subcommands)

Once running, the endpoint is accessible at:

```
http://127.0.0.1:<expvar_port>/debug/vars
```

The port defaults to `5000` unless overridden by `expvar_port` in `datadog.yaml`.

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`comp/core/config`](../core/config.md) | `expvarserver` reads `expvar_port` from `config.Component` during `NewComponent`. The value is consumed once at startup; changing it at runtime has no effect without a restart. |
| [`comp/core/log`](../core/log.md) | `expvarserver` injects `log.Component` to log server-start errors (e.g. port already in use) and shutdown errors via `Errorf`. |
| [`comp/trace/agent`](../trace/agent.md) | The trace-agent process runs its own debug HTTP server (for `/config`, `/config/set`, `/secret/refresh`) which is separate from this component. `expvarserver` is only active in `cmd/agent` (the core agent), not in `cmd/trace-agent`. For the trace-agent debug server, see `comp/trace/agent`. |

### Exposing custom metrics

Any package that imports `expvar` and calls `expvar.NewInt` / `expvar.NewMap` / etc. will have its values published automatically through this server. The standard `expvar` package registers a handler on `http.DefaultServeMux` at `/debug/vars` via its `init()` function; because `expvarserver` binds to `http.DefaultServeMux`, no additional registration is needed.

Example — registering a custom counter visible at `/debug/vars`:

```go
import "expvar"

var myCounter = expvar.NewInt("my_component.events_processed")

// Increment elsewhere:
myCounter.Add(1)
```

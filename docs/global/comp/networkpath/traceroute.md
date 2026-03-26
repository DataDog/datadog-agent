# comp/networkpath/traceroute — Traceroute Component

**Import path:** `github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def`
**Team:** network-path
**Importers:** ~25 packages

## Purpose

`comp/networkpath/traceroute` executes a single on-demand traceroute from the agent host to a given destination, returning the full hop-by-hop network path as a structured `payload.NetworkPath`. It is the low-level primitive used by both the `network_path` integration check and the `npcollector` component to probe connectivity.

The component ships two interchangeable implementations behind the same interface:

- **local** (`impl-local`) — runs the traceroute directly inside the current process using `pkg/networkpath/traceroute/runner`. Used by the core agent and the private action runner, where the agent process has the required privileges or where system-probe is not available.
- **remote** (`impl-remote`) — delegates to the system-probe `TracerouteModule` over a Unix socket HTTP API. Used by the process-agent and similar processes that want to offload privileged network operations to system-probe.

Both variants annotate the result with the local agent hostname (from `comp/core/hostname`) and, when provided, the source container ID.

## Package layout

| Package | Role |
|---|---|
| `comp/networkpath/traceroute/def` | Component interface |
| `comp/networkpath/traceroute/impl-local` | In-process implementation |
| `comp/networkpath/traceroute/impl-remote` | system-probe delegation implementation |
| `comp/networkpath/traceroute/fx-local` | fx `Module()` wiring `impl-local` |
| `comp/networkpath/traceroute/fx-remote` | fx `Module()` wiring `impl-remote` |
| `comp/networkpath/traceroute/mock` | Test mock |

## Component interface

```go
// Package: github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def
type Component interface {
    Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error)
}
```

### `config.Config` (key fields)

Defined in `pkg/networkpath/traceroute/config`:

| Field | Description |
|---|---|
| `DestHostname` | Target hostname or IP address |
| `DestPort` | Destination port (TCP probes) |
| `Protocol` | `TCP`, `UDP`, or `ICMP` |
| `TCPMethod` | `SYN` or `ACK` for TCP traceroutes |
| `MaxTTL` | Maximum number of hops |
| `Timeout` | Per-probe timeout |
| `ReverseDNS` | Whether the runner should resolve hop IPs (npcollector disables this and handles rDNS itself) |
| `SourceContainerID` | Attached to the path source metadata |

### `payload.NetworkPath` (return value)

Defined in `pkg/networkpath/payload`. Contains structured source and destination metadata, a `Traceroute` field with per-run hop lists, and timing information. The `Source.Hostname` field is always populated by the component implementations from the agent hostname.

## fx wiring

Choose the module that matches the execution context:

```go
// Core agent or private action runner (runs traceroute in-process)
import fxlocal "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-local"
fxlocal.Module()

// Process-agent or any process delegating to system-probe
import fxremote "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
fxremote.Module()
```

Both modules register the component as optional (`fxutil.ProvideOptional`) so callers can inject `optional.Option[traceroute.Component]` when the component may not be present.

The **local** implementation requires `hostname.Component`, `telemetry.Component`, and `compdef.Lifecycle` (it starts a background runner goroutine on `OnStart`).

The **remote** implementation requires `hostname.Component`, `log.Component`, and `sysprobeconfig.Component` to locate the system-probe Unix socket.

## Usage

**Injecting as a dependency:**

```go
import traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"

type Requires struct {
    fx.In
    Traceroute traceroute.Component
}

func (c *myComp) runCheck(ctx context.Context) error {
    path, err := c.traceroute.Run(ctx, config.Config{
        DestHostname: "8.8.8.8",
        DestPort:     443,
        Protocol:     payload.ProtocolTCP,
        MaxTTL:       30,
    })
    // path.Traceroute.Runs contains per-run hop data
}
```

**Key callers:**

- `pkg/collector/corechecks/networkpath` — the `network_path` agent check calls `Run` on each check execution to send a path event to the event platform.
- `comp/networkpath/npcollector` — calls `Run` for every connection that passes its scheduling filters, then enriches the result with reverse DNS before forwarding.
- `comp/syntheticstestscheduler` — calls `Run` for synthetic network path tests.
- `pkg/privateactionrunner` — calls `Run` in workflow action contexts.

## Notes

- The remote implementation computes a dynamic HTTP timeout as `timeout * maxTTL + 10s` to account for full TCP traceroute completion time.
- When system-probe's traceroute module is not running, the remote implementation returns a descriptive error prompting the operator to enable `traceroute_module.enabled: true` in `system-probe.yaml`.
- The mock (`comp/networkpath/traceroute/mock`) is provided for unit tests that need to exercise callers without performing real network probes.
- `npcollector` sets `config.Config.ReverseDNS = false` and performs its own rDNS enrichment via `rdnsquerier` after the traceroute completes; by contrast, the `network_path` check sets `ReverseDNS = true` and relies on the runner to resolve hop IPs.

## Related components and packages

| Component / Package | Relationship |
|---|---|
| [`comp/networkpath/npcollector`](npcollector.md) | The primary consumer of this component for automated, traffic-driven tracerouting. Calls `Run` for every connection that passes scheduling filters, then enriches results with rDNS before forwarding. |
| [`pkg/networkpath`](../../pkg/networkpath.md) | Defines the shared types consumed by this component: `config.Config` (input), `payload.NetworkPath` (output), and the `runner.Runner` used by the local implementation. |
| [`comp/forwarder/eventplatform`](../forwarder/eventplatform.md) | Completed `NetworkPath` payloads are ultimately forwarded to the event platform as `EventTypeNetworkPath` events. This component does not call the forwarder directly — callers such as `npcollector` and the `network_path` check handle forwarding. |

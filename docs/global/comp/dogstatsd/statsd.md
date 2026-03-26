> **TL;DR:** A thin fx-injectable factory for `datadog-go/v5/statsd` clients that gives any agent component a pre-configured DogStatsD client without needing to know the server address or tune client options.

# comp/dogstatsd/statsd — StatsD Client Component

**Import path:** `github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def`
**Team:** agent-metric-pipelines
**Importers:** ~22 packages

## Purpose

`comp/dogstatsd/statsd` is a thin fx-injectable factory for `datadog-go/v5/statsd` clients. It gives any agent component a pre-configured StatsD client without having to know the local DogStatsD address or choose sensible defaults for write timeouts, channel mode, or client-side aggregation.

The component is intentionally minimal: it does not own a socket or parse packets. Its sole job is to construct `ddgostatsd.ClientInterface` instances that point at the running DogStatsD server (or any other StatsD endpoint).

## Package layout

| Package | Role |
|---|---|
| `comp/dogstatsd/statsd/def` | `Component` interface |
| `comp/dogstatsd/statsd/impl` | Production implementation (`service` struct) |
| `comp/dogstatsd/statsd/fx` | `Module()` — wires `impl.NewComponent` into fx |
| `comp/dogstatsd/statsd/mock` | Test mock backed by `ddgostatsd.NoOpClient` |
| `comp/dogstatsd/statsd/otel` | OTel-flavoured wrapper (used by the OTel collector pipeline) |

## Key elements

### Key interfaces

```go
type Component interface {
    // Get returns a lazily-created, process-wide shared client.
    // The address is taken from the STATSD_URL environment variable.
    // The client is created on the first call and reused thereafter.
    Get() (ddgostatsd.ClientInterface, error)

    // Create returns a new client using default options.
    // Address resolution: STATSD_URL env var takes precedence,
    // then falls back to 127.0.0.1:8125.
    Create(options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error)

    // CreateForAddr returns a new client that defaults to addr when
    // STATSD_URL is not set.
    CreateForAddr(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error)

    // CreateForHostPort is the same as CreateForAddr but takes host
    // and port as separate arguments.
    CreateForHostPort(host string, port int, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error)
}
```

`ddgostatsd.ClientInterface` is defined in `github.com/DataDog/datadog-go/v5/statsd` and covers the full StatsD API: `Gauge`, `Count`, `Histogram`, `Distribution`, `Set`, `Timing`, `Event`, `ServiceCheck`, `Flush`, `Close`, etc.

### Configuration and build flags

Address resolution priority and default client options are described below.

## Address resolution

All factory methods follow this priority order when selecting a target address:

1. `STATSD_URL` environment variable (if set, always wins)
2. The `addr` / `host:port` argument passed to the factory method
3. Hardcoded fallback: `127.0.0.1:8125`

This means that in a containerised environment you can point every component at a remote DogStatsD endpoint by setting a single environment variable.

## Default client options

Every client created through this component ships with performance-oriented defaults:

| Option | Value | Reason |
|---|---|---|
| `WithChannelMode()` | enabled | Decouples the caller from network I/O |
| `WithClientSideAggregation()` | enabled | Reduces packet count for counters and gauges |
| `WithExtendedClientSideAggregation()` | enabled | Also aggregates histograms/sets client-side |
| `WithWriteTimeout` | 500 ms | Avoids indefinite blocking on slow sockets |
| `WithConnectTimeout` | 3 s | Tolerates brief DogStatsD startup delays |
| `WithTelemetryAddr` | same as client addr | Sends `datadog-go` library telemetry to DogStatsD |

Callers may override any of these by appending their own `ddgostatsd.Option` values; caller-provided options take precedence because they are appended after the defaults.

## fx wiring

```go
// Add to an fx app (typically inside a bundle or command)
statsd.Module()  // from comp/dogstatsd/statsd/fx
```

The component has **no fx dependencies** — it is a pure factory that does not require config, log, or lifecycle hooks. `NewComponent` always succeeds.

## Usage patterns

**Inject and create a client per component:**

```go
import statsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"

type Requires struct {
    fx.In
    Statsd statsd.Component
}

func NewMyComponent(deps Requires) (MyComponent, error) {
    client, err := deps.Statsd.CreateForHostPort("localhost", 8125)
    if err != nil {
        return nil, err
    }
    // use client.Gauge(...), client.Count(...), etc.
}
```

**Use the shared singleton (STATSD_URL must be set):**

```go
client, err := statsdComp.Get()
// Returns the same client on subsequent calls.
```

**Override default options:**

```go
client, err := statsdComp.Create(
    ddgostatsd.WithNamespace("mycomp."),
    ddgostatsd.WithTags([]string{"env:prod"}),
)
```

**Testing:**

```go
import "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/mock"

// Returns a Component backed by ddgostatsd.NoOpClient (discards all metrics).
statsdMock := mock.Mock(t)
```

## Relationship to other components

### comp/dogstatsd/server

`comp/dogstatsd/statsd` and `comp/dogstatsd/server` are complementary but
independent. The **server** receives packets (it is the _listener_ side).
`comp/dogstatsd/statsd` creates **clients** that _send_ metrics to a
DogStatsD endpoint — typically the server running in the same agent process.

A typical agent binary wires both:

```
Subsystem component
  └── statsd.Component.CreateForHostPort("localhost", 8125)
        │  UDP packets
        ▼
comp/dogstatsd/server (listening on :8125)
        │  parsed + enriched MetricSamples
        ▼
comp/aggregator/demultiplexer
```

The server's bound address is available via `server.Component.UDPLocalAddr()`,
which is how `pkg/jmxfetch/jmxfetch.go` tells JMXFetch where to send its
metrics. Clients created through this component can use the same address:

```go
addr := dogstatsdServer.UDPLocalAddr() // e.g. "127.0.0.1:8125"
client, err := statsdComp.CreateForAddr(addr)
```

See [server.md](server.md) for the server-side component API and internal
architecture.

### pkg/telemetry

`pkg/telemetry` and `comp/dogstatsd/statsd` serve different telemetry needs:

| | `pkg/telemetry` | `comp/dogstatsd/statsd` |
|---|---|---|
| Backend | Prometheus / OpenMetrics | DogStatsD UDP client |
| Exposed at | `/telemetry` HTTP endpoint | Datadog intake (via the agent pipeline) |
| Typical users | Internal agent packages registered at package-init | Components that need to emit operational metrics visible in Datadog |
| Aggregation | Server-side (Prometheus scrape) | Client-side (channel mode + CSA) |

`pkg/telemetry.GetStatsTelemetryProvider()` / `RegisterStatsSender` bridge the two: a `StatsTelemetrySender` backed by a `ddgostatsd.ClientInterface` (obtained from `statsd.Component`) can be registered so that telemetry counters/gauges also flow through the DogStatsD pipeline.

See [pkg/telemetry.md](../../../pkg/telemetry.md) for the full telemetry API.

## Key importers

| Package | Usage |
|---|---|
| `comp/trace/agent/impl/agent.go` | Creates a statsd client for APM trace-agent internal metrics |
| `cmd/system-probe/subcommands/run/command.go` | Wires statsd for system-probe health metrics |
| `cmd/security-agent/subcommands/start/command.go` | Wires statsd for security-agent metrics |
| `cmd/process-agent/command/main_common.go` | Wires statsd for process-agent metrics |
| `pkg/compliance/cli/check.go` | Uses `Get()` for compliance check telemetry |
| `comp/dogstatsd/statsd/otel/statsd_otel.go` | OTel bridge wrapping the component |

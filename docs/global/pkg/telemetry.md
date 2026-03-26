> **TL;DR:** Thin compatibility shim that exposes Prometheus/OpenMetrics-backed `Counter`, `Gauge`, and `Histogram` types for agent-internal telemetry — usable at package-init time without a direct dependency on the component system, with a no-op serverless variant.

# pkg/telemetry

## Purpose

`pkg/telemetry` provides a thin compatibility shim for internal agent telemetry. It exposes
Prometheus/OpenMetrics-backed metric types (Counter, Gauge, Histogram) that any agent package
can register at package-init time without a direct dependency on the component system
(`comp/core/telemetry`).

The package has two compile-time implementations:

- **Default** (`!serverless` build tag): delegates to `comp/core/telemetry/telemetryimpl`, which
  registers metrics with the shared Prometheus registry and exposes them at the agent's
  `/telemetry` HTTP endpoint.
- **Serverless** (`serverless` build tag): delegates to `comp/core/telemetry/noopsimpl`, a
  no-op implementation that discards all metrics. This avoids the overhead of Prometheus in
  the Lambda extension build.

The interfaces re-exported here are identical to those defined in `comp/core/telemetry`. In
practice, packages that don't need the full component (e.g. low-level `pkg/` packages with no
fx dependency) use `pkg/telemetry` directly; components inside `comp/` typically receive
`telemetry.Component` via fx injection.

## Key elements

### Key types

### Metric types

| Type | Interface | Description |
|------|-----------|-------------|
| `Counter` | `pkg/telemetry.Counter` | Monotonically increasing counter. Backed by a Prometheus `Counter` per label combination. |
| `Gauge` | `pkg/telemetry.Gauge` | Arbitrary up/down value. Backed by a Prometheus `Gauge`. |
| `Histogram` | `pkg/telemetry.Histogram` | Distribution of observed values. Backed by a Prometheus `Histogram` with explicit buckets. |
| `SimpleCounter` | `pkg/telemetry.SimpleCounter` | Pre-bound counter with a fixed label set (returned by `Counter.WithValues`). Avoids heap allocation on hot paths. |
| `SimpleGauge` | `pkg/telemetry.SimpleGauge` | Pre-bound gauge with a fixed label set. |
| `SimpleHistogram` | `pkg/telemetry.SimpleHistogram` | Pre-bound histogram with a fixed label set. |

The `Counter` / `Gauge` / `Histogram` interfaces carry both a variadic tag API
(`Inc("val1", "val2", ...)`) and a `WithTags(map[string]string)` API for zero-allocation hot
paths.

### Key functions

```go
// Counters
telemetry.NewCounter(subsystem, name string, tags []string, help string) Counter
telemetry.NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter
telemetry.NewSimpleCounter(subsystem, name, help string) SimpleCounter

// Gauges
telemetry.NewGauge(subsystem, name string, tags []string, help string) Gauge
telemetry.NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge
telemetry.NewSimpleGauge(subsystem, name, help string) SimpleGauge

// Histograms
telemetry.NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) Histogram
telemetry.NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts Options) Histogram
telemetry.NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram
telemetry.NewHistogramNoOp() Histogram  // always a no-op, useful in tests
```

All constructors register the metric with the global registry immediately. They are safe to
call at package-init time (inside `var` declarations or `init()`).

### Configuration and build flags

The `serverless` build tag selects `noopsimpl` (discards all metrics). The `Options.NoDoubleUnderscoreSep` field and `Options.DefaultMetric` field control naming and export behavior.

### Options

```go
type Options struct {
    // NoDoubleUnderscoreSep disables the double-underscore separator between
    // subsystem and metric name used to produce dot-separated Datadog metric names.
    // Not compatible with cross-org agent telemetry.
    NoDoubleUnderscoreSep bool

    // DefaultMetric exports this metric via the built-in agent_telemetry core check.
    DefaultMetric bool
}

var DefaultOptions Options  // zero value; double-underscore separator enabled
```

The double-underscore separator (`subsystem__name`) is post-processed by the agent pipeline
into `subsystem.name` in Datadog.

### Wrapper types for migration

Two wrapper types help migrate code that previously used plain atomic integers to Prometheus
metrics while keeping the old read path alive:

| Type | Purpose |
|------|---------|
| `StatCounterWrapper` | Wraps an `atomic.Int64` and a `Counter`. Mutations update both. `Load()` returns the atomic value for code that reads the counter directly. |
| `StatGaugeWrapper` | Same pattern for gauges. Supports `Inc`, `Dec`, `Add`, `Set`, `Load`. |

Constructors: `NewStatCounterWrapper(subsystem, name, tags, description)` and
`NewStatGaugeWrapper(subsystem, name, tags, description)`.

### Stats telemetry bridge

`StatsTelemetryProvider` / `StatsTelemetrySender` bridge the telemetry package to the agent's
internal stats pipeline (used by log tailers, decoders, etc.):

```go
// At agent startup, register a sender (e.g. dogstatsd client):
telemetry.RegisterStatsSender(sender)

// In business logic, emit metrics without caring about the sender:
telemetry.GetStatsTelemetryProvider().Count("metric.name", 1.0, []string{"tag:val"})
telemetry.GetStatsTelemetryProvider().Gauge("metric.name", 42.0, []string{})
```

If no sender is registered the calls are silently discarded.

### GetCompatComponent

```go
func GetCompatComponent() telemetry.Component
```

Returns the global `telemetry.Component` used internally by all constructor functions. Useful
when a package needs to pass a `Component` to a third-party helper but is not itself wired
into fx.

## Usage

### Declaring metrics at package level (most common pattern)

```go
var (
    tlmTxInputBytes = telemetry.NewCounter("transactions", "input_bytes",
        []string{"domain", "endpoint"}, "Incoming transaction sizes in bytes")
    tlmRetryQueueSize = telemetry.NewGauge("transactions", "retry_queue_size",
        []string{"domain"}, "Retry queue size")
)

// In business logic:
tlmTxInputBytes.Inc("datadoghq.com", "series")
tlmRetryQueueSize.Set(float64(len(queue)), "datadoghq.com")
```

### Using WithValues to avoid allocations in hot paths

```go
var tlmEvents = telemetry.NewCounter("dogstatsd", "events",
    []string{"state"}, "DogStatsD event count")

// Pre-bind label values once:
tlmEventsOk  = tlmEvents.WithValues("ok")
tlmEventsErr = tlmEvents.WithValues("error")

// In hot path (no heap allocation):
tlmEventsOk.Inc()
```

### Migration path: StatCounterWrapper

```go
// Replaces: closedConns = &atomic.Int64{}
closedConns = telemetry.NewStatCounterWrapper(
    "tracer", "closed_conns", []string{"ip_proto"}, "Closed TCP connections")

// Increment both the Prometheus counter and the atomic:
closedConns.Inc("tcp")

// Code that still needs to read the raw value:
n := closedConns.Load()
```

### Checking internal metrics programmatically

The underlying `telemetry.Component` (obtained via `GetCompatComponent()`) exposes:

```go
comp.Handler()            // net/http.Handler for /telemetry endpoint
comp.Gather(false)        // []*MetricFamily for the custom registry
comp.GatherText(false, f) // text-format OpenMetrics output
```

## Testing

When unit-testing code that calls `pkg/telemetry` constructors, use
`telemetryimpl.MockModule()` (or `telemetryimpl.NewMock(t)` for non-fx tests)
to get a real in-memory registry with assertion helpers. `NewHistogramNoOp()`
is a useful escape hatch when a histogram dependency must be satisfied but its
values are irrelevant to the test.

For fx-based component tests, replace `telemetryimpl.Module()` with
`telemetryimpl.MockModule()` and resolve `telemetry.Mock` via `fxutil.Test`.
See [`comp/core/telemetry`](../comp/core/telemetry.md) for the full mock API
(`GetCountMetric`, `GetGaugeMetric`, `GetHistogramMetric`, `GetRegistry`).

## Related packages

- [`comp/core/telemetry`](../comp/core/telemetry.md) — defines the `Component` interface and all metric
  interfaces; also documents the fx wiring pattern, mock helpers, and the
  relationship between `pkg/telemetry` and the component.
- `comp/core/telemetry/telemetryimpl` — Prometheus-backed implementation;
  `Module()` is included in `core.Bundle()` automatically.
- `comp/core/telemetry/noopsimpl` — no-op implementation used in serverless
  and tests; selected automatically on the `serverless` build tag.
- [`comp/core/agenttelemetry`](../comp/core/agenttelemetry.md) — periodically
  reads `Gather(true)` from the default registry and ships the collected
  counters/gauges to Datadog as agent-telemetry payloads. Metrics must be
  declared **without** `Options.NoDoubleUnderscoreSep` to appear under the
  expected `subsystem.name` key in a profile.
- [`pkg/util/prometheus`](util/prometheus.md) — a separate utility for
  **parsing** external Prometheus text-format endpoints (kubelet, containerd,
  OTel collector). It is unrelated to `pkg/telemetry`'s metric registration;
  they share only the Prometheus exposition format as a common wire format.
- [`pkg/ebpf/telemetry`](ebpf/telemetry.md) — eBPF-specific telemetry that
  exposes map-error, helper-error, and perf/ring-buffer metrics as Prometheus
  collectors registered with `comp/core/telemetry` via
  `telemetry.RegisterCollector`. The `EBPFErrorsCollector` and
  `NewPerfUsageCollector` are the bridge points.

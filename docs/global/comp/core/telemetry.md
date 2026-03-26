> **TL;DR:** `comp/core/telemetry` provides a shared Prometheus/OpenMetrics registry for agent-internal health metrics (counters, gauges, histograms), exposing them via an HTTP handler and an injectable mock with assertion helpers for tests.

# comp/core/telemetry — Telemetry Component

**Import path:** `github.com/DataDog/datadog-agent/comp/core/telemetry`
**Team:** agent-runtimes
**Importers:** ~98 packages

## Purpose

`comp/core/telemetry` exposes the agent's internal health metrics (counters, gauges, histograms) in Prometheus / OpenMetrics format. Components use it to create and update metric instruments; the agent's HTTP server then serves those metrics via `Handler()` at the `/telemetry` endpoint, where they can be scraped by a monitoring system or consumed by the built-in `agent_telemetry` check.

Using this component instead of calling `prometheus` directly provides:

- A shared, reset-safe Prometheus registry per agent lifecycle.
- A consistent naming convention (subsystem + `__` + name separator by default).
- A "default" registry subset served by the `agent_telemetry` core check (`Options.DefaultMetric`).
- An injectable mock with assertion helpers for unit tests.

## Package layout

| Package | Role |
|---|---|
| `comp/core/telemetry` (root) | `Component` interface, metric type interfaces (`Counter`, `Gauge`, `Histogram`, …), `Options`, `MetricFilter` |
| `telemetryimpl/` | Production implementation; Prometheus registry management; `Module()` and `MockModule()` fx wiring |
| `noopsimpl/` | No-op implementation for builds where telemetry is disabled (e.g. serverless) |

## Key elements

### Key interfaces

#### Component interface

```go
type Component interface {
    // HTTP endpoint for Prometheus scraping
    Handler() http.Handler

    // Registry management
    Reset()
    RegisterCollector(c Collector)
    UnregisterCollector(c Collector) bool

    // Metric constructors
    NewCounter(subsystem, name string, tags []string, help string) Counter
    NewCounterWithOpts(subsystem, name string, tags []string, help string, opts Options) Counter
    NewSimpleCounter(subsystem, name, help string) SimpleCounter
    NewSimpleCounterWithOpts(subsystem, name, help string, opts Options) SimpleCounter

    NewGauge(subsystem, name string, tags []string, help string) Gauge
    NewGaugeWithOpts(subsystem, name string, tags []string, help string, opts Options) Gauge
    NewSimpleGauge(subsystem, name, help string) SimpleGauge
    NewSimpleGaugeWithOpts(subsystem, name, help string, opts Options) SimpleGauge

    NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) Histogram
    NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts Options) Histogram
    NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram
    NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram

    // Programmatic access to gathered metrics
    Gather(defaultGather bool) ([]*MetricFamily, error)
    GatherText(defaultGather bool, filter MetricFilter) (string, error)
}
```

### Key types

#### Tagged vs. Simple variants

Every instrument comes in two flavours:

| Variant | Type | Usage |
|---|---|---|
| Tagged (`Counter`, `Gauge`, `Histogram`) | `NewCounter(subsystem, name, tags, help)` | Multi-dimensional; tag values are supplied on each operation (`Inc("tag_value1", "tag_value2")`) |
| Simple (`SimpleCounter`, `SimpleGauge`, `SimpleHistogram`) | `NewSimpleCounter(subsystem, name, help)` | No per-call tags; useful for scalars |

Tagged instruments also expose `.WithValues(tagValues...)` and `.WithTags(map)` to create a bound `Simple*` instance — useful in hot paths to avoid heap escapes.

#### Counter

```go
type Counter interface {
    InitializeToZero(tagsValue ...string)  // ensure metric appears before first increment
    Inc(tagsValue ...string)
    Add(value float64, tagsValue ...string)
    Delete(tagsValue ...string)
    IncWithTags(tags map[string]string)    // avoids heap escape for hot paths
    AddWithTags(value float64, tags map[string]string)
    WithValues(tagsValue ...string) SimpleCounter
    WithTags(tags map[string]string) SimpleCounter
}
```

#### Gauge

```go
type Gauge interface {
    Set(value float64, tagsValue ...string)
    Inc(tagsValue ...string)
    Dec(tagsValue ...string)
    Add(value float64, tagsValue ...string)
    Sub(value float64, tagsValue ...string)
    Delete(tagsValue ...string)
    DeletePartialMatch(tags map[string]string)
    WithValues(tagsValue ...string) SimpleGauge
    WithTags(tags map[string]string) SimpleGauge
}
```

#### Histogram

```go
type Histogram interface {
    Observe(value float64, tagsValue ...string)
    Delete(tagsValue ...string)
    WithValues(tagsValue ...string) SimpleHistogram
    WithTags(tags map[string]string) SimpleHistogram
}
```

### Configuration and build flags

#### Options

```go
type Options struct {
    // NoDoubleUnderscoreSep: when true, subsystem and name are joined with a
    // single underscore instead of the default double underscore. Incompatible
    // with cross-org agent telemetry.
    NoDoubleUnderscoreSep bool

    // DefaultMetric: when true the metric is registered in the secondary
    // "default" registry consumed by the agent_telemetry core check.
    DefaultMetric bool
}

var DefaultOptions = Options{NoDoubleUnderscoreSep: false}
```

The double-underscore separator (`subsystem__name`) is intentional: the agent pipeline replaces it with a dot before forwarding metrics to Datadog, producing `subsystem.name`.

### Key functions

#### fx wiring

`telemetryimpl.Module()` is included in `core.Bundle()` — no manual registration is needed.

```go
// core.Bundle() already includes telemetryimpl.Module()
core.Bundle(),
```

To consume the component:

```go
import "github.com/DataDog/datadog-agent/comp/core/telemetry"

type Requires struct {
    fx.In
    Telemetry telemetry.Component
}

func NewMyComp(deps Requires) MyComp {
    c := deps.Telemetry.NewCounter("my_subsystem", "events_total",
        []string{"kind"}, "Total number of events processed")
    ...
    c.Inc("inbound")
}
```

Metric instruments are typically created once at component construction time (package-level `var` or inside the constructor) and then mutated on each relevant event.

#### Registering a custom Prometheus Collector

If you have an existing `prometheus.Collector` implementation:

```go
deps.Telemetry.RegisterCollector(myCollector)
// later, if needed:
deps.Telemetry.UnregisterCollector(myCollector)
```

#### Mock

`telemetryimpl.MockModule()` provides an fx module for tests. It exposes both `telemetry.Component` and `telemetry.Mock`. The mock adds assertion helpers:

```go
type Mock interface {
    telemetry.Component
    GetCountMetric(subsystem, name string) ([]Metric, error)
    GetGaugeMetric(subsystem, name string) ([]Metric, error)
    GetHistogramMetric(subsystem, name string) ([]Metric, error)
    GetRegistry() *prometheus.Registry
}
```

Each returned `Metric` exposes `.Tags() map[string]string` and `.Value() float64`.

For non-fx tests use `telemetryimpl.NewMock(t)` directly:

```go
import "github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"

func TestFoo(t *testing.T) {
    tel := telemetryimpl.NewMock(t) // resets registry on t.Cleanup
    c := tel.NewCounter("sub", "ops_total", []string{"status"}, "")
    c.Inc("ok")
    metrics, _ := tel.GetCountMetric("sub", "ops_total")
    assert.Equal(t, 1.0, metrics[0].Value())
}
```

## Relationship to pkg/telemetry

`pkg/telemetry` is a package-level shim that forwards to `comp/core/telemetry` internally. The two APIs are deliberately kept in sync:

| Pattern | When to use |
|---|---|
| `pkg/telemetry.NewCounter(...)` at a package-level `var` | `pkg/` code that has no fx dependency and needs to register a metric at init time. Metrics are registered immediately in the global registry. |
| `deps.Telemetry.NewCounter(...)` inside an fx constructor | `comp/` code where `telemetry.Component` is injected. Preferred for new component code. |

Under the hood `pkg/telemetry` calls `GetCompatComponent()` which returns the same backing `telemetryimpl` instance that fx injects — so both paths write to the same Prometheus registry. On `serverless` builds both delegate to `noopsimpl`, discarding all metrics.

See [`pkg/telemetry`](../../pkg/telemetry.md) for `StatCounterWrapper`, `StatGaugeWrapper`, and the `StatsTelemetryProvider` bridge used by log tailers and decoders.

### Config interaction

`comp/core/telemetry` does not read configuration directly ([`comp/core/config`](config.md)). Configuration-gating of telemetry (e.g. `telemetry.enabled`) is handled by the consumers of `telemetry.Component`, not by the component itself.

### Serving metrics

The `/telemetry` HTTP endpoint is served by `comp/api/api` ([`comp/api/api`](../api/api.md)) which calls `telemetry.Handler()` during CMD server setup. The `agent_telemetry` core check pulls the default-registry subset via `Gather(true)`.

### fxutil integration

In component unit tests, replace `telemetryimpl.Module()` with `telemetryimpl.MockModule()` and resolve `telemetry.Mock` via `fxutil.Test`:

```go
type deps struct {
    fx.In
    Tel telemetry.Mock
}

func TestMyComp(t *testing.T) {
    d := fxutil.Test[deps](t, fx.Options(
        telemetryimpl.MockModule(),
        mycomp.Module(),
    ))
    metrics, _ := d.Tel.GetCountMetric("sub", "ops_total")
    assert.Equal(t, 1.0, metrics[0].Value())
}
```

See [`pkg/util/fxutil`](../../pkg/util/fxutil.md) for the full test helper API.

## Key dependents

- `pkg/aggregator` — counters and gauges for series/sketch pipeline health
- `comp/logs`, `comp/trace`, `comp/forwarder` — per-pipeline throughput and error counters
- [`comp/api/api`](../api/api.md) — serves `Handler()` on the internal HTTP server at `/telemetry`
- The `agent_telemetry` core check — gathers metrics via `Gather(true)` from the default registry

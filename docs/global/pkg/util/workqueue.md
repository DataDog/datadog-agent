> **TL;DR:** Bridges Kubernetes client-go work queue instrumentation hooks to the Datadog Agent's internal telemetry system, automatically emitting standard queue metrics (depth, latency, retries) for any `k8s.io/client-go/util/workqueue` instance.

# pkg/util/workqueue

Import path: `github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry`

## Purpose

Provides a `workqueue.MetricsProvider` implementation that bridges Kubernetes client-go's work queue instrumentation hooks to the Datadog Agent's internal telemetry system (`pkg/telemetry`). Wiring in this provider causes any `k8s.io/client-go/util/workqueue` to automatically emit standard Datadog metrics (depth, adds, latency, work duration, unfinished work, longest-running processor, retries) without changes to the workqueue call sites.

The package lives under `pkg/util/workqueue/telemetry/` (the parent `pkg/util/workqueue/` directory contains only this sub-package).

## Key elements

### `QueueMetricsProvider`

```go
type QueueMetricsProvider struct { ... }

func NewQueueMetricsProvider() *QueueMetricsProvider
```

Implements `workqueue.MetricsProvider`. It holds a `map[string]interface{}` of already-registered metrics guarded by a `sync.Mutex`, so registering the same metric name twice returns the cached instance rather than attempting a duplicate Prometheus registration. This makes it safe to use a single `QueueMetricsProvider` across many queues in the same package.

The recommended pattern is to instantiate a single provider per package as a package-level variable and pass it to every workqueue created in that package.

### Metric methods

All methods implement the `workqueue.MetricsProvider` interface. Each returns a wrapper that satisfies the corresponding `workqueue` metric interface.

| Method | Metric name | Type | Description |
|---|---|---|---|
| `NewDepthMetric(subsystem)` | `queue_depth` | Gauge | Current number of items in the queue |
| `NewAddsMetric(subsystem)` | `queue_adds` | Counter | Total items added to the queue |
| `NewLatencyMetric(subsystem)` | `queue_latency` | Histogram | Time items spend waiting in the queue (seconds) |
| `NewWorkDurationMetric(subsystem)` | `queue_work_duration` | Histogram | Time spent processing each item (seconds) |
| `NewUnfinishedWorkSecondsMetric(subsystem)` | `queue_unfinished_work` | Gauge | Seconds of work not yet completed |
| `NewLongestRunningProcessorSecondsMetric(subsystem)` | `queue_longest_running_processor` | Gauge | Duration of the longest active processor (seconds) |
| `NewRetriesMetric(subsystem)` | `queue_retries` | Counter | Total retry attempts |

Histogram buckets for `queue_latency` are `{.05, .1, .25, .5, 1, 2.5, 5, 10, 60, 300, 600, 1200}` seconds (extended for long-running queues). `queue_work_duration` uses `prometheus.DefBuckets`.

Metric names use `NoDoubleUnderscoreSep` (`telemetry.Options{NoDoubleUnderscoreSep: true}`), preserving the single separator between subsystem and name in the Prometheus output.

### Internal wrappers

`gaugeWrapper`, `counterWrapper`, and `histgramWrapper` adapt the Agent's `telemetry.Gauge`, `telemetry.Counter`, and `telemetry.Histogram` types to the narrower `workqueue.GaugeMetric`, `workqueue.CounterMetric`, and `workqueue.HistogramMetric` interfaces respectively. These are not exported; they exist solely to satisfy the interface contract.

## Usage

### Cluster Agent — autoscaling workqueue

`pkg/clusteragent/autoscaling/workload/telemetry.go` demonstrates the recommended pattern:

```go
// package-level provider, shared by all queues in this package
var autoscalingQueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()

// when creating the queue
wq := workqueue.NewRateLimitingQueueWithConfig(
    workqueue.NewItemExponentialFailureRateLimiter(
        2*time.Second,
        2*time.Minute,
    ),
    workqueue.RateLimitingQueueConfig{
        Name:            "autoscaling",
        MetricsProvider: autoscalingQueueMetricsProvider,
    },
)
```

The same pattern appears in:
- `pkg/clusteragent/autoscaling/cluster/telemetry.go`
- `pkg/clusteragent/languagedetection/telemetry.go`
- `pkg/clusteragent/appsec/injector.go`
- `pkg/sbom/telemetry/telemetry.go`

### Important caveats

- Do not register the same provider with multiple distinct workqueue subsystem names when the queues have different cardinality requirements; the provider deduplicates by metric name only, not by `(name, subsystem)` pair.
- Do not create separate `QueueMetricsProvider` instances for the same workqueue — this causes duplicate Prometheus metric registration panics.

## Relationship to other packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/telemetry` | [../../pkg/telemetry.md](../../pkg/telemetry.md) | `QueueMetricsProvider` builds all its metrics using `pkg/telemetry` constructors (`telemetry.NewGauge`, `telemetry.NewCounter`, `telemetry.NewHistogram`). This means workqueue metrics appear in the same Prometheus registry and at the same `/telemetry` HTTP endpoint as all other agent metrics. The `NoDoubleUnderscoreSep` option (`telemetry.Options{NoDoubleUnderscoreSep: true}`) is required to preserve the single underscore in Prometheus metric names like `queue_depth` rather than producing `queue__depth`. |
| `pkg/clusteragent/autoscaling` | [../../pkg/clusteragent/autoscaling.md](../../pkg/clusteragent/autoscaling.md) | The autoscaling controller in `pkg/clusteragent/autoscaling/workload/` and `cluster/` are the primary consumers. Each sub-package instantiates a single `QueueMetricsProvider` as a package-level variable and passes it to every `workqueue.NewRateLimitingQueueWithConfig` call in that package, following the one-provider-per-package recommendation. `pkg/clusteragent/languagedetection/` and `pkg/clusteragent/appsec/` follow the same pattern. |
| `pkg/sbom/telemetry` | (no dedicated doc) | `pkg/sbom/telemetry/telemetry.go` uses a package-level `QueueMetricsProvider` for the SBOM scan workqueue, demonstrating that the pattern extends beyond the Cluster Agent to any package that uses `k8s.io/client-go` work queues. |

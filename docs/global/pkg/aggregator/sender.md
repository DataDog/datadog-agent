> **TL;DR:** Defines the `Sender` and `SenderManager` interfaces through which checks submit metrics, events, and service checks to the aggregation pipeline, keeping check code decoupled from aggregator internals.

# Package `pkg/aggregator/sender`

## Purpose

The `sender` package defines the interfaces through which checks (Python or Go) submit observability data — metrics, events, service checks, and orchestrator payloads — to the aggregation pipeline.

It contains only interface definitions. The concrete implementation (`checkSender`) lives in `pkg/aggregator` to avoid import cycles. Checks depend solely on this package's interfaces, keeping them decoupled from the aggregator internals.

There are two interfaces:

- **`Sender`** — the per-check submission API, one instance per check run.
- **`SenderManager`** — the factory/registry used by the check scheduler to create, look up, and destroy `Sender` instances.

### Related documentation

| Document | What it covers |
|---|---|
| [`pkg/aggregator`](aggregator.md) | The aggregation pipeline that implements these interfaces |
| [`pkg/metrics`](../metrics/metrics.md) | `MetricSample`, `MetricType`, `Event`, `ServiceCheck` — the data types sent through `Sender` |
| [`pkg/collector/check`](../collector/check.md) | `Check` interface and `CheckBase` which embeds `SenderManager` access |

---

## Key elements

### Key interfaces

### `Sender` interface

```go
type Sender interface {
    // Metric submission
    Gauge(metric string, value float64, hostname string, tags []string)
    GaugeNoIndex(metric string, value float64, hostname string, tags []string)
    Rate(metric string, value float64, hostname string, tags []string)
    Count(metric string, value float64, hostname string, tags []string)
    MonotonicCount(metric string, value float64, hostname string, tags []string)
    MonotonicCountWithFlushFirstValue(metric, value, hostname, tags, flushFirstValue)
    Counter(metric string, value float64, hostname string, tags []string)  // deprecated
    Histogram(metric string, value float64, hostname string, tags []string)
    Historate(metric string, value float64, hostname string, tags []string)
    Distribution(metric string, value float64, hostname string, tags []string)
    HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool)
    GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error
    CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error

    // Events and service checks
    Event(e event.Event)
    ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string)
    EventPlatformEvent(rawEvent []byte, eventType string)

    // Orchestrator
    OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int)
    OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string)

    // Lifecycle
    Commit()
    GetSenderStats() stats.SenderStats
    DisableDefaultHostname(disable bool)
    SetCheckCustomTags(tags []string)
    SetCheckService(service string)
    FinalizeCheckServiceTag()
    SetNoIndex(noIndex bool)
}
```

#### Metric types

The method names map to `metrics.MetricType` constants in [`pkg/metrics`](../metrics/metrics.md). The `CheckSampler` in the aggregator receives these via the channel and calls `ContextMetrics.AddSample()` with the appropriate type.

| Method | `MetricType` | Aggregation behaviour |
|---|---|---|
| `Gauge` | `GaugeType` | Last value within the flush interval is kept. |
| `GaugeNoIndex` | `GaugeType` | Same as `Gauge`, but the series is not indexed by the backend. |
| `Rate` | `RateType` | Computes per-second rate between two consecutive samples. |
| `Count` | `CountType` | Sums values submitted during the flush interval. |
| `MonotonicCount` | `MonotonicCountType` | Reports the delta of a monotonically increasing counter between flushes. |
| `Counter` | `CounterType` | Deprecated alias; prefer `Gauge` for states, `Count` for events. |
| `Histogram` | `HistogramType` | Computes percentile/min/max/avg/count/sum series from all submitted values. |
| `Historate` | `HistorateType` | Like `Histogram`, but values are interpreted as rates. |
| `Distribution` | `DistributionType` | Sends raw values as a DDSketch distribution for server-side percentile accuracy. |
| `HistogramBucket` | — | Submits pre-computed histogram buckets (from an OpenMetrics/Prometheus source) as `metrics.HistogramBucket` that are converted into DDSketch data. |
| `GaugeWithTimestamp` | `GaugeWithTimestampType` | Like `Gauge`, but bypasses aggregation and forwards with an explicit timestamp. |
| `CountWithTimestamp` | `CountWithTimestampType` | Like `Count`, but bypasses aggregation and forwards with an explicit timestamp. |

#### Lifecycle methods

- **`Commit()`** — Must be called once at the end of every check `Run()`. It signals the `CheckSampler` to commit all in-flight samples and clear them in preparation for the next run. Forgetting to call `Commit()` means metrics accumulate indefinitely until the next flush.
- **`DisableDefaultHostname(true)`** — Prevents the agent hostname from being injected automatically. Useful for checks that emit cluster-wide or tag-only metrics.
- **`SetCheckCustomTags(tags)`** — Appends the check's custom tags (from its `datadog.yaml` configuration) to every submission.
- **`SetCheckService(service)`** / **`FinalizeCheckServiceTag()`** — Appends a `service:<name>` tag. `FinalizeCheckServiceTag()` must be called after `SetCheckService()` to commit the tag into the tag list.
- **`SetNoIndex(true)`** — Marks all subsequent metrics in this sender as not to be indexed by the backend (equivalent to `GaugeNoIndex` but applied globally).

### `SenderManager` interface

`SenderManager` is provided to the check scheduler by the `comp/aggregator/demultiplexer` component (see [`demultiplexer.md`](../../comp/aggregator/demultiplexer.md)). It is also the interface through which the fx DI graph provides sender access to packages that need to emit metrics without being a check.

```go
type SenderManager interface {
    GetSender(id checkid.ID) (Sender, error)
    SetSender(Sender, checkid.ID) error
    DestroySender(id checkid.ID)
    GetDefaultSender() (Sender, error)
}
```

| Method | Description |
|---|---|
| `GetSender(id)` | Returns an existing `Sender` for the given check ID, or creates and registers a new one. The `CheckSampler` in `BufferedAggregator` is also registered on first call. |
| `SetSender(sender, id)` | Replaces the `Sender` for the given ID. Used in tests to inject a `MockSender`. |
| `DestroySender(id)` | Removes the sender from the pool and sends a deregister signal to the aggregator so the associated `CheckSampler` is dropped after the next flush. Must be called when a check is unscheduled. The `comp/collector/collector` component's `CheckWrapper` middleware calls this automatically after `Cancel()`. |
| `GetDefaultSender()` | Returns a singleton `Sender` using a zero-value check ID, useful for code paths that need to emit metrics outside of a named check. |

---

## Usage

### In a Go check

The `SenderManager` is passed to `Configure()` and stored on the check struct. `Run()` then calls `GetSender()` each time. Go checks typically embed `corechecks.CheckBase` (see [`corechecks.md`](../collector/corechecks.md)), which wraps `SenderManager` access:

```go
import "github.com/DataDog/datadog-agent/pkg/aggregator/sender"

type MyCheck struct {
    corechecks.CheckBase
    // senderManager is embedded via CheckBase; use c.GetSender()
}

func (c *MyCheck) Run() error {
    sender, err := c.GetSender()
    if err != nil {
        return err
    }

    sender.Gauge("mycheck.cpu_usage", value, "", []string{"host:myhost"})
    sender.ServiceCheck("mycheck.can_connect", servicecheck.ServiceCheckOK, "", nil, "")
    sender.Commit()
    return nil
}
```

`corechecks.CheckBase.GetSender()` is a convenience wrapper around `senderManager.GetSender(c.ID())`. It actually returns a `safeSender` wrapper that copies tag slices before forwarding, preventing accidental in-place mutation. Checks that use `CheckBase` do not need to call `DestroySender()` themselves — the collector's `CheckWrapper` middleware calls it after `Cancel()`.

### In a Python check

The Python bindings expose an equivalent API on the `datadog_checks.base.AgentCheck` class. The Go `Sender` interface is the target contract: when the Python check calls `self.gauge(...)`, the Go binding calls `Sender.Gauge(...)`.

### Injecting a mock in tests

```go
// build tag: test
import "github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"

mockSender := mocksender.NewMockSender(checkID)
mockSender.SetupAcceptAll()

check.Run()

mockSender.AssertMetric(t, "Gauge", "mycheck.cpu_usage", 42.0, "", []string{"host:myhost"})
mockSender.AssertServiceCheck(t, "mycheck.can_connect", servicecheck.ServiceCheckOK, "", nil, "")
mockSender.AssertNumberOfCalls(t, "Commit", 1)
```

`mocksender.NewMockSender` spins up a real (but no-op forwarding) `AgentDemultiplexer` and injects the mock via `SetSender`. All `Sender` methods are recorded as testify mock calls and can be asserted with the helpers in `mocksender/asserts.go`.

### Custom SenderManager implementations

Non-check producers (e.g. `pkg/networkconfigmanagement/sender`) implement `SenderManager` directly to route metric submissions to their own backend without going through the check aggregator. They can pass a compatible `SenderManager` instance wherever the interface is expected.

---

## Concrete implementation

The concrete `checkSender` type in `pkg/aggregator/sender.go` wires the `Sender` interface to the `BufferedAggregator` via Go channels:

- Metric samples are sent as `senderMetricSample` items on `BufferedAggregator.checkItems`.
- Histogram buckets are sent as `senderHistogramBucket` items on the same channel.
- Events and service checks go to their respective dedicated channels.
- `Commit()` sends a special sentinel `senderMetricSample{commit: true}` that triggers `CheckSampler.commit()` in the aggregator goroutine.

Check tags (`SetCheckCustomTags`) are appended client-side in the sender before the sample is enqueued, so the aggregator never needs to know about check-level tag configuration.

Each `MetricSample` constructed by the sender carries a `metrics.MetricSource` (see [`pkg/metrics`](../metrics/metrics.md)) set to the integration's source constant. This value flows through to the serialized `Serie` and is used by the v2 API to populate origin metadata.

### `NoOpSenderManager`

`pkg/aggregator/no_op_sender_manager.go` provides a `NoOpSenderManager` implementation of `SenderManager` that silently discards all operations. Used in contexts where a `SenderManager` is required by the API but no aggregator is present (e.g., some serverless or test scenarios).
